//go:build onnx

package ner

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/daulet/tokenizers"
)

// hfTokenizer adapts github.com/daulet/tokenizers (the Hugging Face Rust core,
// linked through cgo) to the Tokenizer seam.
//
// It reports UTF-8 byte offsets into the original text, so the decoder never
// handles rune offsets. Every offset pair is validated against the text's bounds
// and a pair outside them fails the request: a silently clamped span would hide a
// tokenizer/config mismatch and redact the wrong bytes.
type hfTokenizer struct {
	tok        *tokenizers.Tokenizer
	addSpecial bool
}

// NewHFTokenizer loads a tokenizer.json without adding special tokens, preserving
// the Phase 0 default where a 400-token chunk is exactly 400 model input tokens.
// The file is the only source of the tokenizer; nothing is downloaded or
// discovered implicitly.
func NewHFTokenizer(path string) (Tokenizer, error) {
	return NewHFTokenizerWithOptions(path, false)
}

// NewHFTokenizerWithOptions loads a tokenizer.json and optionally enables the
// model's special tokens (<s>/</s> for XLM-R-style checkpoints). When enabled,
// the tokenizer wraps each encoded sequence with the model's special tokens, so
// the adapter reports them with Special=true and offsets that the decoder drops.
// The chunk size must account for them: ChunkTokens plus the number of special
// tokens per sequence must stay below the model's position limit (514 for XLM-R).
func NewHFTokenizerWithOptions(path string, addSpecial bool) (Tokenizer, error) {
	tok, err := tokenizers.FromFile(path)
	if err != nil {
		return nil, fmt.Errorf("tokenizer %s: %w", path, err)
	}
	return &hfTokenizer{tok: tok, addSpecial: addSpecial}, nil
}

// Tokenize encodes text. Special tokens are added only when the tokenizer was
// created with NewHFTokenizerWithOptions(..., true); by default (NewHFTokenizer)
// they are omitted, matching the Phase 0 default where a 400-token chunk is
// exactly 400 model input tokens. Enabling them wraps each sequence with the
// model's <s>/</s> separators (XLM-R style), which the decoder drops via the
// Special flag. This is opt-in: the real checkpoint's requirement is verified
// in Phase 1 (see TODO.md).
func (h *hfTokenizer) Tokenize(text string) ([]Token, error) {
	enc, err := h.tok.EncodeWithOptionsErr(text, h.addSpecial, tokenizers.WithReturnAllAttributes())
	if err != nil {
		return nil, fmt.Errorf("tokenize: %w", err)
	}
	if len(enc.IDs) != len(enc.Offsets) || len(enc.IDs) != len(enc.Tokens) {
		return nil, fmt.Errorf("tokenize: %d ids, %d offsets, %d tokens; want one of each",
			len(enc.IDs), len(enc.Offsets), len(enc.Tokens))
	}
	if len(enc.IDs) == 0 {
		return nil, nil
	}

	out := make([]Token, len(enc.IDs))
	for i, id := range enc.IDs {
		start, end, err := tokenOffsets(len(text), i, enc.Offsets[i][0], enc.Offsets[i][1])
		if err != nil {
			return nil, err
		}
		special := len(enc.SpecialTokensMask) > i && enc.SpecialTokensMask[i] != 0
		out[i] = Token{
			ID:      int64(id),
			Start:   start,
			End:     end,
			Special: special,
		}
	}
	// WordStart needs the validated offsets of the neighbouring tokens, so it is
	// computed after the whole sequence has been converted.
	for i := range out {
		out[i].WordStart = wordStart(text, enc.Tokens, out, i)
	}
	return out, nil
}

// Close releases the Rust tokenizer.
func (h *hfTokenizer) Close() error { return h.tok.Close() }

// tokenOffsets validates one tokenizer offset pair and converts it to int.
//
// The Hugging Face core reports byte offsets into the original text, so a pair
// outside [0, textLen] means the tokenizer and the adapter disagree. That is an
// error, not something to clamp: a silently shortened span hides a
// tokenizer/config mismatch, and a widened one slices the wrong bytes.
func tokenOffsets(textLen, i int, start, end uint) (int, int, error) {
	if start > uint(textLen) || end > uint(textLen) || end < start {
		return 0, 0, fmt.Errorf("token %d: offsets [%d,%d] are outside the text (length %d)", i, start, end, textLen)
	}
	return int(start), int(end), nil
}

// wordStart reports whether tokens[i] begins a word. The library exposes no word
// ids, so the rule is explicit and the pieces are read in this order:
//
//   - a special token never begins a word (its label is dropped anyway);
//   - a WordPiece continuation ("##ing") never begins one;
//   - a SentencePiece "▁" (U+2581) or byte-level BPE "Ġ" (U+0120) marker always
//     does, including a bare marker piece with no letters of its own;
//   - a zero-width piece with no marker occupies no bytes and cannot begin one;
//   - the piece right after a bare marker continues that marker's word, whatever
//     its offsets say;
//   - otherwise the piece begins a word only when the bytes between it and the
//     nearest earlier token that occupies bytes contain whitespace. Zero-width
//     tokens are skipped as that base, so they cannot hide a word boundary.
//     Touching or overlapping offsets continue the word, and a gap of
//     non-whitespace bytes (a dropped control character) never opens one.
func wordStart(text string, pieces []string, tokens []Token, i int) bool {
	t := tokens[i]
	piece := pieces[i]
	switch {
	case t.Special:
		return false
	case strings.HasPrefix(piece, "##"):
		return false
	case hasWordMarker(piece):
		return true
	case t.End == t.Start:
		return false
	}

	// prev is the nearest earlier non-special token; base is the nearest earlier
	// non-special token that occupies at least one byte.
	prev, base := -1, -1
	for j := i - 1; j >= 0 && base < 0; j-- {
		if tokens[j].Special {
			continue
		}
		if prev < 0 {
			prev = j
			if isBareMarker(pieces[j]) {
				return false
			}
		}
		if tokens[j].End > tokens[j].Start {
			base = j
		}
	}
	if base < 0 {
		return true
	}

	gapStart := tokens[base].End
	if t.Start <= gapStart {
		return false
	}
	return strings.ContainsFunc(text[gapStart:t.Start], unicode.IsSpace)
}

// hasWordMarker reports whether a piece carries a SentencePiece or byte-level BPE
// word-start marker.
func hasWordMarker(piece string) bool {
	return strings.HasPrefix(piece, "▁") || strings.HasPrefix(piece, "Ġ")
}

// isBareMarker reports whether a piece is nothing but a word-start marker.
func isBareMarker(piece string) bool {
	return piece == "▁" || piece == "Ġ"
}
