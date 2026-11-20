package piiscrub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/u007/dataprotect/analyzer"
	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/checksum"
	"github.com/u007/dataprotect/recognizers/judge"
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

func TestScrubberAnonymizeRedactionTyped(t *testing.T) {
	s := NewScrubber()
	ctx := context.Background()

	text := "my IP is 8.8.8.8 and my URL is https://example.com"
	got, err := s.Anonymize(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// IP and URL are redacted types; the output must say what type was removed.
	for _, leak := range []string{"8.8.8.8", "https://example.com"} {
		if strings.Contains(got, leak) {
			t.Errorf("output still contains %q: %q", leak, got)
		}
	}
	if !strings.Contains(got, "[[redactedipaddress1]]") {
		t.Errorf("IP typed marker missing: %q", got)
	}
	if !strings.Contains(got, "[[redactedurl1]]") {
		t.Errorf("URL typed marker missing: %q", got)
	}
}

func TestScrubberRedact(t *testing.T) {
	s := NewScrubber()
	ctx := context.Background()

	text := "Call 555-123-4567 or email a@b.com; SSN 123-45-6789."
	got, err := s.Redact(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, leak := range []string{"555-123-4567", "a@b.com", "123-45-6789"} {
		if strings.Contains(got, leak) {
			t.Errorf("output still contains %q: %q", leak, got)
		}
	}
	if !strings.Contains(got, "[[redactedphone1]]") {
		t.Errorf("phone marker missing: %q", got)
	}
	if !strings.Contains(got, "[[redactedemail1]]") {
		t.Errorf("email marker missing: %q", got)
	}
	if !strings.Contains(got, "[[redactedsocialnumber1]]") {
		t.Errorf("SSN marker missing: %q", got)
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
	if out != "Citizen ID [[redactedthid1]] on file" {
		t.Errorf("got %q, want the Thai ID fully redacted", out)
	}
}

func TestRawLuhnKeepsCardAboveThaiThreshold(t *testing.T) {
	// After the prefix check, the raw Luhn validator no longer reports the
	// Thai-shaped run (first digit 1), so a card-only caller sees nothing.
	// Typing is now the THIDRecognizer's job.
	a := analyzer.New(analyzer.WithMinConfidence(0.98))
	a.AddRecognizer(checksum.NewLuhnValidator())
	entities, err := a.Analyze(context.Background(), "Citizen ID 1000000000009 on file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Fatalf("got %+v, want no entities", entities)
	}
}

func TestScrubberTypesLuhnValidThaiIDAsTHID(t *testing.T) {
	// Scrubber.Analyze retypes the resolved card span, so the Thai ID keeps the
	// card's 0.99 and is reported as TH_ID.
	entities, err := NewScrubber().Analyze(context.Background(), "Citizen ID 1000000000009 on file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 || entities[0].Type != recognizers.EntityTHID {
		t.Fatalf("got %+v, want one TH_ID", entities)
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

// A dash-printed IBAN is redacted whole: no block of it stays visible.
func TestScrubberRedactsDashSeparatedIBAN(t *testing.T) {
	s := NewScrubber()
	out, err := s.Anonymize(context.Background(), "pay AL47-2121-1009-0000-0002-3569-8741 now")
	if err != nil {
		t.Fatalf("Anonymize: %v", err)
	}
	for _, h := range []string{"AL47", "2121", "3569", "8741"} {
		if strings.Contains(out, h) {
			t.Errorf("Anonymize = %q, %q is still visible", out, h)
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

// The default phone and card masks show the last four digits of the value,
// not four digits from the middle of a longer one.
func TestScrubberMasksKeepLastFourDigits(t *testing.T) {
	s := NewScrubber()
	for _, tc := range []struct{ text, want string }{
		{"call +44 20 7946 0958 now", "call ***-***-0958 now"},
		{"call (555) 867-5309 now", "call ***-***-5309 now"},
		{"card 4000000000000000006 now", "card ****-****-****-0006 now"},
	} {
		out, err := s.Anonymize(context.Background(), tc.text)
		if err != nil {
			t.Fatalf("Anonymize(%q): %v", tc.text, err)
		}
		if out != tc.want {
			t.Errorf("Anonymize(%q) = %q, want %q", tc.text, out, tc.want)
		}
	}
}

// A card that follows a dash-printed IBAN is redacted. A chance-passing card
// window that starts on an IBAN digit must not win the overlap and hide the card.
func TestScrubberRedactsCardAfterDashIBAN(t *testing.T) {
	s := NewScrubber()
	out, err := s.Anonymize(context.Background(), "AL47-2121-1009-0000-0002-3569-8741-4111-1111-1111-1111 end")
	if err != nil {
		t.Fatalf("Anonymize: %v", err)
	}
	if want := "[[redactediban1]]-****-****-****-1111 end"; out != want {
		t.Errorf("Anonymize = %q, want %q", out, want)
	}
}

// A card followed by a zero group is still redacted as a card.
func TestScrubberRedactsCardBeforeZeroGroup(t *testing.T) {
	s := NewScrubber()
	out, err := s.Anonymize(context.Background(), "card 4111 1111 1111 1111 0000 end")
	if err != nil {
		t.Fatalf("Anonymize: %v", err)
	}
	if want := "card ****-****-****-1111 0000 end"; out != want {
		t.Errorf("Anonymize = %q, want %q", out, want)
	}
}

// Spellings of one value share a placeholder: spacing in a card, case in an
// email, IP notation, and separators in an IBAN and a phone. A URL is compared
// exactly, so two case variants stay two placeholders.
func TestPseudonymizeSharesCanonicalValues(t *testing.T) {
	s := NewScrubber()
	for _, tc := range []struct {
		text, want string
		restored   map[string]string
	}{
		{
			"card 4111 1111 1111 1111 or 4111111111111111 or 4111-1111-1111-1111 end",
			"card [CREDIT_CARD_1] or [CREDIT_CARD_1] or [CREDIT_CARD_1] end",
			map[string]string{"[CREDIT_CARD_1]": "4111 1111 1111 1111"},
		},
		{
			"mail John@Example.com or john@example.com end",
			"mail [EMAIL_1] or [EMAIL_1] end",
			map[string]string{"[EMAIL_1]": "John@Example.com"},
		},
		{
			"call 555-123-4567 or 5551234567 end",
			"call [PHONE_1] or [PHONE_1] end",
			map[string]string{"[PHONE_1]": "555-123-4567"},
		},
		{
			"pay DE89 3704 0044 0532 0130 00 or DE89370400440532013000 end",
			"pay [IBAN_1] or [IBAN_1] end",
			map[string]string{"[IBAN_1]": "DE89 3704 0044 0532 0130 00"},
		},
		{
			"host 2001:DB8::1 or 2001:db8:0::1 end",
			"host [IP_ADDRESS_1] or [IP_ADDRESS_1] end",
			map[string]string{"[IP_ADDRESS_1]": "2001:DB8::1"},
		},
		{
			"see https://Example.com/a and https://example.com/a end",
			"see [URL_1] and [URL_2] end",
			map[string]string{"[URL_1]": "https://Example.com/a", "[URL_2]": "https://example.com/a"},
		},
	} {
		out, pseudonyms, err := s.Pseudonymize(context.Background(), tc.text)
		if err != nil {
			t.Fatalf("Pseudonymize(%q): %v", tc.text, err)
		}
		if out != tc.want {
			t.Errorf("Pseudonymize(%q) = %q, want %q", tc.text, out, tc.want)
		}
		if len(pseudonyms) != len(tc.restored) {
			t.Errorf("Pseudonymize(%q) map = %v, want %v", tc.text, pseudonyms, tc.restored)
		}
		for k, v := range tc.restored {
			if pseudonyms[k] != v {
				t.Errorf("Pseudonymize(%q) map[%s] = %q, want %q", tc.text, k, pseudonyms[k], v)
			}
		}
	}
}

// A text with no PII that already holds a placeholder-shaped token is an error
// too. The shared map would rewrite that token when the texts are restored.
func TestPseudonymizeAllRejectsLiteralPlaceholderInPIIFreeText(t *testing.T) {
	s := NewScrubber()
	outs, pseudonyms, err := s.PseudonymizeAll(context.Background(), []string{"mail a@b.com", "see [EMAIL_1] now"})
	if err == nil {
		t.Errorf("PseudonymizeAll = %q %v, want a placeholder-collision error", outs, pseudonyms)
	}
	if outs != nil || pseudonyms != nil {
		t.Errorf("on error PseudonymizeAll returned %q %v, want nil and nil", outs, pseudonyms)
	}
}

// PseudonymizeAll shares one mapping across texts, such as table cells, so a value
// that recurs in a later cell gets the placeholder it got in an earlier one.
func TestPseudonymizeAllSharesMappingAcrossTexts(t *testing.T) {
	s := NewScrubber()
	texts := []string{"a 555-123-4567", "b 5551234567", "c 555-999-0000"}
	outs, pseudonyms, err := s.PseudonymizeAll(context.Background(), texts)
	if err != nil {
		t.Fatalf("PseudonymizeAll: %v", err)
	}
	want := []string{"a [PHONE_1]", "b [PHONE_1]", "c [PHONE_2]"}
	for i := range want {
		if outs[i] != want[i] {
			t.Errorf("outs[%d] = %q, want %q", i, outs[i], want[i])
		}
	}
	if len(pseudonyms) != 2 {
		t.Errorf("map = %v, want two placeholders", pseudonyms)
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

// A Luhn-failing card named by a label must be redacted as a card, not as the
// phone number that overlaps its tail. The phone recognizer claims the last
// group and scores 0.98 against the gated card's 0.85, so without an explicit
// card-over-phone precedence the first group stays visible and the mask shown is
// the phone's. Both cases are the gold spans of nemotron-pii-000709 and
// nemotron-pii-001744.
func TestScrubberRedactsGatedCardNotItsPhoneTail(t *testing.T) {
	s := NewScrubber()
	for _, tc := range []struct {
		text    string
		hidden  []string
		visible []string
	}{
		{
			"Credit/Debit Card 3467 284599 8675 will be used",
			[]string{"3467", "284599"},
			[]string{"****-****-****-"},
		},
		{
			"the credit debit card 3014 754803 2190 is on file",
			[]string{"3014", "754803"},
			[]string{"****-****-****-"},
		},
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
				t.Errorf("Anonymize(%q) = %q, want card mask %q", tc.text, out, v)
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

// stubLLMJudge is a test-only Judge implementation for scrubber-level tests.
type stubLLMJudge struct {
	confirm bool
	err     error
}

func (s *stubLLMJudge) Judge(ctx context.Context, entityType recognizers.EntityType, candidate, window string) (bool, error) {
	return s.confirm, s.err
}

// TestScrubberBareMyKadDroppedWithoutJudge verifies that a bare 12-digit MyKad
// with no identity-card keyword is dropped when no LLM judge is configured.
// Default behaviour is byte-identical to running without the weak-candidate
// path.
func TestScrubberBareMyKadDroppedWithoutJudge(t *testing.T) {
	s := NewScrubber()
	ctx := context.Background()

	text := "ref 900515103421 on the form"
	out, err := s.Anonymize(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != text {
		t.Errorf("output was modified: %q, want unchanged %q", out, text)
	}

	// Analyze should also return nothing for the weak candidate.
	entities, err := s.Analyze(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0 (weak candidate dropped without judge)", len(entities))
	}
}

// TestScrubberBareMyKadConfirmedByJudge verifies that a bare 12-digit MyKad
// confirmed by the LLM judge is reported with keyword-gated confidence.
func TestScrubberBareMyKadConfirmedByJudge(t *testing.T) {
	s := NewScrubber(WithLLMJudge(&stubLLMJudge{confirm: true}))
	ctx := context.Background()

	text := "ref 900515103421 on the form"
	entities, err := s.Analyze(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("got %d entities, want 1", len(entities))
	}
	if entities[0].Type != recognizers.EntityMYIC {
		t.Errorf("type = %v, want MY_IC", entities[0].Type)
	}
	if entities[0].Text != "900515103421" {
		t.Errorf("text = %q, want 900515103421", entities[0].Text)
	}
	if entities[0].Confidence != 0.85 {
		t.Errorf("confidence = %v, want 0.85 (keyword-gated)", entities[0].Confidence)
	}
	if entities[0].Weak {
		t.Errorf("Weak = true, want false (confirmed candidate is no longer weak)")
	}
}

// TestScrubberBareMyKadRejectedByJudge verifies that a bare 12-digit MyKad
// rejected by the LLM judge is dropped, even with minConfidence at 0.
func TestScrubberBareMyKadRejectedByJudge(t *testing.T) {
	s := NewScrubber(WithMinConfidence(0.0), WithLLMJudge(&stubLLMJudge{confirm: false}))
	ctx := context.Background()

	text := "order 900515103421 shipped"
	entities, err := s.Analyze(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0 (weak candidate rejected by judge)", len(entities))
	}
}

// TestScrubberBareMyKadDroppedBelowFloor verifies that weak candidates are
// dropped even when minConfidence is at or below the weak score, so long as no
// judge is configured.
func TestScrubberBareMyKadDroppedBelowFloor(t *testing.T) {
	s := NewScrubber(WithMinConfidence(0.50))
	ctx := context.Background()

	text := "ref 900515103421 on the form"
	entities, err := s.Analyze(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0 (weak candidate at 0.50 dropped below floor)", len(entities))
	}
}

// TestScrubberWithLLMJudgeNil verifies that passing nil to WithLLMJudge is a
// no-op and weak candidates are still dropped.
func TestScrubberWithLLMJudgeNil(t *testing.T) {
	s := NewScrubber(WithMinConfidence(0.0), WithLLMJudge(nil))
	ctx := context.Background()

	text := "ref 900515103421 on the form"
	entities, err := s.Analyze(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0 (nil judge drops weak candidates)", len(entities))
	}
}

// TestScrubberWithLLMJudgeTypedNil verifies that a typed-nil judge is treated
// as disabled.
func TestScrubberWithLLMJudgeTypedNil(t *testing.T) {
	var j *stubLLMJudge // nil pointer
	s := NewScrubber(WithMinConfidence(0.0), WithLLMJudge(j))
	ctx := context.Background()

	text := "ref 900515103421 on the form"
	entities, err := s.Analyze(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities, want 0 (typed-nil judge disabled)", len(entities))
	}
}

// TestScrubberBareMyKadEndToEndJudge wires the real recognizers/judge package
// to a Scrubber through an httptest server: yes promotes the bare MyKad to
// 0.85, no drops it.
func TestScrubberBareMyKadEndToEndJudge(t *testing.T) {
	for _, confirm := range []bool{true, false} {
		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Content-Type", "application/json")
			answer := "no"
			if confirm {
				answer = "yes"
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []interface{}{
					map[string]interface{}{
						"message": map[string]interface{}{
							"content": answer,
						},
					},
				},
			})
		}))
		t.Cleanup(srv.Close)

		j, err := judge.New(srv.URL)
		if err != nil {
			t.Fatalf("judge.New: %v", err)
		}

		s := NewScrubber(WithMinConfidence(0.0), WithLLMJudge(j))
		ctx := context.Background()

		text := "ref 900515103421 on the form"
		entities, err := s.Analyze(ctx, text)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if confirm {
			if len(entities) != 1 {
				t.Fatalf("confirm=true: got %d entities, want 1", len(entities))
			}
			if entities[0].Confidence != 0.85 {
				t.Errorf("confirm=true: confidence = %v, want 0.85", entities[0].Confidence)
			}
			if entities[0].Weak {
				t.Errorf("confirm=true: Weak = true, want false")
			}
		} else {
			if len(entities) != 0 {
				t.Errorf("confirm=false: got %d entities, want 0", len(entities))
			}
		}
		if calls != 1 {
			t.Errorf("confirm=%v: judge calls = %d, want 1", confirm, calls)
		}
	}
}
