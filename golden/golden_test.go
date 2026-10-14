package golden

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

var errFakePredictor = errors.New("detector exploded")

// mustJSON marshals a case for writing to the temp dataset dir.
func mustJSON(t *testing.T, c Case) string {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshalling case %s: %v", c.ID, err)
	}
	return string(raw)
}

// mustResolve resolves a case and fails the test if the labels are invalid. It
// keeps the scoring tests readable by not repeating the error check.
func mustResolve(t *testing.T, c Case) []recognizers.Entity {
	t.Helper()
	gold, err := c.resolve()
	if err != nil {
		t.Fatalf("resolve(%s): unexpected error: %v", c.ID, err)
	}
	return gold
}

func entity(typ recognizers.EntityType, start, end int, text string) recognizers.Entity {
	return recognizers.Entity{
		Type:       typ,
		Start:      start,
		End:        end,
		Text:       text,
		Confidence: 1.0,
	}
}

func TestScoreIdenticalPredictionCountsAsTruePositive(t *testing.T) {
	c := Case{
		ID:       "tp",
		Text:     "reach me at ada@example.com",
		Entities: []Label{{Type: recognizers.EntityEmail, Value: "ada@example.com"}},
	}
	gold := mustResolve(t, c)

	// Build the prediction from the resolved label so the test states "an
	// identical span is a TP" without hard-coding an offset that would need
	// editing every time the text changes.
	identical := entity(gold[0].Type, gold[0].Start, gold[0].End, gold[0].Text)
	rep, err := Score(context.Background(), writeCases(t, []Case{c}),
		func(_ context.Context, _ string) ([]Prediction, error) {
			return []Prediction{identical}, nil
		})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	if rep.Exact.TP != 1 || rep.Exact.FP != 0 || rep.Exact.FN != 0 {
		t.Errorf("exact counts = tp %d fp %d fn %d, want 1/0/0\n%s",
			rep.Exact.TP, rep.Exact.FP, rep.Exact.FN, rep.Summary())
	}
	if rep.Exact.Precision != 1.0 || rep.Exact.Recall != 1.0 || rep.Exact.F1 != 1.0 {
		t.Errorf("exact P/R/F1 = %v/%v/%v, want 1/1/1", rep.Exact.Precision, rep.Exact.Recall, rep.Exact.F1)
	}
	// A case with entities is not a negative case.
	if rep.Exact.Negatives != 0 {
		t.Errorf("Negatives = %d, want 0 for a positive case", rep.Exact.Negatives)
	}
}

func TestScoreMissingEntityCountsAsFalseNegative(t *testing.T) {
	c := Case{
		ID:       "fn",
		Text:     "call 555-123-4567 or email ada@example.com",
		Entities: []Label{{Type: recognizers.EntityPhone, Value: "555-123-4567"}},
	}

	// The detector finds the email but not the phone: one FN, one FP.
	emailStart := strings.Index(c.Text, "ada@example.com")
	pred := []Prediction{entity(recognizers.EntityEmail, emailStart, emailStart+len("ada@example.com"), "ada@example.com")}

	rep, err := Score(context.Background(), writeCases(t, []Case{c}),
		func(_ context.Context, _ string) ([]Prediction, error) { return pred, nil })
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	if rep.Exact.TP != 0 || rep.Exact.FP != 1 || rep.Exact.FN != 1 {
		t.Errorf("exact counts = tp %d fp %d fn %d, want 0/1/1\n%s",
			rep.Exact.TP, rep.Exact.FP, rep.Exact.FN, rep.Summary())
	}
	if rep.Exact.Recall != 0.0 {
		t.Errorf("exact Recall = %v, want 0 when the only gold entity is missed", rep.Exact.Recall)
	}
}

// TestResolveRejectsRepeatedValue covers the ambiguity guard: a value occurring
// more than once cannot be located, so the label is a dataset bug, not a
// detector failure.
func TestResolveRejectsRepeatedValue(t *testing.T) {
	c := Case{
		ID:       "dup",
		Text:     "555-123-4567 and again 555-123-4567",
		Entities: []Label{{Type: recognizers.EntityPhone, Value: "555-123-4567"}},
	}
	_, err := c.resolve()
	if err == nil {
		t.Fatal("resolve: want error for a value occurring twice, got nil")
	}
	// The message must say what is wrong AND how to fix it, or a dataset author
	// cannot act on it.
	for _, want := range []string{"dup", "occurs 2 times", "exactly 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("resolve error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestResolveRejectsAbsentValue(t *testing.T) {
	c := Case{
		ID:       "absent",
		Text:     "no entities here at all",
		Entities: []Label{{Type: recognizers.EntityEmail, Value: "ada@example.com"}},
	}
	_, err := c.resolve()
	if err == nil {
		t.Fatal("resolve: want error for a value absent from the text, got nil")
	}
	if !strings.Contains(err.Error(), "occurs 0 times") {
		t.Errorf("resolve error %q should report 0 occurrences", err.Error())
	}
}

func TestResolveRejectsOutOfOrderLabels(t *testing.T) {
	c := Case{
		ID:   "order",
		Text: "ada@example.com then bob@example.org",
		Entities: []Label{
			{Type: recognizers.EntityEmail, Value: "bob@example.org"},
			{Type: recognizers.EntityEmail, Value: "ada@example.com"},
		},
	}
	_, err := c.resolve()
	if err == nil {
		t.Fatal("resolve: want error for labels listed out of text order, got nil")
	}
	if !strings.Contains(err.Error(), "not listed in text order") {
		t.Errorf("resolve error %q should explain the ordering rule", err.Error())
	}
}

func TestResolveRejectsEmptyValue(t *testing.T) {
	c := Case{
		ID:       "empty",
		Text:     "some text",
		Entities: []Label{{Type: recognizers.EntityEmail, Value: ""}},
	}
	if _, err := c.resolve(); err == nil || !strings.Contains(err.Error(), "empty value") {
		t.Fatalf("resolve: want 'empty value' error, got %v", err)
	}
}

// TestHallucinatedTypeReportsZeroPrecision pins the guard that a type nobody
// labelled but the detector invented still appears in ByType with P=0.0. If
// allTypes took keys from gold only, the invented type would be absent from the
// map and its false positive invisible — the metric would look perfect in
// exactly the direction that matters.
func TestHallucinatedTypeReportsZeroPrecision(t *testing.T) {
	c := Case{
		ID:       "hallucinate",
		Text:     "order 12345 shipped on 2026-01-02",
		Entities: []Label{}, // nothing labelled: a negative case
	}
	pred := []Prediction{entity(recognizers.EntityEmail, 6, 16, "12345 ship")}

	rep, err := Score(context.Background(), writeCases(t, []Case{c}),
		func(_ context.Context, _ string) ([]Prediction, error) { return pred, nil })
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	counts, ok := rep.ByType["EMAIL"]
	if !ok {
		t.Fatalf("ByType is missing EMAIL, so an unlabelled false positive is hidden; got keys %v",
			rep.sortedTypeKeys())
	}
	if counts.FP != 1 || counts.TP != 0 {
		t.Errorf("EMAIL counts = tp %d fp %d, want 0/1", counts.TP, counts.FP)
	}
	if p := counts.P(); p != 0.0 {
		t.Errorf("EMAIL precision = %v, want 0.0 for a false positive nobody labelled", p)
	}
	if counts.R() != 1.0 {
		t.Errorf("EMAIL recall = %v, want 1.0 (no gold of that type, so nothing was missed)", counts.R())
	}
	// It is also a negative case that failed to stay clean.
	if rep.Exact.Negatives != 1 || rep.Exact.NegativesClean != 0 {
		t.Errorf("negatives = %d clean %d, want 1/0", rep.Exact.Negatives, rep.Exact.NegativesClean)
	}
	if rep.Exact.Precision != 0.0 {
		t.Errorf("aggregate precision = %v, want 0.0 when the only prediction is wrong", rep.Exact.Precision)
	}
}

// TestZeroDenominatorCountsAsOne pins ratio()'s convention: no predictions means
// perfect precision, not zero. Zero precision would make a detector look worse
// the more thoroughly it avoids false positives.
func TestZeroDenominatorCountsAsOne(t *testing.T) {
	if p := (Counts{TP: 0, FP: 0}).P(); p != 1.0 {
		t.Errorf("Counts{0,0}.P() = %v, want 1.0: no predictions is perfect precision", p)
	}
	if r := (Counts{TP: 0, FN: 0}).R(); r != 1.0 {
		t.Errorf("Counts{0,0}.R() = %v, want 1.0", r)
	}
	if f := (Counts{TP: 0, FP: 0, FN: 0}).F1(); f != 1.0 {
		t.Errorf("Counts{0,0,0}.F1() = %v, want 1.0", f)
	}
	// A negative case the detector leaves alone must be clean at 1.0.
	c := Case{ID: "clean-neg", Text: "the weather is fine today", Entities: []Label{}}
	rep, err := Score(context.Background(), writeCases(t, []Case{c}),
		func(_ context.Context, _ string) ([]Prediction, error) { return nil, nil })
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if rep.Exact.Precision != 1.0 || rep.Exact.Recall != 1.0 {
		t.Errorf("clean negative P/R = %v/%v, want 1/1", rep.Exact.Precision, rep.Exact.Recall)
	}
	if rep.Exact.NegativesClean != 1 {
		t.Errorf("NegativesClean = %d, want 1", rep.Exact.NegativesClean)
	}
}

// TestF1OfTwoZeroesIsZero guards the one place 1 is NOT the right answer: with
// nothing predicted and nothing labelled, the harmonic mean has no terms, and
// claiming 1 would report an empty case as a perfect score.
func TestF1OfTwoZeroesIsZero(t *testing.T) {
	if f := (Counts{TP: 0, FP: 3, FN: 3}).F1(); f != 0.0 {
		t.Errorf("F1 with tp=0 = %v, want 0.0", f)
	}
}

// TestOverlappingPredictionsCreditOneGold guards the `used` array: two
// predictions covering the same entity must not both count as finding it.
func TestOverlappingPredictionsCreditOneGold(t *testing.T) {
	c := Case{
		ID:       "dup-pred",
		Text:     "mail ada@example.com today",
		Entities: []Label{{Type: recognizers.EntityEmail, Value: "ada@example.com"}},
	}
	gold := mustResolve(t, c)
	start := gold[0].Start
	// One exact hit plus a wider prediction over the same bytes.
	pred := []Prediction{
		entity(recognizers.EntityEmail, start, start+len("ada@example.com"), "ada@example.com"),
		entity(recognizers.EntityEmail, start-5, start+len("ada@example.com"), "mail ada@example.com"),
	}

	rep, err := Score(context.Background(), writeCases(t, []Case{c}),
		func(_ context.Context, _ string) ([]Prediction, error) { return pred, nil })
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if rep.Exact.TP != 1 || rep.Exact.FP != 1 {
		t.Errorf("exact tp %d fp %d, want 1/1: only one prediction may claim the gold span\n%s",
			rep.Exact.TP, rep.Exact.FP, rep.Summary())
	}
	if rep.Exact.FN != 0 {
		t.Errorf("exact FN = %d, want 0", rep.Exact.FN)
	}
	// Under overlap matching the wider prediction overlaps the gold but the gold
	// was already consumed by the exact hit, so it is also one FP.
	if rep.Overlap.TP != 1 || rep.Overlap.FP != 1 {
		t.Errorf("overlap tp %d fp %d, want 1/1", rep.Overlap.TP, rep.Overlap.FP)
	}
}

// TestExactAndOverlapDivergeOnMisSizedSpan is the reason both granularities are
// reported: a detector that finds the entity but mis-sizes it loses exact recall
// while keeping overlap recall, which localises the defect.
func TestExactAndOverlapDivergeOnMisSizedSpan(t *testing.T) {
	c := Case{
		ID:       "mis-sized",
		Text:     "phone 555-123-4567 ok",
		Entities: []Label{{Type: recognizers.EntityPhone, Value: "555-123-4567"}},
	}
	gold := mustResolve(t, c)
	// Include the space before the number, so it overlaps but is not identical.
	start := gold[0].Start - 1
	pred := []Prediction{entity(recognizers.EntityPhone, start, gold[0].End, c.Text[start:gold[0].End])}

	rep, err := Score(context.Background(), writeCases(t, []Case{c}),
		func(_ context.Context, _ string) ([]Prediction, error) { return pred, nil })
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if rep.Exact.TP != 0 {
		t.Errorf("exact TP = %d, want 0 for a mis-sized span", rep.Exact.TP)
	}
	if rep.Overlap.TP != 1 {
		t.Errorf("overlap TP = %d, want 1: the spans share bytes", rep.Overlap.TP)
	}
	if rep.Exact.Recall >= rep.Overlap.Recall {
		t.Errorf("exact recall %v should be lower than overlap recall %v",
			rep.Exact.Recall, rep.Overlap.Recall)
	}
}

// TestPredictorErrorIsRecordedNotScored keeps "could not answer" distinct from
// "answered wrong": a detector error must not be laundered into an FN.
func TestPredictorErrorIsRecordedNotScored(t *testing.T) {
	c := Case{
		ID:       "pred-error",
		Text:     "mail ada@example.com",
		Entities: []Label{{Type: recognizers.EntityEmail, Value: "ada@example.com"}},
	}
	rep, err := Score(context.Background(), writeCases(t, []Case{c}),
		func(_ context.Context, _ string) ([]Prediction, error) {
			return nil, errFakePredictor
		})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if len(rep.Errors) != 1 {
		t.Fatalf("Errors = %v, want exactly one", rep.Errors)
	}
	if !strings.Contains(rep.Errors[0], "pred-error") || !strings.Contains(rep.Errors[0], "predictor error") {
		t.Errorf("error %q should name the case and the failure kind", rep.Errors[0])
	}
	if rep.Exact.TP+rep.Exact.FP+rep.Exact.FN != 0 {
		t.Errorf("counts = %d/%d/%d, want all zero: a skipped case must not be scored",
			rep.Exact.TP, rep.Exact.FP, rep.Exact.FN)
	}
}

// TestBrokenLabelIsRecordedAsError proves a bad label surfaces as an error
// rather than silently inflating recall.
func TestBrokenLabelIsRecordedAsError(t *testing.T) {
	c := Case{
		ID:       "bad-label",
		Text:     "555-123-4567 twice 555-123-4567",
		Entities: []Label{{Type: recognizers.EntityPhone, Value: "555-123-4567"}},
	}
	rep, err := Score(context.Background(), writeCases(t, []Case{c}),
		func(_ context.Context, _ string) ([]Prediction, error) { return nil, nil })
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if len(rep.Errors) != 1 {
		t.Fatalf("Errors = %v, want one for the ambiguous label", rep.Errors)
	}
	if rep.Exact.TP+rep.Exact.FP+rep.Exact.FN != 0 {
		t.Errorf("a case with an unusable label must not contribute counts, got %d/%d/%d",
			rep.Exact.TP, rep.Exact.FP, rep.Exact.FN)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	// "negative" was a real field that nothing read. A hand-written dataset using
	// it must now fail loudly instead of being silently ignored.
	dir := t.TempDir()
	raw := `[{"id":"x","text":"hello","entities":[],"negative":true}]`
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load: want error for unknown field 'negative', got nil")
	}
	if !strings.Contains(err.Error(), "negative") {
		t.Errorf("Load error %q should name the unknown field", err.Error())
	}
}

func TestLoadRejectsEmptyDataset(t *testing.T) {
	// An empty corpus must not score 1.0 across the board via the
	// zero-denominator convention.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Error("Load: want error for a file parsing to zero cases, got nil")
	}

	empty := t.TempDir()
	if _, err := Load(empty); err == nil {
		t.Error("Load: want error for a directory with no *.json files, got nil")
	}
}

func TestLoadRejectsInvalidLabels(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"no id", `[{"text":"hello","entities":[]}]`, "no id"},
		{"duplicate id", `[{"id":"a","text":"x","entities":[]},{"id":"a","text":"y","entities":[]}]`, "duplicate case id"},
		{"no text", `[{"id":"a","entities":[]}]`, "no text"},
		{"bad type", `[{"id":"a","text":"x","entities":[{"type":"NOPE","value":"x"}]}]`, "NOPE"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte(tc.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(dir)
			if err == nil {
				t.Fatalf("Load: want error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Load error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestLoadOrdersFilesDeterministically(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"c.json", "a.json", "b.json"} {
		raw := `[{"id":"` + strings.TrimSuffix(n, ".json") + `","text":"x","entities":[]}]`
		if err := os.WriteFile(filepath.Join(dir, n), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cases, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var ids []string
	for _, c := range cases {
		ids = append(ids, c.ID)
	}
	if got := strings.Join(ids, ","); got != "a,b,c" {
		t.Errorf("Load order = %q, want %q", got, "a,b,c")
	}
}

func TestSummaryReportsEverySection(t *testing.T) {
	c := Case{
		ID:       "sum",
		Text:     "mail ada@example.com",
		Entities: []Label{{Type: recognizers.EntityEmail, Value: "ada@example.com"}},
	}
	gold := mustResolve(t, c)
	rep, err := Score(context.Background(), writeCases(t, []Case{c}),
		func(_ context.Context, _ string) ([]Prediction, error) {
			return []Prediction{entity(gold[0].Type, gold[0].Start, gold[0].End, gold[0].Text)}, nil
		})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	s := rep.Summary()
	for _, want := range []string{"exact-span match", "overlap match", "negative cases", "by type", "EMAIL"} {
		if !strings.Contains(s, want) {
			t.Errorf("Summary missing %q:\n%s", want, s)
		}
	}
}

// writeCases writes cases to a temp dataset directory. The runner loads from
// disk, so tests go through the same path as a real dataset run.
func writeCases(t *testing.T, cases []Case) string {
	t.Helper()
	dir := t.TempDir()
	var b strings.Builder
	b.WriteByte('[')
	for i, c := range cases {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(mustJSON(t, c))
	}
	b.WriteByte(']')
	if err := os.WriteFile(filepath.Join(dir, "cases.json"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}
