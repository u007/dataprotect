//go:build integration

// Package piiscrub_test holds the NER accuracy gate.
//
// The gate lives behind the `integration` build tag because the NER path needs
// a running sidecar, and the offline suite must not depend on one. Run it
// with:
//
//	go build -o /tmp/piiscrub-sidecar ./sidecar/cmd/piiscrub-sidecar
//	/tmp/piiscrub-sidecar &
//	PII_SCRUB_NLP_URL=http://127.0.0.1:8787 go test -tags integration ./...
//
// The gate measures TWO numbers, and the distinction between them is the
// point of this file.
//
// TYPED metrics (the golden package's own, type-sensitive):
//
//	The NER types are PERSON, ORGANIZATION and LOCATION, and the golden
//	scorer only credits a prediction whose TYPE matches the label's type.
//	So calling "Tobias Lindgren" an ORGANIZATION when the label says PERSON
//	is a miss AND a false positive. That is the right measure of the
//	detector's typing.
//
// SPAN metrics (type-agnostic, computed here):
//
//	All three NER types carry the SAME operator in the default scrubber:
//	redact. A span the detector found and covered exactly, but typed
//	differently, is redacted just the same — the PII does not escape. So
//	for the question "does the PII get removed", the typed metric
//	understates what the caller actually gets.
//
// Both are gated. The typed gate catches a typing regression; the span gate
// catches a coverage regression. Neither is allowed to hide behind the other.
package piiscrub_test

import (
	"context"
	"os"
	"testing"

	"github.com/u007/dataprotect/golden"
	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/nlp"
)

// nerDatasetDir holds the NER golden cases. It is relative to this package,
// which is the root module, so the path starts at the repository root.
const nerDatasetDir = "testdata/golden/ner"

// The measured baseline.
//
// The gate scores ALL FOUR files in testdata/golden/ner/ (124 cases: a
// 49-case development set, a 25-case holdout, an 18-case final set and a
// 32-case structured-document set).
//
//	typed:  P=0.8395 R=0.8718 (tp=68 fp=13 fn=10)
//	span:   P=0.9012 R=0.9359 (tp=73 fp=8 fn=5)
//
// That is the figure over the combined set, so it is optimistic: the
// development set is 49 of the 124 cases and the decoder's rules were written
// against it.
//
// The structured-document set is the one that found the real blind spot.
// The first three sets were all prose, and the corpora then showed the
// decoder reporting JSON keys, HTTP headers and spreadsheet field labels as
// people and places. It is the regression guard for the two rules that fix
// that class, and the gate was mutation-verified against it: disabling the
// field-label rule drops typed precision to 0.7907 and this gate fails.
//
// The GENERALISATION figure is the FIRST evaluation of the final set alone,
// on text the decoder had never seen:
//
//	typed:  P=0.5333 R=0.6154 (tp=8 fp=7 fn=5)   -> 16 labelled entities
//	span:   P=0.8667 R=1.0000 (tp=13 fp=2 fn=0)  -> 13 labelled entities
//
// That first evaluation is the honest number, and it is recorded here rather
// than the flattering re-run. The gap between it and the combined figure is
// the cost of writing the rules against the development set.
//
// One label correction was made after that first evaluation, and it is
// documented in scripts/gen_ner_dataset.py: a street address was relabelled
// from a negative to a LOCATION, because a street IS a place and the detector
// was right. The figure above predates the correction, so it is the worse
// number.
//
// The gate bounds below are set BELOW the measurements, so a regression
// fails and an improvement is a deliberate bump rather than a silent drift.
const (
	// Typed (type-sensitive) bounds.
	minNerTypedPrecision = 0.80
	minNerTypedRecall    = 0.85

	// Span (type-agnostic) bounds. The span gate is what protects the
	// caller's data: every NER type redacts, so covering the right bytes
	// is what stops PII reaching an LLM.
	minNerSpanPrecision = 0.84
	minNerSpanRecall    = 0.88
)

// nerRecognizer returns a client for the sidecar, or skips the test.
//
// A skip is the right outcome when the sidecar is not running: the NER path
// is opt-in by design, and an offline checkout must still be able to run the
// integration tag without a sidecar. The skip message says which URL was
// tried, so a CI job that expected the sidecar to be up does not look green
// for the wrong reason.
//
// The gate REQUIRES the sidecar when PII_SCRUB_NLP_REQUIRE=1. That is the
// switch a CI job sets: a green run then means the gate actually ran, not
// that it quietly skipped. Without it, a broken sidecar would produce a
// passing build that tested nothing.
func nerRecognizer(t *testing.T) *nlp.NLPRecognizer {
	t.Helper()
	url := os.Getenv("PII_SCRUB_NLP_URL")
	binary := os.Getenv("PII_SCRUB_NLP_BINARY")
	if binary != "" && url != "" {
		t.Fatal("PII_SCRUB_NLP_URL and PII_SCRUB_NLP_BINARY are both set; set only one (URL uses an external sidecar, BINARY starts a managed child)")
	}
	if binary != "" {
		m, err := nlp.ManagedFromEnv(context.Background())
		if err != nil {
			t.Skipf("managed sidecar: %v", err)
		}
		t.Cleanup(func() { _ = m.Close() })
		return m.NLPRecognizer
	}
	if url == "" {
		t.Skip("PII_SCRUB_NLP_URL is not set; the NER gate needs a running sidecar")
	}
	r := nlp.New(url)
	if _, err := r.Recognize(context.Background(), "probe"); err != nil {
		if os.Getenv("PII_SCRUB_NLP_REQUIRE") == "1" {
			t.Fatalf("sidecar at %s is not reachable (%v) and PII_SCRUB_NLP_REQUIRE=1, "+
				"so this gate must run rather than skip", url, err)
		}
		t.Skipf("sidecar at %s is not reachable (%v); the NER gate needs a running sidecar", url, err)
	}
	return r
}

// nerPredictions adapts the client to the golden scorer's predictor.
func nerPredictions(r *nlp.NLPRecognizer) func(context.Context, string) ([]golden.Prediction, error) {
	return func(ctx context.Context, text string) ([]golden.Prediction, error) {
		ents, err := r.Recognize(ctx, text)
		if err != nil {
			return nil, err
		}
		out := make([]golden.Prediction, 0, len(ents))
		for _, e := range ents {
			out = append(out, golden.Prediction(e))
		}
		return out, nil
	}
}

// TestNERTypedAccuracy gates the detector's TYPING against the NER golden set.
//
// The bounds are the measured development-set baseline, rounded down. A
// typing regression fails here even when the span gate still passes, which is
// the point: a span that is covered but mistyped still redacts, so a typing
// regression is a quality loss rather than a data loss, and it is gated at
// the level it deserves.
func TestNERTypedAccuracy(t *testing.T) {
	r := nerRecognizer(t)

	rep, err := golden.Score(context.Background(), nerDatasetDir, nerPredictions(r))
	if err != nil {
		t.Fatalf("scoring NER dataset: %v", err)
	}
	if len(rep.Errors) > 0 {
		t.Fatalf("label or predictor errors in the NER dataset: %v", rep.Errors)
	}
	t.Logf("typed: %s", rep.Summary())

	if rep.Exact.Precision < minNerTypedPrecision {
		t.Errorf("typed precision %.4f below the bound %.4f", rep.Exact.Precision, minNerTypedPrecision)
	}
	if rep.Exact.Recall < minNerTypedRecall {
		t.Errorf("typed recall %.4f below the bound %.4f", rep.Exact.Recall, minNerTypedRecall)
	}
}

// TestNERSpanAccuracy gates the detector's COVERAGE, ignoring the type.
//
// This is the gate that matters for the caller's data. Every NER type
// carries the redact operator, so a span covered exactly is removed whether
// the detector called it a person or an organisation. A regression here means
// PII is surviving into redacted text.
//
// The metric is computed here rather than through the golden package because
// the golden scorer matches on type. Type-agnostic matching would need a
// second scorer, and the arithmetic is short enough to state outright.
func TestNERSpanAccuracy(t *testing.T) {
	r := nerRecognizer(t)

	cases, err := golden.Load(nerDatasetDir)
	if err != nil {
		t.Fatalf("loading NER dataset: %v", err)
	}
	if len(cases) < 120 {
		t.Fatalf("NER dataset has %d cases, want at least 120; a small dataset makes every "+
			"metric coarse and lets a regression hide in the rounding", len(cases))
	}

	tp, fp, fn := 0, 0, 0
	negTotal, negClean := 0, 0
	for _, c := range cases {
		ents, err := r.Recognize(context.Background(), c.Text)
		if err != nil {
			t.Fatalf("case %q: %v", c.ID, err)
		}
		gold := goldSpans(t, c)
		pred := make([]span, 0, len(ents))
		for _, e := range ents {
			pred = append(pred, span{e.Start, e.End})
		}
		used := make([]bool, len(gold))
		for _, p := range pred {
			hit := -1
			for i, g := range gold {
				if used[i] {
					continue
				}
				if p == g {
					hit = i
					break
				}
			}
			if hit >= 0 {
				used[hit] = true
				tp++
			} else {
				fp++
			}
		}
		for _, ok := range used {
			if !ok {
				fn++
			}
		}
		if len(gold) == 0 {
			negTotal++
			if len(pred) == 0 {
				negClean++
			}
		}
	}

	precision := 0.0
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	recall := 0.0
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	t.Logf("span-level (type-agnostic): P=%.4f R=%.4f (tp=%d fp=%d fn=%d); negatives clean %d/%d",
		precision, recall, tp, fp, fn, negClean, negTotal)

	if precision < minNerSpanPrecision {
		t.Errorf("span precision %.4f below the bound %.4f; a span the detector covered "+
			"but the label did not is redacted anyway, so this is the metric that protects "+
			"the caller's data", precision, minNerSpanPrecision)
	}
	if recall < minNerSpanRecall {
		t.Errorf("span recall %.4f below the bound %.4f; a missed span is PII that survives "+
			"into redacted text", recall, minNerSpanRecall)
	}
}

// span is a byte range in a case's text.
type span struct{ start, end int }

// goldSpans resolves a case's labels to byte ranges, type-agnostic.
func goldSpans(t *testing.T, c golden.Case) []span {
	t.Helper()
	out := make([]span, 0, len(c.Entities))
	cursor := 0
	for i, l := range c.Entities {
		idx := -1
		for j := cursor; j+len(l.Value) <= len(c.Text); j++ {
			if c.Text[j:j+len(l.Value)] == l.Value {
				idx = j
				break
			}
		}
		if idx < 0 {
			t.Fatalf("case %q label %d: value %q is not in the text after byte %d",
				c.ID, i, l.Value, cursor)
		}
		out = append(out, span{idx, idx + len(l.Value)})
		cursor = idx + len(l.Value)
	}
	return out
}

// TestNEREntityTypesAreKnown checks that the sidecar only emits types the
// root module knows.
//
// The sidecar runs in its own process and can be replaced, so a type it
// invents would reach the root's EntityTypeFromString and be dropped there.
// Dropping it is safe but silent, and a type mismatch is a configuration
// error worth failing on.
func TestNEREntityTypesAreKnown(t *testing.T) {
	r := nerRecognizer(t)
	ents, err := r.Recognize(context.Background(),
		"Contact Ingrid Larsen at Northgate Ventures in Copenhagen about the invoice.")
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) == 0 {
		t.Fatal("no entities in a sentence that has three")
	}
	for _, e := range ents {
		switch e.Type {
		case recognizers.EntityPerson, recognizers.EntityOrganization, recognizers.EntityLocation:
		default:
			t.Errorf("sidecar emitted unknown entity type %q", e.Type)
		}
	}
}
