package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/u007/dataprotect"
	"github.com/u007/dataprotect/sidecar/internal/ner"
	"github.com/u007/dataprotect/sidecar/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	addr := flag.String("addr", "127.0.0.1:8787", "listen address")
	backendName := flag.String("backend", "prose", "ner backend")
	modelDir := flag.String("model", "", "directory containing model.onnx and config.json (onnx backend)")
	tokenizerPath := flag.String("tokenizer", "", "path to tokenizer.json (onnx backend)")
	onnxRuntimeLib := flag.String("onnxruntime-lib", "", "path to libonnxruntime (onnx backend)")
	flag.Parse()

	b, err := newBackend(*backendName, ner.ONNXConfig{
		ModelDir:       *modelDir,
		TokenizerPath:  *tokenizerPath,
		ONNXRuntimeLib: *onnxRuntimeLib,
	})
	if err != nil {
		return err
	}
	defer closeBackend(b)

	scrubber := piiscrub.NewScrubber()

	srv := server.New(*addr, b, scrubber)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

func newBackend(name string, cfg ner.ONNXConfig) (ner.NERBackend, error) {
	switch name {
	case "prose":
		return ner.NewProseBackend(), nil
	case "onnx":
		b, err := ner.NewONNXBackend(cfg)
		if err != nil {
			return nil, fmt.Errorf("onnx backend: %w", err)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("unknown backend: %s", name)
	}
}

// closeBackend releases backend resources after the HTTP server has stopped.
// A backend without a Close method owns no resources and is left alone.
func closeBackend(b ner.NERBackend) {
	c, ok := b.(io.Closer)
	if !ok {
		return
	}
	if err := c.Close(); err != nil {
		log.Printf("closing %s backend: %v", b.Name(), err)
	}
}
