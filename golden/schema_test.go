package golden

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// TestLoadSkipsSchemaSidecar is the regression that makes the reserved name
// necessary: Load reads every *.json in the directory, so a corpus sidecar
// written beside the cases is decoded as []Case and the WHOLE dataset fails
// with "cannot unmarshal object into Go value of type []golden.Case". The
// converter writes both files into one directory, so without this skip the
// corpus scoring mode cannot load anything.
func TestLoadSkipsSchemaSidecar(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cases-000.json"),
		[]byte(`[{"id":"c1","text":"hello","entities":[]}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	// A Schema object, i.e. a JSON object and NOT an array of cases.
	if err := os.WriteFile(filepath.Join(dir, SchemaFilename),
		[]byte(`{"corpus":"gretel-pii-finance","in_schema":["EMAIL"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cases, err := Load(dir)
	if err != nil {
		t.Fatalf("Load with a schema sidecar present: unexpected error: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("Load: got %d cases, want 1 — the sidecar must not be parsed as a case file", len(cases))
	}
	if cases[0].ID != "c1" {
		t.Errorf("Load: case id = %q, want %q", cases[0].ID, "c1")
	}
}

// TestLoadSchemaReadsSidecar is the positive path: the converter's schema must
// round-trip, including its in-schema type list, or the scoring filter would
// silently start with an empty set and count every prediction as out-of-schema.
func TestLoadSchemaReadsSidecar(t *testing.T) {
	dir := t.TempDir()
	raw := `{
		"corpus": "gretel-pii-finance",
		"source": "testdata/external/gretel-pii-finance/data/combined_test-00000-of-00001.parquet",
		"source_sha256": "abc123",
		"source_revision": "7b844d16738527a04264f50214cb426a4cea0897",
		"source_rows": 5596,
		"limit": 2000,
		"cases": 1986,
		"offset_unit": "bytes",
		"in_schema": ["EMAIL", "IBAN", "PHONE"],
		"unmapped_labels": ["company", "date", "name"],
		"overlapping_pairs": 16,
		"negatives": 1400,
		"generated_by": "tools/corpusbench convert"
	}`
	if err := os.WriteFile(filepath.Join(dir, SchemaFilename), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := LoadSchema(dir)
	if err != nil {
		t.Fatalf("LoadSchema: unexpected error: %v", err)
	}
	if s.Corpus != "gretel-pii-finance" {
		t.Errorf("Corpus = %q, want %q", s.Corpus, "gretel-pii-finance")
	}
	if s.SourceRows != 5596 || s.Limit != 2000 || s.Cases != 1986 {
		t.Errorf("counts = (%d,%d,%d), want (5596,2000,1986)", s.SourceRows, s.Limit, s.Cases)
	}
	if s.OverlappingPairs != 16 || s.Negatives != 1400 {
		t.Errorf("(overlapping_pairs,negatives) = (%d,%d), want (16,1400)", s.OverlappingPairs, s.Negatives)
	}
	want := []recognizers.EntityType{recognizers.EntityEmail, "IBAN", recognizers.EntityPhone}
	if len(s.InSchema) != len(want) {
		t.Fatalf("InSchema = %v, want %v", s.InSchema, want)
	}
	for i, typ := range want {
		if s.InSchema[i] != typ {
			t.Errorf("InSchema[%d] = %q, want %q", i, s.InSchema[i], typ)
		}
	}
	if s.OffsetUnit != "bytes" {
		t.Errorf("OffsetUnit = %q, want %q", s.OffsetUnit, "bytes")
	}
	if len(s.UnmappedLabels) != 3 || s.UnmappedLabels[0] != "company" {
		t.Errorf("UnmappedLabels = %v, want [company date name]", s.UnmappedLabels)
	}
}

// TestLoadSchemaRejectsUnknownField mirrors Load's DisallowUnknownFields rule.
// The converter is the only writer, so an unknown field is version skew or a
// hand-edit — a loud failure beats a silently unread field.
func TestLoadSchemaRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, SchemaFilename),
		[]byte(`{"corpus":"x","nonsense":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSchema(dir)
	if err == nil {
		t.Fatal("LoadSchema: want error for unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "nonsense") {
		t.Errorf("LoadSchema error %q should name the unknown field", err.Error())
	}
}

// TestLoadSchemaRejectsCaseArray catches the misnamed-file case: a case array
// left at schema.json must not decode into a zero Schema that would then mean
// "no in-schema types" and push every prediction out of scoring.
func TestLoadSchemaRejectsCaseArray(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, SchemaFilename),
		[]byte(`[{"id":"x","text":"hello","entities":[]}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSchema(dir)
	if err == nil {
		t.Fatalf("LoadSchema: want error for a case array, got nil (Schema=%+v)", s)
	}
}

// TestLoadSchemaRejectsUnknownEntityType fails loudly on a type the detector
// cannot emit. A typo in in_schema would otherwise exclude a real type from
// scoring while looking like a valid configuration.
func TestLoadSchemaRejectsUnknownEntityType(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, SchemaFilename),
		[]byte(`{"corpus":"x","in_schema":["EMIAL"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSchema(dir)
	if err == nil {
		t.Fatal("LoadSchema: want error for unknown entity type, got nil")
	}
	if !strings.Contains(err.Error(), "EMIAL") {
		t.Errorf("LoadSchema error %q should name the bad type", err.Error())
	}
}

// TestLoadSchemaRejectsMissingSidecar: an absent schema.json is a different
// state from an unreadable one, and scoring a corpus without its schema would
// fall back to counting every detection — the FP trap the spec exists to avoid.
func TestLoadSchemaRejectsMissingSidecar(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadSchema(dir); err == nil {
		t.Fatal("LoadSchema: want error for a missing schema.json, got nil")
	}
}
