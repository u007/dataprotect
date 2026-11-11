package analyzer

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/u007/dataprotect/recognizers"
)

// stubRecognizer returns a fixed entity list, for wiring tests.
type stubRecognizer struct {
	name     string
	entities []recognizers.Entity
	err      error
}

func (s *stubRecognizer) Name() string { return s.name }
func (s *stubRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	return s.entities, s.err
}

func TestAnalyzeEmpty(t *testing.T) {
	a := New()
	entities, err := a.Analyze(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0", len(entities))
	}
}

func TestAnalyzeSortsByStart(t *testing.T) {
	a := New()
	a.AddRecognizer(&stubRecognizer{
		name: "late",
		entities: []recognizers.Entity{
			{Type: recognizers.EntityURL, Start: 20, End: 30, Text: "late", Confidence: 0.9},
		},
	})
	a.AddRecognizer(&stubRecognizer{
		name: "early",
		entities: []recognizers.Entity{
			{Type: recognizers.EntityEmail, Start: 0, End: 5, Text: "early", Confidence: 0.9},
			{Type: recognizers.EntityIP, Start: 10, End: 15, Text: "mid", Confidence: 0.9},
		},
	})

	entities, err := a.Analyze(context.Background(), "some text here for padding")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 3 {
		t.Fatalf("got %d entities, want 3", len(entities))
	}
	if entities[0].Start != 0 || entities[1].Start != 10 || entities[2].Start != 20 {
		t.Errorf("entities not sorted by start: %d, %d, %d",
			entities[0].Start, entities[1].Start, entities[2].Start)
	}
}

func TestAnalyzeConfidenceFilter(t *testing.T) {
	a := New(WithMinConfidence(0.8))
	a.AddRecognizer(&stubRecognizer{
		name: "mixed",
		entities: []recognizers.Entity{
			{Type: recognizers.EntityEmail, Start: 0, End: 5, Text: "keep", Confidence: 0.95},
			{Type: recognizers.EntityPhone, Start: 10, End: 15, Text: "drop", Confidence: 0.5},
		},
	})

	entities, err := a.Analyze(context.Background(), "keep  drop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("got %d entities, want 1", len(entities))
	}
	if entities[0].Text != "keep" {
		t.Errorf("got %q, want keep", entities[0].Text)
	}
}

// Overlapping detections must collapse to one entity, keeping the higher
// confidence, so downstream operators never rewrite overlapping spans twice.
func TestAnalyzeRemovesOverlaps(t *testing.T) {
	a := New()
	a.AddRecognizer(&stubRecognizer{
		name: "low",
		entities: []recognizers.Entity{
			{Type: recognizers.EntityCreditCard, Start: 0, End: 16, Text: "4111111111111111", Confidence: 0.7},
		},
	})
	a.AddRecognizer(&stubRecognizer{
		name: "high",
		entities: []recognizers.Entity{
			{Type: recognizers.EntitySSN, Start: 4, End: 10, Text: "111111", Confidence: 0.99},
		},
	})

	entities, err := a.Analyze(context.Background(), "4111111111111111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("got %d entities, want 1 after overlap removal: %+v", len(entities), entities)
	}
	if entities[0].Confidence != 0.99 {
		t.Errorf("kept confidence %f, want the higher 0.99", entities[0].Confidence)
	}
}

func TestAnalyzePropagatesError(t *testing.T) {
	a := New()
	a.AddRecognizer(&stubRecognizer{name: "boom", err: context.Canceled})

	if _, err := a.Analyze(context.Background(), "text"); err != context.Canceled {
		t.Errorf("got err %v, want context.Canceled", err)
	}
}

// stubJudge confirms or rejects weak candidates for test purposes.
type stubJudge struct {
	confirm bool
	err     error
}

func (s *stubJudge) Judge(ctx context.Context, entityType recognizers.EntityType, candidate, window string) (bool, error) {
	return s.confirm, s.err
}

// TestAnalyzeWeakCandidateDroppedWithoutJudge verifies that a weak candidate
// is dropped when no judge is configured, even with minConfidence below its
// score.
func TestAnalyzeWeakCandidateDroppedWithoutJudge(t *testing.T) {
	a := New(WithMinConfidence(0.0))
	a.AddRecognizer(&stubRecognizer{
		name: "weak",
		entities: []recognizers.Entity{
			{Type: recognizers.EntityMYIC, Start: 0, End: 12, Text: "900515103421", Confidence: 0.50, Weak: true},
		},
	})

	entities, err := a.Analyze(context.Background(), "900515103421 on file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0 (weak candidate dropped without judge)", len(entities))
	}
}

// TestAnalyzeWeakCandidateConfirmedByJudge verifies that a weak candidate
// confirmed by the judge is promoted to keywordGateConfidence and reported.
func TestAnalyzeWeakCandidateConfirmedByJudge(t *testing.T) {
	a := New(WithMinConfidence(0.7), WithLLMJudge(&stubJudge{confirm: true}))
	a.AddRecognizer(&stubRecognizer{
		name: "weak",
		entities: []recognizers.Entity{
			{Type: recognizers.EntityMYIC, Start: 0, End: 12, Text: "900515103421", Confidence: 0.50, Weak: true},
		},
	})

	entities, err := a.Analyze(context.Background(), "MyKad 900515103421 on file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("got %d entities, want 1", len(entities))
	}
	if entities[0].Confidence != keywordGateConfidence {
		t.Errorf("confidence = %v, want %v (promoted by judge)", entities[0].Confidence, keywordGateConfidence)
	}
	if entities[0].Weak {
		t.Errorf("Weak = true, want false (confirmed candidate is no longer weak)")
	}
}

// TestAnalyzeWeakCandidateRejectedByJudge verifies that a weak candidate
// rejected by the judge is dropped.
func TestAnalyzeWeakCandidateRejectedByJudge(t *testing.T) {
	a := New(WithMinConfidence(0.0), WithLLMJudge(&stubJudge{confirm: false}))
	a.AddRecognizer(&stubRecognizer{
		name: "weak",
		entities: []recognizers.Entity{
			{Type: recognizers.EntityMYIC, Start: 0, End: 12, Text: "900515103421", Confidence: 0.50, Weak: true},
		},
	})

	entities, err := a.Analyze(context.Background(), "order 900515103421 shipped")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0 (weak candidate rejected by judge)", len(entities))
	}
}

// TestAnalyzeJudgeErrorPropagates verifies that a judge error aborts Analyze.
func TestAnalyzeJudgeErrorPropagates(t *testing.T) {
	a := New(WithLLMJudge(&stubJudge{err: context.Canceled}))
	a.AddRecognizer(&stubRecognizer{
		name: "weak",
		entities: []recognizers.Entity{
			{Type: recognizers.EntityMYIC, Start: 0, End: 12, Text: "900515103421", Confidence: 0.50, Weak: true},
		},
	})

	if _, err := a.Analyze(context.Background(), "900515103421 on file"); err != context.Canceled {
		t.Errorf("got err %v, want context.Canceled", err)
	}
}

// TestAnalyzeTypedNilJudgeTreatedAsDisabled verifies that a typed-nil Judge
// (a non-nil interface wrapping a nil pointer) is treated as disabled, so
// weak candidates are dropped instead of panicking.
func TestAnalyzeTypedNilJudgeTreatedAsDisabled(t *testing.T) {
	var j *stubJudge // nil pointer
	a := New(WithMinConfidence(0.0), WithLLMJudge(j))
	a.AddRecognizer(&stubRecognizer{
		name: "weak",
		entities: []recognizers.Entity{
			{Type: recognizers.EntityMYIC, Start: 0, End: 12, Text: "900515103421", Confidence: 0.50, Weak: true},
		},
	})
	entities, err := a.Analyze(context.Background(), "900515103421 on file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0 (typed-nil judge disabled)", len(entities))
	}
}

// TestAnalyzeJudgeErrorDiscardsStrongEntities verifies that a judge error
// aborts Analyze and discards every entity, including strong non-weak ones.
// This is intentional fail-closed behavior: the caller must know the judge
// failed rather than receive a partial result with weak candidates silently
// skipped.
func TestAnalyzeJudgeErrorDiscardsStrongEntities(t *testing.T) {
	a := New(WithMinConfidence(0.0), WithLLMJudge(&stubJudge{err: context.Canceled}))
	a.AddRecognizer(&stubRecognizer{
		name: "strong",
		entities: []recognizers.Entity{
			{Type: recognizers.EntityEmail, Start: 0, End: 16, Text: "a@b.com", Confidence: 0.85, Weak: false},
		},
	})
	a.AddRecognizer(&stubRecognizer{
		name: "weak",
		entities: []recognizers.Entity{
			{Type: recognizers.EntityMYIC, Start: 20, End: 32, Text: "900515103421", Confidence: 0.50, Weak: true},
		},
	})

	_, err := a.Analyze(context.Background(), "a@b.com and 900515103421")
	if err != context.Canceled {
		t.Errorf("got err %v, want context.Canceled (strong entities discarded on judge error)", err)
	}
}

// TestWindowAroundMultiByte verifies that windowAround never splits a rune
// when the surrounding text contains multi-byte characters.
func TestWindowAroundMultiByte(t *testing.T) {
	// "café " is 6 bytes (e-acute is 2 bytes); candidate follows.
	text := "café 900515103421 on file"
	// locate the candidate byte offsets
	start := len("café ")
	end := start + 12
	got := windowAround(text, start, end)
	if !utf8.ValidString(got) {
		t.Fatalf("windowAround produced invalid UTF-8: %q", got)
	}
	// The left window should include the multi-byte prefix without splitting it.
	if !strings.Contains(got, "café") {
		t.Errorf("left context missing multi-byte prefix: %q", got)
	}
}
