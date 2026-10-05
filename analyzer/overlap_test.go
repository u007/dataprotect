package analyzer

import (
	"context"
	"testing"

	"dataprotect/recognizers"
	"dataprotect/recognizers/checksum"
	"dataprotect/recognizers/regex"
)

// newRealAnalyzer wires the actual recognizers, so overlap resolution is
// exercised against spans that genuinely collide in production text rather than
// against synthetic stubs.
func newRealAnalyzer() *Analyzer {
	a := New()
	a.AddRecognizer(regex.NewEmailRecognizer())
	a.AddRecognizer(regex.NewPhoneRecognizer())
	a.AddRecognizer(regex.NewCreditCardRecognizer())
	a.AddRecognizer(regex.NewSSNRecognizer())
	a.AddRecognizer(regex.NewIPRecognizer())
	a.AddRecognizer(regex.NewIBANRecognizer())
	a.AddRecognizer(checksum.NewLuhnValidator())
	a.AddRecognizer(checksum.NewIBANValidator())
	return a
}

// newDefaultAnalyzer mirrors the recognizer set that piiscrub.NewScrubber wires:
// regex recognizers only, NO checksum validators. It is a separate configuration
// on purpose. With the Luhn validator present the card scores 0.99 and beats the
// phone's 0.98 in overlap resolution, which masks a phone pattern that is too
// greedy — so the default set is the one that actually constrains the phone
// recognizer, and it must be tested separately.
func newDefaultAnalyzer() *Analyzer {
	a := New()
	a.AddRecognizer(regex.NewEmailRecognizer())
	a.AddRecognizer(regex.NewPhoneRecognizer())
	a.AddRecognizer(regex.NewCreditCardRecognizer())
	a.AddRecognizer(regex.NewSSNRecognizer())
	a.AddRecognizer(regex.NewIPRecognizer())
	a.AddRecognizer(regex.NewIBANRecognizer())
	return a
}

func typesOf(entities []recognizers.Entity) map[recognizers.EntityType]int {
	out := map[recognizers.EntityType]int{}
	for _, e := range entities {
		out[e.Type]++
	}
	return out
}

// A 16-digit card sits inside the digit range a phone pattern would otherwise
// claim. The card must win, and the phone must not steal the first ten digits.
func TestRealOverlapsCardBeatsPhone(t *testing.T) {
	a := newRealAnalyzer()
	entities, err := a.Analyze(context.Background(), "pay with card 4111111111111111 ok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := typesOf(entities)
	if got[recognizers.EntityCreditCard] != 1 {
		t.Errorf("want exactly 1 CREDIT_CARD, got %v", got)
	}
	if got[recognizers.EntityPhone] != 0 {
		t.Errorf("phone matched digits belonging to a card: %v", got)
	}
	for _, e := range entities {
		if e.Text != "pay with card 4111111111111111 ok"[e.Start:e.End] {
			t.Errorf("span mismatch for %+v", e)
		}
	}
}

// An IBAN embeds long alphanumeric runs whose digits can match SSN or phone
// patterns. Only the IBAN may survive.
func TestRealOverlapsIBANBeatsDigitPatterns(t *testing.T) {
	a := newRealAnalyzer()
	text := "pay to IBAN GB82 WEST 1234 5698 7654 32 today"
	entities, err := a.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := typesOf(entities)
	if got[recognizers.EntityIBAN] != 1 {
		t.Errorf("want exactly 1 IBAN, got %v (%+v)", got, entities)
	}
	if got[recognizers.EntitySSN]+got[recognizers.EntityPhone]+got[recognizers.EntityCreditCard] != 0 {
		t.Errorf("a digit pattern matched inside the IBAN: %v", got)
	}
}

// A bare 10-digit run is a phone number, not a card, even though it satisfies
// neither checksum. Guards the other direction: over-eager card matching.
func TestRealOverlapsBareDigitsArePhone(t *testing.T) {
	a := newRealAnalyzer()
	entities, err := a.Analyze(context.Background(), "call 5551234567 now")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := typesOf(entities)
	if got[recognizers.EntityPhone] != 1 {
		t.Errorf("want exactly 1 PHONE, got %v (%+v)", got, entities)
	}
	if got[recognizers.EntityCreditCard] != 0 {
		t.Errorf("a 10-digit phone was also reported as a card: %v", got)
	}
}

// Overlapping spans must never both survive: a downstream operator would splice
// the same region twice and corrupt the surrounding text.
func TestRealOverlapsLeaveNoNestedSpans(t *testing.T) {
	a := newRealAnalyzer()
	tests := []string{
		"card 4111111111111111 end",
		"IBAN GB82 WEST 1234 5698 7654 32 end",
		"ssn 123-45-6789 card 4111111111111111",
		"mail a@b.com call 5551234567 card 4111111111111111 ip 8.8.8.8",
	}
	for _, text := range tests {
		entities, err := a.Analyze(context.Background(), text)
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", text, err)
		}
		for i := 0; i < len(entities); i++ {
			for j := i + 1; j < len(entities); j++ {
				a1, b1 := entities[i].Start, entities[i].End
				a2, b2 := entities[j].Start, entities[j].End
				if a1 < b2 && a2 < b1 {
					t.Errorf("%q: overlapping entities %+v and %+v", text, entities[i], entities[j])
				}
			}
		}
	}
}

// Two recognizers matching the same span must collapse to exactly one entity.
func TestRealOverlapsSameSpanCollapses(t *testing.T) {
	a := New()
	a.AddRecognizer(regex.NewCreditCardRecognizer())
	a.AddRecognizer(checksum.NewLuhnValidator())

	// Both recognizers accept this span; the higher confidence wins.
	entities, err := a.Analyze(context.Background(), "4111111111111111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("want 1 entity after collapse, got %d: %+v", len(entities), entities)
	}
	if entities[0].Type != recognizers.EntityCreditCard {
		t.Errorf("kept %q, want CREDIT_CARD", entities[0].Type)
	}
	if entities[0].Confidence != 0.99 {
		t.Errorf("kept confidence %f, want the Luhn validator's 0.99", entities[0].Confidence)
	}
}

// The phone recognizer must not match a digit run that belongs to a longer
// number. This is the assertion the whole \b anchoring rests on, and it is
// checked at the recognizer level because overlap resolution can hide it: in a
// configuration with the Luhn validator the card outscores the phone and the
// greedy phone match never surfaces.
func TestPhoneRecognizerDoesNotClaimCardDigits(t *testing.T) {
	r := regex.NewPhoneRecognizer()
	ctx := context.Background()

	for _, text := range []string{
		"4111111111111111",
		"pay with card 4111111111111111 ok",
		"4111 1111 1111 1111",
	} {
		entities, err := r.Recognize(ctx, text)
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", text, err)
		}
		if len(entities) != 0 {
			t.Errorf("%q: phone matched %d time(s): %+v", text, len(entities), entities)
		}
	}
}

// Under the DEFAULT (no checksum) recognizer set the card scores 0.95 and the
// phone 0.98, so a greedy phone pattern would win and the card would be
// anonymized with the phone's mask pattern. This pins the end-to-end outcome.
func TestDefaultAnalyzerCardNotStolenByPhone(t *testing.T) {
	a := newDefaultAnalyzer()
	entities, err := a.Analyze(context.Background(), "pay with card 4111111111111111 ok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := typesOf(entities)
	if got[recognizers.EntityCreditCard] != 1 {
		t.Errorf("want exactly 1 CREDIT_CARD, got %v", got)
	}
	if got[recognizers.EntityPhone] != 0 {
		t.Errorf("phone stole the card's digits: %v (%+v)", got, entities)
	}
}
