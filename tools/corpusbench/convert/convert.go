package convert

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/parquet-go/parquet-go"
	"github.com/u007/dataprotect/golden"
	"github.com/u007/dataprotect/recognizers"
)

// casesPerFile is the chunk size for the emitted case sequence. A single file
// holding every case would be a multi-megabyte blob, while a chunk stays
// inspectable; 500 is recorded here because no size was pinned elsewhere, and
// the naming is zero-padded so lexical order equals numeric order.
const casesPerFile = 500

// convertParquet reads one corpus file and turns its records into golden cases
// plus the schema sidecar that describes them.
//
// It fails fast on anything it cannot convert. A record that is skipped rather
// than reported would drop gold labels, and dropped gold inflates precision in
// the finished report — the silent failure this whole pipeline exists to avoid.
// Every error names the corpus, the file and the row, so a failure is
// actionable rather than a bare count of rejected rows.
func convertParquet[T any](corpus, dataRoot, relPath string, limit int,
	extract func(T) (text, spansRaw string),
	parseSpans func(string) ([]Span, error),
	labelMap map[string]recognizers.EntityType,
) ([]golden.Case, golden.Schema, error) {
	path := filepath.Join(dataRoot, relPath)
	revision, err := readProvenance(dataRoot, corpus)
	if err != nil {
		return nil, golden.Schema{}, err
	}
	sum, err := fileSHA256(path)
	if err != nil {
		return nil, golden.Schema{}, err
	}

	raw, err := os.Open(path)
	if err != nil {
		return nil, golden.Schema{}, fmt.Errorf("corpus %s: opening %s: %w", corpus, path, err)
	}
	defer raw.Close()
	info, err := raw.Stat()
	if err != nil {
		return nil, golden.Schema{}, fmt.Errorf("corpus %s: stating %s: %w", corpus, path, err)
	}
	pf, err := parquet.OpenFile(raw, info.Size())
	if err != nil {
		return nil, golden.Schema{}, fmt.Errorf("corpus %s: opening parquet %s: %w", corpus, path, err)
	}
	sourceRows := int(pf.NumRows())

	reader := parquet.NewGenericReader[T](pf)
	defer reader.Close()

	var (
		cases     []golden.Case
		negatives int
		overlaps  int
		unmapped  = map[string]bool{}
		row       int
		buf       = make([]T, 1024)
	)
	for {
		if limit > 0 && row >= limit {
			break
		}
		n, readErr := reader.Read(buf)
		for i := 0; i < n; i++ {
			if limit > 0 && row >= limit {
				break
			}
			text, spansRaw := extract(buf[i])
			if text == "" {
				return nil, golden.Schema{}, fmt.Errorf(
					"corpus %s: file %s row %d: record text is empty; a case with no text "+
						"cannot be scored and would be rejected downstream", corpus, relPath, row)
			}
			spans, err := parseSpans(spansRaw)
			if err != nil {
				return nil, golden.Schema{}, fmt.Errorf(
					"corpus %s: file %s row %d: %w", corpus, relPath, row, err)
			}
			labels, unmappedNames, err := BuildLabels(text, spans, labelMap)
			if err != nil {
				return nil, golden.Schema{}, fmt.Errorf(
					"corpus %s: file %s row %d: %w", corpus, relPath, row, err)
			}
			for _, name := range unmappedNames {
				unmapped[name] = true
			}
			unlabelled, err := BuildUnlabelled(text, spans, labelMap)
			if err != nil {
				return nil, golden.Schema{}, fmt.Errorf(
					"corpus %s: file %s row %d: %w", corpus, relPath, row, err)
			}
			if len(labels) == 0 {
				negatives++
			}
			pairs, err := countOverlappingPairs(labels)
			if err != nil {
				return nil, golden.Schema{}, fmt.Errorf(
					"corpus %s: file %s row %d: %w", corpus, relPath, row, err)
			}
			overlaps += pairs
			cases = append(cases, golden.Case{
				ID:         fmt.Sprintf("%s-%06d", corpus, row),
				Text:       text,
				Entities:   labels,
				Unlabelled: unlabelled,
			})
			row++
		}
		done, err := endOfRead(corpus, relPath, row, n, readErr)
		if err != nil {
			return nil, golden.Schema{}, err
		}
		if done {
			break
		}
	}

	if len(cases) == 0 {
		return nil, golden.Schema{}, fmt.Errorf(
			"corpus %s: file %s produced no cases (source_rows=%d, limit=%d)",
			corpus, relPath, sourceRows, limit)
	}

	return cases, golden.Schema{
		Corpus:             corpus,
		Source:             relPath,
		SourceSHA256:       sum,
		SourceRevision:     revision,
		SourceRows:         sourceRows,
		Limit:              limit,
		Cases:              len(cases),
		OffsetUnit:         "bytes",
		InSchema:           inSchemaTypes(labelMap),
		UnmappedLabels:     sortedKeys(unmapped),
		OverlappingPairs:   overlaps,
		UnlabelledRecorded: true,
		Negatives:          negatives,
		GeneratedBy:        "tools/corpusbench convert",
	}, nil
}

// endOfRead decides whether a parquet read ends the scan. Rows with a nil error
// continue the scan; io.EOF is the one clean end, and any rows returned with it
// have already been converted by the caller. A read that yields no rows and no
// error is refused rather than treated as the end: stopping there would write a
// partial dataset with nothing to say it was partial.
func endOfRead(corpus, relPath string, row, n int, readErr error) (bool, error) {
	switch {
	case readErr == io.EOF:
		return true, nil
	case readErr != nil:
		return false, fmt.Errorf("corpus %s: file %s: reading row %d: %w", corpus, relPath, row, readErr)
	case n == 0:
		return false, fmt.Errorf("corpus %s: file %s: read returned no rows and no error at row %d; "+
			"stopping here would write a partial dataset", corpus, relPath, row)
	}
	return false, nil
}

// inSchemaTypes is the sorted set of types the label map can produce. It is
// derived from the MAP rather than from the records seen, so two samples of the
// same corpus declare the same scope: a type the corpus is able to label stays
// in schema even when this sample happens not to contain one, and a detection
// of it is then judged on its merits instead of excluded.
func inSchemaTypes(labelMap map[string]recognizers.EntityType) []recognizers.EntityType {
	seen := map[recognizers.EntityType]bool{}
	for _, typ := range labelMap {
		seen[typ] = true
	}
	out := make([]recognizers.EntityType, 0, len(seen))
	for typ := range seen {
		out = append(out, typ)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// readProvenance reads the revision the fetch script pinned for a corpus.
// It is required rather than optional: schema.json records which corpus version
// a number came from, and an empty revision there would let a report be
// mistaken for one generated from a different release of the same dataset.
func readProvenance(dataRoot, corpus string) (string, error) {
	path := filepath.Join(dataRoot, corpus, "PROVENANCE.txt")
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("corpus %s: reading %s (a conversion must record which dataset "+
			"revision it used): %w", corpus, path, err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if rest, ok := strings.CutPrefix(line, "revision:"); ok {
			rev := strings.TrimSpace(rest)
			if rev == "" {
				return "", fmt.Errorf("corpus %s: %s has an empty revision", corpus, path)
			}
			return rev, nil
		}
	}
	return "", fmt.Errorf("corpus %s: %s has no revision line", corpus, path)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("checksumming %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("checksumming %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Write emits the case sequence and the schema sidecar into dir.
//
// Files are named cases-000000.json, cases-000001.json, ... Six digits
// rather than three so lexical order still matches numeric order past file
// 999: a 3-digit name renders cases-1000.json, which sorts before
// cases-999.json, and golden.Load reads in sorted filename order — so the
// loaded dataset would drop the tail on a large corpus.
//
// An empty case set is an error: golden.Load rejects a dataset that parses to
// zero cases, because with no cases every metric collapses to the
// zero-denominator value of 1 and an empty directory would report perfect
// accuracy. Writing one would produce output that cannot be loaded.
func Write(dir string, cases []golden.Case, schema golden.Schema) error {
	if len(cases) == 0 {
		return fmt.Errorf("refusing to write an empty dataset: golden.Load would reject it " +
			"(no cases means no denominators, and every metric collapses to 1)")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	// Replace whatever case files already sit here. An output directory is
	// reused in normal use — vary LIMIT, re-fetch a corpus revision — and a
	// chunk left behind by an earlier, larger conversion would survive this
	// rewrite. golden.Load would then read more cases than this schema
	// declares, so schema.json's `cases` would understate the sample actually
	// scored and a sampled report could be mistaken for a full-corpus one.
	stale, err := filepath.Glob(filepath.Join(dir, "cases-*.json"))
	if err != nil {
		return fmt.Errorf("matching existing case files in %s: %w", dir, err)
	}
	for _, name := range stale {
		if err := os.Remove(name); err != nil {
			return fmt.Errorf("removing stale case file %s: %w", filepath.Base(name), err)
		}
	}

	for start, index := 0, 0; start < len(cases); start, index = start+casesPerFile, index+1 {
		end := start + casesPerFile
		if end > len(cases) {
			end = len(cases)
		}
		raw, err := marshalCases(cases[start:end])
		if err != nil {
			return fmt.Errorf("encoding cases %d..%d: %w", start, end-1, err)
		}
		name := fmt.Sprintf("cases-%06d.json", index)
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	raw, err := marshalSchema(schema)
	if err != nil {
		return fmt.Errorf("encoding schema: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, golden.SchemaFilename), raw, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", golden.SchemaFilename, err)
	}
	return nil
}

// marshalCases encodes a chunk of cases. Indentation is deliberate: a
// converted dataset is read by golden.Load and occasionally by a person, and
// one case per line keeps a diff of two conversions readable.
func marshalCases(cases []golden.Case) ([]byte, error) {
	var b strings.Builder
	b.WriteByte('[')
	for i, c := range cases {
		if i > 0 {
			b.WriteString(",\n")
		}
		raw, err := json.Marshal(c)
		if err != nil {
			return nil, err
		}
		b.Write(raw)
	}
	b.WriteString("]\n")
	return []byte(b.String()), nil
}

// marshalSchema encodes the sidecar with the same shape golden.LoadSchema
// expects, so a mismatch between writer and reader fails in the first test that
// round-trips rather than in a later scoring run.
func marshalSchema(schema golden.Schema) ([]byte, error) {
	raw, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}
