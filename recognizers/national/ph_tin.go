package national

import (
	"context"
	"regexp"

	"github.com/u007/dataprotect/recognizers"
)

// phTinShape matches a Philippine TIN: nine digits in three groups of three,
// optionally followed by a three-digit branch code (000 = head office).
// Segment 1 (first digit): 0 = Corporation, 1-9 = Individual taxpayer category.
// Segment 3 (last core digit): checksum digit per NIRC §236; the exact checksum
// formula (Annex A of RMO 23-91) was not found online, so the digit is
// accepted as-is without formula verification. See TODO.md for the open
// follow-up on the official checksum spec.
//
// Recognize rejects a match that ends right before a '-', since that means the
// 9-digit core was matched inside a longer run such as "123-456-789-ABC"
// where "-ABC" is not a valid branch code.
var phTinShape = regexp.MustCompile(`^\d{3}-\d{3}-\d{3}(?:-\d{3})?$`)

// PHTINValidator checks the shape and classification digit of a Philippine TIN.
type PHTINValidator struct{}

// NewPHTINValidator creates a TIN validator.
func NewPHTINValidator() *PHTINValidator { return &PHTINValidator{} }

// Validate reports whether tin is a well-formed Philippine TIN: nine digits in
// three groups of three, an optional three-digit branch code, and a valid
// classification digit (0 = corporation, 1-9 = individual). The core TIN is
// always nine digits; branch codes extend it to twelve digits.
//
// The ninth digit is a checksum per NIRC §236, but the checksum formula
// (Annex A of RMO 23-91) was not found online, so it is not verified here.
func (v *PHTINValidator) Validate(tin string) bool {
	if !phTinShape.MatchString(tin) {
		return false
	}
	// The first digit is the classification code. 0 means corporation;
	// 1-9 are individual taxpayer categories. Any value 0-9 is accepted.
	classification := tin[0] - '0'
	return classification <= 9
}

// PHTINRecognizer finds Philippine TIN numbers in text.
type PHTINRecognizer struct {
	pattern   *regexp.Regexp
	validator *PHTINValidator
}

// NewPHTINRecognizer creates a TIN recognizer.
func NewPHTINRecognizer() *PHTINRecognizer {
	return &PHTINRecognizer{
		pattern:   regexp.MustCompile(`\b` + phTinShape.String()[1:len(phTinShape.String())-1] + `\b`),
		validator: NewPHTINValidator(),
	}
}

// Name returns the recognizer name.
func (r *PHTINRecognizer) Name() string { return "national_ph_tin" }

// Recognize detects valid Philippine TIN numbers in text.
//
// A match ending right before a '-' is rejected: that means the 9-digit core
// was found inside a longer run such as "123-456-789-ABC" where "-ABC" is not
// a valid 3-digit branch code, so the core alone is not a complete TIN.
func (r *PHTINRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringIndex(text, -1) {
		if m[1] < len(text) && text[m[1]] == '-' {
			continue
		}
		tin := text[m[0]:m[1]]
		if !r.validator.Validate(tin) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityPHTIN,
			Start:      m[0],
			End:        m[1],
			Text:       tin,
			Confidence: 0.80,
		})
	}
	return entities, nil
}
