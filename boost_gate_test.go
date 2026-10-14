package piiscrub

import (
	"context"
	"testing"

	"dataprotect/analyzer"
	"dataprotect/golden"
)

// TestContextBoostIsNoWorseOnSeed scores the seed set twice, once with the
// default pipeline and once with context boosting on. The golden gate runs the
// default pipeline only, so without this check boosting could be enabled and
// degrade results unnoticed. Boosting must not lower exact precision or recall.
func TestContextBoostIsNoWorseOnSeed(t *testing.T) {
	off := scoreSeed(t, NewScrubber())
	on := scoreSeed(t, newScrubber(analyzer.WithContextBoost()))

	t.Logf("boost off: exact P=%.4f R=%.4f (tp=%d fp=%d fn=%d)",
		off.Exact.Precision, off.Exact.Recall, off.Exact.TP, off.Exact.FP, off.Exact.FN)
	t.Logf("boost on:  exact P=%.4f R=%.4f (tp=%d fp=%d fn=%d)",
		on.Exact.Precision, on.Exact.Recall, on.Exact.TP, on.Exact.FP, on.Exact.FN)

	if on.Exact.Precision < off.Exact.Precision {
		t.Errorf("boosting lowered exact precision: %.4f -> %.4f", off.Exact.Precision, on.Exact.Precision)
	}
	if on.Exact.Recall < off.Exact.Recall {
		t.Errorf("boosting lowered exact recall: %.4f -> %.4f", off.Exact.Recall, on.Exact.Recall)
	}
}

// scoreSeed runs the seed set through s and fails on any dataset or predictor error.
func scoreSeed(t *testing.T, s *Scrubber) *golden.Report {
	t.Helper()
	rep, err := golden.Score(context.Background(), "testdata/golden",
		func(ctx context.Context, text string) ([]golden.Prediction, error) {
			return s.Analyze(ctx, text)
		})
	if err != nil {
		t.Fatalf("scoring golden dataset: %v", err)
	}
	if len(rep.Errors) > 0 {
		t.Fatalf("dataset/predictor errors (%d): %v", len(rep.Errors), rep.Errors)
	}
	return rep
}
