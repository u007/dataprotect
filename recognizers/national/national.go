// Package national detects national identity numbers for Southeast Asian
// countries. Each recognizer finds candidates with a regex and keeps only those
// its validator accepts, so a shape match alone is never reported.
//
// The country tax and registration numbers that have no checksum (IN_PAN,
// SG_UEN, MY_SSM and MY_TIN) are keyword-gated like the rules in
// recognizers/finance and recognizers/identity: the value is reported only after
// a label for it, the span is the value alone, and every one scores 0.85. They
// are in this package because each is a country's own number, not a card or
// banking identifier.
//
// Coverage is deliberately narrow. Singapore NRIC/FIN is checked by its check
// letter for the S, T, F and G prefixes; M is not supported because its check
// table is unconfirmed. Malaysia MyKad has no checksum, so only the hyphenated
// printed form is matched and its date of birth must be a real date. Thailand's
// citizen ID is checked by its mod-11 check digit, in both the hyphenated and
// the bare 13-digit form. Malaysia passports are matched only next to a passport
// keyword, because one letter and eight digits is too short to report on shape
// alone. Singapore and Thailand passports are not covered yet.
package national

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/u007/dataprotect/recognizers"
)

// sgWeights are the NRIC check-digit weights for the seven digits.
var sgWeights = [7]int{2, 7, 6, 5, 4, 3, 2}

// sgCheckTable maps (sum offset + weighted sum) mod 11 to the check letter for
// each supported prefix. S and T share one table, F and G share another.
// The M prefix (FIN series launched 1 January 2022) is NOT supported: its
// check-letter table has not been confirmed against an official ICA/PRADO spec.
// See TODO.md for the open follow-up.
var sgCheckTable = map[byte]string{
	'S': "JZIHGFEDCBA",
	'T': "JZIHGFEDCBA",
	'F': "XWUTRQPNMLK",
	'G': "XWUTRQPNMLK",
}

// sgOffset is the value added to the weighted sum before the modulo, by prefix.
var sgOffset = map[byte]int{'S': 0, 'T': 4, 'F': 0, 'G': 4}

var sgShape = regexp.MustCompile(`^[STFG]\d{7}[A-Z]$`)

// weakConfidence is the score for bare-digit MyKad candidates that have no
// identity-card keyword. It is deliberately below the default 0.70 floor so
// the optional LLM second pass is the only path that can promote them.
const weakConfidence = 0.50

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
//
// A number needs a word boundary before it, unless the label NRIC or FIN comes
// first. So "NRICS1234567D" is found, but an NRIC-shaped run inside a longer
// token is not. Without that rule a 0.99 NRIC would win the overlap against a
// shape-only 0.95 entity such as a bitcoin address and leave the rest of it
// visible. Only the number is captured.
func NewSGNRICRecognizer() *SGNRICRecognizer {
	return &SGNRICRecognizer{
		pattern:   regexp.MustCompile(`\b(?:(?i:nric|fin)\s?)?([STFG]\d{7}[A-Z])\b`),
		validator: NewSGNRICValidator(),
	}
}

// Name returns the recognizer name.
func (r *SGNRICRecognizer) Name() string { return "national_sg_nric" }

// Recognize detects valid Singapore NRIC and FIN numbers.
func (r *SGNRICRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringSubmatchIndex(text, -1) {
		id := text[m[2]:m[3]]
		if !r.validator.Validate(id) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntitySGNRIC,
			Start:      m[2],
			End:        m[3],
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

// MYICRecognizer finds Malaysia MyKad numbers in two forms. The printed
// hyphenated form is matched on its own. Twelve bare digits are matched only
// after an identity-card keyword, because a bare 12-digit run has no shape that
// separates it from any other number. Bare digits without a keyword are emitted
// as weak candidates (confidence below the default 0.70 floor) so the optional
// LLM second pass can confirm them; with the pass disabled they are dropped.
type MYICRecognizer struct {
	pattern   *regexp.Regexp
	keyed     *regexp.Regexp
	bare      *regexp.Regexp
	validator *MYICValidator
}

// NewMYICRecognizer creates a Malaysia MyKad recognizer.
//
// The keyed pattern takes Purview's identity-card keywords (MyKad, IC, KP,
// NRIC and so on), one bounded separator, then up to two filler words (is,
// number, no, as in "my ic number is ..."), then twelve digits. The two filler
// words must differ: "no no" is not a filler run, so it does not match. RE2
// has no backreferences, so the distinct pairs are spelled out. Only the digits
// are captured. The boundary before the keyword stops "ic" matching inside a
// word, and the mandatory separator keeps the keyword to a whole token.
//
// Caveat: keywords such as "ic no", "i/c no", "kp no" and "k/p no" already
// contain "no", so "ic no no <digits>" still matches as keyword plus one
// filler (the distinctness rule only applies to the two filler slots, not to
// a "no" that is part of the keyword). Per the 2026-11-06 decision, such
// ambiguity is not special-cased in the regex; it is left to the optional LLM
// second pass.
func NewMYICRecognizer() *MYICRecognizer {
	return &MYICRecognizer{
		pattern:   regexp.MustCompile(`\b\d{6}-\d{2}-\d{4}\b`),
		keyed:     regexp.MustCompile(`(?i)\b(?:digital application card|i/c no|i/c|ic no|ic|id card|identification card|identity card|k/p no|k/p|kad akuan diri|kad aplikasi digital|kad pengenalan malaysia|kp no|kp|mykad|mykas|mykid|mypr|mytentera|malaysia identity card|malaysian identity card|nric|personal identification card)[^A-Za-z0-9]{1,8}(?:(?:is|number|no)[^A-Za-z0-9]{1,8}|is[^A-Za-z0-9]{1,8}(?:number|no)[^A-Za-z0-9]{1,8}|number[^A-Za-z0-9]{1,8}(?:is|no)[^A-Za-z0-9]{1,8}|no[^A-Za-z0-9]{1,8}(?:is|number)[^A-Za-z0-9]{1,8})?(\d{12})\b`),
		bare:      regexp.MustCompile(`\b\d{12}\b`),
		validator: NewMYICValidator(),
	}
}

// Name returns the recognizer name.
func (r *MYICRecognizer) Name() string { return "national_my_ic" }

// Recognize detects valid Malaysia MyKad numbers, hyphenated or keyed bare.
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
	for _, m := range r.keyed.FindAllStringSubmatchIndex(text, -1) {
		digits := text[m[2]:m[3]]
		// Normalise to the printed form so the same date check applies.
		ic := digits[0:6] + "-" + digits[6:8] + "-" + digits[8:12]
		if !r.validator.Validate(ic) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityMYIC,
			Start:      m[2],
			End:        m[3],
			Text:       digits,
			Confidence: 0.85,
		})
	}

	// Bare 12-digit runs without an identity-card keyword are weak candidates.
	// They are emitted below the default floor so the optional LLM second pass
	// can confirm them; with the pass disabled they are dropped by the filter.
	// Skip spans already claimed by the keyed path to avoid duplicates.
	claimed := keyedSpans(text, r.keyed)
	for _, m := range r.bare.FindAllStringIndex(text, -1) {
		if overlapsAny(m[0], m[1], claimed) {
			continue
		}
		// Belt and suspenders beyond \b: a match must not sit inside a longer
		// digit run. RE2 has no lookaround, so check the neighbours directly.
		if (m[0] > 0 && isDigit(text[m[0]-1])) || (m[1] < len(text) && isDigit(text[m[1]])) {
			continue
		}
		digits := text[m[0]:m[1]]
		ic := digits[0:6] + "-" + digits[6:8] + "-" + digits[8:12]
		if !r.validator.Validate(ic) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityMYIC,
			Start:      m[0],
			End:        m[1],
			Text:       digits,
			Confidence: weakConfidence,
			Weak:       true,
		})
	}
	return entities, nil
}

// keyedSpans returns the [start,end) spans of all matches of the keyed pattern,
// so the bare path can skip overlaps.
func keyedSpans(text string, re *regexp.Regexp) [][2]int {
	var spans [][2]int
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		// Group 1 (m[2],m[3]) is the captured digits; the full match is m[0],m[1].
		spans = append(spans, [2]int{m[0], m[1]})
	}
	return spans
}

// overlapsAny reports whether [s,e) overlaps any existing span.
func overlapsAny(s, e int, spans [][2]int) bool {
	for _, sp := range spans {
		if s < sp[1] && e > sp[0] {
			return true
		}
	}
	return false
}

// isDigit reports whether b is an ASCII digit.
func isDigit(b byte) bool { return b >= '0' && b <= '9' }

var thShape = regexp.MustCompile(`^\d(?:-\d{4}-\d{5}-\d{2}-\d|\d{12})$`)

// THIDValidator checks a Thai citizen ID by its mod-11 check digit.
type THIDValidator struct{}

// NewTHIDValidator creates a Thai citizen ID validator.
func NewTHIDValidator() *THIDValidator { return &THIDValidator{} }

// Validate reports whether id is a 13-digit Thai citizen ID, hyphenated or bare,
// with a correct check digit. The check digit is (11 - sum mod 11) mod 10,
// where sum weights the first 12 digits 13 down to 2. A leading zero is not a
// valid Thai citizen ID (the first digit is always 1-9), so reject those even
// when mod-11 passes.
func (v *THIDValidator) Validate(id string) bool {
	if !thShape.MatchString(id) {
		return false
	}
	digits := strings.ReplaceAll(id, "-", "")
	if digits[0] == '0' {
		return false
	}
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

// passportLead is the keyword gate shared by the passport recognizers: "passport"
// or "pasport", optional label words from Purview's list, then a bounded run of
// separators. Each recognizer appends its own number shape and one capture group.
const passportLead = `(?i)pas{1,2}port(?:[^A-Za-z0-9]{0,8}(?:numbers?|nombor|no))*[^A-Za-z0-9]{1,8}`

// SGPassportRecognizer finds Singapore passport numbers that sit next to a
// passport keyword. The same keyword gate as MYPassportRecognizer applies, and
// the number has no checksum to fall back on.
//
// The shape is E or K, seven digits, then one letter. Purview's definition
// gives this shape, and the older ICA BioPass description matches the E form.
// The K prefix and the seven-digit count are not confirmed by a primary source:
// PRADO (the EU register) is unreachable, so this is recorded as unverified.
type SGPassportRecognizer struct {
	pattern *regexp.Regexp
}

// NewSGPassportRecognizer creates a Singapore passport recognizer.
func NewSGPassportRecognizer() *SGPassportRecognizer {
	return &SGPassportRecognizer{
		pattern: regexp.MustCompile(passportLead + `([EK]\d{7}[A-Z])\b`),
	}
}

// Name returns the recognizer name.
func (r *SGPassportRecognizer) Name() string { return "national_sg_passport" }

// Recognize detects Singapore passport numbers that follow a passport keyword.
// The confidence matches the Malaysia keyword-backed level.
func (r *SGPassportRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringSubmatchIndex(text, -1) {
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntitySGPassport,
			Start:      m[2],
			End:        m[3],
			Text:       text[m[2]:m[3]],
			Confidence: 0.85,
		})
	}
	return entities, nil
}

// MYPassportRecognizer finds Malaysia passport numbers that sit next to a
// passport keyword. The keyword is the only gate: the number has no checksum and
// is too short for its shape to be reported alone.
//
// The pattern takes "passport" or "pasport" (no leading word boundary, so
// "ePassport" matches), then optional label words from Purview's list (number,
// no, nombor), then a bounded run of 1 to 8 non-alphanumeric characters, then
// the number. Any punctuation or space counts as a separator, including dashes,
// en dashes, non-breaking spaces, fullwidth colons, pipes, quotes, equals signs
// and angle brackets. The run must be non-empty, so a keyword glued to a letter
// ("passportA12345678") does not match, and the bounded run keeps a long
// punctuation run from reaching across a sentence. Only the number is captured,
// so a detection spans the number and not the keyword.
//
// The type name says Malaysia, but the rule checks the keyword and the A/H/K
// shape only, not nationality. Other countries use one letter and eight digits
// too, so a hit is a passport-like number under a passport keyword.
type MYPassportRecognizer struct {
	pattern *regexp.Regexp
}

// NewMYPassportRecognizer creates a Malaysia passport recognizer.
func NewMYPassportRecognizer() *MYPassportRecognizer {
	return &MYPassportRecognizer{
		pattern: regexp.MustCompile(passportLead + `([AHK]\d{8})\b`),
	}
}

// Name returns the recognizer name.
func (r *MYPassportRecognizer) Name() string { return "national_my_passport" }

// Recognize detects Malaysia passport numbers that follow a passport keyword.
// The confidence matches the keyword-backed level of the Purview definition.
func (r *MYPassportRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringSubmatchIndex(text, -1) {
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityMYPassport,
			Start:      m[2],
			End:        m[3],
			Text:       text[m[2]:m[3]],
			Confidence: 0.85,
		})
	}
	return entities, nil
}

// IsThaiIDSpan reports whether text[start:end] is a Thai citizen ID, bounded the
// way the Thai pattern's word boundaries bound it. A glued run stays a card.
// It is the shared retyping rule used by piiscrub.Scrubber.Analyze and the
// structured CSV path, so a Thai ID is never reported as CREDIT_CARD there.
func IsThaiIDSpan(text string, start, end int) bool {
	if start > 0 && isWordByte(text[start-1]) {
		return false
	}
	if end < len(text) && isWordByte(text[end]) {
		return false
	}
	return NewTHIDValidator().Validate(text[start:end])
}

// isWordByte matches the characters that Go's \b word boundary treats as word
// characters.
func isWordByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}
