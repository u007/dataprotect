package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/u007/dataprotect/golden"
)

// writeProvenance mirrors what scripts/fetch-dataset.sh records. The converter
// requires it — schema.json must state which dataset revision a number came
// from, so a fixture that omitted it would be testing a dataset nobody fetched.
func writeProvenance(t *testing.T, dataRoot, corpus string) {
	t.Helper()
	dir := filepath.Join(dataRoot, corpus)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "dataset: " + corpus + "\nrevision: 0123456789abcdef0123456789abcdef01234567\n"
	if err := os.WriteFile(filepath.Join(dir, "PROVENANCE.txt"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeNemotronFixture(t *testing.T, dataRoot string, rows []nemotronRow) string {
	t.Helper()
	writeProvenance(t, dataRoot, "nemotron-pii")
	return writeParquet(t, filepath.Join(dataRoot, "nemotron-pii", "data", "test-00000-of-00001.parquet"),
		func(w *parquet.GenericWriter[nemotronRow]) error {
			_, err := w.Write(rows)
			return err
		})
}

func writeGretelFixture(t *testing.T, dataRoot string, rows []gretelRow) string {
	t.Helper()
	writeProvenance(t, dataRoot, "gretel-pii-finance")
	return writeParquet(t, filepath.Join(dataRoot, "gretel-pii-finance", "data", "combined_test-00000-of-00001.parquet"),
		func(w *parquet.GenericWriter[gretelRow]) error {
			_, err := w.Write(rows)
			return err
		})
}

func writeParquet[T any](t *testing.T, path string, fill func(*parquet.GenericWriter[T]) error) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := parquet.NewGenericWriter[T](f)
	if err := fill(w); err != nil {
		t.Fatalf("writing parquet fixture: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing parquet fixture: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// Spec §7.5: overlapping gold is valid annotation nesting. It is counted for
// visibility and kept as gold — never a conversion failure.
func TestConvertKeepsOverlappingGold(t *testing.T) {
	root := t.TempDir()
	const text = "call 555-123-4567 ext2"
	writeNemotronFixture(t, root, []nemotronRow{{
		Text: text,
		Spans: `[{'start': 5, 'end': 17, 'text': '555-123-4567', 'label': 'phone_number'}, ` +
			`{'start': 10, 'end': 22, 'text': '23-4567 ext2', 'label': 'phone_number'}]`,
	}})

	cases, schema, err := Nemotron(root, 0)
	if err != nil {
		t.Fatalf("Nemotron: unexpected error: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("got %d cases, want 1", len(cases))
	}
	if len(cases[0].Entities) != 2 {
		t.Fatalf("got %d labels, want 2 — overlapping gold must be KEPT", len(cases[0].Entities))
	}
	if schema.OverlappingPairs != 1 {
		t.Errorf("OverlappingPairs = %d, want 1", schema.OverlappingPairs)
	}
	// Both labels must carry offsets; a nil offset would drop this into the
	// substring path, which rejects repeated values.
	for i, l := range cases[0].Entities {
		if l.Start == nil || l.End == nil {
			t.Fatalf("label %d has no offsets: %+v", i, l)
		}
	}
}

// Records with no in-schema label are negative cases, and they are what make
// precision a real number. They must never be treated as failures.
func TestConvertRecordsNegativeCases(t *testing.T) {
	root := t.TempDir()
	writeNemotronFixture(t, root, []nemotronRow{
		{Text: "no PII at all here", Spans: `[{'start': 3, 'end': 5, 'text': 'no', 'label': 'first_name'}]`},
	})

	cases, schema, err := Nemotron(root, 0)
	if err != nil {
		t.Fatalf("Nemotron: unexpected error: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("got %d cases, want 1", len(cases))
	}
	if len(cases[0].Entities) != 0 {
		t.Errorf("labels = %+v, want none — only first_name was present and it is unmapped", cases[0].Entities)
	}
	if schema.Negatives != 1 {
		t.Errorf("Negatives = %d, want 1", schema.Negatives)
	}
}

// Spec D6: a report must name the dataset revision and the exact input bytes it
// came from, or two runs against different releases of the same corpus would
// look comparable.
func TestConvertRecordsRevisionAndChecksum(t *testing.T) {
	root := t.TempDir()
	writeNemotronFixture(t, root, []nemotronRow{
		{Text: "call 555-123-4567", Spans: `[{'start': 5, 'end': 17, 'text': '555-123-4567', 'label': 'phone_number'}]`},
	})

	_, schema, err := Nemotron(root, 0)
	if err != nil {
		t.Fatalf("Nemotron: unexpected error: %v", err)
	}
	if schema.SourceRevision == "" {
		t.Error("SourceRevision is empty; the fetch script pins one and schema.json must carry it")
	}
	if len(schema.SourceRevision) != 40 {
		t.Errorf("SourceRevision = %q, want the 40-char SHA the fetch script records", schema.SourceRevision)
	}
	if schema.SourceSHA256 == "" {
		t.Error("SourceSHA256 is empty; the exact input bytes must be pinned")
	}
	if len(schema.SourceSHA256) != 64 {
		t.Errorf("SourceSHA256 = %q, want 64 hex chars", schema.SourceSHA256)
	}
	if schema.Cases != 1 {
		t.Errorf("Cases = %d, want 1", schema.Cases)
	}
	if schema.Source != nemotronRelPath {
		t.Errorf("Source = %q, want %q", schema.Source, nemotronRelPath)
	}
}

// A row the converter cannot read must abort with the row identified. Skipping
// it would drop gold labels silently and inflate precision in the report.
func TestConvertFailsFastNamesTheRow(t *testing.T) {
	root := t.TempDir()
	writeNemotronFixture(t, root, []nemotronRow{
		{Text: "fine 555-123-4567", Spans: `[{'start': 5, 'end': 17, 'text': '555-123-4567', 'label': 'phone_number'}]`},
		{Text: "broken row", Spans: `[{'start': 0, `},
	})

	_, _, err := Nemotron(root, 0)
	if err == nil {
		t.Fatal("Nemotron: want an error for an unparseable row, got nil")
	}
	for _, want := range []string{"nemotron", "row 1", "spans"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should contain %q", err.Error(), want)
		}
	}
}

// Spec §5.2: a record whose text is empty. golden.Load rejects a case with no
// text, so writing one would produce a dataset the scorer refuses to load —
// and if it were loaded anyway, an empty document would make every detection a
// false positive against a corpus that could not have annotated it.
func TestConvertFailsFastOnEmptyCaseText(t *testing.T) {
	root := t.TempDir()
	writeNemotronFixture(t, root, []nemotronRow{
		{Text: "", Spans: `[]`},
	})

	_, _, err := Nemotron(root, 0)
	if err == nil {
		t.Fatal("Nemotron: want an error for an empty record, got nil")
	}
	for _, want := range []string{"nemotron-pii", "row 0", "empty"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should contain %q", err.Error(), want)
		}
	}
}

// The sample bound must be honoured, or a report claiming 2000 rows would in
// fact describe all 100000.
func TestConvertHonoursLimit(t *testing.T) {
	root := t.TempDir()
	var rows []nemotronRow
	for i := 0; i < 5; i++ {
		rows = append(rows, nemotronRow{Text: "no pii here", Spans: `[]`})
	}
	writeNemotronFixture(t, root, rows)

	cases, schema, err := Nemotron(root, 2)
	if err != nil {
		t.Fatalf("Nemotron: unexpected error: %v", err)
	}
	if len(cases) != 2 {
		t.Errorf("got %d cases, want 2 (limit applied)", len(cases))
	}
	if schema.SourceRows != 5 {
		t.Errorf("SourceRows = %d, want 5 (the full row count, not the sample)", schema.SourceRows)
	}
	if schema.Limit != 2 {
		t.Errorf("Limit = %d, want 2 (the sample must be recorded)", schema.Limit)
	}
}

// Determinism is an AGENTS.md requirement: the same corpus and limit must
// produce byte-identical output, or two runs cannot be compared and a published
// number cannot be reproduced.
func TestConvertDeterministic(t *testing.T) {
	root := t.TempDir()
	writeNemotronFixture(t, root, []nemotronRow{
		{Text: "call 555-123-4567", Spans: `[{'start': 5, 'end': 17, 'text': '555-123-4567', 'label': 'phone_number'}]`},
		{Text: "mail a@b.com", Spans: `[{'start': 5, 'end': 12, 'text': 'a@b.com', 'label': 'email'}]`},
		{Text: "no pii at all", Spans: `[]`},
	})

	first := mustConvertAndWrite(t, root, "run-a")
	second := mustConvertAndWrite(t, root, "run-b")

	if len(first) == 0 {
		t.Fatal("no files produced")
	}
	if len(first) != len(second) {
		t.Fatalf("file counts differ: %d vs %d", len(first), len(second))
	}
	for name, want := range first {
		got, ok := second[name]
		if !ok {
			t.Errorf("run-b is missing %s", name)
			continue
		}
		if !bytes.Equal(want, got) {
			t.Errorf("%s differs between runs (%d vs %d bytes)", name, len(want), len(got))
		}
	}
}

// Write must emit the sorted fixed-width sequence plus the sidecar, and the
// output must load through golden.Load — including with schema.json present
// beside the cases, which is the layout that broke before Task 1.
func TestWriteEmitsSortedOutputAndSidecar(t *testing.T) {
	root := t.TempDir()
	writeNemotronFixture(t, root, []nemotronRow{
		{Text: "call 555-123-4567", Spans: `[{'start': 5, 'end': 17, 'text': '555-123-4567', 'label': 'phone_number'}]`},
	})

	dir := filepath.Join(t.TempDir(), "out")
	cases, schema, err := Nemotron(root, 0)
	if err != nil {
		t.Fatalf("Nemotron: unexpected error: %v", err)
	}
	if err := Write(dir, cases, schema); err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var caseFiles, sawSchema bool
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
		switch {
		case e.Name() == "schema.json":
			sawSchema = true
		case strings.HasPrefix(e.Name(), "cases-") && strings.HasSuffix(e.Name(), ".json"):
			caseFiles = true
		}
	}
	if !sawSchema {
		t.Errorf("schema.json missing; files = %v", names)
	}
	if !caseFiles {
		t.Errorf("no cases-*.json emitted; files = %v", names)
	}

	// Deterministic file naming: a fixed 6-digit width, so lexical order equals
	// numeric order and STAYS equal past file 999. At 500 cases per file a
	// 3-digit name would render cases-1000.json, which sorts before
	// cases-999.json — silently breaking the sorted-read contract golden.Load
	// depends on. Assert the width itself, not a couple of sample names.
	for _, n := range names {
		if !strings.HasPrefix(n, "cases-") || !strings.HasSuffix(n, ".json") {
			continue
		}
		digits := strings.TrimSuffix(strings.TrimPrefix(n, "cases-"), ".json")
		if len(digits) != 6 {
			t.Errorf("case file %q: width %d, want exactly 6 zero-padded digits", n, len(digits))
			continue
		}
		for _, r := range digits {
			if r < '0' || r > '9' {
				t.Errorf("case file %q is not zero-padded digits", n)
				break
			}
		}
	}
}

// TestWriteReplacesStaleCaseFiles pins that Write replaces whatever
// cases-*.json already sit in dir. Reusing an output directory is the normal
// workflow (vary LIMIT, re-fetch a corpus revision), and before this fix a
// stale chunk from an earlier, larger conversion survived the rewrite:
// golden.Load then read MORE cases than schema.json reports, so the benchmark
// header understated the sample it scored and could attribute a number to
// corpus bytes it was not measured on.
func TestWriteReplacesStaleCaseFiles(t *testing.T) {
	root := t.TempDir()
	writeNemotronFixture(t, root, []nemotronRow{
		{Text: "call 555-123-4567", Spans: `[{'start': 5, 'end': 17, 'text': '555-123-4567', 'label': 'phone_number'}]`},
	})

	dir := filepath.Join(t.TempDir(), "out")

	// An earlier, larger conversion left chunks behind in the target directory.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "cases-004.json")
	if err := os.WriteFile(stale, []byte(`[{"id":"stale-000000","text":"old row","entities":[]}]`), 0o644); err != nil {
		t.Fatal(err)
	}

	cases, schema, err := Nemotron(root, 0)
	if err != nil {
		t.Fatalf("Nemotron: unexpected error: %v", err)
	}
	if err := Write(dir, cases, schema); err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}

	loaded, err := golden.Load(dir)
	if err != nil {
		t.Fatalf("golden.Load: unexpected error: %v", err)
	}
	if len(loaded) != len(cases) {
		t.Errorf("golden.Load returned %d cases, want %d — stale case files survive Write, "+
			"so schema.json's cases count misdescribes what is scored",
			len(loaded), len(cases))
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale %s still present after Write", filepath.Base(stale))
	}
}

// mustConvertAndWrite converts and writes into a fresh directory, returning
// every emitted file keyed by name.
func mustConvertAndWrite(t *testing.T, root, name string) map[string][]byte {
	t.Helper()
	cases, schema, err := Nemotron(root, 0)
	if err != nil {
		t.Fatalf("Nemotron: unexpected error: %v", err)
	}
	dir := filepath.Join(t.TempDir(), name)
	if err := Write(dir, cases, schema); err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = b
	}
	return out
}

// The written output must be loadable by the very scorer that will read it, so
// this asserts through golden.LoadSchema and golden.Load rather than a local
// re-implementation that could drift away from them.
func TestWrittenOutputLoadsThroughGolden(t *testing.T) {
	root := t.TempDir()
	writeNemotronFixture(t, root, []nemotronRow{
		{Text: "call 555-123-4567", Spans: `[{'start': 5, 'end': 17, 'text': '555-123-4567', 'label': 'phone_number'}]`},
	})
	dir := filepath.Join(t.TempDir(), "out")
	cases, schema, err := Nemotron(root, 0)
	if err != nil {
		t.Fatalf("Nemotron: unexpected error: %v", err)
	}
	if err := Write(dir, cases, schema); err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}

	sch, err := golden.LoadSchema(dir)
	if err != nil {
		t.Fatalf("golden.LoadSchema on written output: %v", err)
	}
	if sch.Corpus == "" {
		t.Error("written schema has an empty corpus name")
	}
	if sch.OffsetUnit != "bytes" {
		t.Errorf("OffsetUnit = %q, want bytes", sch.OffsetUnit)
	}
	if len(sch.InSchema) == 0 {
		t.Error("written schema has no in_schema types; the scoring filter would exclude everything")
	}

	loaded, err := golden.Load(dir)
	if err != nil {
		t.Fatalf("golden.Load on written output (schema.json beside the cases): %v", err)
	}
	if len(loaded) != len(cases) {
		t.Errorf("golden.Load returned %d cases, want %d", len(loaded), len(cases))
	}
	if loaded[0].ID != cases[0].ID {
		t.Errorf("case id = %q, want %q", loaded[0].ID, cases[0].ID)
	}
}

// casesPerFile+1 cases must split into a full first chunk and a single-case
// second one, with the order and the count preserved through golden.Load. The
// boundary is where an off-by-one in the chunk arithmetic would drop or
// duplicate a case, so it is tested at exactly that size.
func TestWriteChunkBoundaryKeepsEveryCaseInOrder(t *testing.T) {
	total := casesPerFile + 1
	cases := make([]golden.Case, 0, total)
	for i := 0; i < total; i++ {
		cases = append(cases, golden.Case{ID: fmt.Sprintf("case-%04d", i), Text: "no pii here"})
	}
	dir := t.TempDir()
	if err := Write(dir, cases, golden.Schema{Corpus: "synthetic", Cases: total}); err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}

	wantSizes := map[string]int{"cases-000000.json": casesPerFile, "cases-000001.json": 1}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	gotFiles := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "cases-") {
			continue
		}
		gotFiles++
		want, ok := wantSizes[e.Name()]
		if !ok {
			t.Errorf("unexpected case file %s", e.Name())
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var chunk []golden.Case
		if err := json.Unmarshal(raw, &chunk); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if len(chunk) != want {
			t.Errorf("%s holds %d cases, want %d", e.Name(), len(chunk), want)
		}
	}
	if gotFiles != len(wantSizes) {
		t.Errorf("got %d case files, want %d", gotFiles, len(wantSizes))
	}

	loaded, err := golden.Load(dir)
	if err != nil {
		t.Fatalf("golden.Load: unexpected error: %v", err)
	}
	if len(loaded) != total {
		t.Fatalf("golden.Load returned %d cases, want %d", len(loaded), total)
	}
	for i, c := range loaded {
		if want := fmt.Sprintf("case-%04d", i); c.ID != want {
			t.Fatalf("loaded[%d].ID = %q, want %q (dropped, duplicated or reordered at the seam)", i, c.ID, want)
		}
	}
}

// A read that returns no rows and no error must not end the scan: that is how a
// partial dataset gets written with no sign it was partial. Only io.EOF ends it.
func TestEndOfReadOnlyEOFEndsTheScan(t *testing.T) {
	boom := fmt.Errorf("disk gone")
	cases := []struct {
		name    string
		n       int
		readErr error
		done    bool
		wantErr bool
	}{
		{"rows and no error continue", 3, nil, false, false},
		{"io.EOF ends cleanly", 0, io.EOF, true, false},
		{"rows with io.EOF end cleanly", 2, io.EOF, true, false},
		{"zero rows and no error is refused", 0, nil, false, true},
		{"a read error is reported", 0, boom, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			done, err := endOfRead("nemotron-pii", "data/x.parquet", 7, tc.n, tc.readErr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if done != tc.done {
				t.Errorf("done = %v, want %v", done, tc.done)
			}
		})
	}
	_, err := endOfRead("nemotron-pii", "data/x.parquet", 7, 0, nil)
	if err == nil || !strings.Contains(err.Error(), "row 7") || !strings.Contains(err.Error(), "data/x.parquet") {
		t.Errorf("zero-row error %v should name the file and row", err)
	}
}
