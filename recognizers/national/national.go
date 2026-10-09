// Package national detects national identity numbers for Southeast Asian
// countries. Each recognizer finds candidates with a regex and keeps only those
// its validator accepts, so a shape match alone is never reported.
//
// Coverage is deliberately narrow. Singapore NRIC/FIN is checked by its check
// letter for the S, T, F and G prefixes; M is not supported because its check
// table is unconfirmed. Malaysia MyKad has no checksum, so only the hyphenated
// printed form is matched and its date of birth must be a real date. Thailand's
// citizen ID is checked by its mod-11 check digit, in both the hyphenated and
// the bare 13-digit form. Passports are not covered yet.
package national

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"dataprotect/recognizers"
)

// sgWeights are the NRIC check-digit weights for the seven digits.
var sgWeights = [7]int{2, 7, 6, 5, 4, 3, 2}

// sgCheckTable maps (sum offset + weighted sum) mod 11 to the check letter for
// each supported prefix. S and T share one table, F and G share another.
var sgCheckTable = map[byte]string{
	'S': "JZIHGFEDCBA",
	'T': "JZIHGFEDCBA",
	'F': "XWUTRQPNMLK",
	'G': "XWUTRQPNMLK",
}

// sgOffset is the value added to the weighted sum before the modulo, by prefix.
var sgOffset = map[byte]int{'S': 0, 'T': 4, 'F': 0, 'G': 4}

var sgShape = regexp.MustCompile(`^[STFG]\d{7}[A-Z]$`)

// SGNRICValidator checks a Singapore NRIC or FIN by its check letter.
type SGNRICValidator struct{}

// NewSGNRICValidator creates a Singapore NRIC/FIN validator.
func NewSGNRICValidator() *SGNRICValidator { return &SGNRICValidator{} }

// Validate reports whether id is a well-formed NRIC/FIN with a correct check letter.
func (v *SGNRICValidator) Validate(id string) bool {
	if !sgShape.MatchString(id) {
		return false
	}
	prefix := id[0]
	sum := sgOffset[prefix]
	for i, w := range sgWeights {
		sum += int(id[1+i]-'0') * w
	}
	return sgCheckTable[prefix][sum%11] == id[8]
}

// SGNRICRecognizer finds Singapore NRIC and FIN numbers in text.
type SGNRICRecognizer struct {
	pattern   *regexp.Regexp
	validator *SGNRICValidator
}

// NewSGNRICRecognizer creates a Singapore NRIC/FIN recognizer.
func NewSGNRICRecognizer() *SGNRICRecognizer {
	return &SGNRICRecognizer{
		pattern:   regexp.MustCompile(`\b[STFG]\d{7}[A-Z]\b`),
		validator: NewSGNRICValidator(),
	}
}

// Name returns the recognizer name.
func (r *SGNRICRecognizer) Name() string { return "national_sg_nric" }

// Recognize detects valid Singapore NRIC and FIN numbers.
func (r *SGNRICRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringIndex(text, -1) {
		id := text[m[0]:m[1]]
		if !r.validator.Validate(id) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntitySGNRIC,
			Start:      m[0],
			End:        m[1],
			Text:       id,
			Confidence: 0.99,
		})
	}
	return entities, nil
}

var myShape = regexp.MustCompile(`^\d{6}-\d{2}-\d{4}$`)

// MYICValidator checks the shape and date of birth of a Malaysia MyKad number.
// MyKad has no checksum, so a valid date is the only structural check.
type MYICValidator struct{}

// NewMYICValidator creates a Malaysia MyKad validator.
func NewMYICValidator() *MYICValidator { return &MYICValidator{} }

// Validate reports whether ic has the YYMMDD-PB-###G shape and a real date of birth.
func (v *MYICValidator) Validate(ic string) bool {
	if !myShape.MatchString(ic) {
		return false
	}
	yy, _ := strconv.Atoi(ic[0:2])
	mm, _ := strconv.Atoi(ic[2:4])
	dd, _ := strconv.Atoi(ic[4:6])
	return mm >= 1 && mm <= 12 && dd >= 1 && dd <= daysIn(mm, yy)
}

// daysIn returns the number of days in month mm. A two-digit year cannot say
// which century it belongs to, so a leap day is accepted for every yy divisible
// by 4. That keeps 29 February valid for 2000 and admits the one century year
// that is not leap (1900), which a two-digit year cannot tell apart from 2000.
func daysIn(mm, yy int) int {
	switch mm {
	case 2:
		if yy%4 == 0 {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	default:
		return 31
	}
}

// MYICRecognizer finds Malaysia MyKad numbers in their printed hyphenated form.
type MYICRecognizer struct {
	pattern   *regexp.Regexp
	validator *MYICValidator
}

// NewMYICRecognizer creates a Malaysia MyKad recognizer.
func NewMYICRecognizer() *MYICRecognizer {
	return &MYICRecognizer{
		pattern:   regexp.MustCompile(`\b\d{6}-\d{2}-\d{4}\b`),
		validator: NewMYICValidator(),
	}
}

// Name returns the recognizer name.
func (r *MYICRecognizer) Name() string { return "national_my_ic" }

// Recognize detects valid Malaysia MyKad numbers.
func (r *MYICRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringIndex(text, -1) {
		ic := text[m[0]:m[1]]
		if !r.validator.Validate(ic) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityMYIC,
			Start:      m[0],
			End:        m[1],
			Text:       ic,
			Confidence: 0.85,
		})
	}
	return entities, nil
}

var thShape = regexp.MustCompile(`^\d(?:-\d{4}-\d{5}-\d{2}-\d|\d{12})$`)

// THIDValidator checks a Thai citizen ID by its mod-11 check digit.
type THIDValidator struct{}

// NewTHIDValidator creates a Thai citizen ID validator.
func NewTHIDValidator() *THIDValidator { return &THIDValidator{} }

// Validate reports whether id is a 13-digit Thai citizen ID, hyphenated or bare,
// with a correct check digit. The check digit is (11 - sum mod 11) mod 10,
// where sum weights the first 12 digits 13 down to 2.
func (v *THIDValidator) Validate(id string) bool {
	if !thShape.MatchString(id) {
		return false
	}
	digits := strings.ReplaceAll(id, "-", "")
	sum := 0
	for i := 0; i < 12; i++ {
		sum += int(digits[i]-'0') * (13 - i)
	}
	return (11-sum%11)%10 == int(digits[12]-'0')
}

// THIDRecognizer finds Thai citizen ID numbers in text.
type THIDRecognizer struct {
	pattern   *regexp.Regexp
	validator *THIDValidator
}

// NewTHIDRecognizer creates a Thai citizen ID recognizer.
func NewTHIDRecognizer() *THIDRecognizer {
	return &THIDRecognizer{
		pattern:   regexp.MustCompile(`\b\d(?:-\d{4}-\d{5}-\d{2}-\d|\d{12})\b`),
		validator: NewTHIDValidator(),
	}
}

// Name returns the recognizer name.
func (r *THIDRecognizer) Name() string { return "national_th_id" }

// Recognize detects valid Thai citizen ID numbers.
func (r *THIDRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringIndex(text, -1) {
		id := text[m[0]:m[1]]
		if !r.validator.Validate(id) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityTHID,
			Start:      m[0],
			End:        m[1],
			Text:       id,
			Confidence: 0.95,
		})
	}
	return entities, nil
}
