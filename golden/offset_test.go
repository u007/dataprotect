package golden

import (
	"strings"
	"testing"

	"dataprotect/recognizers"
)

// sp/ep build the pointers an offset label carries. Label.Start/End are *int so
// a span at byte 0 is distinguishable from "offsets not given".
func sp(v int) *int { return &v }
func ep(v int) *int { return &v }

// TestResolveUsesExplicitOffsetsForRepeatedValues is the D3 case: 13.9% of
// nemotron positives and 14.9% of gretel positives repeat an in-schema value,
// and the substring path rejects those outright as ambiguous. Offsets locate
// each occurrence without asking the value to be unique.
func TestResolveUsesExplicitOffsetsForRepeatedValues(t *testing.T) {
	c := Case{
		ID:   "dup",
		Text: "Card 4111 1111 1111 1111 and 4111 1111 1111 1111 again",
		Entities: []Label{
			{Type: recognizers.EntityCreditCard, Value: "4111 1111 1111 1111", Start: sp(5), End: ep(24)},
			{Type: recognizers.EntityCreditCard, Value: "4111 1111 1111 1111", Start: sp(29), End: ep(48)},
		},
	}
	gold, err := c.resolve()
	if err != nil {
		t.Fatalf("resolve: unexpected error: %v", err)
	}
	if len(gold) != 2 {
		t.Fatalf("resolve: got %d entities, want 2", len(gold))
	}
	if gold[0].Start != 5 || gold[0].End != 24 {
		t.Errorf("gold[0] = %d..%d, want 5..24", gold[0].Start, gold[0].End)
	}
	if gold[1].Start != 29 || gold[1].End != 48 {
		t.Errorf("gold[1] = %d..%d, want 29..48", gold[1].Start, gold[1].End)
	}
	// The substring path would have rejected this: prove the ambiguity guard
	// still fires for labels that give no offsets.
	if _, err := c.resolve(); err != nil {
		t.Errorf("resolve with offsets should not report ambiguity: %v", err)
	}
}

// TestResolveSubstringPathUnchanged pins backwards compatibility: the committed
// 18-case seed set gives no offsets, and its spans must be byte-for-byte what
// they have always been.
func TestResolveSubstringPathUnchanged(t *testing.T) {
	c := Case{
		ID:       "seed",
		Text:     "reach me at ada@example.com",
		Entities: []Label{{Type: recognizers.EntityEmail, Value: "ada@example.com"}},
	}
	gold, err := c.resolve()
	if err != nil {
		t.Fatalf("resolve: unexpected error: %v", err)
	}
	if len(gold) != 1 {
		t.Fatalf("resolve: got %d entities, want 1", len(gold))
	}
	// Measured, not assumed: "reach me at " is 12 bytes, ada@example.com is 15.
	if gold[0].Start != 12 || gold[0].End != 27 {
		t.Errorf("substring span = %d..%d, want 12..27 (unchanged by the offset branch)",
			gold[0].Start, gold[0].End)
	}
	if gold[0].Text != "ada@example.com" {
		t.Errorf("Text = %q, want %q", gold[0].Text, "ada@example.com")
	}
}

// TestResolveRejectsHalfSpecifiedOffsets: one pointer set and one nil is a
// malformed label. Silently treating nil as zero would move the span.
func TestResolveRejectsHalfSpecifiedOffsets(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start *int
		end   *int
	}{
		{"start only", sp(5), nil},
		{"end only", nil, ep(24)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Case{
				ID:   "half",
				Text: "Card 4111 1111 1111 1111",
				Entities: []Label{
					{Type: recognizers.EntityCreditCard, Value: "4111 1111 1111 1111",
						Start: tc.start, End: tc.end},
				},
			}
			_, err := c.resolve()
			if err == nil {
				t.Fatal("resolve: want an error when only one offset is given, got nil")
			}
			if !strings.Contains(err.Error(), "half-specified") {
				t.Errorf("error %q should say the offsets are half-specified", err.Error())
			}
		})
	}
}

// TestResolveRejectsMidRuneOffset: corpus offsets arrive as character indices.
// If one were mistaken for a byte index it would land inside a multi-byte rune,
// and the resulting entity would slice invalid UTF-8 straight into the report.
func TestResolveRejectsMidRuneOffset(t *testing.T) {
	text := "日本語" // three 3-byte runes
	for _, tc := range []struct {
		name  string
		start int
		end   int
	}{
		{"end inside first rune", 0, 2},
		{"start inside first rune", 1, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Value mirrors the slice so the boundary check, not a value
			// mismatch, is what must reject this.
			c := Case{
				ID:   "mid",
				Text: text,
				Entities: []Label{
					{Type: recognizers.EntityEmail, Value: text[tc.start:tc.end],
						Start: sp(tc.start), End: sp(tc.end)},
				},
			}
			_, err := c.resolve()
			if err == nil {
				t.Fatalf("resolve: want an error for a mid-rune offset (%d..%d), got nil",
					tc.start, tc.end)
			}
			if !strings.Contains(err.Error(), "rune boundary") {
				t.Errorf("error %q should name the rune-boundary problem", err.Error())
			}
		})
	}
}

// TestResolveRejectsOutOfBoundsOffset catches an off-by-one against the text
// length before the slice itself panics or, worse, silently clips.
func TestResolveRejectsOutOfBoundsOffset(t *testing.T) {
	text := "Card 4111 1111 1111 1111"
	for _, tc := range []struct {
		name  string
		start int
		end   int
	}{
		{"end past the text", 5, len(text) + 1},
		{"start negative", -1, 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Case{
				ID:   "oob",
				Text: text,
				Entities: []Label{
					{Type: recognizers.EntityCreditCard, Value: "4111 1111 1111 1111",
						Start: sp(tc.start), End: sp(tc.end)},
				},
			}
			_, err := c.resolve()
			if err == nil {
				t.Fatalf("resolve: want an error for %d..%d, got nil", tc.start, tc.end)
			}
			if !strings.Contains(err.Error(), "out of bounds") {
				t.Errorf("error %q should name the bounds problem", err.Error())
			}
		})
	}
}

// TestResolveRejectsZeroLengthOffset: an empty span can never match a
// prediction, so it would count as a false negative with nothing to point at.
func TestResolveRejectsZeroLengthOffset(t *testing.T) {
	c := Case{
		ID:   "zero",
		Text: "Card 4111 1111 1111 1111",
		Entities: []Label{
			{Type: recognizers.EntityCreditCard, Value: "4111 1111 1111 1111",
				Start: sp(5), End: sp(5)},
		},
	}
	_, err := c.resolve()
	if err == nil {
		t.Fatal("resolve: want an error for a zero-length span, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error %q should say the span is empty", err.Error())
	}
}

// TestResolveRejectsValueOffsetMismatch is the converter's safety net: Value is
// derived from the same slice as the offsets, so they must agree. A mismatch
// means an offset translation bug, and the label would otherwise be reported
// with text it does not cover.
func TestResolveRejectsValueOffsetMismatch(t *testing.T) {
	c := Case{
		ID:   "mismatch",
		Text: "Card 4111 1111 1111 1111",
		Entities: []Label{
			{Type: recognizers.EntityCreditCard, Value: "9999 9999 9999 9999",
				Start: sp(5), End: ep(24)},
		},
	}
	_, err := c.resolve()
	if err == nil {
		t.Fatal("resolve: want an error when Value does not match the offsets, got nil")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Errorf("error %q should say Value and offsets disagree", err.Error())
	}
}
