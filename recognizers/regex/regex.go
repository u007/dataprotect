// Package regex provides regex-based PII recognizers.
package regex

import (
	"context"
	"regexp"
	"strings"

	"dataprotect/recognizers"
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
type SSNRecognizer struct {
	pattern *regexp.Regexp
}

// NewSSNRecognizer creates a new SSN recognizer.
func NewSSNRecognizer() *SSNRecognizer {
	return &SSNRecognizer{
		pattern: regexp.MustCompile(`\b\d{3}[\s.\-]?\d{2}[\s.\-]?\d{4}\b`),
	}
}

// Name returns the recognizer name.
func (r *SSNRecognizer) Name() string { return "regex_ssn" }

// Recognize detects SSNs in text.
func (r *SSNRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	matches := r.pattern.FindAllStringIndex(text, -1)
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
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityURL,
			Start:      m[0],
			End:        m[1],
			Text:       text[m[0]:m[1]],
			Confidence: 0.99,
		})
	}
	return entities, nil
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

// BitcoinRecognizer detects bitcoin addresses.
type BitcoinRecognizer struct {
	pattern *regexp.Regexp
}

// NewBitcoinRecognizer creates a new bitcoin recognizer.
func NewBitcoinRecognizer() *BitcoinRecognizer {
	return &BitcoinRecognizer{
		pattern: regexp.MustCompile(`\b[13][a-km-zA-HJ-NP-Z1-9]{25,34}\b|\bbc1[a-zA-HJ-NP-Z0-9]{25,39}\b`),
	}
}

// Name returns the recognizer name.
func (r *BitcoinRecognizer) Name() string { return "regex_bitcoin" }

// Recognize detects bitcoin addresses in text.
func (r *BitcoinRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	matches := r.pattern.FindAllStringIndex(text, -1)
	entities := make([]recognizers.Entity, 0, len(matches))
	for _, m := range matches {
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
