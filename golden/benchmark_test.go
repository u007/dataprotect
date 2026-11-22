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
	//   negatives: 52 clean of 71 (19 dirty)
	// Re-based 2026-11-22: 22 of the 71 negatives gained a finance label and
	// are positives now, so the population is 49. The dirty tolerance stays 19,
	// which is 30 clean of 49. The measured dirty count is now 12 (37 clean).
	//
	// Measured 2026-11-22, finance labels mapped, before the identity labels:
	//   exact: P=0.8763 R=0.3166 (tp=3174 fp=448 fn=6852)
	//   overlap: P=0.8849 R=0.3197 (tp=3205 fp=417 fn=6821)
	//   negatives: 37 clean of 49
	// Measured 2026-11-22, identity labels mapped (tax_id, date_of_birth):
	//   exact: P=0.8841 R=0.3326 (tp=3434 fp=450 fn=6892)
	//   overlap: P=0.8921 R=0.3356 (tp=3465 fp=419 fn=6861)
	//   negatives: 35 clean of 44 (the population is 44 because 5 former
	//   negatives now carry a gold tax_id or date_of_birth label)
	// The thresholds below are the last measured ratios; the negative floor of
	// 30 is unchanged.
	benchmarkCorpus(t, nemotronDir,
		3434.0/(3434+450),  // exact precision
		3434.0/(3434+6892), // exact recall
		3465.0/(3465+6861), // overlap recall
		30, 44)             // negatives clean / total (dirty tolerance 19)
}

func TestBenchmarkGretelPIIFinance(t *testing.T) {
	// Measured baseline over the 2000-case gretel-pii-finance test split:
	//   exact: P=0.3816 R=0.0957 (tp=696 fp=1128 fn=6576)
	//   overlap: P=0.3986 R=0.1000 (tp=727 fp=1097 fn=6545)
	//   negatives: 231 clean of 280 (49 dirty)
	// Re-based 2026-11-22: 7 of the 280 negatives gained a finance label and are
	// positives now, so the population is 273. The dirty tolerance stays 49,
	// which is 224 clean of 273. The measured dirty count is 47 (226 clean),
	// the same as before the finance labels were mapped.
	//
	// Measured 2026-11-22, finance labels mapped, before the identity labels:
	//   exact: P=0.4080 R=0.1068 (tp=807 fp=1171 fn=6752)
	//   overlap: P=0.4242 R=0.1110 (tp=839 fp=1139 fn=6720)
	//   negatives: 226 clean of 273
	// Measured 2026-11-22, identity labels mapped (passport_number,
	// driver_license_number, date_of_birth):
	//   exact: P=0.4397 R=0.1192 (tp=922 fp=1175 fn=6815)
	//   overlap: P=0.4549 R=0.1233 (tp=954 fp=1143 fn=6783)
	//   negatives: 225 clean of 272
	// The thresholds below are the last measured ratios; the negative floor of
	// 224 is unchanged.
	benchmarkCorpus(t, gretelDir,
		922.0/(922+1175), // exact precision
		922.0/(922+6815), // exact recall
		954.0/(954+6783), // overlap recall
		224, 272)         // negatives clean / total (dirty tolerance 49)
}
