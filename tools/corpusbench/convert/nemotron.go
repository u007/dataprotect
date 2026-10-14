package convert

import (
	"github.com/u007/dataprotect/golden"
)

// corpusNemotron is the directory name fetch-dataset.sh creates for this corpus
// and the value recorded in schema.json's "corpus" field.
const corpusNemotron = "nemotron-pii"

// nemotronRelPath is the file the conversion reads: the held-out test split at
// the revision pinned in PROVENANCE.txt. The train split is deliberately not
// used — measuring a detector against the data a model would train on is not
// an evaluation.
const nemotronRelPath = "nemotron-pii/data/test-00000-of-00001.parquet"

// nemotronRow is the subset of nemotron's columns this converter reads.
// parquet-go matches by name, so the other seven columns of the real file
// (uid, domain, document_type, document_description, document_format, locale,
// text_tagged) are ignored rather than required.
type nemotronRow struct {
	Text  string `parquet:"text"`
	Spans string `parquet:"spans"`
}

// Nemotron converts nemotron's test split into golden cases and their schema.
//
// limit bounds how many records are read in file order; 0 means all of them.
// The bound is recorded in the returned schema, so a report generated from a
// sample cannot be mistaken for a full-corpus measurement.
//
// Every failure is an error rather than a skipped record: dropping a row would
// drop its gold labels, and unlabelled entities then score as false positives
// in the finished report.
func Nemotron(dataRoot string, limit int) ([]golden.Case, golden.Schema, error) {
	return convertParquet[nemotronRow](
		corpusNemotron, dataRoot, nemotronRelPath, limit,
		func(r nemotronRow) (text, spans string) { return r.Text, r.Spans },
		NemotronSpans,
		NemotronLabels,
	)
}
