// Package checksum provides checksum-based validation for PII entities.
package checksum

import (
	"context"
	"fmt"
	"sort"
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
	number = strings.ReplaceAll(number, "\t", "")

	if len(number) < 13 || len(number) > 19 {
		return false
	}
	// An all-zero run passes Luhn, but no issuer issues it.
	if strings.Trim(number, "0") == "" {
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
	ibans, err := NewIBANValidator().Recognize(ctx, text)
	if err != nil {
		return nil, err
	}
	entities := make([]recognizers.Entity, 0)
	for _, c := range candidates {
		for _, card := range v.cardSpans(c, ibans) {
			entities = append(entities, recognizers.Entity{
				Type:       recognizers.EntityCreditCard,
				Start:      card.start,
				End:        card.end,
				Text:       card.text,
				Confidence: card.conf,
			})
		}
	}
	return entities, nil
}

// maxCardDigits is the longest card number, and so the most digits a window
// of groups can hold. It bounds the window search to a few groups per start.
const maxCardDigits = 19

// cardSpans returns the cards a run of digits holds. A run that passes Luhn
// whole is one card. A run that fails is searched for card windows: each
// window of consecutive digit groups that is 13 to 19 digits, passes Luhn and
// has a card layout. Every valid window is found, overlapping windows are
// merged into one span, and the merged spans are reported. Merging keeps a
// chance-passing window from cutting a card short: every digit of the card
// stays inside a reported span.
//
// The run is joined by space, hyphen and dot, so a card followed by a CVV, a
// year, or another card reaches here as one run. Without the search, the whole
// run fails Luhn and the card is not reported at all.
//
// A run inside a valid IBAN is the IBAN's own digits and is not searched.
// A single group must sit between separators (space, hyphen or dot) or the
// candidate's edges. A 13 to 19 digit group that passes Luhn is a card whatever
// joins it, so a card next to a CVV joined by `-` or `.` is reported.
// validCardPrefix reports whether the first digit of a card number is a valid
// major card network prefix. Digits 0, 1, 7, 8, 9 are not used by any major
// network, so a number starting with one of them cannot be a card even if it
// passes Luhn by chance (for example 1748503543012, which passes Luhn but is
// not a card).
func validCardPrefix(number string) bool {
	if len(number) == 0 {
		return false
	}
	switch number[0] {
	case '2', '3', '4', '5', '6':
		return true
	}
	return false
}

func (v *LuhnValidator) cardSpans(c cardCandidate, ibans []recognizers.Entity) []cardCandidate {
	// A whole run that passes Luhn is reported only when it also has a valid
	// card network prefix. Without this, a 13-19 digit run whose first digit is
	// not a major-network prefix (0, 1, 7, 8, 9) — e.g. a Thai citizen ID
	// that happens to pass Luhn, or a random digit run — would be reported as
	// a card. The window search below already requires validCardPrefix; this
	// keeps the whole-run path consistent with it (see TODO.md 2026-10-19,
	// "Whole-run Luhn pass skips the network-prefix and layout checks").
	stripped := strings.ReplaceAll(c.text, " ", "")
	stripped = strings.ReplaceAll(stripped, "-", "")
	stripped = strings.ReplaceAll(stripped, ".", "")
	stripped = strings.ReplaceAll(stripped, "\t", "")
	if v.Validate(c.text) && validCardPrefix(stripped) {
		c.conf = 0.99
		return []cardCandidate{c}
	}
	if inside(ibans, c.start, c.end) {
		return nil
	}
	var groups []cardGroup
	for i := 0; i < len(c.text); {
		if !isDigit(c.text[i]) {
			i++
			continue
		}
		start := i
		for i < len(c.text) && isDigit(c.text[i]) {
			i++
		}
		groups = append(groups, cardGroup{start, i})
	}

	// Each start tries at most maxCardDigits digits of groups, so the search is
	// linear in the run. Windows are collected in order of start, then end.
	type window struct{ first, last int }
	var valid []window
	for a := range groups {
		digits := 0
		for b := a; b < len(groups); b++ {
			digits += groups[b].end - groups[b].start
			if digits > maxCardDigits {
				break
			}
			if digits < 13 || !cardGrouping(c.text, groups[a:b+1]) {
				continue
			}
			// A window that shares a digit with a valid IBAN is the IBAN's digits,
			// not a card. If it passed Luhn by chance it would win the overlap
			// against the IBAN and hide the card that follows it.
			if overlapsAny(ibans, c.start+groups[a].start, c.start+groups[b].end) {
				continue
			}
			if a == b && !boundedBySeparator(c.text, groups[a]) {
				continue
			}
			windowText := c.text[groups[a].start:groups[b].end]
			if v.Validate(windowText) && validCardPrefix(windowText) {
				valid = append(valid, window{a, b})
			}
		}
	}

	// Merge windows that share a group, so one span covers each overlapping
	// cluster of cards.
	var cards []cardCandidate
	for i := 0; i < len(valid); {
		first, last := valid[i].first, valid[i].last
		for i++; i < len(valid) && valid[i].first <= last; i++ {
			if valid[i].last > last {
				last = valid[i].last
			}
		}
		cards = append(cards, cardCandidate{
			text:  c.text[groups[first].start:groups[last].end],
			start: c.start + groups[first].start,
			end:   c.start + groups[last].end,
			conf:  0.99,
		})
	}
	return cards
}

// inside reports whether [start, end) lies within one of the entities.
//
// entities must be in start order and must not overlap, as the IBAN recognizer
// reports them. Then the only entity that can contain the span is the last one
// that starts at or before it, which a binary search finds.
func inside(entities []recognizers.Entity, start, end int) bool {
	i := sort.Search(len(entities), func(i int) bool { return entities[i].Start > start }) - 1
	return i >= 0 && entities[i].Start <= start && end <= entities[i].End
}

// overlapsAny reports whether [start, end) shares a byte with one of the
// entities. The entities must be in start order and must not overlap.
func overlapsAny(entities []recognizers.Entity, start, end int) bool {
	i := sort.Search(len(entities), func(i int) bool { return entities[i].End > start })
	return i < len(entities) && entities[i].Start < end
}

// boundedBySeparator reports whether a single group sits between a card
// separator or the candidate's edges on both sides.
func boundedBySeparator(text string, g cardGroup) bool {
	left := g.start == 0 || isCardSeparator(text[g.start-1])
	right := g.end == len(text) || isCardSeparator(text[g.end])
	return left && right
}

// cardGrouping reports whether a window of digit groups is laid out like a
// card. A single group always is. A window of several groups must use one
// separator character, exactly once between each pair of groups, and one of
// the layouts a card number is printed in: 4-4-4-4 (last group 1 to 4 digits),
// 4-6-5 (Amex) or 4-6-4 (Diners). A mixed or unusual layout is not a card, so a
// number glued to a neighbouring digit group is never reported as one.
func cardGrouping(text string, groups []cardGroup) bool {
	if len(groups) == 1 {
		return true
	}
	for i := 0; i+1 < len(groups); i++ {
		gap := groups[i].end
		if groups[i+1].start != gap+1 || text[gap] != text[groups[0].end] {
			return false
		}
	}
	lengths := make([]int, len(groups))
	for i, g := range groups {
		lengths[i] = g.end - g.start
	}
	last := lengths[len(lengths)-1]
	if last >= 1 && last <= 4 {
		uniform := true
		for _, n := range lengths[:len(lengths)-1] {
			if n != 4 {
				uniform = false
			}
		}
		if uniform {
			return true
		}
	}
	return equalInts(lengths, []int{4, 6, 5}) || equalInts(lengths, []int{4, 6, 4})
}

// equalInts reports whether a and b hold the same values in the same order.
func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// cardGroup is one maximal run of digits inside a candidate, by byte offsets.
type cardGroup struct{ start, end int }

type cardCandidate struct {
	text  string
	start int
	end   int
	conf  float64
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
	return c == ' ' || c == '-' || c == '.' || c == '\t'
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
	iban = strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(iban))
	if len(iban) < 15 || len(iban) > 34 {
		return false
	}
	// The length is the country's when the registry table has the country: a
	// prefix of a longer IBAN can pass mod-97 by chance, and its length is wrong
	// for the country it names. A country the table does not list keeps the
	// general range, so an unknown IBAN is still reported.
	if want, ok := ibanLength[iban[:2]]; ok && len(iban) != want {
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
		iban, ok := v.ibanSpan(c)
		if !ok {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityIBAN,
			Start:      iban.start,
			End:        iban.end,
			Text:       iban.text,
			Confidence: 0.99,
		})
	}
	return entities, nil
}

// ibanSpan returns the IBAN a candidate holds. The candidate can run on past
// the IBAN into uppercase words, a BIC or a digit group, because a space
// continues it when the next block is uppercase or numeric.
//
// An unspaced IBAN, whose first block holds 15 or more characters, ends at its
// first separator. A spaced IBAN is the longest prefix cut at a separator that
// has the printed shape and passes mod-97. Cuts are tried from the right, so
// words after the IBAN are dropped one block at a time. A separator is a space
// or a dash. Each cut that passes the
// shape check still has about a 1 in 97 chance of passing mod-97 by accident,
// so the shape check is what keeps short accidental matches out.
func (v *IBANValidator) ibanSpan(c ibanCandidate) (ibanCandidate, bool) {
	if v.Validate(c.text) {
		return c, true
	}
	first := strings.IndexAny(c.text, " -")
	if first >= ibanUnspacedMinBlock {
		return c.prefix(first, v)
	}
	for k := strings.LastIndexAny(c.text, " -"); k > 0; k = strings.LastIndexAny(c.text[:k], " -") {
		if iban, ok := c.prefix(k, v); ok {
			return iban, true
		}
	}
	return ibanCandidate{}, false
}

// ibanUnspacedMinBlock is the shortest first block that marks an unspaced IBAN.
// The shortest IBAN is 15 characters, so a block this long cannot be a group of
// a spaced IBAN, whose blocks are 4 characters.
const ibanUnspacedMinBlock = 15

// prefix returns the candidate cut at k, if that prefix is a valid IBAN with the
// canonical shape of a printed IBAN.
func (c ibanCandidate) prefix(k int, v *IBANValidator) (ibanCandidate, bool) {
	text := c.text[:k]
	if !canonicalIBANShape(text) || !v.Validate(text) {
		return ibanCandidate{}, false
	}
	return ibanCandidate{text: text, start: c.start, end: c.start + k}, true
}

// canonicalIBANShape reports whether s is a printed IBAN: a country code of two
// uppercase letters and two check digits, uppercase letters and digits only,
// single separators between blocks (a space or a dash, either or both), and every
// block 4 characters except a last block of 1 to 4. An unspaced IBAN is one block and is checked only for the country
// code, because its length is the country's.
func canonicalIBANShape(s string) bool {
	if len(s) < 4 || !isUpperLetter(s[0]) || !isUpperLetter(s[1]) || !isDigit(s[2]) || !isDigit(s[3]) {
		return false
	}
	blocks := strings.Split(strings.ReplaceAll(s, "-", " "), " ")
	for _, block := range blocks {
		for i := 0; i < len(block); i++ {
			if !isUpperLetter(block[i]) && !isDigit(block[i]) {
				return false
			}
		}
	}
	if len(blocks) == 1 {
		return true
	}
	for i, block := range blocks {
		last := i == len(blocks)-1
		if last && (len(block) < 1 || len(block) > 4) {
			return false
		}
		if !last && len(block) != 4 {
			return false
		}
	}
	return true
}

// isUpperLetter reports whether c is an ASCII uppercase letter.
func isUpperLetter(c byte) bool {
	return c >= 'A' && c <= 'Z'
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
			if (text[end] == ' ' || text[end] == '-') &&
				significant+1 <= ibanMaxSignificant && hasShortAlphanumBlock(text, end+1) {
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
