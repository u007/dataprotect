package national

import (
	"github.com/u007/dataprotect/internal/keyword"
	"github.com/u007/dataprotect/recognizers"
)

// IN_PAN: Indian Permanent Account Number. Source:
// https://en.wikipedia.org/wiki/Permanent_account_number (fetched 2026-11-22).
//
// The source states: a ten-character alpha-numeric unique identifier; characters
// 1 to 5 are letters, 6 to 9 are numerals, and 10 is a letter; the fourth
// character is the holder type, one of A, B, C, F, G, H, L, J, P or T. Its
// example is AAAPZ1234C. No check formula is published, so the shape alone is
// used, which is why the rule is keyword-gated.
//
// The overlap with IN_GSTIN is decided in analyzer.winsOverlap: a GSTIN carries
// a PAN at its characters 3 to 12, and IN_GSTIN wins that span. The word
// boundary in the label gate already keeps a PAN from matching inside a GSTIN.
var inPANValue = `[A-Z]{3}[ABCFGHLJPT][A-Z][0-9]{4}[A-Z]`

var inPANPattern = keyword.LabelGate(`(?i:permanent\s+account\s+number|PAN)`, inPANValue)

// NewINPANRecognizer creates an Indian PAN recognizer.
func NewINPANRecognizer() recognizers.Recognizer {
	return &keywordRule{name: "national_in_pan", typ: recognizers.EntityINPAN, pattern: inPANPattern}
}
