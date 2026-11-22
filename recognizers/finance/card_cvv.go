package finance

import (
	"context"
	"regexp"

	"github.com/u007/dataprotect/recognizers"
)

// cvvPattern matches a three- or four-digit value after a card-security label.
// The \b boundaries mean no digit may sit next to the value, so a run of five
// or more digits is never cut to four.
//
// Length: three digits for Visa, Mastercard, Discover, JCB and Diners, four
// for American Express. Source: https://en.wikipedia.org/wiki/Card_security_code
// (fetched 2026-11-22).
//
// Corpus evidence: every labelled value is three digits (gretel
// credit_card_security_code 55 of 55; nemotron cvv 88 of 88). No corpus value
// is four digits, so the four-digit form rests on the Wikipedia source alone.
// Labels seen in the corpora: "CVV", "CVV code", "security code", "Credit Card
// Security Code" (gretel 24 with "security code"; nemotron 81 with "cvv").
var cvvPattern = labelGate(`(?i:cvv2?|cvc2?|csc|cav|cid|security\s+code|card\s+verification(?:\s+(?:value|code|number))?)`, `\d{3,4}`)

// CardCVVRecognizer finds card security codes that follow a CVV-style label.
type CardCVVRecognizer struct {
	pattern *regexp.Regexp
}

// NewCardCVVRecognizer creates a card security code (CVV) recognizer.
func NewCardCVVRecognizer() *CardCVVRecognizer {
	return &CardCVVRecognizer{pattern: cvvPattern}
}

// Name returns the recognizer name.
func (r *CardCVVRecognizer) Name() string { return "finance_card_cvv" }

// Recognize detects three- or four-digit card security codes after a label.
// Only the digits are reported.
func (r *CardCVVRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringSubmatchIndex(text, -1) {
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityCardCVV,
			Start:      m[2],
			End:        m[3],
			Text:       text[m[2]:m[3]],
			Confidence: keywordScore,
		})
	}
	return entities, nil
}
