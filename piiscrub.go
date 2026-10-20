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
	"github.com/u007/dataprotect/recognizers/secret"
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
	a.AddRecognizer(secret.NewSecretRecognizer())

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
	an.SetOperator(recognizers.EntitySecret, operators.NewRedactOperator())

	return &Scrubber{
		analyzer:   a,
		anonymizer: an,
	}
}

// Analyze detects PII entities in text.
//
// A credit card whose span is a Thai citizen ID is returned as TH_ID. This
// runs after overlap resolution, so the span keeps the card's confidence and
// the overlap outcome is unchanged. Note: since the Luhn validator now
// requires a valid card network prefix, a Thai ID (first digit 1) is no
// longer reported as a card by Luhn; the THIDRecognizer covers it instead.
// This retyping path remains for any card-shaped span that is also a Thai ID.
func (s *Scrubber) Analyze(ctx context.Context, text string) ([]recognizers.Entity, error) {
	entities, err := s.analyzer.Analyze(ctx, text)
	if err != nil {
		return nil, err
	}
	for i := range entities {
		if entities[i].Type == recognizers.EntityCreditCard && national.IsThaiIDSpan(text, entities[i].Start, entities[i].End) {
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

// PseudonymizeAll detects PII in each text and pseudonymizes all of them with one
// mapping, so a value that recurs in any text gets the same placeholder. It is
// for inputs split into texts, such as table cells. See Pseudonymize.
func (s *Scrubber) PseudonymizeAll(ctx context.Context, texts []string) ([]string, map[string]string, error) {
	entities := make([][]recognizers.Entity, len(texts))
	for i, text := range texts {
		found, err := s.Analyze(ctx, text)
		if err != nil {
			return nil, nil, err
		}
		entities[i] = found
	}
	return s.anonymizer.PseudonymizeAll(texts, entities)
}
