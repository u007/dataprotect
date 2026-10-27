package national

import (
	"context"
	"regexp"

	"github.com/u007/dataprotect/recognizers"
)

// ingstinShape matches a GSTIN shape: two-digit state code, ten-character PAN,
// one entity code (never 0), a fixed Z, and one check digit.
var ingstinShape = regexp.MustCompile(`^\d{2}[A-Z]{5}\d{4}[A-Z][1-9A-Z]Z[0-9A-Z]$`)

// ingstinValues maps each base-36 character to its numeric value.
var ingstinValues = [256]int{}

func init() {
	for i := 0; i < 10; i++ {
		ingstinValues[byte('0'+i)] = i
	}
	for i := 0; i < 26; i++ {
		ingstinValues[byte('A'+i)] = 10 + i
	}
}

// ingstinCharset is the ordered 36-character alphabet for the mod-36 check digit.
var ingstinCharset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// ingstinCheckDigit computes the GSTIN mod-36 check digit over the first
// fourteen characters. Each character's value (its index in the 36-character
// charset) is multiplied by an alternating weight of 1 or 2 (starting at 1),
// each product is folded back into a single base-36 value via
// floor(product/36) + product%36, and the folded values are summed. The check
// digit is the charset character at index (36 - sum%36) % 36.
//
// This is the modulus-36 Luhn variant published by GSTN for GSTIN validation.
func ingstinCheckDigit(first14 string) byte {
	sum := 0
	for i := 0; i < 14; i++ {
		val := ingstinValues[first14[i]]
		weight := 1
		if i%2 == 1 {
			weight = 2
		}
		product := val * weight
		sum += product/36 + product%36
	}
	return ingstinCharset[(36-sum%36)%36]
}

// INGSTINValidator checks a GSTIN by its mod-36 check digit.
type INGSTINValidator struct{}

// NewINGSTINValidator creates a GSTIN validator.
func NewINGSTINValidator() *INGSTINValidator { return &INGSTINValidator{} }

// Validate reports whether gstin is a well-formed GSTIN with a correct
// mod-36 check digit.
//
// The shape is checked first: two-digit state code, ten-character PAN,
// entity code 1-9A-Z (never 0), fixed Z, and one check digit. The checksum
// is a modulus-36 Luhn variant over the first fourteen characters.
// Sources: gstinapi.com ("published by GSTN"), multiple independent
// implementations. The algorithm is widely restated but no primary GSTN
// document was found online; see TODO.md for the open follow-up.
func (v *INGSTINValidator) Validate(gstin string) bool {
	if !ingstinShape.MatchString(gstin) {
		return false
	}
	return ingstinCheckDigit(gstin[:14]) == gstin[14]
}

// INGSTINRecognizer finds Indian GSTIN numbers in text.
type INGSTINRecognizer struct {
	pattern   *regexp.Regexp
	validator *INGSTINValidator
}

// NewINGSTINRecognizer creates a GSTIN recognizer.
func NewINGSTINRecognizer() *INGSTINRecognizer {
	return &INGSTINRecognizer{
		pattern:   regexp.MustCompile(`\b` + ingstinShape.String()[1:len(ingstinShape.String())-1] + `\b`),
		validator: NewINGSTINValidator(),
	}
}

// Name returns the recognizer name.
func (r *INGSTINRecognizer) Name() string { return "national_in_gstin" }

// Recognize detects valid GSTIN numbers in text.
func (r *INGSTINRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringIndex(text, -1) {
		gstin := text[m[0]:m[1]]
		if !r.validator.Validate(gstin) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityINGSTIN,
			Start:      m[0],
			End:        m[1],
			Text:       gstin,
			Confidence: 0.90,
		})
	}
	return entities, nil
}
