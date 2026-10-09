package golden

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dataprotect/recognizers"
)

// writeSchemaFixture writes one case file plus a schema sidecar into a fresh
// temp dir and returns it. The sidecar must live beside the cases, because that
// is exactly the layout that made Load fail before Task 1 added the skip.
func writeSchemaFixture(t *testing.T, caseJSON, schemaJSON string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cases-000.json"), []byte(caseJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, SchemaFilename), []byte(schemaJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// trapDataset is the corpus-shaped fixture from TODO.md's FP-inflation trap: the
// dataset labels PHONE only, and the detector also reports an EMAIL. A naive
// scorer counts that EMAIL as a false positive because scorePair credits a
// prediction only against a gold label of the SAME type.
//
// trapText with its byte offsets, all measured not assumed:
//
//	Call 555-123-4567 or a@b.com, or 555-999-0000.
//	          ^gold   ^email        ^second phone
//	5..17          21..28            33..45
const (
	trapText         = "Call 555-123-4567 or a@b.com, or 555-999-0000."
	trapGoldJSON     = `[{"id":"t1","text":"Call 555-123-4567 or a@b.com, or 555-999-0000.","entities":[{"type":"PHONE","value":"555-123-4567"}]}]`
	trapSchemaJSON   = `{"corpus":"gretel-pii-finance","in_schema":["PHONE"],"offset_unit":"bytes","unlabelled_recorded":true}`
	trapGoldStart    = 5
	trapGoldEnd      = 17
	trapEmailStart   = 21
	trapEmailEnd     = 28
	trapSecondStart  = 33
	trapSecondEnd    = 45
	trapDefaultFPCnt = 2
)

func trapPredictions(_ context.Context, _ string) ([]Prediction, error) {
	return []Prediction{
		{Type: recognizers.EntityPhone, Start: trapGoldStart, End: trapGoldEnd,
			Text: "555-123-4567", Confidence: 1},
		// Out-of-schema: this dataset has no EMAIL label at all.
		{Type: recognizers.EntityEmail, Start: trapEmailStart, End: trapEmailEnd,
			Text: "a@b.com", Confidence: 1},
		// In-schema with no gold. A filter that excluded every unmatched
		// prediction would score FP == 0 here and wrongly pass, so this case
		// is what keeps the filter honest.
		{Type: recognizers.EntityPhone, Start: trapSecondStart, End: trapSecondEnd,
			Text: "555-999-0000", Confidence: 1},
	}, nil
}

// TestOutOfSchemaDetectionIsNotCountedAsFP is the failure TODO.md records: a
// corpus labelling only PHONE would otherwise score a correct EMAIL detection as
// a false positive, inflating FP and depressing precision for no defect of ours.
func TestOutOfSchemaDetectionIsNotCountedAsFP(t *testing.T) {
	dir := writeSchemaFixture(t, trapGoldJSON, trapSchemaJSON)

	rep, err := Score(context.Background(), dir, trapPredictions,
		WithInSchema([]recognizers.EntityType{recognizers.EntityPhone}))
	if err != nil {
		t.Fatalf("Score: unexpected error: %v", err)
	}
	if rep.Exact.TP != 1 {
		t.Errorf("Exact.TP = %d, want 1 (the gold PHONE)", rep.Exact.TP)
	}
	// The unmatched IN-SCHEMA PHONE must survive as a real false positive.
	// Excluding it too would let a filter that drops every unmatched
	// prediction report FP == 0 and look correct.
	if rep.Exact.FP != 1 {
		t.Errorf("Exact.FP = %d, want 1 (only the unmatched in-schema PHONE); report:\n%s",
			rep.Exact.FP, rep.Summary())
	}
	if rep.OutOfSchema == nil || rep.OutOfSchema[string(recognizers.EntityEmail)] != 1 {
		t.Errorf("OutOfSchema[EMAIL] = %v, want 1", rep.OutOfSchema)
	}
	if _, ok := rep.OutOfSchema[string(recognizers.EntityPhone)]; ok {
		t.Errorf("OutOfSchema must not mention in-schema type PHONE: %v", rep.OutOfSchema)
	}
	// Overlap matching applies the same filter; leaving it unfiltered would
	// make overlap recall disagree with exact for a reason that is not a
	// boundary bug.
	if rep.Overlap.FP != 1 {
		t.Errorf("Overlap.FP = %d, want 1", rep.Overlap.FP)
	}
	if rep.ByType[string(recognizers.EntityEmail)] != (Counts{}) {
		t.Errorf("ByType[EMAIL] = %+v, want zero Counts — out-of-schema types are not scored",
			rep.ByType[string(recognizers.EntityEmail)])
	}
}

// TestScoreWithoutSchemaCountsEveryPrediction is the control: with no option the
// filter is off, so behaviour is byte-identical to what the seed-set gate has
// always had, and both false positives are counted. It also proves the trap is
// real rather than theoretical — the same fixture scores 2 FP unfiltered.
func TestScoreWithoutSchemaCountsEveryPrediction(t *testing.T) {
	dir := writeSchemaFixture(t, trapGoldJSON, trapSchemaJSON)

	rep, err := Score(context.Background(), dir, trapPredictions)
	if err != nil {
		t.Fatalf("Score: unexpected error: %v", err)
	}
	if rep.Exact.FP != trapDefaultFPCnt {
		t.Errorf("Exact.FP = %d, want %d (EMAIL + unmatched PHONE) — the trap this filter exists to close",
			rep.Exact.FP, trapDefaultFPCnt)
	}
	if len(rep.OutOfSchema) != 0 {
		t.Errorf("OutOfSchema = %v, want empty when no schema option is given", rep.OutOfSchema)
	}
	if rep.Exact.TP != 1 {
		t.Errorf("Exact.TP = %d, want 1", rep.Exact.TP)
	}
}

// TestScoreRejectsGoldOutsideSchema closes the other half of the same trap. If
// gold labels a type the schema excludes, every prediction of that type is
// filtered out first, so the gold can never match: scoring would silently count
// FNs for entities the detector was never allowed to report. Fail loudly instead.
func TestScoreRejectsGoldOutsideSchema(t *testing.T) {
	gold := `[{"id":"g1","text":"write to a@b.com today","entities":[{"type":"EMAIL","value":"a@b.com"}]}]`
	schema := `{"corpus":"gretel-pii-finance","in_schema":["PHONE"],"offset_unit":"bytes","unlabelled_recorded":true}`
	dir := writeSchemaFixture(t, gold, schema)

	_, err := Score(context.Background(), dir, func(context.Context, string) ([]Prediction, error) {
		return nil, nil
	}, WithInSchema([]recognizers.EntityType{recognizers.EntityPhone}))
	if err == nil {
		t.Fatal("Score: want an error when gold type is outside in_schema, got nil")
	}
	if !strings.Contains(err.Error(), string(recognizers.EntityEmail)) {
		t.Errorf("Score error %q should name the offending gold type", err.Error())
	}
}

// TestSummaryReportsOutOfSchema pins the human-facing output: an out-of-schema
// count that never reaches the printed report would be invisible to anyone
// reading a benchmark failure.
func TestSummaryReportsOutOfSchema(t *testing.T) {
	dir := writeSchemaFixture(t, trapGoldJSON, trapSchemaJSON)

	rep, err := Score(context.Background(), dir, trapPredictions,
		WithInSchema([]recognizers.EntityType{recognizers.EntityPhone}))
	if err != nil {
		t.Fatalf("Score: unexpected error: %v", err)
	}
	summary := rep.Summary()
	if !strings.Contains(summary, "out-of-schema") {
		t.Errorf("Summary missing the out-of-schema line:\n%s", summary)
	}
	if !strings.Contains(summary, "EMAIL") {
		t.Errorf("Summary should name the excluded type:\n%s", summary)
	}
}
