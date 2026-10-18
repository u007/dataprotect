// Package regex provides regex-based PII recognizers.
package regex

import (
	"context"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/checksum"
)

// EmailRecognizer detects email addresses.
type EmailRecognizer struct {
	pattern *regexp.Regexp
}

// NewEmailRecognizer creates a new email recognizer.
func NewEmailRecognizer() *EmailRecognizer {
	// RFC 5322 simplified pattern. The local part and domain labels take Unicode
	// letters, numbers and marks (IDN addresses). The TLD is a punycode label
	// (xn--...) or Unicode letters and marks; Go's regexp is leftmost-first, so
	// the punycode branch is listed first. Accepted cost: with no space, the local
	// part takes the CJK or Thai text before the @.
	return &EmailRecognizer{
		pattern: regexp.MustCompile(`[\p{L}\p{N}\p{M}._%+\-]+@[\p{L}\p{N}\p{M}.\-]+\.(?:xn--[a-zA-Z0-9\-]+|\p{L}[\p{L}\p{M}]+)`),
	}
}

// Name returns the recognizer name.
func (r *EmailRecognizer) Name() string { return "regex_email" }

// Recognize detects email addresses in text.
func (r *EmailRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	matches := r.pattern.FindAllStringIndex(text, -1)
	entities := make([]recognizers.Entity, 0, len(matches))
	for _, m := range matches {
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityEmail,
			Start:      m[0],
			End:        m[1],
			Text:       text[m[0]:m[1]],
			Confidence: 0.99,
		})
	}
	return entities, nil
}

// PhoneRecognizer detects phone numbers.
type PhoneRecognizer struct {
	pattern *regexp.Regexp
}

// NewPhoneRecognizer creates a new phone recognizer.
//
// The alternation has three branches because the leading anchor cannot be the
// same for all of them:
//
//  1. A parenthesised area code, e.g. "(555) 867-5309", optionally preceded by
//     a "1" country code.
//  2. An international number introduced by "+", e.g. "+44 20 7946 0958" or
//     "+1-555-123-4567". This branch REQUIRES a separator after the country
//     code: without one, "\+\d{1,3}...\d{1,9}" matches any "+" followed by four
//     or more digits, so arithmetic like "1+23456789" and short strings like
//     "+12345" would be read as phone numbers. The old pattern's leading \b hid
//     that at a word boundary; now that the branch is anchorless it must earn
//     its own precision. An unseparated "+442079460958" is therefore not
//     matched, which matches the previous behaviour at a boundary.
//  3. A bare US number, e.g. "555-123-4567".
//
// Branches 1 and 2 must NOT carry a leading \b. A \b matches only at a word
// boundary, and it cannot match between a space (or the start of the text) and
// "(" or "+", because both sides are non-word characters. With the anchor
// present the engine skips the optional "(" / "+" and starts the match one byte
// later, so "(555) 867-5309" was reported as "555) 867-5309" and
// "+1-555-123-4567" as "1-555-123-4567" — a span that is one byte too far to the
// right. The recognizer still reported one PHONE, so a hit-count test could not
// see it; only an exact-span assertion can, which is what the phone table in
// regex_test.go now makes.
//
// Branch 3 starts with a digit and DOES keep the \b. Without it the unanchored
// alternation matches the first ten digits of any longer digit run, so a
// 16-digit credit card was detected as a phone number — and since the phone
// recognizer scores higher, overlap resolution kept the wrong entity type and
// the phone mask pattern mangled the card.
//
// Every branch is therefore left-bounded, just not all by the same construct:
// branch 1 by "(" (or, when a "1" country code is present, by the \b that the
// "1" carries — a bare "1" attached to a word is not a country code), branch 2
// by "+", and branch 3 by its \b. A digit run can contain neither "(" nor "+",
// which is why those two branches need no \b of their own.
func NewPhoneRecognizer() *PhoneRecognizer {
	// Parenthesised (optional "1") | international "+" | bare US, in that order
	// so the more specific paren branch wins a leftmost match such as
	// "+1 (555) 867-5309".
	return &PhoneRecognizer{
		pattern: regexp.MustCompile(`(?:\+1[\s.\-]?|\b1[\s.\-]?)?\(\d{3}\)[\s.\-]?\d{3}[\s.\-]?\d{4}\b|\+\d{1,3}[\s.\-](?:\(\d{1,4}\)|\d{1,4})[\s.\-]?\d{1,4}[\s.\-]?\d{1,9}\b|\b(?:\+?1[\s.\-]?)?\d{3}[\s.\-]?\d{3}[\s.\-]?\d{4}\b`),
	}
}

// Name returns the recognizer name.
func (r *PhoneRecognizer) Name() string { return "regex_phone" }

// Recognize detects phone numbers in text.
func (r *PhoneRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	matches := r.pattern.FindAllStringIndex(text, -1)
	entities := make([]recognizers.Entity, 0, len(matches))
	for _, m := range matches {
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityPhone,
			Start:      m[0],
			End:        m[1],
			Text:       text[m[0]:m[1]],
			Confidence: 0.98,
		})
	}
	return entities, nil
}

// CreditCardRecognizer detects credit card numbers.
type CreditCardRecognizer struct {
	pattern *regexp.Regexp
}

// NewCreditCardRecognizer creates a new credit card recognizer.
func NewCreditCardRecognizer() *CreditCardRecognizer {
	// Major card patterns: Visa, MasterCard, Amex, Discover, Diners, JCB
	return &CreditCardRecognizer{
		pattern: regexp.MustCompile(`\b(?:\d{4}[\s.\-]?){3}\d{4}\b|\b\d{15,16}\b`),
	}
}

// Name returns the recognizer name.
func (r *CreditCardRecognizer) Name() string { return "regex_credit_card" }

// Recognize detects credit card numbers in text.
func (r *CreditCardRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	matches := r.pattern.FindAllStringIndex(text, -1)
	entities := make([]recognizers.Entity, 0, len(matches))
	for _, m := range matches {
		card := strings.ReplaceAll(text[m[0]:m[1]], " ", "")
		card = strings.ReplaceAll(card, "-", "")
		card = strings.ReplaceAll(card, ".", "")
		if len(card) >= 13 && len(card) <= 19 {
			entities = append(entities, recognizers.Entity{
				Type:       recognizers.EntityCreditCard,
				Start:      m[0],
				End:        m[1],
				Text:       text[m[0]:m[1]],
				Confidence: 0.95,
			})
		}
	}
	return entities, nil
}

// SSNRecognizer detects US Social Security numbers.
//
// A US SSN is 3-2-4, and both separator positions are required when the value
// appears inside other text. Every separator used to be optional, which made
// any bare nine-digit run match: `Account Number: 123456789`,
// `CAS No.: 123456789` and `Passport Number: 322996077` were all read as SSNs,
// costing 449 of 483 precision on gretel (exact P=0.0929). A single separator
// is nearly as loose — corpus longitudes like 177.978032 matched it.
//
// A bare nine-digit value is still accepted when the digits ARE the whole
// input: a declared field, a CSV cell or a standalone token carries no
// surrounding context to be ambiguous with, and the batch processor analyses
// one cell at a time. That carve-out is what keeps a bare `ssn` column value
// scrubbed while refusing the same digits in a sentence.
//
// The recall cost of requiring separators is nil on the evidence: 220 of the
// 232 SSN labels across both corpora are dashed and NONE is bare, while all
// 714 bare nine-digit matches sat inside prose.
type SSNRecognizer struct {
	pattern *regexp.Regexp // canonical 3-2-4 with a separator at both positions
	bare    *regexp.Regexp // a bare nine-digit value, anchored to the whole input
}

// NewSSNRecognizer creates a new SSN recognizer.
func NewSSNRecognizer() *SSNRecognizer {
	return &SSNRecognizer{
		pattern: regexp.MustCompile(`\b\d{3}[\s.\-]\d{2}[\s.\-]\d{4}\b`),
		bare:    regexp.MustCompile(`^\d{9}$`),
	}
}

// Name returns the recognizer name.
func (r *SSNRecognizer) Name() string { return "regex_ssn" }

// Recognize detects SSNs in text.
func (r *SSNRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	matches := r.pattern.FindAllStringIndex(text, -1)

	// The whole-value carve-out: a bare nine-digit run counts only when the
	// digits are the entire (whitespace-trimmed) input. A non-empty result of
	// TrimSpace that fails ^\d{9}$ contributes nothing, and strings.Index on a
	// successful match therefore locates the leading trim exactly.
	if trimmed := strings.TrimSpace(text); r.bare.MatchString(trimmed) {
		lead := strings.Index(text, trimmed)
		matches = append(matches, []int{lead, lead + len(trimmed)})
	}

	entities := make([]recognizers.Entity, 0, len(matches))
	for _, m := range matches {
		ssn := strings.ReplaceAll(text[m[0]:m[1]], " ", "")
		ssn = strings.ReplaceAll(ssn, "-", "")
		ssn = strings.ReplaceAll(ssn, ".", "")
		if len(ssn) == 9 {
			entities = append(entities, recognizers.Entity{
				Type:       recognizers.EntitySSN,
				Start:      m[0],
				End:        m[1],
				Text:       text[m[0]:m[1]],
				Confidence: 0.99,
			})
		}
	}
	return entities, nil
}

// IPRecognizer detects IPv4 and IPv6 addresses.
//
// IPv4 is matched by shape. An IPv6 candidate is a run of hex groups joined by
// colons, reported only when netip parses it as an IPv6 address. That keeps
// clock times (12:30:45) and MAC addresses out, since neither is an address. The
// IPv4 tail of a mapped address (::ffff:192.0.2.1) is part of the IPv6 span, so
// it is not reported again on its own.
type IPRecognizer struct {
	pattern *regexp.Regexp
}

// ipv6Candidate is at least two colons with hex groups between them, optionally
// ending in a dotted IPv4 tail. The IPv4 tail is tried first, so a mapped
// address is not cut short at its last hex group.
var ipv6Candidate = regexp.MustCompile(`(?:[0-9A-Fa-f]{0,4}:){2,}(?:(?:[0-9]{1,3}\.){3}[0-9]{1,3}|[0-9A-Fa-f]{0,4})`)

// NewIPRecognizer creates a new IP recognizer.
func NewIPRecognizer() *IPRecognizer {
	return &IPRecognizer{
		pattern: regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\b`),
	}
}

// Name returns the recognizer name.
func (r *IPRecognizer) Name() string { return "regex_ip" }

// Recognize detects IPv4 and IPv6 addresses in text.
func (r *IPRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	v6 := ipv6Spans(text)

	// Both lists are in start order, and the IPv6 spans do not overlap, so one
	// forward walk finds the IPv4 matches that sit inside an IPv6 span.
	ipv4 := r.pattern.FindAllStringIndex(text, -1)
	entities := make([]recognizers.Entity, 0, len(ipv4)+len(v6))
	j := 0
	for _, m := range ipv4 {
		for j < len(v6) && v6[j].End <= m[0] {
			j++
		}
		if j < len(v6) && v6[j].Start <= m[0] && m[1] <= v6[j].End {
			continue
		}
		// Skip version strings such as "Upgrade the agent to 3.10.2.7": a
		// dotted quad preceded by a version/upgrade/agent cue is a version
		// number, not an address.
		start := m[0] - 30
		if start < 0 {
			start = 0
		}
		prefix := strings.ToLower(text[start:m[0]])
		if strings.Contains(prefix, "upgrade") || strings.Contains(prefix, "version") || strings.Contains(prefix, "agent") {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityIP,
			Start:      m[0],
			End:        m[1],
			Text:       text[m[0]:m[1]],
			Confidence: 0.99,
		})
	}
	entities = append(entities, v6...)
	sort.Slice(entities, func(i, j int) bool { return entities[i].Start < entities[j].Start })
	return entities, nil
}

// ipv6Spans returns the IPv6 addresses in text, in order. A candidate that is not
// an address gets one retry from just after its first single colon, so an address
// after a label (id:fe80::1) is not lost to the label. The scan resumes after the
// candidate either way, so a long run is read once.
func ipv6Spans(text string) []recognizers.Entity {
	var spans []recognizers.Entity
	from := 0
	for from < len(text) {
		m := ipv6Candidate.FindStringIndex(text[from:])
		if m == nil {
			break
		}
		start, end := from+m[0], from+m[1]
		if e, ok := ipv6Span(text, start, end); ok {
			spans = append(spans, e)
		} else if k := firstSingleColon(text, start, end); k >= 0 {
			if e, ok := ipv6Span(text, k+1, end); ok {
				spans = append(spans, e)
			}
		}
		from = end
	}
	return spans
}

// ipv6Span returns text[start:end] as an IPv6 entity when it is a standalone
// address. A single colon at either end is a label separator, not part of the
// address, so it is trimmed before the check.
func ipv6Span(text string, start, end int) (recognizers.Entity, bool) {
	if start < end && text[start] == ':' && start+1 < end && text[start+1] != ':' {
		start++
	}
	if end-start >= 2 && text[end-1] == ':' && text[end-2] != ':' {
		end--
	}
	if start >= end || leftGlue(text, start) || rightGlue(text, end) {
		return recognizers.Entity{}, false
	}
	candidate := text[start:end]
	addr, err := netip.ParseAddr(candidate)
	// intentionally not logged: a candidate that is not an address is the normal negative case
	if err != nil || !addr.Is6() || !strings.ContainsAny(candidate, "0123456789abcdefABCDEF") {
		return recognizers.Entity{}, false
	}
	return recognizers.Entity{
		Type:       recognizers.EntityIP,
		Start:      start,
		End:        end,
		Text:       candidate,
		Confidence: 0.99,
	}, true
}

// firstSingleColon returns the index of the first colon in text[start:end] that
// is not part of a "::" run, or -1 when there is none.
func firstSingleColon(text string, start, end int) int {
	for i := start; i < end; i++ {
		if text[i] != ':' {
			continue
		}
		if (i > start && text[i-1] == ':') || (i+1 < end && text[i+1] == ':') {
			continue
		}
		return i
	}
	return -1
}

// leftGlue reports whether the byte before start joins the candidate to a longer
// token: a letter or digit, or a colon that continues a "::" run. A single colon
// before the candidate is a label separator (IP:2001:db8::1).
func leftGlue(text string, start int) bool {
	if start == 0 {
		return false
	}
	c := text[start-1]
	if c == ':' {
		return start >= 2 && text[start-2] == ':'
	}
	return isAlnumByte(c)
}

// rightGlue reports whether the byte at end joins the candidate to a longer
// token: a letter or digit, a colon that continues a "::" run, or a dot that
// begins another number (2001:db8::1.5).
func rightGlue(text string, end int) bool {
	if end == len(text) {
		return false
	}
	c := text[end]
	switch c {
	case ':':
		return end+1 < len(text) && text[end+1] == ':'
	case '.':
		return end+1 < len(text) && text[end+1] >= '0' && text[end+1] <= '9'
	}
	return isAlnumByte(c)
}

// isAlnumByte reports whether c is an ASCII letter or digit.
func isAlnumByte(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// URLRecognizer detects URLs.
//
// The pattern takes every character up to whitespace or a delimiter, so a URL
// that ends a sentence or a markdown span would keep the punctuation around it.
// Recognize trims that trailing punctuation with trimURL. The policy is
// deliberately narrow: a trailing `.,;:!?*'` and backtick are always dropped,
// and a closing `)` or `]` only when the URL has no matching opener. A
// Wikipedia-style `Foo_(bar)` keeps its paren. The set follows what the corpus
// shows: nemotron's URL disagreements are trailing `.` and `,`, then unmatched
// `)`, `**`, and backticks. A `'` is trimmed because the pattern accepts it
// and a single-quoted URL is far more common than one that ends in `'`.
type URLRecognizer struct {
	pattern *regexp.Regexp
}

// NewURLRecognizer creates a new URL recognizer.
func NewURLRecognizer() *URLRecognizer {
	return &URLRecognizer{
		pattern: regexp.MustCompile("[hH][tT][tT][pP][sS]?://[^\\s<>\"{}|\\\\^]+"),
	}
}

// Name returns the recognizer name.
func (r *URLRecognizer) Name() string { return "regex_url" }

// Recognize detects URLs in text.
func (r *URLRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	matches := r.pattern.FindAllStringIndex(text, -1)
	entities := make([]recognizers.Entity, 0, len(matches))
	for _, m := range matches {
		u := trimURL(text[m[0]:m[1]])
		// A scheme with nothing after it ("http://." trims to "http://") is not a URL.
		if strings.HasSuffix(u, "://") {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityURL,
			Start:      m[0],
			End:        m[0] + len(u),
			Text:       u,
			Confidence: 0.99,
		})
	}
	return entities, nil
}

// trimURL drops trailing sentence punctuation from a URL match, repeating until
// the end is stable so that "https://x.com/a)." loses both the ")" and the ".".
// Every trimmed byte is ASCII, so the result stays on a rune boundary.
func trimURL(u string) string {
	for u != "" {
		last := u[len(u)-1]
		switch {
		case strings.IndexByte(".,;:!?*'`", last) >= 0:
		case last == ')' && strings.Count(u, "(") < strings.Count(u, ")"):
		case last == ']' && strings.Count(u, "[") < strings.Count(u, "]"):
		default:
			return u
		}
		u = u[:len(u)-1]
	}
	return u
}

// IBANRecognizer detects IBANs.
type IBANRecognizer struct {
	pattern *regexp.Regexp
}

// NewIBANRecognizer creates a new IBAN recognizer.
func NewIBANRecognizer() *IBANRecognizer {
	return &IBANRecognizer{
		pattern: regexp.MustCompile(`\b[A-Z]{2}\d{2}[\s]?[A-Z0-9]{4}[\s]?[A-Z0-9]{4}[\s]?[A-Z0-9]{4}[\s]?[A-Z0-9]{4}[\s]?[A-Z0-9]{2,4}\b`),
	}
}

// Name returns the recognizer name.
func (r *IBANRecognizer) Name() string { return "regex_iban" }

// Recognize detects IBANs in text.
func (r *IBANRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	matches := r.pattern.FindAllStringIndex(text, -1)
	entities := make([]recognizers.Entity, 0, len(matches))
	for _, m := range matches {
		iban := strings.ReplaceAll(text[m[0]:m[1]], " ", "")
		if len(iban) >= 15 && len(iban) <= 34 {
			entities = append(entities, recognizers.Entity{
				Type:       recognizers.EntityIBAN,
				Start:      m[0],
				End:        m[1],
				Text:       text[m[0]:m[1]],
				Confidence: 0.95,
			})
		}
	}
	return entities, nil
}

// BitcoinRecognizer detects bitcoin addresses whose checksum verifies.
type BitcoinRecognizer struct {
	pattern   *regexp.Regexp
	validator *checksum.BitcoinValidator
}

// NewBitcoinRecognizer creates a new bitcoin recognizer.
func NewBitcoinRecognizer() *BitcoinRecognizer {
	return &BitcoinRecognizer{
		pattern:   regexp.MustCompile(`\b[13][a-km-zA-HJ-NP-Z1-9]{25,34}\b|\b[bB][cC]1[a-zA-HJ-NP-Z0-9]{25,87}\b`),
		validator: checksum.NewBitcoinValidator(),
	}
}

// Name returns the recognizer name.
func (r *BitcoinRecognizer) Name() string { return "regex_bitcoin" }

// Recognize detects bitcoin addresses in text. A shape match whose checksum
// fails is not an address, so it is not reported.
func (r *BitcoinRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	matches := r.pattern.FindAllStringIndex(text, -1)
	entities := make([]recognizers.Entity, 0, len(matches))
	for _, m := range matches {
		if !r.validator.Validate(text[m[0]:m[1]]) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityBitcoin,
			Start:      m[0],
			End:        m[1],
			Text:       text[m[0]:m[1]],
			Confidence: 0.95,
		})
	}
	return entities, nil
}
