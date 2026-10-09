// Package piiscrub provides a simple API for PII detection and anonymization.
package piiscrub

import (
	"context"

	"dataprotect/analyzer"
	"dataprotect/anonymizer"
	"dataprotect/operators"
	"dataprotect/recognizers"
	"dataprotect/recognizers/checksum"
	"dataprotect/recognizers/national"
	"dataprotect/recognizers/regex"
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
	a := analyzer.New(analyzer.WithMinConfidence(0.7))
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

	an := anonymizer.New()
	an.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))
	an.SetOperator(recognizers.EntityPhone, operators.NewMaskOperator("***-***-####"))
	an.SetOperator(recognizers.EntityCreditCard, operators.NewMaskOperator("****-****-****-####"))
	an.SetOperator(recognizers.EntitySSN, operators.NewMaskOperator("***-**-####"))
	an.SetOperator(recognizers.EntityIP, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityURL, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityIBAN, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityBitcoin, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntitySGNRIC, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityMYIC, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityTHID, operators.NewRedactOperator())

	return &Scrubber{
		analyzer:   a,
		anonymizer: an,
	}
}

// Analyze detects PII entities in text.
func (s *Scrubber) Analyze(ctx context.Context, text string) ([]recognizers.Entity, error) {
	return s.analyzer.Analyze(ctx, text)
}

// Anonymize detects and anonymizes PII in text.
func (s *Scrubber) Anonymize(ctx context.Context, text string) (string, error) {
	entities, err := s.analyzer.Analyze(ctx, text)
	if err != nil {
		return "", err
	}
	return s.anonymizer.Anonymize(ctx, text, entities)
}
