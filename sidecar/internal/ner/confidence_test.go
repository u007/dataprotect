package ner

import "testing"

// TestDefaultConfidenceWindow pins the contract between the sidecar's score and
// the root scrubber's confidence filter and overlap resolution.
//
// Both boundaries live outside this module:
//
//   - the default scrubber floor is 0.70 (root piiscrub.go, newScrubber), and
//   - the lowest built-in recognizer is 0.85 (national MYIC, SG passport and
//     MY passport rules in recognizers/national/national.go).
//
// Below the floor, the analyzer filters every NLP entity out and the sidecar is
// silently useless. At or above the ceiling, a 0.85 tie against a national ID
// or passport can keep the NLP span, breaking the rule that NLP fills gaps and
// never overrides a typed detection. The root module cannot import this
// constant (the sidecar imports the root module), so this test is the guard
// that keeps the number honest.
func TestDefaultConfidenceWindow(t *testing.T) {
	const (
		defaultScrubberFloor = 0.70
		lowestBuiltin        = 0.85
	)

	if DefaultConfidence <= defaultScrubberFloor {
		t.Fatalf("DefaultConfidence = %v, must be strictly above the default scrubber floor %v or every NLP entity is filtered out",
			DefaultConfidence, defaultScrubberFloor)
	}
	if DefaultConfidence >= lowestBuiltin {
		t.Fatalf("DefaultConfidence = %v, must be strictly below the lowest built-in recognizer %v or NLP can displace a typed detection on overlap",
			DefaultConfidence, lowestBuiltin)
	}
}
