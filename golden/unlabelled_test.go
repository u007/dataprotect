package golden

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// writeCorpus writes one case file, and a schema sidecar when recorded is not
// nil, into a fresh directory and returns the directory.
func writeCorpus(t *testing.T, cases []Case, recorded *bool) string {
	t.Helper()
	dir := t.TempDir()
	raw, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cases-000000.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if recorded != nil {
		sch := Schema{Corpus: "synthetic", InSchema: []recognizers.EntityType{recognizers.EntityEmail, recognizers.EntityPhone}, UnlabelledRecorded: *recorded}
		schRaw, err := json.Marshal(sch)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, SchemaFilename), schRaw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func boolPtr(b bool) *bool { return &b }

func intPtr(i int) *int { return &i }

// text: "mail a@b.co or call 555-867-5309 today"
const unlText = "mail a@b.co or call 555-867-5309 today"

func unlCase() Case {
	return Case{
		ID:       "c1",
		Text:     unlText,
		Entities: []Label{{Type: recognizers.EntityEmail, Value: "a@b.co"}},
		Unlabelled: []UnlabelledSpan{
			{Label: "account_number", Value: "555-867-5309", Start: intPtr(20), End: intPtr(32)},
		},
	}
}

// The unlabelled span is not a false positive, but the headline keeps it as one.
func TestUnlabelledDetectionIsBucketedNotCountedAsFP(t *testing.T) {
	dir := writeCorpus(t, []Case{unlCase()}, boolPtr(true))
	predict := func(ctx context.Context, text string) ([]Prediction, error) {
		return []Prediction{
			{Type: recognizers.EntityEmail, Start: 5, End: 11, Text: "a@b.co"},
			{Type: recognizers.EntityPhone, Start: 20, End: 32, Text: "555-867-5309"},
		}, nil
	}
	rep, err := Score(context.Background(), dir, predict)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if rep.Exact.FP != 1 {
		t.Errorf("headline exact FP = %d, want 1 (headline must not move)", rep.Exact.FP)
	}
	if rep.Unlabelled == nil {
		t.Fatal("Unlabelled report missing for a corpus dataset")
	}
	if rep.Unlabelled.ExactRemoved != 1 {
		t.Errorf("ExactRemoved = %d, want 1", rep.Unlabelled.ExactRemoved)
	}
	if got := rep.Unlabelled.ByLabel["PHONE on account_number"]; got != 1 {
		t.Errorf("ByLabel[PHONE on account_number] = %d, want 1 (got %v)", got, rep.Unlabelled.ByLabel)
	}
	if rep.Unlabelled.ExactUpperBoundP != 1 {
		t.Errorf("ExactUpperBoundP = %v, want 1", rep.Unlabelled.ExactUpperBoundP)
	}
	if rep.Exact.Precision == rep.Unlabelled.ExactUpperBoundP {
		t.Error("upper bound should differ from the headline when detections were bucketed")
	}
}

// A detection overlapping mapped gold of another type stays a plain FP: the
// wrong type is an error, not a scope question.
func TestOverlapWithMappedGoldStaysFP(t *testing.T) {
	c := Case{
		ID:       "c2",
		Text:     "call 555-867-5309 now",
		Entities: []Label{{Type: recognizers.EntitySSN, Value: "555-867-5309"}},
		Unlabelled: []UnlabelledSpan{
			{Label: "account_number", Value: "555-867-5309", Start: intPtr(5), End: intPtr(17)},
		},
	}
	dir := writeCorpus(t, []Case{c}, boolPtr(true))
	predict := func(ctx context.Context, text string) ([]Prediction, error) {
		return []Prediction{{Type: recognizers.EntityPhone, Start: 5, End: 17, Text: "555-867-5309"}}, nil
	}
	rep, err := Score(context.Background(), dir, predict)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if rep.Unlabelled.ExactRemoved != 0 || rep.Unlabelled.OverlapRemoved != 0 {
		t.Errorf("removed exact=%d overlap=%d, want 0 and 0: overlap with mapped gold must stay FP",
			rep.Unlabelled.ExactRemoved, rep.Unlabelled.OverlapRemoved)
	}
	if rep.Exact.FP != 1 {
		t.Errorf("exact FP = %d, want 1", rep.Exact.FP)
	}
}

// A boundary miss under exact matching overlaps mapped gold of the same type,
// so it stays a plain FP even though the overlap policy would count it as a TP.
func TestExactBoundaryMissAgainstMappedGoldStaysFP(t *testing.T) {
	c := Case{
		ID:       "c3",
		Text:     "call 555-867-5309 now",
		Entities: []Label{{Type: recognizers.EntityPhone, Value: "555-867-5309"}},
		Unlabelled: []UnlabelledSpan{
			{Label: "account_number", Value: "867-5309", Start: intPtr(9), End: intPtr(17)},
		},
	}
	dir := writeCorpus(t, []Case{c}, boolPtr(true))
	predict := func(ctx context.Context, text string) ([]Prediction, error) {
		return []Prediction{{Type: recognizers.EntityPhone, Start: 5, End: 16, Text: "555-867-530"}}, nil
	}
	rep, err := Score(context.Background(), dir, predict)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if rep.Unlabelled.ExactRemoved != 0 {
		t.Errorf("ExactRemoved = %d, want 0: a boundary miss against mapped gold is not unlabelled", rep.Unlabelled.ExactRemoved)
	}
}

// A corpus schema without the flag was written by an older converter and does
// not say whether its unlabelled spans were recorded. Scoring it would report a
// bucket of zero that means nothing, so the score fails and names the fix.
func TestStaleSchemaWithoutUnlabelledFlagFailsLoudly(t *testing.T) {
	dir := writeCorpus(t, []Case{unlCase()}, boolPtr(false))
	predict := func(ctx context.Context, text string) ([]Prediction, error) { return nil, nil }
	_, err := Score(context.Background(), dir, predict)
	if err == nil {
		t.Fatal("Score on a schema without unlabelled_recorded: want error, got nil")
	}
	if !strings.Contains(err.Error(), "make corpus-convert") {
		t.Errorf("error %q should tell the user to re-run make corpus-convert", err)
	}
}

// Unlabelled offsets get the same checks as label offsets. A mismatch is a
// broken label, reported in Errors and never silently scored.
func TestUnlabelledSpanOffsetMismatchIsAnError(t *testing.T) {
	c := unlCase()
	c.Unlabelled[0].Value = "wrong"
	dir := writeCorpus(t, []Case{c}, boolPtr(true))
	predict := func(ctx context.Context, text string) ([]Prediction, error) { return nil, nil }
	rep, err := Score(context.Background(), dir, predict)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if len(rep.Errors) != 1 || !strings.Contains(rep.Errors[0], "does not match") {
		t.Errorf("Errors = %v, want one offset/value mismatch error", rep.Errors)
	}
}

// The upper bound and the removed count must agree with the raw arithmetic:
// precision with the bucket removed is TP / (TP + FP - removed).
func TestUpperBoundArithmetic(t *testing.T) {
	dir := writeCorpus(t, []Case{unlCase()}, boolPtr(true))
	predict := func(ctx context.Context, text string) ([]Prediction, error) {
		return []Prediction{
			{Type: recognizers.EntityEmail, Start: 5, End: 11, Text: "a@b.co"},
			{Type: recognizers.EntityPhone, Start: 20, End: 32, Text: "555-867-5309"},
			{Type: recognizers.EntityPhone, Start: 0, End: 4, Text: "mail"},
		}, nil
	}
	rep, err := Score(context.Background(), dir, predict)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	// TP=1 (email). FP=2 (phone on account_number, phone on "mail"). Removed=1.
	want := 1.0 / float64(1+2-1)
	if rep.Unlabelled.ExactUpperBoundP != want {
		t.Errorf("ExactUpperBoundP = %v, want %v", rep.Unlabelled.ExactUpperBoundP, want)
	}
	if rep.Unlabelled.ExactRemoved != 1 {
		t.Errorf("ExactRemoved = %d, want 1", rep.Unlabelled.ExactRemoved)
	}
}

// A hand-labelled seed has no schema and no unlabelled spans, so it gets no
// unlabelled report and its summary is unchanged.
func TestHandLabelledDatasetHasNoUnlabelledReport(t *testing.T) {
	dir := writeCorpus(t, []Case{{ID: "h", Text: "a@b.co", Entities: []Label{{Type: recognizers.EntityEmail, Value: "a@b.co"}}}}, nil)
	predict := func(ctx context.Context, text string) ([]Prediction, error) {
		return []Prediction{{Type: recognizers.EntityEmail, Start: 0, End: 6, Text: "a@b.co"}}, nil
	}
	rep, err := Score(context.Background(), dir, predict)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if rep.Unlabelled != nil {
		t.Errorf("Unlabelled = %+v, want nil for a hand-labelled dataset", rep.Unlabelled)
	}
	if strings.Contains(rep.Summary(), "unlabelled") {
		t.Errorf("summary mentions unlabelled for a hand-labelled dataset:\n%s", rep.Summary())
	}
}
