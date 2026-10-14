// Package checksum provides checksum-based validation for PII entities.
package checksum

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/u007/dataprotect/recognizers"
)

// LuhnValidator validates credit card numbers using the Luhn algorithm.
type LuhnValidator struct{}

// NewLuhnValidator creates a new Luhn validator.
func NewLuhnValidator() *LuhnValidator {
	return &LuhnValidator{}
}

// Name returns the validator name.
func (v *LuhnValidator) Name() string { return "checksum_luhn" }

// Validate checks if a number passes the Luhn checksum.
func (v *LuhnValidator) Validate(number string) bool {
	number = strings.ReplaceAll(number, " ", "")
	number = strings.ReplaceAll(number, "-", "")
	number = strings.ReplaceAll(number, ".", "")

	if len(number) < 13 || len(number) > 19 {
		return false
	}

	sum := 0
	alternate := false
	for i := len(number) - 1; i >= 0; i-- {
		digit, err := strconv.Atoi(string(number[i]))
		if err != nil {
			return false
		}
		if alternate {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		alternate = !alternate
	}
	return sum%10 == 0
}

// Recognize validates credit card candidates from text.
func (v *LuhnValidator) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	// Find all number sequences that could be credit cards
	candidates := findCardCandidates(text)
	entities := make([]recognizers.Entity, 0)
	for _, c := range candidates {
		if v.Validate(c.text) {
			entities = append(entities, recognizers.Entity{
				Type:       recognizers.EntityCreditCard,
				Start:      c.start,
				End:        c.end,
				Text:       c.text,
				Confidence: 0.99,
			})
		}
	}
	return entities, nil
}

type cardCandidate struct {
	text  string
	start int
	end   int
}

// findCardCandidates finds runs of digits with optional -/. separators.
// A separator must sit BETWEEN digits: a trailing separator belongs to the
// surrounding prose ("...1111 and"), so it must not extend the candidate span
// or Luhn validation runs against text the caller never matched.
func findCardCandidates(text string) []cardCandidate {
	var candidates []cardCandidate
	i := 0
	for i < len(text) {
		if !isDigit(text[i]) {
			i++
			continue
		}
		start := i
		for i < len(text) && (isDigit(text[i]) || isCardSeparator(text[i])) {
			i++
		}
		// Rewind past trailing separators so the span ends on the last digit.
		for i > start && isCardSeparator(text[i-1]) {
			i--
		}
		candidates = append(candidates, cardCandidate{
			text:  text[start:i],
			start: start,
			end:   i,
		})
	}
	return candidates
}

func isCardSeparator(c byte) bool {
	return c == ' ' || c == '-' || c == '.'
}

// IBANValidator validates IBANs using mod-97 checksum.
type IBANValidator struct{}

// NewIBANValidator creates a new IBAN validator.
func NewIBANValidator() *IBANValidator {
	return &IBANValidator{}
}

// Name returns the validator name.
func (v *IBANValidator) Name() string { return "checksum_iban" }

// Validate checks if an IBAN passes the mod-97 checksum.
func (v *IBANValidator) Validate(iban string) bool {
	iban = strings.ToUpper(strings.ReplaceAll(iban, " ", ""))
	if len(iban) < 15 || len(iban) > 34 {
		return false
	}

	// Move first 4 chars to end
	rearranged := iban[4:] + iban[:4]

	// Convert letters to numbers (A=10, B=11, ..., Z=35)
	var numeric strings.Builder
	for _, c := range rearranged {
		if c >= 'A' && c <= 'Z' {
			numeric.WriteString(strconv.Itoa(int(c-'A') + 10))
		} else if c >= '0' && c <= '9' {
			numeric.WriteRune(c)
		} else {
			return false
		}
	}

	// Mod-97 check
	numStr := numeric.String()
	if len(numStr) < 2 {
		return false
	}

	// Process in chunks to avoid overflow
	remainder := 0
	for i := 0; i < len(numStr); i++ {
		digit, _ := strconv.Atoi(string(numStr[i]))
		remainder = (remainder*10 + digit) % 97
	}

	return remainder == 1
}

// Recognize validates IBAN candidates from text.
func (v *IBANValidator) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	candidates := findIBANCandidates(text)
	entities := make([]recognizers.Entity, 0)
	for _, c := range candidates {
		if v.Validate(c.text) {
			entities = append(entities, recognizers.Entity{
				Type:       recognizers.EntityIBAN,
				Start:      c.start,
				End:        c.end,
				Text:       c.text,
				Confidence: 0.99,
			})
		}
	}
	return entities, nil
}

type ibanCandidate struct {
	text  string
	start int
	end   int
}

// ibanMaxSignificant is the ISO 13616 limit on IBAN length, excluding spaces.
const ibanMaxSignificant = 34

// findIBANCandidates finds IBAN-shaped runs in text.
//
// An IBAN is at most 34 alphanumerics, conventionally printed in blocks of four
// separated by single spaces ("GB82 WEST 1234 5698 7654 32"). A space is only
// consumed when a full block of four alphanumerics follows it, which is what
// separates a spaced IBAN from the prose around it: in "DE89...13000 and bad",
// the space before "and" is followed by three letters, so the run stops at the
// IBAN instead of absorbing the sentence and failing mod-97.
func findIBANCandidates(text string) []ibanCandidate {
	var candidates []ibanCandidate
	for i := 0; i < len(text)-3; {
		if !isIBANStart(text[i], text[i+1], text[i+2], text[i+3]) {
			i++
			continue
		}
		// Consume the country code, check digits, then alphanumeric blocks.
		end := i + 4
		// significant counts IBAN CHARACTERS, which ISO 13616 caps at 34.
		// Separating spaces are presentation, not content, so they must not be
		// counted: doing so inflated the running total, tripped the cap early,
		// and truncated the candidate before its final block. A valid spaced
		// IBAN was then dropped silently, because the truncated string failed
		// mod-97 even though Validate passed on the whole value.
		significant := 4
		for end < len(text) {
			if isLetter(text[end]) || isDigit(text[end]) {
				end++
				significant++
				continue
			}
			// A space continues the IBAN only if alphanumerics follow it.
			//
			// The block after a space must be 1 to 4 characters, not exactly 4.
			// Requiring exactly 4 rejected the FINAL space of every IBAN printed
			// in the conventional format, because the last block is short — an
			// IBAN is at most 34 significant characters, and 34 is not a multiple
			// of 4, so the closing group is routinely 2 or 3 characters. The run
			// therefore stopped one character early, the truncated string failed
			// mod-97, and a valid IBAN was dropped silently.
			//
			// Requiring at least 1 is what keeps prose out: "DE89 3704 ... and
			// bad" stops at the space before "and", which is followed by letters
			// A space can extend the span only while at least one further
			// character could still fit inside the 34-character limit.
			if text[end] == ' ' && significant+1 <= ibanMaxSignificant &&
				hasShortAlphanumBlock(text, end+1) {
				end++
				continue
			}
			break
		}
		candidates = append(candidates, ibanCandidate{
			text:  text[i:end],
			start: i,
			end:   end,
		})
		i = end
	}
	return candidates
}

// hasShortAlphanumBlock reports whether text[pos:] begins with 1 to 4
// characters drawn from the IBAN alphabet, stopping at the first that is not.
//
// Two properties matter, and both were wrong before:
//
//   - The block is 1 to 4 characters, not exactly 4. An IBAN is at most 34
//     significant characters and 34 is not a multiple of 4, so the closing
//     block of a conventionally-formatted IBAN is routinely 2 or 3 characters.
//     Requiring exactly 4 rejected the final space of every such IBAN, the
//     candidate stopped one character early, and the truncated string failed
//     mod-97 — dropping a valid IBAN silently, while Validate passed on the
//     same value given whole.
//
//   - Lowercase letters disqualify the block. This is what separates an
//     internal space from a space before prose. "GB82 WEST 1234 5698 7654 32"
//     has an uppercase letter block and must continue; the trailing " today"
//     and the " and bad" in the unspaced case are lowercase and must stop the
//     run. ISO 13616 writes an IBAN in uppercase letters and digits only, so
//     this is the format's own rule rather than a heuristic.
//
// A lowercase IBAN written into prose is therefore not matched. That is the
// safer direction of error: a miss leaves text unredacted, whereas absorbing
// prose would hand the anonymizer a wrong span and corrupt the output.
func hasShortAlphanumBlock(text string, pos int) bool {
	if pos >= len(text) {
		return false
	}
	n := 0
	for pos+n < len(text) && n < 4 {
		c := text[pos+n]
		isDigit := c >= '0' && c <= '9'
		isUpper := c >= 'A' && c <= 'Z'
		if !isDigit && !isUpper {
			break
		}
		n++
	}
	return n >= 1
}

// isIBANStart reports whether the four bytes at the cursor form the IBAN
// prefix: two letters (country code) followed by two digits (check digits).
func isIBANStart(a, b, c, d byte) bool {
	return isLetter(a) && isLetter(b) && isDigit(c) && isDigit(d)
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// Ensure validators implement Recognizer
var _ recognizers.Recognizer = (*LuhnValidator)(nil)
var _ recognizers.Recognizer = (*IBANValidator)(nil)

// Compile-time check
var _ = fmt.Sprintf
