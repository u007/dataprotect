package finance

import (
	"context"
	"regexp"

	"github.com/u007/dataprotect/recognizers"
)

// The ABA routing transit number is nine digits. Two checks apply, and both are
// required:
//
//   - the checksum (3(d1 + d4 + d7) + 7(d2 + d5 + d8) + (d3 + d6 + d9)) mod 10 = 0
//   - the first two digits lie in 00-12, 21-32, 61-72 or 80
//
// Source: https://en.wikipedia.org/wiki/ABA_routing_transit_number (fetched
// 2026-11-22).
//
// Corpus evidence: the labelled routing numbers in the corpora are mostly not
// valid ABA numbers. Gretel bank_routing_number: 1 of 56 passes both checks
// (6 pass the checksum alone). Nemotron bank_routing_number: 24 of 182 pass
// both checks (24 pass the checksum alone, so the range rule removed none of
// them). The corpus values are synthetic, so recall on them is low by design;
// the rule is not loosened to match them.
var routingPattern = labelGate(`(?i:routing\s+(?:transit\s+)?(?:number|no))|RTN|ABA`, `\d{9}`)

// USRoutingValidator checks a nine-digit ABA routing number.
type USRoutingValidator struct{}

// NewUSRoutingValidator creates an ABA routing number validator.
func NewUSRoutingValidator() *USRoutingValidator { return &USRoutingValidator{} }

// Validate reports whether s is nine digits with a valid ABA checksum and a
// first-two-digit prefix in an assigned range.
func (v *USRoutingValidator) Validate(s string) bool {
	if len(s) != 9 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	d := func(i int) int { return int(s[i] - '0') }
	sum := 3*(d(0)+d(3)+d(6)) + 7*(d(1)+d(4)+d(7)) + (d(2) + d(5) + d(8))
	if sum%10 != 0 {
		return false
	}
	prefix := d(0)*10 + d(1)
	return prefix <= 12 || (prefix >= 21 && prefix <= 32) || (prefix >= 61 && prefix <= 72) || prefix == 80
}

// USRoutingRecognizer finds ABA routing numbers that follow a routing label.
type USRoutingRecognizer struct {
	pattern   *regexp.Regexp
	validator *USRoutingValidator
}

// NewUSRoutingRecognizer creates a US routing number recognizer.
func NewUSRoutingRecognizer() *USRoutingRecognizer {
	return &USRoutingRecognizer{pattern: routingPattern, validator: NewUSRoutingValidator()}
}

// Name returns the recognizer name.
func (r *USRoutingRecognizer) Name() string { return "finance_us_routing" }

// Recognize detects valid ABA routing numbers that follow a label. Only the
// nine digits are reported.
func (r *USRoutingRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringSubmatchIndex(text, -1) {
		value := text[m[2]:m[3]]
		if !r.validator.Validate(value) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityUSRouting,
			Start:      m[2],
			End:        m[3],
			Text:       value,
			Confidence: keywordScore,
		})
	}
	return entities, nil
}
