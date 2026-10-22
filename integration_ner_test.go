//go:build integration

package piiscrub

import (
	"context"
	"os"
	"testing"

	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/nlp"
)

func TestIntegrationNER(t *testing.T) {
	url := os.Getenv("PII_SCRUB_NLP_URL")
	if url == "" {
		t.Skip("PII_SCRUB_NLP_URL not set")
	}
	s := NewScrubberWithRecognizers(nlp.New(url))
	entities, err := s.Analyze(context.Background(), "John Smith lives in New York.")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("entities: %+v", entities)
	found := false
	for _, e := range entities {
		if e.Type == recognizers.EntityPerson {
			found = true
		}
	}
	if !found {
		t.Fatal("expected PERSON entity")
	}
}

// TestNERConfidenceContract pins the real sidecar's emitted score to the
// constant the combination test uses. The two modules cannot share it (the
// sidecar imports this module), so without this test a change to
// sidecar/internal/ner.DefaultConfidence could leave the root tests asserting
// a value the sidecar no longer emits.
func TestNERConfidenceContract(t *testing.T) {
	url := os.Getenv("PII_SCRUB_NLP_URL")
	if url == "" {
		if os.Getenv("PII_SCRUB_NLP_REQUIRE") != "" {
			t.Fatal("PII_SCRUB_NLP_URL not set")
		}
		t.Skip("PII_SCRUB_NLP_URL not set")
	}

	entities, err := nlp.New(url).Recognize(context.Background(), "John Smith lives in New York.")
	if err != nil {
		t.Fatal(err)
	}
	if len(entities) == 0 {
		t.Fatal("expected at least one NLP entity")
	}
	for _, e := range entities {
		if e.Confidence != nlpCombinationConfidence {
			t.Errorf("%s %q confidence = %v, want %v (sidecar DefaultConfidence)",
				e.Type, e.Text, e.Confidence, nlpCombinationConfidence)
		}
	}
}
