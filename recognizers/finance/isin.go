package finance

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/u007/dataprotect/internal/keyword"
	"github.com/u007/dataprotect/recognizers"
)

// ISIN is an International Securities Identification Number, ISO 6166. Source:
// https://en.wikipedia.org/wiki/International_Securities_Identification_Number
// (fetched 2026-11-22).
//
// The source states: always 12 characters; two letters (the ISO 3166-1 alpha-2
// issuing country), nine alphanumeric characters (the NSIN), and one numeric
// check digit. The check digit is computed by converting each letter to two
// digits by taking its ASCII code and subtracting 55 (A=10 ... Z=35), then
// running the Luhn check over the resulting digit string. The source's
// examples are US0378331005, AU0000XVGZA3 and GB0002634946.
//
// A 12-character uppercase token with a Luhn check passes about one time in
// ten, so the shape alone is not enough. The value is reported only after the
// keyword ISIN, and only when the Luhn check also passes. Scores 0.85.
var isinValue = `[A-Z]{2}[A-Z0-9]{9}[0-9]`

var isinPattern = keyword.LabelGate(`(?i:ISIN)`, isinValue)

// ISINRecognizer finds ISINs that follow an ISIN label and pass the Luhn check.
type ISINRecognizer struct {
	pattern *regexp.Regexp
}

// NewISINRecognizer creates an ISIN recognizer.
func NewISINRecognizer() *ISINRecognizer {
	return &ISINRecognizer{pattern: isinPattern}
}

// Name returns the recognizer name.
func (r *ISINRecognizer) Name() string { return "finance_isin" }

// Recognize detects ISINs that follow a label. Only the value is reported.
func (r *ISINRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringSubmatchIndex(text, -1) {
		value := text[m[2]:m[3]]
		if !ValidISIN(value) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityISIN,
			Start:      m[2],
			End:        m[3],
			Text:       value,
			Confidence: keywordScore,
		})
	}
	return entities, nil
}

// ValidISIN reports whether s is a 12-character ISIN whose check digit is
// correct. The shape is checked first, then the Luhn check runs over the digit
// string made by expanding each letter to its two-digit value (A=10 ... Z=35).
func ValidISIN(s string) bool {
	if !isinShape.MatchString(s) {
		return false
	}
	var digits strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			digits.WriteString(strconv.Itoa(int(c-'A') + 10))
		} else {
			digits.WriteByte(c)
		}
	}
	return luhnDigits(digits.String())
}

var isinShape = regexp.MustCompile(`^[A-Z]{2}[A-Z0-9]{9}[0-9]$`)

// luhnDigits reports whether a string of decimal digits passes the Luhn check:
// from the rightmost digit, every second digit is doubled (with 9 subtracted
// from doubles above 9), and the sum must be a multiple of 10.
func luhnDigits(d string) bool {
	sum := 0
	double := false
	for i := len(d) - 1; i >= 0; i-- {
		v := int(d[i] - '0')
		if double {
			v *= 2
			if v > 9 {
				v -= 9
			}
		}
		sum += v
		double = !double
	}
	return sum%10 == 0
}
