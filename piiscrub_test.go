package piiscrub

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/u007/dataprotect/analyzer"
	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/checksum"
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

func TestScrubberFindsSGPassportWithKeyword(t *testing.T) {
	s := NewScrubber()
	text := "Passport Number: E1234567A was copied; 1234567A is a different record."

	entities, err := s.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 || entities[0].Type != recognizers.EntitySGPassport {
		t.Fatalf("want one SG_PASSPORT and nothing else, got %+v", entities)
	}
	if got := text[entities[0].Start:entities[0].End]; got != "E1234567A" {
		t.Errorf("SG_PASSPORT span = %q, want %q", got, "E1234567A")
	}
}

func TestScrubberBitcoinSpanNotDisplacedByEmbeddedNRIC(t *testing.T) {
	// A valid address followed by an NRIC-shaped run. The bitcoin span must stay
	// whole and the NRIC must still be found, so neither wins the overlap over
	// the other's text.
	s := NewScrubber()
	text := "send 1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa and S1234567D now"
	entities, err := s.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 2 {
		t.Fatalf("want one BITCOIN and one SG_NRIC, got %+v", entities)
	}
	byType := map[recognizers.EntityType]string{}
	for _, e := range entities {
		byType[e.Type] = text[e.Start:e.End]
	}
	if got := byType[recognizers.EntityBitcoin]; got != "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa" {
		t.Errorf("BITCOIN span = %q, want the whole address", got)
	}
	if got := byType[recognizers.EntitySGNRIC]; got != "S1234567D" {
		t.Errorf("SG_NRIC span = %q, want %q", got, "S1234567D")
	}
}

func TestScrubberBadChecksumTokenIsNotBitcoin(t *testing.T) {
	// The shape of a bitcoin address with a failing checksum is not an address,
	// so it is not reported. The NRIC-shaped tail is glued to letters, which the
	// NRIC rule does not match without its label (see TODO.md).
	s := NewScrubber()
	entities, err := s.Analyze(context.Background(), "send 1Ab3CdEf5Gh7Jk9Mn2Pq4RsS1234567D now")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Fatalf("want no entities for a bad-checksum token, got %+v", entities)
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

func TestRawLuhnKeepsCardAboveThaiThreshold(t *testing.T) {
	// The raw Luhn validator reports a card at 0.99, and a threshold of 0.98
	// keeps it. Typing it as TH_ID is the Scrubber's job, so a card-only caller
	// is unaffected by the Thai decision.
	a := analyzer.New(analyzer.WithMinConfidence(0.98))
	a.AddRecognizer(checksum.NewLuhnValidator())
	entities, err := a.Analyze(context.Background(), "Citizen ID 1000000000009 on file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 || entities[0].Type != recognizers.EntityCreditCard {
		t.Fatalf("got %+v, want one CREDIT_CARD", entities)
	}
}

func TestScrubberTypesLuhnValidThaiIDAsTHID(t *testing.T) {
	// Scrubber.Analyze retypes the resolved card span, so the Thai ID keeps the
	// card's 0.99 and is reported as TH_ID.
	entities, err := NewScrubber().Analyze(context.Background(), "Citizen ID 1000000000009 on file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 || entities[0].Type != recognizers.EntityTHID || entities[0].Confidence != 0.99 {
		t.Fatalf("got %+v, want one TH_ID at 0.99", entities)
	}
}

func TestScrubberEveryEntityTypeHasOperator(t *testing.T) {
	// An entity of each type must come out changed. A type with no operator
	// makes Anonymize fail, so a missing operator cannot slip past unredacted.
	s := NewScrubber()
	for _, typ := range recognizers.AllEntityTypes {
		out, err := s.anonymizer.Anonymize(context.Background(), "x", []recognizers.Entity{
			{Type: typ, Start: 0, End: 1, Text: "x", Confidence: 0.99},
		})
		if err != nil {
			t.Fatalf("type %s: unexpected error: %v", typ, err)
		}
		if out == "x" {
			t.Errorf("type %s has no operator in NewScrubber; its spans would be left unredacted", typ)
		}
	}
}

func TestAllEntityTypesParse(t *testing.T) {
	for _, typ := range recognizers.AllEntityTypes {
		got, err := recognizers.EntityTypeFromString(string(typ))
		if err != nil || got != typ {
			t.Errorf("EntityTypeFromString(%q) = %q, %v; want %q", typ, got, err, typ)
		}
	}
}

// A valid card followed by a space and digits was not redacted at all. The
// card and the digit group formed one run that failed Luhn.
func TestScrubberRedactsCardNextToDigitGroup(t *testing.T) {
	s := NewScrubber()
	for _, text := range []string{
		"card 4111111111111111 123",
		"4111111111111111 2024",
		"4012888888881881 4012888888881881",
	} {
		out, err := s.Anonymize(context.Background(), text)
		if err != nil {
			t.Fatalf("Anonymize(%q): %v", text, err)
		}
		if strings.Contains(out, "4111111111111111") || strings.Contains(out, "4012888888881881") {
			t.Errorf("Anonymize(%q) = %q, a card number is still visible", text, out)
		}
	}
}

// An email with a Unicode domain is redacted whole, and the output stays valid
// UTF-8 (the span is cut on rune boundaries).
func TestScrubberRedactsUnicodeEmail(t *testing.T) {
	s := NewScrubber()
	for _, text := range []string{
		"write юзер@пример.рф today",
		"mail a@example.рф now",
	} {
		out, err := s.Anonymize(context.Background(), text)
		if err != nil {
			t.Fatalf("Anonymize(%q): %v", text, err)
		}
		if !utf8.ValidString(out) {
			t.Errorf("Anonymize(%q) = %q, not valid UTF-8", text, out)
		}
		for _, h := range []string{"пример", "юзер", "example.рф"} {
			if strings.Contains(out, h) {
				t.Errorf("Anonymize(%q) = %q, %q is still visible", text, out, h)
			}
		}
	}
}

// A phone number next to a card or another digit group keeps both values
// hidden. Overlap resolution keeps the higher-confidence span, so a phone span
// that ran into a card would lose to it and leave the phone digits visible.
func TestScrubberRedactsPhoneNextToCardOrDigitGroup(t *testing.T) {
	s := NewScrubber()
	for _, tc := range []struct {
		text   string
		hidden []string
	}{
		{"+55 11 98456 5666 4111 1111 1111 1111", []string{"98456", "5666", "4111111111111111"}},
		{"+1 555 123 4567 4111 1111 1111 1111", []string{"555 123 4567", "4111 1111 1111 1111"}},
		{"+44 20 7946 0958 123-45-6789", []string{"7946 0958", "123-45-6789"}},
	} {
		out, err := s.Anonymize(context.Background(), tc.text)
		if err != nil {
			t.Fatalf("Anonymize(%q): %v", tc.text, err)
		}
		for _, h := range tc.hidden {
			if strings.Contains(out, h) {
				t.Errorf("Anonymize(%q) = %q, %q is still visible", tc.text, out, h)
			}
		}
	}
}

// An IPv6 address is redacted whole, including the IPv4 tail of a mapped address.
// A clock time is not an address and stays visible.
func TestScrubberRedactsIPv6Address(t *testing.T) {
	s := NewScrubber()
	for _, tc := range []struct {
		text    string
		hidden  []string
		visible []string
	}{
		{"host 2001:db8::1 up", []string{"2001:db8"}, nil},
		{"map ::ffff:192.0.2.1 end", []string{"::ffff", "192.0.2.1"}, nil},
		{"meet at 12:30:45 today", nil, []string{"12:30:45"}},
	} {
		out, err := s.Anonymize(context.Background(), tc.text)
		if err != nil {
			t.Fatalf("Anonymize(%q): %v", tc.text, err)
		}
		for _, h := range tc.hidden {
			if strings.Contains(out, h) {
				t.Errorf("Anonymize(%q) = %q, %q is still visible", tc.text, out, h)
			}
		}
		for _, v := range tc.visible {
			if !strings.Contains(out, v) {
				t.Errorf("Anonymize(%q) = %q, %q was redacted", tc.text, out, v)
			}
		}
	}
}

// Cards that the IBAN guard or a chance-passing window used to leave visible.
func TestScrubberRedactsCardAfterLettersAndChancePrefix(t *testing.T) {
	s := NewScrubber()
	for _, text := range []string{
		"Ref AB12 4111 1111 1111 1111",
		"ID01 4111111111111111 123",
		"1004 4111 1111 1111 1111",
	} {
		out, err := s.Anonymize(context.Background(), text)
		if err != nil {
			t.Fatalf("Anonymize(%q): %v", text, err)
		}
		if strings.Contains(out, "4111") || strings.Contains(out, "111111111111") {
			t.Errorf("Anonymize(%q) = %q, card digits are still visible", text, out)
		}
	}
}

// restorePlaceholders is the inverse of Pseudonymize: each placeholder back to
// its original value.
func restorePlaceholders(out string, pseudonyms map[string]string) string {
	var pairs []string
	for placeholder, original := range pseudonyms {
		pairs = append(pairs, placeholder, original)
	}
	return strings.NewReplacer(pairs...).Replace(out)
}

func TestScrubberPseudonymize(t *testing.T) {
	s := NewScrubber()
	text := "Contact a@example.com or a@example.com, or b@example.org. Card 4111111111111111. NRIC S1234567D."
	out, pseudonyms, err := s.Pseudonymize(context.Background(), text)
	if err != nil {
		t.Fatalf("Pseudonymize: %v", err)
	}
	want := "Contact [EMAIL_1] or [EMAIL_1], or [EMAIL_2]. Card [CREDIT_CARD_1]. NRIC [SG_NRIC_1]."
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	wantMap := map[string]string{
		"[EMAIL_1]":       "a@example.com",
		"[EMAIL_2]":       "b@example.org",
		"[CREDIT_CARD_1]": "4111111111111111",
		"[SG_NRIC_1]":     "S1234567D",
	}
	if len(pseudonyms) != len(wantMap) {
		t.Fatalf("map = %v, want %v", pseudonyms, wantMap)
	}
	for k, v := range wantMap {
		if pseudonyms[k] != v {
			t.Errorf("map[%s] = %q, want %q", k, pseudonyms[k], v)
		}
	}
	if got := restorePlaceholders(out, pseudonyms); got != text {
		t.Errorf("restore(out) = %q, want %q", got, text)
	}
}
