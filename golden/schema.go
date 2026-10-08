package golden

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"dataprotect/recognizers"
)

// SchemaFilename is the reserved sidecar name carrying a dataset's label
// schema beside its case files.
//
// It is reserved because Load reads every *.json in a directory: without an
// explicit skip, this file would be decoded as []Case and the whole dataset
// would fail to load with "cannot unmarshal object into Go value of type
// []golden.Case". A case file deliberately given this name is therefore not
// loaded — which is why LoadSchema rejects a case array here loudly rather
// than returning an empty schema.
const SchemaFilename = "schema.json"

// Schema declares which entity types a dataset labels, so scoring can tell a
// real false positive from a detection the dataset never claimed to annotate.
//
// The distinction matters for corpus datasets: a corpus labelling only PHONE
// must not have a correct EMAIL detection counted as a false positive, because
// the corpus simply does not annotate EMAIL at all. Scores produced without a
// schema treat every detection as in-schema, which is the right default for the
// hand-labelled seed set.
//
// Fields mirror what the converter records, including the sampling bounds, so a
// report generated from a subset can never be mistaken for a full-corpus one.
type Schema struct {
	// Corpus is the dataset name, e.g. "gretel-pii-finance".
	Corpus string `json:"corpus"`
	// Source is the path of the file the cases were converted from.
	Source string `json:"source"`
	// SourceSHA256 is the checksum of that file, pinning the exact input.
	SourceSHA256 string `json:"source_sha256"`
	// SourceRevision is the upstream dataset revision the file was fetched at.
	SourceRevision string `json:"source_revision"`
	// SourceRows is the total row count in Source, before any sampling.
	SourceRows int `json:"source_rows"`
	// Limit is the row bound applied during conversion; 0 means all rows.
	Limit int `json:"limit"`
	// Cases is the number of case files produced.
	Cases int `json:"cases"`
	// OffsetUnit records what label offsets are measured in. The converter
	// always emits "bytes"; corpus offsets arrive as character indices and are
	// translated, so anything else would mean the labels are unusable.
	OffsetUnit string `json:"offset_unit"`
	// InSchema is the set of types this dataset labels, sorted.
	InSchema []recognizers.EntityType `json:"in_schema"`
	// UnmappedLabels lists corpus labels the converter deliberately dropped,
	// sorted, so the filter's scope is auditable rather than implicit.
	UnmappedLabels []string `json:"unmapped_labels"`
	// OverlappingPairs counts gold label pairs that overlap. Overlap is valid
	// annotation nesting and is kept, not resolved away.
	OverlappingPairs int `json:"overlapping_pairs"`
	// Negatives counts cases with no in-schema label, which is what makes the
	// precision number meaningful.
	Negatives int `json:"negatives"`
	// GeneratedBy names the tool and command that wrote the dataset.
	GeneratedBy string `json:"generated_by"`
}

// LoadSchema reads the schema sidecar from dir.
//
// Unknown fields are rejected for the same reason Load rejects them: the
// converter is the only writer, so an unknown field is version skew or a
// hand-edit, and a silently ignored one would mean the scoring filter ran with
// a schema the reader did not understand.
//
// Each InSchema entry must be a type the detector can emit. A typo there would
// exclude a real type from scoring while looking like a valid configuration,
// so it fails loudly with the offending value.
func LoadSchema(dir string) (Schema, error) {
	var s Schema
	path := filepath.Join(dir, SchemaFilename)
	raw, err := os.ReadFile(path)
	if err != nil {
		return Schema{}, fmt.Errorf("reading schema sidecar %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return Schema{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	for i, typ := range s.InSchema {
		if _, err := recognizers.EntityTypeFromString(string(typ)); err != nil {
			return Schema{}, fmt.Errorf("%s: in_schema[%d]: %w", path, i, err)
		}
	}
	return s, nil
}
