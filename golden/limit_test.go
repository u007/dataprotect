package golden

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A case limit scores a PREFIX of the loaded cases, in file order, so a
// limited run is deterministic and reproducible.
//
// The limit exists for the expensive detectors: the NLP sidecar costs about
// 20 ms per text, so a full corpus takes about 40 seconds and a developer
// wants a faster signal. The trade is that a limited run is a SAMPLE, and
// nothing about the report may let a reader mistake it for a full-corpus
// figure — hence Report.Cases reports what was actually scored.
func TestWithCaseLimitScoresAPrefix(t *testing.T) {
	dir := t.TempDir()
	cases := `[` +
		`{"id":"c1","text":"one","entities":[]},` +
		`{"id":"c2","text":"two","entities":[]},` +
		`{"id":"c3","text":"three","entities":[]}]`
	if err := os.WriteFile(filepath.Join(dir, "cases-000.json"), []byte(cases), 0o600); err != nil {
		t.Fatal(err)
	}

	predict := func(ctx context.Context, text string) ([]Prediction, error) {
		return nil, nil
	}

	full, err := Score(context.Background(), dir, predict)
	if err != nil {
		t.Fatalf("scoring: %v", err)
	}
	if full.Cases != 3 {
		t.Errorf("full run scored %d cases, want 3", full.Cases)
	}

	limited, err := Score(context.Background(), dir, predict, WithCaseLimit(2))
	if err != nil {
		t.Fatalf("scoring with a limit: %v", err)
	}
	if limited.Cases != 2 {
		t.Errorf("limited run scored %d cases, want 2", limited.Cases)
	}
}

// A limit of zero or less means no limit, which is the default.
func TestWithCaseLimitZeroMeansNoLimit(t *testing.T) {
	dir := t.TempDir()
	cases := `[` +
		`{"id":"c1","text":"one","entities":[]},` +
		`{"id":"c2","text":"two","entities":[]}]`
	if err := os.WriteFile(filepath.Join(dir, "cases-000.json"), []byte(cases), 0o600); err != nil {
		t.Fatal(err)
	}

	predict := func(ctx context.Context, text string) ([]Prediction, error) {
		return nil, nil
	}

	rep, err := Score(context.Background(), dir, predict, WithCaseLimit(0))
	if err != nil {
		t.Fatalf("scoring: %v", err)
	}
	if rep.Cases != 2 {
		t.Errorf("scored %d cases, want 2: a limit of zero must not limit", rep.Cases)
	}
}

// A limit larger than the dataset must not fail. It is a cap, not a demand.
func TestWithCaseLimitLargerThanDataset(t *testing.T) {
	dir := t.TempDir()
	cases := `[{"id":"c1","text":"one","entities":[]}]`
	if err := os.WriteFile(filepath.Join(dir, "cases-000.json"), []byte(cases), 0o600); err != nil {
		t.Fatal(err)
	}

	predict := func(ctx context.Context, text string) ([]Prediction, error) {
		return nil, nil
	}

	rep, err := Score(context.Background(), dir, predict, WithCaseLimit(1000))
	if err != nil {
		t.Fatalf("scoring: %v", err)
	}
	if rep.Cases != 1 {
		t.Errorf("scored %d cases, want 1", rep.Cases)
	}
}
