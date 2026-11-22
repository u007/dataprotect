package national

import (
	"github.com/u007/dataprotect/internal/keyword"
	"github.com/u007/dataprotect/recognizers"
)

// MY_TIN: Malaysia tax identification number. Source: IRBM / OECD, "Malaysia -
// Information on Tax Identification Numbers", updated March 2023,
// https://www.hasil.gov.my/wp-content/uploads/malaysia-tin.pdf, Section II
// (fetched 2026-11-22).
//
// The source gives individual TINs with the prefix IG followed by 9 to 11 digits
// (examples IG115002000, IG4040080091, IG56003500070). Its earlier prefixes SG
// and OG were "converted to IG" with the numeric part unchanged. Only IG is
// accepted here, the narrower reading. Non-individual TINs use the codes C, CS,
// D, E, F, FA, PT, TA, TC, TN, TR, TP, J and LE, each followed by digits (the
// examples are C20880050010, D4800990020, E91005500060 and F10234567090). The
// source's earlier examples had one digit fewer, because a 0 was appended from
// 2 January 2023; the 9-to-11-digit range covers both.
//
// A single optional space may sit between the code and the digits. The rule is
// keyword-gated. TAX_ID also takes the label "TIN", so TAX_ID yields to MY_TIN on
// an exact overlap (analyzer.winsOverlap).
var myTINValue = `(?:IG|CS|FA|PT|TA|TC|TN|TR|TP|LE|C|D|E|F|J) ?\d{9,11}`

var myTINPattern = keyword.LabelGate(
	`(?i:tax\s+identification\s+number|nombor\s+pengenalan\s+cukai|no\.?\s*cukai\s+pendapatan|no\.?\s*cukai|income\s+tax\s+no|TIN)`,
	myTINValue,
)

// NewMYTINRecognizer creates a Malaysia TIN recognizer.
func NewMYTINRecognizer() recognizers.Recognizer {
	return &keywordRule{name: "national_my_tin", typ: recognizers.EntityMYTIN, pattern: myTINPattern}
}
