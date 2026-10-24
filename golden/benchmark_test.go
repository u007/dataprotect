package golden_test

import (
	"context"
	"testing"

	piiscrub "github.com/u007/dataprotect"
	"github.com/u007/dataprotect/golden"
)

// nemotronDir and gretelDir are the converted external corpora, relative to
// this package's directory, so the gate runs from any working directory.
//
// They are NOT pass/fail gates on the hand-labelled seed set. They are the
// first measurements over real-world PII corpora: exact-match precision and
// recall on 2000-case samples, recorded as ratios so any regression is visible
// immediately. The numbers below are the measured baselines from
// `scripts/benchmark.sh testdata/external/<corpus>/converted`; any drift from
// them fails the test.
//
// The corpora label only a subset of the detector's types (see each schema's
// in_schema list), so detections of unlabelled types are out of scope and are
// not counted as false positives. Detections that land on an unlabelled span
// (a span the corpus records but does not label) are set aside and reported
// separately — they are not accuracy.

const nemotronDir = "../testdata/external/nemotron-pii/converted"
const gretelDir = "../testdata/external/gretel-pii-finance/converted"

func benchmarkCorpus(t *testing.T, dir string,
	minExactPrecision, minExactRecall, minOverlapRecall float64,
	minNegativesClean, minNegatives int) {
	t.Helper()

	cases, err := golden.Load(dir)
	if err != nil {
		t.Fatalf("loading %s: %v", dir, err)
	}

	sch, err := golden.LoadSchema(dir)
	if err != nil {
		t.Fatalf("loading schema for %s: %v", dir, err)
	}

	rep, err := golden.Score(context.Background(), dir,
		func(ctx context.Context, text string) ([]golden.Prediction, error) {
			s := piiscrub.NewScrubber()
			return s.Analyze(ctx, text)
		}, golden.WithInSchema(sch.InSchema))
	if err != nil {
		t.Fatalf("scoring %s: %v", dir, err)
	}

	if len(rep.Errors) > 0 {
		t.Fatalf("%s produced scoring errors: %v", dir, rep.Errors)
	}
	if rep.Cases != len(cases) {
		t.Fatalf("%s: scored %d cases, loaded %d", dir, rep.Cases, len(cases))
	}

	if rep.Exact.Precision < minExactPrecision {
		t.Errorf("%s: exact precision %.4f below baseline %.4f\n%s",
			dir, rep.Exact.Precision, minExactPrecision, rep.Summary())
	}
	if rep.Exact.Recall < minExactRecall {
		t.Errorf("%s: exact recall %.4f below baseline %.4f\n%s",
			dir, rep.Exact.Recall, minExactRecall, rep.Summary())
	}
	if rep.Overlap.Recall < minOverlapRecall {
		t.Errorf("%s: overlap recall %.4f below baseline %.4f\n%s",
			dir, rep.Overlap.Recall, minOverlapRecall, rep.Summary())
	}
	if rep.Overlap.Recall < rep.Exact.Recall {
		t.Errorf("%s: overlap recall %.4f below exact recall %.4f — impossible",
			dir, rep.Overlap.Recall, rep.Exact.Recall)
	}
	if rep.Exact.NegativesClean < minNegativesClean {
		t.Errorf("%s: only %d of %d negative cases clean (baseline %d)",
			dir, rep.Exact.NegativesClean, rep.Exact.Negatives, minNegativesClean)
	}
	t.Logf("%s: %s", dir, rep.Summary())
}

func TestBenchmarkNemotronPII(t *testing.T) {
	// Measured baseline over the 2000-case nemotron-pii test split:
	//   exact: P=0.8357 R=0.2886 (tp=2655 fp=522 fn=6546)
	//   overlap: P=0.8455 R=0.2919 (tp=2686 fp=491 fn=6515)
	//   negatives: 52 clean of 71
	benchmarkCorpus(t, nemotronDir,
		2655.0/(2655+522),  // exact precision
		2655.0/(2655+6546), // exact recall
		2686.0/(2686+6515), // overlap recall
		52, 71)             // negatives clean / total
}

func TestBenchmarkGretelPIIFinance(t *testing.T) {
	// Measured baseline over the 2000-case gretel-pii-finance test split:
	//   exact: P=0.3816 R=0.0957 (tp=696 fp=1128 fn=6576)
	//   overlap: P=0.3986 R=0.1000 (tp=727 fp=1097 fn=6545)
	//   negatives: 231 clean of 280
	benchmarkCorpus(t, gretelDir,
		696.0/(696+1128), // exact precision
		696.0/(696+6576), // exact recall
		727.0/(727+6545), // overlap recall
		231, 280)         // negatives clean / total
}
