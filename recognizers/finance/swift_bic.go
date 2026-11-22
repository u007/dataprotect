package finance

import (
	"context"
	"regexp"

	"github.com/u007/dataprotect/recognizers"
)

// swiftBICPattern is the ISO 9362 shape: a four-letter institution code, a
// two-letter country code, a two-character location code (letters or digits),
// and an optional three-character branch code (letters or digits, "XXX" for
// the primary office). The value is 8 or 11 characters, uppercase only.
//
// The country code is accepted as any two uppercase letters, not checked
// against a country list. The IBAN country table in checksum/iban_length.go
// lists IBAN countries only, so it would reject a BIC from the United States,
// Singapore or Malaysia. The corpora label BICs whose country is US (84
// nemotron, 29 gretel), so that table is not used here.
//
// Source: https://en.wikipedia.org/wiki/ISO_9362 (fetched 2026-11-22).
//
// Corpus evidence (testdata/external, unlabelled spans before this rule was
// mapped): gretel swift_bic_code values are all 11 characters (76 of 76 match
// this shape); nemotron swift_bic values are 8 to 12 characters, and 74 of 96
// match this shape (the other 22 are 9, 10 or 12 characters).
var swiftBICValue = `[A-Z]{4}[A-Z]{2}[A-Z0-9]{2}(?:[A-Z0-9]{3})?`

var swiftBICPattern = labelGate(`(?i:swift(?:\s?/\s?bic)?|bic)(?:\s+(?i:code))?`, swiftBICValue)

// SWIFTBICRecognizer finds SWIFT/BIC codes that follow a SWIFT or BIC label.
type SWIFTBICRecognizer struct {
	pattern *regexp.Regexp
}

// NewSWIFTBICRecognizer creates a SWIFT/BIC recognizer.
func NewSWIFTBICRecognizer() *SWIFTBICRecognizer {
	return &SWIFTBICRecognizer{pattern: swiftBICPattern}
}

// Name returns the recognizer name.
func (r *SWIFTBICRecognizer) Name() string { return "finance_swift_bic" }

// Recognize detects SWIFT/BIC codes that follow a label. Only the code is reported.
func (r *SWIFTBICRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringSubmatchIndex(text, -1) {
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntitySWIFTBIC,
			Start:      m[2],
			End:        m[3],
			Text:       text[m[2]:m[3]],
			Confidence: keywordScore,
		})
	}
	return entities, nil
}
