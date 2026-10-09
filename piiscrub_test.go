package piiscrub

import (
	"context"
	"strings"
	"testing"

	"dataprotect/analyzer"
	"dataprotect/recognizers"
	"dataprotect/recognizers/checksum"
)

func TestScrubberAnonymize(t *testing.T) {
	s := NewScrubber()
	ctx := context.Background()

	text := "Contact john@example.com or call 555-123-4567. Card 4111111111111111."
	got, err := s.Anonymize(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, leak := range []string{"john@example.com", "555-123-4567", "4111111111111111"} {
		if strings.Contains(got, leak) {
			t.Errorf("output still contains %q: %q", leak, got)
		}
	}
	if !strings.Contains(got, "<EMAIL>") {
		t.Errorf("email was not replaced: %q", got)
	}
	if !strings.Contains(got, "123-4567") && !strings.Contains(got, "4567") {
		t.Errorf("phone tail was not preserved: %q", got)
	}
	if !strings.Contains(got, "1111") {
		t.Errorf("card tail was not preserved: %q", got)
	}
	if !strings.HasPrefix(got, "Contact ") {
		t.Errorf("surrounding prose was damaged: %q", got)
	}
}

func TestScrubberAnalyze(t *testing.T) {
	s := NewScrubber()
	ctx := context.Background()

	text := "mail a@b.com about card 4111111111111111"
	entities, err := s.Analyze(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	types := map[recognizers.EntityType]bool{}
	for _, e := range entities {
		types[e.Type] = true
		if e.Text != text[e.Start:e.End] {
			t.Errorf("span mismatch for %+v", e)
		}
	}
	if !types[recognizers.EntityEmail] {
		t.Errorf("email not detected, got types %v", types)
	}
	// The card is recognized as a credit card; it may also match phone and SSN
	// patterns, and overlap resolution keeps only the highest-confidence one.
	if !types[recognizers.EntityCreditCard] {
		t.Errorf("credit card not detected, got types %v", types)
	}
}

func TestScrubberNoPII(t *testing.T) {
	s := NewScrubber()
	text := "The weather is nice today."
	got, err := s.Anonymize(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != text {
		t.Errorf("text without PII was modified: %q", got)
	}
}

func TestScrubberEmpty(t *testing.T) {
	s := NewScrubber()
	got, err := s.Anonymize(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// The checksum validators must be part of the default pipeline. Without the
// Luhn validator, regex.CreditCardRecognizer reports ANY 13-19 digit run as a
// card, so a number that fails Luhn is anonymized as a card number it is not —
// and worse, a random long digit string is redacted as if it were payment data.
//
// This asserts the wiring, not the arithmetic: LuhnValidate and
// IBANValidate are unit-tested in the checksum package.
func TestNewScrubberIncludesChecksumValidators(t *testing.T) {
	s := NewScrubber()
	var names []string
	for _, r := range s.analyzer.Recognizers() {
		names = append(names, r.Name())
	}
	want := map[string]bool{
		"checksum_luhn": false,
		"checksum_iban": false,
	}
	for _, n := range names {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for n, found := range want {
		if !found {
			t.Errorf("NewScrubber does not register %s; got recognizers %v", n, names)
		}
	}
}

// A Luhn-FAILING card number must not be reported as a card. This is the
// end-to-end consequence of registering the validator: the checksum rejects it
// and the regex recognizer must not paper over that by matching the shape.
//
// 4111 1111 1111 1112 differs from the valid 4111 1111 1111 1111 in its final
// digit, and fails the Luhn check.
func TestScrubberRejectsLuhnFailingCard(t *testing.T) {
	s := NewScrubber()
	text := "Test fixture 4111 1111 1111 1112 is not a real card."

	entities, err := s.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range entities {
		if e.Type == recognizers.EntityCreditCard {
			t.Errorf("reported a Luhn-failing number as a CREDIT_CARD: %q (span %d-%d)",
				e.Text, e.Start, e.End)
		}
	}
}

// The positive case must still be detected, or the rejection above would be
// satisfied by breaking cards entirely rather than by validating them.
func TestScrubberAcceptsLuhnValidCard(t *testing.T) {
	s := NewScrubber()
	text := "The customer charged 4111 1111 1111 1111 twice."

	entities, err := s.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range entities {
		if e.Type == recognizers.EntityCreditCard {
			found = true
			if got := text[e.Start:e.End]; got != "4111 1111 1111 1111" {
				t.Errorf("card span = %q, want %q", got, "4111 1111 1111 1111")
			}
		}
	}
	if !found {
		t.Errorf("a Luhn-VALID card was not detected: %+v", entities)
	}
}

// A spaced, mod-97-valid IBAN must be found end to end. The checksum validator's
// candidate finder previously truncated this exact shape and dropped it
// silently; this pins the user-visible outcome, not just the validator.
func TestScrubberFindsSpacedIBAN(t *testing.T) {
	s := NewScrubber()
	text := "pay to GB82 WEST 1234 5698 7654 32 today"

	entities, err := s.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, e := range entities {
		if e.Type == recognizers.EntityIBAN {
			found = true
			if got := text[e.Start:e.End]; got != "GB82 WEST 1234 5698 7654 32" {
				t.Errorf("IBAN span = %q, want %q", got, "GB82 WEST 1234 5698 7654 32")
			}
		}
	}
	if !found {
		t.Errorf("a valid spaced IBAN was not detected: %+v", entities)
	}
}

func TestScrubberFindsMYPassportWithKeyword(t *testing.T) {
	s := NewScrubber()
	text := "Passport Number: A12345678 was copied; 322996077 is a different record."

	entities, err := s.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 || entities[0].Type != recognizers.EntityMYPassport {
		t.Fatalf("want one MY_PASSPORT and nothing else, got %+v", entities)
	}
	if got := text[entities[0].Start:entities[0].End]; got != "A12345678" {
		t.Errorf("MY_PASSPORT span = %q, want %q", got, "A12345678")
	}
}

func TestScrubberBitcoinSpanNotDisplacedByEmbeddedNRIC(t *testing.T) {
	// The bitcoin span is shape-only at 0.95. An NRIC-shaped run at its end must
	// not win the overlap and leave the address prefix visible.
	s := NewScrubber()
	text := "send 1Ab3CdEf5Gh7Jk9Mn2Pq4RsS1234567D now"
	entities, err := s.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 || entities[0].Type != recognizers.EntityBitcoin {
		t.Fatalf("want one BITCOIN and nothing else, got %+v", entities)
	}
	if got := text[entities[0].Start:entities[0].End]; got != "1Ab3CdEf5Gh7Jk9Mn2Pq4RsS1234567D" {
		t.Errorf("BITCOIN span = %q, want the whole token", got)
	}
}

func TestScrubberRedactsLuhnValidThaiIDAsThaiID(t *testing.T) {
	// A Thai ID that also passes Luhn is typed TH_ID and fully redacted, not
	// masked as a card with its check digit showing.
	s := NewScrubber()
	out, err := s.Anonymize(context.Background(), "Citizen ID 1000000000009 on file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Citizen ID [REDACTED] on file" {
		t.Errorf("got %q, want the Thai ID fully redacted", out)
	}
}

func TestLuhnThaiIDSurvivesHighThresholdWithoutThaiRecognizer(t *testing.T) {
	// A Luhn-valid Thai ID must stay redacted when the Thai recognizer is not
	// wired and the caller raises the confidence threshold above 0.95.
	a := analyzer.New(analyzer.WithMinConfidence(0.98))
	a.AddRecognizer(checksum.NewLuhnValidator())
	entities, err := a.Analyze(context.Background(), "Citizen ID 1000000000009 on file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 || entities[0].Type != recognizers.EntityTHID {
		t.Fatalf("got %+v, want one TH_ID", entities)
	}
}
