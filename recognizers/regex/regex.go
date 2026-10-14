// Package regex provides regex-based PII recognizers.
package regex

import (
	"context"
	"regexp"
	"strings"

	"dataprotect/recognizers"
	"dataprotect/recognizers/checksum"
)

// EmailRecognizer detects email addresses.
type EmailRecognizer struct {
	pattern *regexp.Regexp
}

// NewEmailRecognizer creates a new email recognizer.
func NewEmailRecognizer() *EmailRecognizer {
	// RFC 5322 simplified pattern
	return &EmailRecognizer{
		pattern: regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
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
		pattern: regexp.MustCompile(`(?:\+1[\s.\-]?|\b1[\s.\-]?)?\(\d{3}\)[\s.\-]?\d{3}[\s.\-]?\d{4}\b|\+\d{1,3}[\s.\-]\d{1,4}[\s.\-]?\d{1,4}[\s.\-]?\d{1,9}\b|\b(?:\+?1[\s.\-]?)?\d{3}[\s.\-]?\d{3}[\s.\-]?\d{4}\b`),
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

// IPRecognizer detects IP addresses.
type IPRecognizer struct {
	pattern *regexp.Regexp
}

// NewIPRecognizer creates a new IP recognizer.
func NewIPRecognizer() *IPRecognizer {
	return &IPRecognizer{
		pattern: regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\b`),
	}
}

// Name returns the recognizer name.
func (r *IPRecognizer) Name() string { return "regex_ip" }

// Recognize detects IP addresses in text.
func (r *IPRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	matches := r.pattern.FindAllStringIndex(text, -1)
	entities := make([]recognizers.Entity, 0, len(matches))
	for _, m := range matches {
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityIP,
			Start:      m[0],
			End:        m[1],
			Text:       text[m[0]:m[1]],
			Confidence: 0.99,
		})
	}
	return entities, nil
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
		pattern: regexp.MustCompile("https?://[^\\s<>\"{}|\\\\^]+"),
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
		pattern:   regexp.MustCompile(`\b[13][a-km-zA-HJ-NP-Z1-9]{25,34}\b|\bbc1[a-zA-HJ-NP-Z0-9]{25,87}\b`),
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
