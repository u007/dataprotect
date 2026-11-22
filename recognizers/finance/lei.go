package finance

import (
	"context"
	"regexp"

	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/checksum"
)

// LEI is a Legal Entity Identifier, ISO 17442. Source:
// https://en.wikipedia.org/wiki/Legal_Entity_Identifier (fetched 2026-11-22).
//
// The source states: a 20-character alphanumeric string; the first four
// characters identify the issuing LOU; characters 5 to 18 are the unique
// alphanumeric string assigned to the organization; the last two are check
// digits calculated with MOD-97-10 as per ISO/IEC 7064. Its example is
// 54930084UKLVMY22DS16. The shape is therefore 18 uppercase letters or digits
// followed by two digits, and the check is ISO 7064 MOD 97-10 over the whole
// string with no rearrangement.
//
// No keyword is required, because the check digits carry the evidence, as for
// IN_GSTIN. The check passes about one random candidate in 97, so a 20-digit
// run with no label is a known false-positive risk; see TODO.md and the corpus
// counts in SPEC.md.
var leiPattern = regexp.MustCompile(`\b[A-Z0-9]{18}[0-9]{2}\b`)

// leiScore matches IN_GSTIN, the other checksum-only national identifier.
const leiScore = 0.90

// LEIRecognizer finds Legal Entity Identifiers whose MOD 97-10 check passes.
type LEIRecognizer struct {
	pattern *regexp.Regexp
}

// NewLEIRecognizer creates a LEI recognizer.
func NewLEIRecognizer() *LEIRecognizer {
	return &LEIRecognizer{pattern: leiPattern}
}

// Name returns the recognizer name.
func (r *LEIRecognizer) Name() string { return "finance_lei" }

// Recognize detects LEIs whose check digits verify. Only the value is reported.
func (r *LEIRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringIndex(text, -1) {
		lei := text[m[0]:m[1]]
		if !checksum.Mod97Valid(lei) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityLEI,
			Start:      m[0],
			End:        m[1],
			Text:       lei,
			Confidence: leiScore,
		})
	}
	return entities, nil
}
