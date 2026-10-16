package golden_test

import (
	"context"
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
		if !golden.PseudonymRoundTrip(c.Text, out, pseudonyms) {
			t.Errorf("case %s: pseudonymized text %q does not restore to %q", c.ID, out, c.Text)
		}
	}
}
