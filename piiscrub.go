// Package piiscrub provides a simple API for PII detection and anonymization.
package piiscrub

import (
	"context"

	"dataprotect/analyzer"
	"dataprotect/anonymizer"
	"dataprotect/operators"
	"dataprotect/recognizers"
	"dataprotect/recognizers/regex"
)

// Scrubber provides a high-level API for PII detection and anonymization.
type Scrubber struct {
	analyzer   *analyzer.Analyzer
	anonymizer *anonymizer.Anonymizer
}

// NewScrubber creates a new scrubber with default recognizers and operators.
func NewScrubber() *Scrubber {
	a := analyzer.New(analyzer.WithMinConfidence(0.7))
	a.AddRecognizer(regex.NewEmailRecognizer())
	a.AddRecognizer(regex.NewPhoneRecognizer())
	a.AddRecognizer(regex.NewCreditCardRecognizer())
	a.AddRecognizer(regex.NewSSNRecognizer())
	a.AddRecognizer(regex.NewIPRecognizer())
	a.AddRecognizer(regex.NewURLRecognizer())
	a.AddRecognizer(regex.NewIBANRecognizer())
	a.AddRecognizer(regex.NewBitcoinRecognizer())

	an := anonymizer.New()
	an.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))
	an.SetOperator(recognizers.EntityPhone, operators.NewMaskOperator("***-***-####"))
	an.SetOperator(recognizers.EntityCreditCard, operators.NewMaskOperator("****-****-****-####"))
	an.SetOperator(recognizers.EntitySSN, operators.NewMaskOperator("***-**-####"))
	an.SetOperator(recognizers.EntityIP, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityURL, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityIBAN, operators.NewRedactOperator())
	an.SetOperator(recognizers.EntityBitcoin, operators.NewRedactOperator())

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
