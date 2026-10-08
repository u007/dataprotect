package golden_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	piiscrub "dataprotect"
	"dataprotect/golden"
)

// datasetDir is the checked-in hand-labelled seed set. It is relative to this
// package, so the gate runs from any working directory.
const datasetDir = "../testdata/golden"

// minCases guards against a green gate produced by an empty or truncated
// dataset. With few cases every metric is coarse, and a single flipped case
// moves recall by a large fraction — so a shrinking dataset would silently
// weaken the gate rather than fail it.
const minCases = 12

// Gate thresholds.
//
// These are the MEASURED baseline, not a target. The seed set is 18 cases, far
// too small to claim SPEC.md's ">99% precision/recall"; claiming it here would
// make the gate a rubber stamp. Each constant is the EXACT measured value, so
// any regression fails immediately and any improvement is a deliberate
// threshold bump in this file rather than a silent drift.
//
// The measured baseline is exact P=1.0000 R=1.0000 (tp=13 fp=0 fn=0) and
// overlap P=1.0000 R=1.0000 (tp=13 fp=0 fn=0); all six negative cases are
// clean. It is perfect, which is a property of this 18-case seed set, NOT a
// claim about the detector in general — SPEC.md's ">99%" needs the ~500-case
// corpora in testdata/external, which nothing can score yet (see TODO.md). With
// 13 positives one flipped case still moves recall by ~8%, so the number is
// coarse; it is kept exact so a regression fails immediately.
//
// This baseline was reached by fixing the parenthesised-phone defect (the phone
// recognizer dropped the leading "("), which had been the single exact false
// negative AND the single exact false positive — exact P/R were 12/13 before.
// That is the gate working as intended: the defect was measurable, and fixing
// it raised the recorded baseline instead of breaking it.
//
// An earlier baseline (exact P=0.8571 R=0.9231, tp=12 fp=2 fn=1) preceded the
// checksum-validator wiring. CREDIT_CARD went from P=0.7500 to P=1.0000 and all
// negative cases became clean, because cards are now validated rather than
// matched on shape alone.
//
// While a threshold sits below 1.0, write it as the measured tp/(tp+fp) and
// tp/(tp+fn) RATIO rather than a rounded literal: 10/11 is 0.9090909..., so a
// literal 0.9091 is a hair ABOVE the true value and the gate would fail on a run
// that exactly met its baseline — a rounding trap, not a regression. Deriving
// the constant from the counts makes the comparison exact and self-documenting.
//
// The current baseline is perfect, so all three are exactly 1.0 and are written
// as literals: a ratio with an equal numerator and denominator (13.0/13.0) adds
// nothing here and only draws a "identical expressions" lint warning. The counts
// each literal represents are in the trailing comments. If a case is added that
// is not yet passed, re-introduce the ratio form for the affected constant.
const (
	minExactPrecision = 1.0 // tp=13 fp=0 -> 13/13
	minExactRecall    = 1.0 // tp=13 fn=0 -> 13/13
	minOverlapRecall  = 1.0 // tp=13 fn=0 -> 13/13
)

// TestGoldenDatasetAccuracy scores the real detector against the hand-labelled
// seed set and fails on regression.
//
// Labels in testdata/golden are written from the TEXT, never copied from the
// detector's output. Deriving labels that way would make this gate circular: it
// would assert that the detector agrees with itself and could never catch a
// wrong detector, which is the entire purpose of the exercise.
func TestGoldenDatasetAccuracy(t *testing.T) {
	cases, err := golden.Load(datasetDir)
	if err != nil {
		t.Fatalf("loading golden dataset: %v", err)
	}
	if len(cases) < minCases {
		t.Fatalf("golden dataset has %d cases, want at least %d; a small dataset makes every "+
			"metric coarse and lets a regression hide in the rounding", len(cases), minCases)
	}

	scrubber := piiscrub.NewScrubber()
	rep, err := golden.Score(context.Background(), datasetDir,
		func(ctx context.Context, text string) ([]golden.Prediction, error) {
			return scrubber.Analyze(ctx, text)
		})
	if err != nil {
		t.Fatalf("scoring golden dataset: %v", err)
	}

	// A broken label or a detector error is a dataset/tooling failure, not a
	// quality regression. Reporting it separately stops a mislabelled case from
	// being read as "the detector got worse".
	if len(rep.Errors) > 0 {
		t.Errorf("dataset/predictor errors (%d) — these invalidate the run:\n%v", len(rep.Errors), rep.Errors)
	}
	if rep.Exact.TP+rep.Exact.FP+rep.Exact.FN == 0 {
		t.Fatalf("no entities were scored at all; the dataset is not exercising the detector:\n%s", rep.Summary())
	}
	// Without negative cases precision is a tautology: predicting nothing scores
	// perfectly. A dataset of only positive cases must not pass the gate.
	if rep.Exact.Negatives == 0 {
		t.Error("dataset contains no negative cases, so precision is not a real measurement")
	}
	if rep.Exact.NegativesClean != rep.Exact.Negatives {
		t.Logf("note: %d of %d negative cases produced detections (known defects, see testdata notes)",
			rep.Exact.Negatives-rep.Exact.NegativesClean, rep.Exact.Negatives)
	}

	if rep.Exact.Precision < minExactPrecision {
		t.Errorf("exact precision %.4f is below the %.2f baseline (false positives regressed)\n%s",
			rep.Exact.Precision, minExactPrecision, rep.Summary())
	}
	if rep.Exact.Recall < minExactRecall {
		t.Errorf("exact recall %.4f is below the %.2f baseline (detections regressed)\n%s",
			rep.Exact.Recall, minExactRecall, rep.Summary())
	}
	// Overlap recall is the more forgiving measure and sits higher: it credits a
	// span the detector found but mis-sized. A gap between the two is the
	// diagnostic that localises a boundary bug.
	if rep.Overlap.Recall < minOverlapRecall {
		t.Errorf("overlap recall %.4f is below the %.2f baseline\n%s",
			rep.Overlap.Recall, minOverlapRecall, rep.Summary())
	}
	if rep.Overlap.Recall < rep.Exact.Recall {
		t.Errorf("overlap recall %.4f is BELOW exact recall %.4f, which is impossible — "+
			"overlap matching is strictly more permissive, so this indicates a scoring bug",
			rep.Overlap.Recall, rep.Exact.Recall)
	}
}

// TestGoldenDatasetHasNoDuplicateLabels checks dataset hygiene independent of
// any detector: every labelled value must be unambiguous within its own text.
// resolve() already rejects these, but asserting it here means a future case
// edit that introduces one fails with a clear dataset error rather than
// silently dropping the case from the metrics.
func TestGoldenDatasetHasNoDuplicateLabels(t *testing.T) {
	cases, err := golden.Load(datasetDir)
	if err != nil {
		t.Fatalf("loading golden dataset: %v", err)
	}
	seenIDs := make(map[string]bool, len(cases))
	for _, c := range cases {
		if seenIDs[c.ID] {
			t.Errorf("duplicate case id %q", c.ID)
		}
		seenIDs[c.ID] = true

		if len(c.Entities) == 0 {
			continue
		}
		// Labels must be listed in text order, or resolve() rejects the case.
		cursor := 0
		for i, l := range c.Entities {
			rel := indexFrom(c.Text, l.Value, cursor)
			if rel < 0 {
				t.Errorf("case %q label %d (%q) is absent or out of text order", c.ID, i, l.Value)
				break
			}
			cursor = rel + len(l.Value)
		}
	}
}

// indexFrom is strings.Index over text[cursor:], or -1 when cursor is past the
// end or the value is not present at or after it.
func indexFrom(text, value string, cursor int) int {
	if cursor > len(text) {
		return -1
	}
	rel := strings.Index(text[cursor:], value)
	if rel < 0 {
		return -1
	}
	return cursor + rel
}

// TestGoldenDatasetPathExists documents where the seed set lives, so a
// missing directory is a clear failure rather than a confusing empty-metric run.
func TestGoldenDatasetPathExists(t *testing.T) {
	if _, err := golden.Load(filepath.Clean(datasetDir)); err != nil {
		t.Fatalf("golden dataset not loadable at %s: %v", datasetDir, err)
	}
}
