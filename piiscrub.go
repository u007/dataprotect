// Package piiscrub provides a simple API for PII detection and anonymization.
package piiscrub

import (
	"context"

	"github.com/u007/dataprotect/analyzer"
	"github.com/u007/dataprotect/anonymizer"
	"github.com/u007/dataprotect/operators"
	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/checksum"
	"github.com/u007/dataprotect/recognizers/national"
	"github.com/u007/dataprotect/recognizers/regex"
)

// Scrubber provides a high-level API for PII detection and anonymization.
type Scrubber struct {
	analyzer   *analyzer.Analyzer
	anonymizer *anonymizer.Anonymizer
}

// NewScrubber creates a new scrubber with default recognizers and operators.
//
// The checksum validators are part of the default pipeline, not an opt-in, and
// the regex card recognizer is deliberately NOT wired alongside the Luhn one.
// Wiring both cannot work: for a Luhn-FAILING number the validator emits
// nothing while the regex recognizer still matches the shape at 0.95, so the
// false positive survives with nothing to overlap it away. Cards are therefore
// detected by Luhn alone, which produces identical spans for a valid card
// (verified for the spaced, hyphenated and plain forms) and none for an
// invalid one.
//
// regex.NewCreditCardRecognizer remains available for callers who want
// shape-only detection with no checksum, but that is the wrong default for a
// scrubber: it reports any 13-19 digit run, so a random long number is
// anonymized as if it were payment data.
//
// Because the Luhn validator scores 0.99 against the phone recognizer's 0.98, a
// greedy phone pattern can no longer be caught by observing which entity wins —
// the card masks it. That is why TestPhoneRecognizerDoesNotClaimCardDigits
// asserts at the recognizer level rather than through Analyze.
func NewScrubber() *Scrubber {
	return newScrubber()
}

// newScrubber builds the default scrubber with extra analyzer options. Only
// tests reach it, to score the same pipeline with context boosting on.
func newScrubber(opts ...analyzer.Option) *Scrubber {
	a := analyzer.New(append([]analyzer.Option{analyzer.WithMinConfidence(0.7)}, opts...)...)
	a.AddRecognizer(regex.NewEmailRecognizer())
	a.AddRecognizer(regex.NewPhoneRecognizer())
	a.AddRecognizer(regex.NewSSNRecognizer())
	a.AddRecognizer(regex.NewIPRecognizer())
	a.AddRecognizer(regex.NewURLRecognizer())
	a.AddRecognizer(regex.NewBitcoinRecognizer())
	a.AddRecognizer(checksum.NewLuhnValidator())
	a.AddRecognizer(checksum.NewIBANValidator())
	a.AddRecognizer(national.NewSGNRICRecognizer())
	a.AddRecognizer(national.NewMYICRecognizer())
	a.AddRecognizer(national.NewTHIDRecognizer())
	a.AddRecognizer(national.NewMYPassportRecognizer())
	a.AddRecognizer(national.NewSGPassportRecognizer())

	an := anonymizer.New()
	an.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))
	an.SetOperator(recognizers.EntityPhone, operators.NewTailMaskOperator("***-***-####"))
	an.SetOperator(recognizers.EntityCreditCard, operators.NewTailMaskOperator("****-****-****-####"))
	an.SetOperator(recognizers.EntitySSN, operators.NewMaskOperator("***-**-####"))
	an.SetOperator(recognizers.EntityIP, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityURL, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityIBAN, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityBitcoin, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntitySGNRIC, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityMYIC, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityTHID, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityMYPassport, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntitySGPassport, operators.NewRedactOperator())

	return &Scrubber{
		analyzer:   a,
		anonymizer: an,
	}
}

// thIDValidator decides whether a card span is a Thai citizen ID.
var thIDValidator = national.NewTHIDValidator()

// Analyze detects PII entities in text.
//
// A credit card whose span is a Thai citizen ID is returned as TH_ID. This runs
// after overlap resolution, so the span keeps the card's confidence and the
// overlap outcome is unchanged. The recognizers themselves always report a card.
func (s *Scrubber) Analyze(ctx context.Context, text string) ([]recognizers.Entity, error) {
	entities, err := s.analyzer.Analyze(ctx, text)
	if err != nil {
		return nil, err
	}
	for i := range entities {
		if entities[i].Type == recognizers.EntityCreditCard && isThaiIDSpan(text, entities[i].Start, entities[i].End) {
			entities[i].Type = recognizers.EntityTHID
		}
	}
	return entities, nil
}

// Anonymize detects and anonymizes PII in text.
func (s *Scrubber) Anonymize(ctx context.Context, text string) (string, error) {
	entities, err := s.Analyze(ctx, text)
	if err != nil {
		return "", err
	}
	return s.anonymizer.Anonymize(ctx, text, entities)
}

// Pseudonymize detects PII in text and replaces each value with a numbered
// placeholder such as [EMAIL_1]. It returns the redacted text and a map from
// each placeholder to its original value. The map holds raw PII, so the caller
// must protect it. See anonymizer.Pseudonymize for the numbering rules.
func (s *Scrubber) Pseudonymize(ctx context.Context, text string) (string, map[string]string, error) {
	entities, err := s.Analyze(ctx, text)
	if err != nil {
		return "", nil, err
	}
	return s.anonymizer.Pseudonymize(text, entities)
}

// isThaiIDSpan reports whether text[start:end] is a Thai citizen ID, bounded the
// way the Thai pattern's word boundaries bound it. A glued run stays a card.
func isThaiIDSpan(text string, start, end int) bool {
	if start > 0 && isWordByte(text[start-1]) {
		return false
	}
	if end < len(text) && isWordByte(text[end]) {
		return false
	}
	return thIDValidator.Validate(text[start:end])
}

// isWordByte matches the characters that Go's \b word boundary treats as word
// characters.
func isWordByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}
