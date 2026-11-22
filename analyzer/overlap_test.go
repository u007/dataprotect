package analyzer

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/checksum"
	"github.com/u007/dataprotect/recognizers/regex"
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

// A Luhn-failing card that a label names still outranks the phone pattern
// overlapping it. The gated card scores 0.85 against the phone's 0.98, so
// confidence alone hands the span to the phone: the phone masks its own sub-run
// and leaves the card's first group in the clear ("Credit/Debit Card 3467
// ***-***-8675"), which is a partial leak of a card number. The phone span is
// always a strict sub-run of the card window, because a card window needs 13 or
// more digits while a phone match needs at most the tail of them, so keeping the
// card loses no coverage.
func TestRealOverlapsGatedCardBeatsPhone(t *testing.T) {
	a := newRealAnalyzer()
	for _, text := range []string{
		"Credit/Debit Card 3467 284599 8675 will be used",
		"the credit debit card 3014 754803 2190 is on file",
	} {
		entities, err := a.Analyze(context.Background(), text)
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", text, err)
		}
		got := typesOf(entities)
		if got[recognizers.EntityCreditCard] != 1 {
			t.Errorf("%q: want exactly 1 CREDIT_CARD, got %v (%+v)", text, got, entities)
			continue
		}
		if got[recognizers.EntityPhone] != 0 {
			t.Errorf("%q: phone stole a span inside a gated card: %v", text, entities)
			continue
		}
		for _, e := range entities {
			if e.Type == recognizers.EntityCreditCard && e.Text != text[e.Start:e.End] {
				t.Errorf("%q: card span is not the whole card number: %q", text, e.Text)
			}
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

// Each keyword-gated finance type names its value, so it beats PHONE (0.98)
// even though it scores lower. The phone span here is a sub-run of the value,
// so a phone winning would leave the label's value partly visible.
func TestFinanceTypesBeatPhoneOnOverlap(t *testing.T) {
	financeTypes := []recognizers.EntityType{
		recognizers.EntitySWIFTBIC,
		recognizers.EntityUSRouting,
		recognizers.EntityCardCVV,
		recognizers.EntityBankAccount,
		recognizers.EntityPIN,
	}
	for _, ft := range financeTypes {
		// Finance span earlier than the phone, then later: the winner must not
		// depend on which span comes first.
		for _, order := range []struct {
			name             string
			finStart, finEnd int
			phStart, phEnd   int
		}{
			{"finance first", 0, 9, 4, 14},
			{"phone first", 4, 14, 0, 9},
		} {
			a := New()
			a.AddRecognizer(&stubRecognizer{name: "fin", entities: []recognizers.Entity{
				{Type: ft, Start: order.finStart, End: order.finEnd, Text: "x", Confidence: 0.85},
			}})
			a.AddRecognizer(&stubRecognizer{name: "phone", entities: []recognizers.Entity{
				{Type: recognizers.EntityPhone, Start: order.phStart, End: order.phEnd, Text: "x", Confidence: 0.98},
			}})
			entities, err := a.Analyze(context.Background(), "0123456789012345")
			if err != nil {
				t.Fatalf("%s/%s: unexpected error: %v", ft, order.name, err)
			}
			if len(entities) != 1 || entities[0].Type != ft {
				t.Errorf("%s/%s: want the finance type alone, got %+v", ft, order.name, entities)
			}
		}
	}
}

// The finance rule is narrow: it is the PHONE pair only. A finance value that
// overlaps a higher-confidence non-phone entity still loses on confidence.
func TestFinanceTypeLosesToHigherConfidenceNonPhone(t *testing.T) {
	a := New()
	a.AddRecognizer(&stubRecognizer{name: "fin", entities: []recognizers.Entity{
		{Type: recognizers.EntityPIN, Start: 0, End: 6, Text: "482913", Confidence: 0.85},
	}})
	a.AddRecognizer(&stubRecognizer{name: "ssn", entities: []recognizers.Entity{
		{Type: recognizers.EntitySSN, Start: 2, End: 9, Text: "x", Confidence: 0.99},
	}})
	entities, err := a.Analyze(context.Background(), "482913 ....")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 || entities[0].Type != recognizers.EntitySSN {
		t.Errorf("want SSN to win on confidence, got %+v", entities)
	}
}

// A CREDIT_CARD beats a finance type on overlap at every score, so a CVV or PIN
// keyword cannot take digits that belong to a card. The CVV here scores higher
// than the card and still loses.
func TestCreditCardBeatsFinanceTypesOnOverlap(t *testing.T) {
	for _, ft := range []recognizers.EntityType{recognizers.EntityCardCVV, recognizers.EntityPIN, recognizers.EntityBankAccount} {
		for _, order := range []struct {
			name               string
			cardStart, cardEnd int
			finStart, finEnd   int
		}{
			{"card first", 0, 16, 12, 16},
			{"finance first", 12, 16, 0, 16},
		} {
			a := New()
			a.AddRecognizer(&stubRecognizer{name: "card", entities: []recognizers.Entity{
				{Type: recognizers.EntityCreditCard, Start: order.cardStart, End: order.cardEnd, Text: "x", Confidence: 0.85},
			}})
			a.AddRecognizer(&stubRecognizer{name: "fin", entities: []recognizers.Entity{
				{Type: ft, Start: order.finStart, End: order.finEnd, Text: "x", Confidence: 0.90},
			}})
			entities, err := a.Analyze(context.Background(), "4111111111111111")
			if err != nil {
				t.Fatalf("%s/%s: unexpected error: %v", ft, order.name, err)
			}
			if len(entities) != 1 || entities[0].Type != recognizers.EntityCreditCard {
				t.Errorf("%s/%s: want CREDIT_CARD alone, got %+v", ft, order.name, entities)
			}
		}
	}
}

// The identity types (TAX_ID, PASSPORT, DRIVER_LICENSE, DATE_OF_BIRTH) are
// keyword-gated like the finance types, so each beats PHONE at 0.98 on either
// side of the overlap, and loses to a CREDIT_CARD at any score.
func TestIdentityTypesBeatPhoneAndLoseToCardOnOverlap(t *testing.T) {
	identityTypes := []recognizers.EntityType{
		recognizers.EntityTaxID,
		recognizers.EntityPassport,
		recognizers.EntityDriverLicense,
		recognizers.EntityDateOfBirth,
	}
	for _, it := range identityTypes {
		for _, order := range []struct {
			name           string
			idStart, idEnd int
			phStart, phEnd int
		}{
			{"identity first", 0, 9, 4, 14},
			{"phone first", 4, 14, 0, 9},
		} {
			a := New()
			a.AddRecognizer(&stubRecognizer{name: "id", entities: []recognizers.Entity{
				{Type: it, Start: order.idStart, End: order.idEnd, Text: "x", Confidence: 0.85},
			}})
			a.AddRecognizer(&stubRecognizer{name: "phone", entities: []recognizers.Entity{
				{Type: recognizers.EntityPhone, Start: order.phStart, End: order.phEnd, Text: "x", Confidence: 0.98},
			}})
			entities, err := a.Analyze(context.Background(), "0123456789012345")
			if err != nil {
				t.Fatalf("%s/%s: unexpected error: %v", it, order.name, err)
			}
			if len(entities) != 1 || entities[0].Type != it {
				t.Errorf("%s/%s: want the identity type alone, got %+v", it, order.name, entities)
			}
		}
		for _, order := range []struct {
			name               string
			cardStart, cardEnd int
			idStart, idEnd     int
		}{
			{"card first", 0, 16, 12, 16},
			{"identity first", 12, 16, 0, 16},
		} {
			a := New()
			a.AddRecognizer(&stubRecognizer{name: "card", entities: []recognizers.Entity{
				{Type: recognizers.EntityCreditCard, Start: order.cardStart, End: order.cardEnd, Text: "x", Confidence: 0.85},
			}})
			a.AddRecognizer(&stubRecognizer{name: "id", entities: []recognizers.Entity{
				{Type: it, Start: order.idStart, End: order.idEnd, Text: "x", Confidence: 0.90},
			}})
			entities, err := a.Analyze(context.Background(), "4111111111111111")
			if err != nil {
				t.Fatalf("%s/%s: unexpected error: %v", it, order.name, err)
			}
			if len(entities) != 1 || entities[0].Type != recognizers.EntityCreditCard {
				t.Errorf("%s/%s: want CREDIT_CARD alone, got %+v", it, order.name, entities)
			}
		}
	}
}

// A TAX_ID yields to SSN, PH_TIN and IN_GSTIN on the same span at every score.
// PH_TIN scores 0.80, below the TAX_ID's 0.85, so confidence alone would hand
// the span to TAX_ID; the explicit rule keeps PH_TIN. SSN (0.99) and IN_GSTIN
// (0.90) win on confidence too, and the test pins all three.
func TestTaxIDYieldsToSpecificTypesOnSameSpan(t *testing.T) {
	for _, specific := range []struct {
		typ   recognizers.EntityType
		score float64
	}{
		{recognizers.EntitySSN, 0.99},
		{recognizers.EntityPHTIN, 0.80},
		{recognizers.EntityINGSTIN, 0.90},
	} {
		for _, order := range []string{"tax id first", "specific first"} {
			taxID := recognizers.Entity{Type: recognizers.EntityTaxID, Start: 0, End: 11, Text: "x", Confidence: 0.85}
			other := recognizers.Entity{Type: specific.typ, Start: 0, End: 11, Text: "x", Confidence: specific.score}
			a := New()
			if order == "tax id first" {
				a.AddRecognizer(&stubRecognizer{name: "tax", entities: []recognizers.Entity{taxID}})
				a.AddRecognizer(&stubRecognizer{name: "spec", entities: []recognizers.Entity{other}})
			} else {
				a.AddRecognizer(&stubRecognizer{name: "spec", entities: []recognizers.Entity{other}})
				a.AddRecognizer(&stubRecognizer{name: "tax", entities: []recognizers.Entity{taxID}})
			}
			entities, err := a.Analyze(context.Background(), "12-3456789")
			if err != nil {
				t.Fatalf("%s/%s: unexpected error: %v", specific.typ, order, err)
			}
			if len(entities) != 1 || entities[0].Type != specific.typ {
				t.Errorf("%s/%s: want %s alone, got %+v", specific.typ, order, specific.typ, entities)
			}
		}
	}
}

// A generic PASSPORT yields to MY_PASSPORT and SG_PASSPORT on the same span.
// All three score 0.85, so the tie would otherwise go to the earlier span.
func TestGenericPassportYieldsToCountryPassportsOnSameSpan(t *testing.T) {
	for _, country := range []recognizers.EntityType{recognizers.EntityMYPassport, recognizers.EntitySGPassport} {
		for _, order := range []string{"generic first", "country first"} {
			generic := recognizers.Entity{Type: recognizers.EntityPassport, Start: 0, End: 9, Text: "x", Confidence: 0.85}
			specific := recognizers.Entity{Type: country, Start: 0, End: 9, Text: "x", Confidence: 0.85}
			a := New()
			if order == "generic first" {
				a.AddRecognizer(&stubRecognizer{name: "generic", entities: []recognizers.Entity{generic}})
				a.AddRecognizer(&stubRecognizer{name: "country", entities: []recognizers.Entity{specific}})
			} else {
				a.AddRecognizer(&stubRecognizer{name: "country", entities: []recognizers.Entity{specific}})
				a.AddRecognizer(&stubRecognizer{name: "generic", entities: []recognizers.Entity{generic}})
			}
			entities, err := a.Analyze(context.Background(), "E1234567A")
			if err != nil {
				t.Fatalf("%s/%s: unexpected error: %v", country, order, err)
			}
			if len(entities) != 1 || entities[0].Type != country {
				t.Errorf("%s/%s: want %s alone, got %+v", country, order, country, entities)
			}
		}
	}
}
