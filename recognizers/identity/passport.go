package identity

import (
	"github.com/u007/dataprotect/internal/keyword"
	"github.com/u007/dataprotect/recognizers"
)

// passportValue is the generic passport number shape seen in labelled corpus
// values: nine digits, or one letter and eight digits.
//
// Corpus evidence (gretel-pii-finance passport_number, 45 values, unlabelled
// before this rule was mapped): 23 are nine digits, 21 are one letter and eight
// digits, and one is a label glued to digits (not matched). A window survey finds
// the word "passport" before 30 of the 45; the rule reports 24. Labels without
// the word, such as "access code", "passcode" and "policy number", are missed.
//
// The keyword is required: a bare nine-digit or letter-and-eight-digit run is
// not reported. MY_PASSPORT and SG_PASSPORT are more specific shapes with their
// own rules; on overlap they win over this generic rule (analyzer.winsOverlap).
const passportValue = `(?:[A-Z]\d{8}|\d{9})`

// passportKeyword is "passport", optionally followed by "number" or "no".
const passportKeyword = `(?i:passport(?:\s+(?:number|no\.?))?)`

var passportPattern = keyword.LabelGate(passportKeyword, passportValue)

// NewPassportRecognizer creates a generic passport number recognizer.
func NewPassportRecognizer() recognizers.Recognizer {
	return &labelRecognizer{name: "identity_passport", typ: recognizers.EntityPassport, pattern: passportPattern}
}

// regexp is used by the value constants in the sibling files of this package.
