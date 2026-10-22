package piiscrub

import (
	"context"
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// fixedRecognizer emits one PERSON span at a fixed confidence, so the
// confidence filter can be tested without depending on any built-in score.
type fixedRecognizer struct {
	text       string
	confidence float64
}

func (r fixedRecognizer) Name() string { return "fixed" }

func (r fixedRecognizer) Recognize(_ context.Context, text string) ([]recognizers.Entity, error) {
	start := strings.Index(text, r.text)
	if start < 0 {
		return nil, nil
	}
	return []recognizers.Entity{{
		Type:       recognizers.EntityPerson,
		Start:      start,
		End:        start + len(r.text),
		Text:       r.text,
		Confidence: r.confidence,
	}}, nil
}

// TestScrubberWithMinConfidence covers the public WithMinConfidence option and
// the default floor it overrides.
func TestScrubberWithMinConfidence(t *testing.T) {
	const text = "John Smith lives here"

	tests := []struct {
		name       string
		confidence float64
		threshold  float64 // 0 means "use the default 0.70"
		wantKept   bool
	}{
		{name: "default floor keeps 0.80", confidence: 0.80, wantKept: true},
		{name: "default floor keeps 0.70", confidence: 0.70, wantKept: true},
		{name: "default floor drops 0.69", confidence: 0.69, wantKept: false},
		{name: "threshold above score drops it", confidence: 0.80, threshold: 0.85, wantKept: false},
		{name: "threshold equal to score keeps it", confidence: 0.80, threshold: 0.80, wantKept: true},
		{name: "threshold below score keeps it", confidence: 0.90, threshold: 0.85, wantKept: true},
		{name: "lower threshold keeps a low score", confidence: 0.50, threshold: 0.40, wantKept: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []ScrubberOption{
				WithRecognizer(fixedRecognizer{text: "John Smith", confidence: tt.confidence}),
			}
			if tt.threshold != 0 {
				opts = append(opts, WithMinConfidence(tt.threshold))
			}

			entities, err := NewScrubber(opts...).Analyze(context.Background(), text)
			if err != nil {
				t.Fatalf("Analyze: %v", err)
			}

			kept := len(entities) == 1
			if kept != tt.wantKept {
				t.Fatalf("kept = %v, want %v (confidence %v, threshold %v): %+v",
					kept, tt.wantKept, tt.confidence, tt.threshold, entities)
			}
		})
	}
}
