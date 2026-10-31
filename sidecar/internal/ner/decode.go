package ner

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/u007/dataprotect/recognizers"
)

// Token is one tokenizer output. Offsets are UTF-8 byte offsets into the
// original text; the decoder never handles rune offsets.
type Token struct {
	ID        int64
	Start     int  // byte offset in the original text
	End       int  // byte offset, exclusive
	Special   bool // tokenizer special mask, never inferred from the index
	WordStart bool // true when the token begins a word
}

// Tokenizer produces tokens with byte offsets.
type Tokenizer interface {
	Tokenize(text string) ([]Token, error)
}

// Session runs one ONNX inference.
type Session interface {
	InputNames() []string
	Run(ctx context.Context, feeds map[string][]int64) ([][]float32, error)
}

// rawSpan is an unexported intermediate span carrying the softmax probability
// used for overlap tie-breaking. Confidence is fixed later.
type rawSpan struct {
	start int
	end   int
	typ   recognizers.EntityType
	prob  float64
	toks  int
}

const (
	maxSpanTokens = 16
	maxSpanBytes  = 256
)

// Decode turns token-level argmax labels into entity spans. It is pure Go and
// has no dependency on ONNX Runtime or the tokenizer implementation.
func Decode(text string, tokens []Token, logits [][]float32, labels []string) ([]recognizers.Entity, error) {
	if len(tokens) != len(logits) {
		return nil, fmt.Errorf("decode: %d tokens but %d logit rows", len(tokens), len(logits))
	}
	for i, row := range logits {
		if len(row) == 0 {
			return nil, fmt.Errorf("decode: empty logit row at token %d", i)
		}
		if len(row) != len(labels) {
			return nil, fmt.Errorf("decode: logit row %d has %d values, want %d labels", i, len(row), len(labels))
		}
	}

	var open *rawSpan
	var spans []rawSpan
	closeOpen := func() {
		if open == nil {
			return
		}
		if open.toks <= maxSpanTokens && (open.end-open.start) <= maxSpanBytes {
			segment := text
			if open.start >= 0 && open.end <= len(text) {
				segment = text[open.start:open.end]
			}
			if !isWhitespaceOnly(segment) {
				spans = append(spans, *open)
			}
		}
		open = nil
	}

	for i, tok := range tokens {
		// Clamp offsets defensively; the adapter is the real owner of this.
		if tok.Start < 0 {
			tok.Start = 0
		}
		if tok.End > len(text) {
			tok.End = len(text)
		}
		if tok.End < tok.Start {
			tok.End = tok.Start
		}

		if tok.Special {
			closeOpen()
			continue
		}

		idx, prob := argmax(logits[i])
		tag := ""
		if idx >= 0 && idx < len(labels) {
			tag = labels[idx]
		}

		if tag == "O" || tag == "" {
			closeOpen()
			continue
		}

		typ, ok := mapLabel(tag)
		if !ok {
			closeOpen()
			continue
		}

		prefix := labelPrefix(tag)

		// Continuation of an open span (I-/E-) takes precedence over the
		// word-start rule: a multi-word entity spans separate words.
		if open != nil && typ == open.typ && (prefix == "I" || prefix == "E") {
			open.end = tok.End
			if prob > open.prob {
				open.prob = prob
			}
			open.toks++
			if open.toks > maxSpanTokens || (open.end-open.start) > maxSpanBytes {
				open = nil
			} else if prefix == "E" {
				closeOpen()
			}
			continue
		}

		// A continuation (subword) piece never opens a span.
		if !tok.WordStart {
			// It may still continue an open span of the same type.
			if open != nil && typ == open.typ {
				open.end = tok.End
				if prob > open.prob {
					open.prob = prob
				}
				open.toks++
				if open.toks > maxSpanTokens || (open.end-open.start) > maxSpanBytes {
					open = nil
				}
				continue
			}
			// Drop the token; an open span survives.
			continue
		}

		// Word start: close any open span first.
		closeOpen()

		open = &rawSpan{start: tok.Start, end: tok.End, typ: typ, prob: prob, toks: 1}
		if prefix == "S" || prefix == "E" || prefix == "" {
			closeOpen()
		}
	}
	closeOpen()

	spans = dedupSpans(spans)
	spans = resolveOverlaps(spans)

	out := make([]recognizers.Entity, 0, len(spans))
	for _, s := range spans {
		out = append(out, recognizers.Entity{
			Type:       s.typ,
			Start:      s.start,
			End:        s.end,
			Text:       text[s.start:s.end],
			Confidence: 0.80,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		return out[i].End < out[j].End
	})
	return out, nil
}

func mapLabel(tag string) (recognizers.EntityType, bool) {
	base := tag
	for _, p := range []string{"B-", "I-", "E-", "S-"} {
		if strings.HasPrefix(base, p) {
			base = base[len(p):]
			break
		}
	}
	switch strings.ToUpper(base) {
	case "PER", "PERSON":
		return recognizers.EntityPerson, true
	case "ORG", "ORGANIZATION":
		return recognizers.EntityOrganization, true
	case "LOC", "GPE", "LOCATION":
		return recognizers.EntityLocation, true
	}
	return "", false
}

func labelPrefix(tag string) string {
	for _, p := range []string{"B-", "I-", "E-", "S-"} {
		if strings.HasPrefix(tag, p) {
			return p[:1]
		}
	}
	return ""
}

func argmax(vals []float32) (int, float64) {
	best := 0
	bestVal := vals[0]
	for i := 1; i < len(vals); i++ {
		if vals[i] > bestVal {
			bestVal = vals[i]
			best = i
		}
	}
	var sum float64
	for _, v := range vals {
		sum += math.Exp(float64(v) - float64(bestVal))
	}
	prob := 1.0
	if sum > 0 {
		prob = 1.0 / sum
	}
	return best, prob
}

func isWhitespaceOnly(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func dedupSpans(spans []rawSpan) []rawSpan {
	seen := make(map[string]bool)
	out := spans[:0]
	for _, s := range spans {
		key := fmt.Sprintf("%d|%d|%s", s.start, s.end, s.typ)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

func resolveOverlaps(spans []rawSpan) []rawSpan {
	if len(spans) == 0 {
		return spans
	}
	sorted := append([]rawSpan(nil), spans...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].start != sorted[j].start {
			return sorted[i].start < sorted[j].start
		}
		return sorted[i].end < sorted[j].end
	})

	var groups [][]rawSpan
	var cur []rawSpan
	curEnd := sorted[0].end
	for _, s := range sorted {
		if len(cur) > 0 && s.start < curEnd {
			cur = append(cur, s)
			if s.end > curEnd {
				curEnd = s.end
			}
		} else {
			if len(cur) > 0 {
				groups = append(groups, cur)
			}
			cur = []rawSpan{s}
			curEnd = s.end
		}
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}

	out := make([]rawSpan, 0, len(groups))
	for _, g := range groups {
		best := g[0]
		for _, s := range g[1:] {
			bestLen := best.end - best.start
			sLen := s.end - s.start
			if sLen > bestLen || (sLen == bestLen && s.prob > best.prob) {
				best = s
			}
		}
		out = append(out, best)
	}
	return out
}
