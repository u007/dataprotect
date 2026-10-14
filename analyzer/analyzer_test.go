package analyzer

import (
	"context"
	"testing"

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
