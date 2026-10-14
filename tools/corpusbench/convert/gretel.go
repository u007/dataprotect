package convert

import (
	"encoding/json"
	"fmt"

	"github.com/u007/dataprotect/golden"
)

// gretelSpan is the decoded shape of one pii_spans element. Fields are pointers
// so a missing member is distinguishable from a legitimate zero: decoding
// {"start":1} into plain ints would silently produce a span at 0..0 with an
// empty label, which is not an annotation at all.
type gretelSpan struct {
	Start *int    `json:"start"`
	End   *int    `json:"end"`
	Label *string `json:"label"`
}

// GretelSpans parses the value of gretel's `pii_spans` column.
//
// Unlike nemotron's repr, this column is valid JSON, so the standard decoder
// handles it. Two deliberate choices:
//
//   - Unknown members are tolerated. The corpus may gain fields without that
//     invalidating annotations the converter does not read, and rejecting the
//     whole file for an additive change would be a fallback in reverse —
//     refusing data we can in fact use.
//   - Missing or mistyped members are errors. json.Unmarshal already fails on
//     a wrong type (so `{"start":"one"}` is rejected), and nil pointers catch
//     the absent case that a plain int would turn into a silent 0.
//
// Malformed input must fail loudly: a truncated span list drops gold labels,
// and every dropped label becomes a false positive that is never counted.
func GretelSpans(raw string) ([]Span, error) {
	var decoded []gretelSpan
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("spans: %w", err)
	}
	if len(decoded) == 0 {
		return nil, nil
	}
	out := make([]Span, 0, len(decoded))
	for i, d := range decoded {
		switch {
		case d.Start == nil:
			return nil, fmt.Errorf("spans: element %d has no \"start\"", i)
		case d.End == nil:
			return nil, fmt.Errorf("spans: element %d has no \"end\"", i)
		case d.Label == nil:
			return nil, fmt.Errorf("spans: element %d has no \"label\"", i)
		case *d.Label == "":
			return nil, fmt.Errorf("spans: element %d has an empty \"label\"", i)
		}
		out = append(out, Span{Start: *d.Start, End: *d.End, Label: *d.Label})
	}
	return out, nil
}

// corpusGretel is the directory name fetch-dataset.sh creates for this corpus
// and the value recorded in schema.json's "corpus" field.
const corpusGretel = "gretel-pii-finance"

// gretelRelPath is the combined held-out test split across all of gretel's
// languages. It is used rather than a per-language file so one conversion
// covers the whole corpus; the separate language files are a deferred item in
// TODO.md.
const gretelRelPath = "gretel-pii-finance/data/combined_test-00000-of-00001.parquet"

// gretelRow is the subset of gretel's columns this converter reads. The real
// file has seventeen columns, including two LIST columns, none of which are
// needed for these three.
type gretelRow struct {
	Text  string `parquet:"generated_text"`
	Spans string `parquet:"pii_spans"`
}

// Gretel converts gretel's combined test split into golden cases and their
// schema. It exists alongside Nemotron because the two label different sets:
// gretel labels iban and nemotron does not, while nemotron labels url and
// gretel does not. Converting one corpus would leave a recognizer unmeasured.
//
// limit bounds how many records are read in file order; 0 means all of them.
func Gretel(dataRoot string, limit int) ([]golden.Case, golden.Schema, error) {
	return convertParquet[gretelRow](
		corpusGretel, dataRoot, gretelRelPath, limit,
		func(r gretelRow) (text, spans string) { return r.Text, r.Spans },
		GretelSpans,
		GretelLabels,
	)
}
