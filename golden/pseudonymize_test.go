package golden_test

import (
	"context"
	"strings"
	"testing"

	piiscrub "github.com/u007/dataprotect"
	"github.com/u007/dataprotect/golden"
)

// TestPseudonymizeRoundTripOnSeed checks the restore promise on every seed case:
// substituting each placeholder back to its original value reproduces the input.
// The gate does not score Pseudonymize, so this is its only check over the seed.
func TestPseudonymizeRoundTripOnSeed(t *testing.T) {
	cases, err := golden.Load(datasetDir)
	if err != nil {
		t.Fatalf("loading seed: %v", err)
	}
	s := piiscrub.NewScrubber()
	for _, c := range cases {
		out, pseudonyms, err := s.Pseudonymize(context.Background(), c.Text)
		if err != nil {
			t.Errorf("case %s: %v", c.ID, err)
			continue
		}
		var pairs []string
		for placeholder, original := range pseudonyms {
			pairs = append(pairs, placeholder, original)
		}
		if got := strings.NewReplacer(pairs...).Replace(out); got != c.Text {
			t.Errorf("case %s: restore(out) = %q, want %q", c.ID, got, c.Text)
		}
	}
}
