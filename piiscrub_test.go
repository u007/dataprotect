package piiscrub

import (
	"context"
	"strings"
	"testing"

	"dataprotect/recognizers"
)

func TestScrubberAnonymize(t *testing.T) {
	s := NewScrubber()
	ctx := context.Background()

	text := "Contact john@example.com or call 555-123-4567. Card 4111111111111111."
	got, err := s.Anonymize(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, leak := range []string{"john@example.com", "555-123-4567", "4111111111111111"} {
		if strings.Contains(got, leak) {
			t.Errorf("output still contains %q: %q", leak, got)
		}
	}
	if !strings.Contains(got, "<EMAIL>") {
		t.Errorf("email was not replaced: %q", got)
	}
	if !strings.Contains(got, "123-4567") && !strings.Contains(got, "4567") {
		t.Errorf("phone tail was not preserved: %q", got)
	}
	if !strings.Contains(got, "1111") {
		t.Errorf("card tail was not preserved: %q", got)
	}
	if !strings.HasPrefix(got, "Contact ") {
		t.Errorf("surrounding prose was damaged: %q", got)
	}
}

func TestScrubberAnalyze(t *testing.T) {
	s := NewScrubber()
	ctx := context.Background()

	text := "mail a@b.com about card 4111111111111111"
	entities, err := s.Analyze(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	types := map[recognizers.EntityType]bool{}
	for _, e := range entities {
		types[e.Type] = true
		if e.Text != text[e.Start:e.End] {
			t.Errorf("span mismatch for %+v", e)
		}
	}
	if !types[recognizers.EntityEmail] {
		t.Errorf("email not detected, got types %v", types)
	}
	// The card is recognized as a credit card; it may also match phone and SSN
	// patterns, and overlap resolution keeps only the highest-confidence one.
	if !types[recognizers.EntityCreditCard] {
		t.Errorf("credit card not detected, got types %v", types)
	}
}

func TestScrubberNoPII(t *testing.T) {
	s := NewScrubber()
	text := "The weather is nice today."
	got, err := s.Anonymize(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != text {
		t.Errorf("text without PII was modified: %q", got)
	}
}

func TestScrubberEmpty(t *testing.T) {
	s := NewScrubber()
	got, err := s.Anonymize(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
