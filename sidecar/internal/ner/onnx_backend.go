//go:build onnx

package ner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/sidecar/internal/ner/onnxrt"
)

// ONNXBackend is the ONNX token-classification backend. It owns the tokenizer,
// the session and the label set; the span logic (assembly, guards, aliases,
// dedup, overlap) lives in Decode and is shared with every other backend.
//
// Each request tokenises the whole text once, plans 400-token chunks with a
// 64-token overlap, runs one inference per chunk, and decodes each chunk against
// the whole text. Because the tokens carry document-global byte offsets, a span
// is reported at its position in the original text without any rebasing.
type ONNXBackend struct {
	tokenizer Tokenizer
	session   Session
	labels    []string
}

// closer is implemented by a session that owns a resource.
type closer interface {
	Close() error
}

// NewONNXBackend validates the artifact paths, loads config.json's id2label,
// initialises ONNX Runtime, opens the model and loads the tokenizer. Every path
// is explicit: nothing is downloaded and nothing is discovered implicitly.
func NewONNXBackend(cfg ONNXConfig) (NERBackend, error) {
	if err := ValidateONNXConfig(cfg); err != nil {
		return nil, err
	}

	configPath := filepath.Join(cfg.ModelDir, "config.json")
	labels, err := loadID2Label(configPath)
	if err != nil {
		return nil, err
	}

	modelPath := filepath.Join(cfg.ModelDir, "model.onnx")
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("onnx: model %s: %w", modelPath, err)
	}

	// Hash both artifacts before any native handle is opened, so the only
	// resources that exist past this point are the ones closed on each error path.
	modelSum, err := fileSHA256(modelPath)
	if err != nil {
		return nil, err
	}
	tokenizerSum, err := fileSHA256(cfg.TokenizerPath)
	if err != nil {
		return nil, err
	}

	if err := onnxrt.InitEnvironment(cfg.ONNXRuntimeLib); err != nil {
		return nil, err
	}
	sess, err := onnxrt.NewSession(modelPath)
	if err != nil {
		return nil, err
	}
	// Fail at startup, not on the first request, when the model's output
	// channel count and config.json's id2label disagree.
	if n := sess.LabelCount(); n > 0 && n != len(labels) {
		closeSession(sess)
		return nil, fmt.Errorf("onnx: model %s has %d output channels but %s declares %d labels",
			modelPath, n, configPath, len(labels))
	}

	tok, err := NewHFTokenizerWithOptions(cfg.TokenizerPath, cfg.AddSpecialTokens)
	if err != nil {
		closeSession(sess)
		return nil, err
	}

	log.Printf("onnx: backend ready model=%s sha256=%s tokenizer=%s sha256=%s inputs=%v labels=%d",
		modelPath, modelSum, cfg.TokenizerPath, tokenizerSum, sess.InputNames(), len(labels))

	return &ONNXBackend{tokenizer: tok, session: sess, labels: labels}, nil
}

// closeResource releases a value that owns a native handle. A value without a
// Close method owns nothing and is a no-op.
func closeResource(r any) error {
	if c, ok := r.(closer); ok {
		return c.Close()
	}
	return nil
}

// closeSession releases a session on an error path, where the caller is already
// returning the original failure; a close error is logged, not returned.
func closeSession(sess Session) {
	if err := closeResource(sess); err != nil {
		log.Printf("onnx: closing session: %v", err)
	}
}

// Close releases the session and the tokenizer. The backend must not be used
// after Close.
func (b *ONNXBackend) Close() error {
	return errors.Join(closeResource(b.session), closeResource(b.tokenizer))
}

// Name reports the backend name used by the -backend flag.
func (b *ONNXBackend) Name() string { return "onnx" }

// Recognize returns the entities the model finds in text. A failure in any chunk
// fails the request: partial entities are never returned.
func (b *ONNXBackend) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if text == "" {
		return nil, nil
	}

	tokens, err := b.tokenizer.Tokenize(text)
	if err != nil {
		return nil, fmt.Errorf("onnx: tokenize: %w", err)
	}
	if len(tokens) == 0 {
		return nil, nil
	}

	chunks, err := planChunks(len(tokens))
	if err != nil {
		return nil, err
	}

	perChunk := make([][]recognizers.Entity, 0, len(chunks))
	for _, chunk := range chunks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		part := tokens[chunk.Start:chunk.End]
		feeds, err := buildFeeds(part, b.session.InputNames())
		if err != nil {
			return nil, err
		}
		logits, err := b.session.Run(ctx, feeds)
		if err != nil {
			return nil, fmt.Errorf("onnx: chunk starting at token %d: %w", chunk.Start, err)
		}
		entities, err := Decode(text, part, logits, b.labels)
		if err != nil {
			return nil, fmt.Errorf("onnx: chunk starting at token %d: %w", chunk.Start, err)
		}
		perChunk = append(perChunk, entities)
	}
	return mergeChunkEntities(perChunk), nil
}

// fileSHA256 returns the hex SHA-256 of a file. The hash is logged at startup so
// an operator can match the loaded artifacts against the ones that were
// measured.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("onnx: %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("onnx: hashing %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
