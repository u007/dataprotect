package national

import (
	"github.com/u007/dataprotect/internal/keyword"
	"github.com/u007/dataprotect/recognizers"
)

// SG_UEN: Singapore Unique Entity Number. Source: OECD, "Singapore - Information
// on Tax Identification Numbers", Section II,
// https://www.oecd.org/tax/automatic-exchange/crs-implementation-and-assistance/tax-identification-numbers/Singapore-TIN.pdf
// (fetched 2026-11-22).
//
// The source gives three forms. UEN Business (ROB) is NNNNNNNNC, nine
// characters. UEN Local Company (ROC) is YYYYNNNNNC, ten characters, where YYYY
// is a year. UEN Others is SYYPQNNNNC or TYYPQNNNNC, ten characters, where PQ
// is two uppercase letters for the entity type. C is a check alphabet and N is
// a digit.
//
// Two narrowings are applied, both chosen as the narrower reading. The year is
// 19xx or 20xx, not any four digits. C is any uppercase letter, as the source
// says "check alphabet" but does not list the letters. The check-letter
// algorithm is not published, so the shape alone is checked, and the rule is
// keyword-gated. Foreign-company (F and FDDD) forms and the ASGD and ITR forms
// are not covered.
//
// Shapes do not collide with SG_NRIC, which starts with a letter S, T, F or G
// and has seven digits before its check letter. The "Others" form starts with a
// letter but has two digits, then two letters, then four digits, so it is not
// an NRIC either.
var sgUENValue = `\d{8}[A-Z]|(?:19|20)\d{2}\d{5}[A-Z]|[ST]\d{2}[A-Z]{2}\d{4}[A-Z]`

var sgUENPattern = keyword.LabelGate(
	`(?i:UEN|unique\s+entity\s+number|business\s+registration\s+no|company\s+registration\s+no|co\.\s*reg\.\s*no)`,
	sgUENValue,
)

// NewSGUENRecognizer creates a Singapore UEN recognizer.
func NewSGUENRecognizer() recognizers.Recognizer {
	return &keywordRule{name: "national_sg_uen", typ: recognizers.EntitySGUEN, pattern: sgUENPattern}
}
