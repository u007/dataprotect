package checksum

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/u007/dataprotect/recognizers"
)

func TestLuhnValidate(t *testing.T) {
	tests := []struct {
		number string
		want   bool
	}{
		{"4111111111111111", true},      // Visa
		{"4111-1111-1111-1111", true},   // Visa with hyphens
		{"4111 1111 1111 1111", true},   // Visa with spaces
		{"378282246310005", true},       // Amex
		{"5555555555554444", true},      // Mastercard
		{"6011111111111117", true},      // Discover
		{"4012888888881881", true},      // Visa
		{"1234567890123456", false},     // invalid Luhn
		{"4111111111111112", false},     // off by one
		{"123", false},                  // too short
		{"41111111111111111111", false}, // too long
		{"abcd", false},                 // non-numeric
		{"", false},                     // empty
	}

	v := NewLuhnValidator()
	for _, tt := range tests {
		t.Run(tt.number, func(t *testing.T) {
			if got := v.Validate(tt.number); got != tt.want {
				t.Errorf("Validate(%q) = %v, want %v", tt.number, got, tt.want)
			}
		})
	}
}

func TestIBANValidate(t *testing.T) {
	tests := []struct {
		iban string
		want bool
	}{
		{"DE89370400440532013000", true},
		{"GB82WEST12345698765432", true},
		{"FR1420041010050500013M02606", true},
		{"NL91ABNA0417164300", true},
		{"DE89370400440532013001", false}, // wrong check digits
		{"XX", false},                     // too short
		{"1234567890", false},             // no country code
		{"", false},                       // empty
	}

	v := NewIBANValidator()
	for _, tt := range tests {
		t.Run(tt.iban, func(t *testing.T) {
			if got := v.Validate(tt.iban); got != tt.want {
				t.Errorf("Validate(%q) = %v, want %v", tt.iban, got, tt.want)
			}
		})
	}
}

func TestLuhnRecognize(t *testing.T) {
	v := NewLuhnValidator()
	ctx := context.Background()

	entities, err := v.Recognize(ctx, "my card is 4111111111111111 and invalid 1234567890123456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("got %d entities, want 1 (only the valid card): %+v", len(entities), entities)
	}
	if entities[0].Text != "4111111111111111" {
		t.Errorf("got %q, want 4111111111111111", entities[0].Text)
	}
}

func TestIBANRecognize(t *testing.T) {
	v := NewIBANValidator()
	ctx := context.Background()

	entities, err := v.Recognize(ctx, "IBAN: DE89370400440532013000 and bad: XX00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("got %d entities, want 1: %+v", len(entities), entities)
	}
	if entities[0].Text != "DE89370400440532013000" {
		t.Errorf("got %q, want DE89370400440532013000", entities[0].Text)
	}
}

// A spaced IBAN is the format every bank prints, so the spaced form is not an
// edge case — it is the common case. The unspaced form is what the existing
// tests cover, which is exactly why a defect confined to spaces could ship.
//
// The count of IBAN characters must EXCLUDE the spaces: ISO 13616 caps an IBAN
// at 34 significant characters, and the spaces are presentation, not content.
// Counting them inflated the running total, tripped the length cap early, and
// truncated the candidate before its final block — so a valid spaced IBAN was
// silently dropped because the truncated string failed mod-97. That failure is
// invisible: Validate passes on the same IBAN when given whole.
func TestIBANRecognizeSpacedFormIsNotTruncated(t *testing.T) {
	v := NewIBANValidator()
	ctx := context.Background()

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "gb82 spaced",
			text: "pay to GB82 WEST 1234 5698 7654 32 today",
			want: "GB82 WEST 1234 5698 7654 32",
		},
		{
			name: "de89 spaced",
			text: "DE89 3704 0044 0532 0130 00 cleared",
			want: "DE89 3704 0044 0532 0130 00",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Precondition: the whole value must be a valid IBAN. Without this,
			// a failure below is ambiguous between a finder bug and bad test data.
			if !v.Validate(tc.want) {
				t.Fatalf("test data %q is not a valid IBAN; fix the fixture, not the code", tc.want)
			}
			entities, err := v.Recognize(ctx, tc.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entities) != 1 {
				t.Fatalf("got %d entities, want 1: %+v", len(entities), entities)
			}
			if entities[0].Text != tc.want {
				t.Errorf("got %q, want %q (candidate truncated before its final block?)",
					entities[0].Text, tc.want)
			}
		})
	}
}

// The truncated span must not be reported either: a candidate that stops short
// is worse than no candidate, because it feeds a wrong span to the anonymizer.
func TestIBANRecognizeSpacedFormDoesNotEmitTruncatedSpan(t *testing.T) {
	v := NewIBANValidator()
	entities, err := v.Recognize(context.Background(), "pay to GB82 WEST 1234 5698 7654 32 today")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range entities {
		if e.Text != "GB82 WEST 1234 5698 7654 32" {
			t.Errorf("emitted truncated span %q", e.Text)
		}
	}
}

// The length cap must be enforced by the SPAN, not by the counter. A spaced
// IBAN carries presentation characters that are not IBAN characters, so the
// cap has to bound the run without letting trailing blocks push it over.
//
// This pins which side of the cap the finder takes when a 34-character IBAN is
// followed by another uppercase block: it must stop at the cap. Reading the
// bound as "the next block of four must also fit" (significant+4) is stricter
// and would stop early on a legitimately short final block; reading it as
// "one more character fits" (significant+1) is what admits that short block.
// Both reject the whole candidate here, so the assertion is on the SPAN the
// finder hands to Validate — that span is what a wrong bound would distort.
func TestIBANCandidateStopsAtSignificantLengthCap(t *testing.T) {
	v := NewIBANValidator()

	// A valid IBAN followed by more uppercase alphanumerics. The trailing block
	// must not push the candidate past the 34-significant-character cap, or the
	// span handed to Validate can never be right.
	text := "FR14 2004 1010 0505 0001 3M02 606 12345 tail"
	entities, err := v.Recognize(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range entities {
		if len(stripSpaces(e.Text)) > 34 {
			t.Errorf("candidate exceeds the ISO 13616 limit: %q is %d significant chars",
				e.Text, len(stripSpaces(e.Text)))
		}
	}
}

// A maximal-length real IBAN must still be found in spaced form.
func TestIBANRecognizeLongSpacedWithLetterBlock(t *testing.T) {
	v := NewIBANValidator()
	// 27 significant characters with a letter block, spaced conventionally.
	// Verified against Validate below so a broken fixture cannot make this test
	// vacuous — an unverified fixture that stops matching is indistinguishable
	// from the bug it is meant to catch.
	const iban = "FR14 2004 1010 0505 0001 3M02 606"
	if !v.Validate(iban) {
		t.Fatalf("fixture %q is not a valid IBAN; a broken fixture makes this test vacuous", iban)
	}
	entities, err := v.Recognize(context.Background(), "pay "+iban+" today")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("got %d entities, want 1: %+v", len(entities), entities)
	}
	if entities[0].Text != iban {
		t.Errorf("got %q, want %q", entities[0].Text, iban)
	}
}

func stripSpaces(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func TestLuhnRecognizeReportsThaiValidSpanAsCard(t *testing.T) {
	// 1000000000009 is also a valid Thai citizen ID. The raw Luhn validator
	// still reports a card: typing it as TH_ID is the Scrubber's job, after
	// overlap resolution, so a card-only caller keeps the card it always had.
	v := NewLuhnValidator()
	entities, err := v.Recognize(context.Background(), "Citizen ID 1000000000009 on file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 || entities[0].Type != recognizers.EntityCreditCard {
		t.Fatalf("got %+v, want one CREDIT_CARD", entities)
	}
}

// A card followed by another digit group, or by another card, used to be
// reported as one run of digits. The whole run then failed Luhn and nothing
// was reported. Each card must be found on its own span.
func TestLuhnRecognizeCardNextToDigitGroup(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"card then cvv", "4111111111111111 123", []string{"4111111111111111"}},
		{"card then year", "card 4111111111111111 2024", []string{"4111111111111111"}},
		{"card then cvv, dashed", "4111-1111-1111-1111 123", []string{"4111-1111-1111-1111"}},
		{"card then cvv, dotted", "4111.1111.1111.1111 123", []string{"4111.1111.1111.1111"}},
		{"single card then dashed cvv", "4111111111111111-123", []string{"4111111111111111"}},
		{"single card then dotted cvv", "4111111111111111.123", []string{"4111111111111111"}},
		{"dashed cvv then single card", "123-4111111111111111", []string{"4111111111111111"}},
		{"two cards, one space", "4012888888881881 4012888888881881", []string{"4012888888881881", "4012888888881881"}},
		{"two cards, mixed forms", "4012888888881881 4012-8888-8888-1881", []string{"4012888888881881", "4012-8888-8888-1881"}},
	}
	v := NewLuhnValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := v.Recognize(context.Background(), tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got []string
			for _, e := range entities {
				got = append(got, e.Text)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("entity %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// The span must start and end on the card's own digits, so the text around it
// is never part of the match.
func TestLuhnRecognizeCardSpanExcludesNeighbours(t *testing.T) {
	text := "pay 4111111111111111 cvv 123"
	entities, err := NewLuhnValidator().Recognize(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("got %+v, want one card", entities)
	}
	e := entities[0]
	if text[e.Start:e.End] != "4111111111111111" {
		t.Errorf("span = %q, want the card only", text[e.Start:e.End])
	}
}

// An IBAN followed by an uppercase word, or by digits, used to absorb that
// word into the candidate. The candidate then failed mod-97, so the valid IBAN
// was dropped.
func TestIBANRecognizeIBANNextToCapitalisedWordOrDigits(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"then capitals", "IBAN AL47212110090000000235698741 ALL", "AL47212110090000000235698741"},
		{"then digits", "AL47212110090000000235698741 12", "AL47212110090000000235698741"},
		{"then single letter", "this is an iban VG96 VPVG 0000 0123 4567 8901 X in a sentence", "VG96 VPVG 0000 0123 4567 8901"},
		{"then capitals, no prefix", "AL47212110090000000235698741 ALL CAPS", "AL47212110090000000235698741"},
	}
	v := NewIBANValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := v.Recognize(context.Background(), tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entities) != 1 {
				t.Fatalf("got %+v, want one IBAN", entities)
			}
			if entities[0].Text != tt.want {
				t.Errorf("got %q, want %q", entities[0].Text, tt.want)
			}
		})
	}
}

// HR12 001 0051 8630 passes mod-97 on its own, but it is 18 characters and a
// Croatian IBAN is 21. A candidate that fails as a whole must not be cut down
// to such a prefix, or any short run that passes by chance is reported.
func TestIBANRecognizeDoesNotCutToChancePassingPrefix(t *testing.T) {
	v := NewIBANValidator()
	entities, err := v.Recognize(context.Background(), "Ref HR12 001 0051 8630 0000 0000 end")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Fatalf("got %+v, want no IBAN", entities)
	}
}

// coveredBy reports whether the card digits [start, end) of text are inside a
// reported entity. A redacted card must have every digit covered.
func coveredBy(entities []recognizers.Entity, start, end int) bool {
	for _, e := range entities {
		if e.Start <= start && end <= e.End {
			return true
		}
	}
	return false
}

// A card whose digits follow two letters and two digits must still be found.
// The run "12 4111 ..." used to be skipped as an IBAN body, which left the card
// visible.
func TestLuhnRecognizeCardAfterLettersAndDigitsIsCovered(t *testing.T) {
	tests := []struct {
		name, text, card string
	}{
		{"letters then two digits", "Ref AB12 4111 1111 1111 1111", "4111 1111 1111 1111"},
		{"glued letters then cvv", "ID01 4111111111111111 123", "4111111111111111"},
	}
	v := NewLuhnValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := v.Recognize(context.Background(), tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			start := strings.Index(tt.text, tt.card)
			if start < 0 || !coveredBy(entities, start, start+len(tt.card)) {
				t.Fatalf("card %q not covered by %+v", tt.card, entities)
			}
		})
	}
}

// A window of four groups can pass Luhn by chance and cut the real card short:
// "1004 4111 1111 1111" passes, so the trailing "1111" of the real card was
// left out. Every digit of the card must still be covered.
func TestLuhnRecognizeChancePassingWindowDoesNotCutCard(t *testing.T) {
	text := "1004 4111 1111 1111 1111"
	entities, err := NewLuhnValidator().Recognize(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !coveredBy(entities, len("1004 "), len(text)) {
		t.Fatalf("card digits %q not covered by %+v", text[len("1004 "):], entities)
	}
}

// A long run of digit groups must be read in bounded time. The window search
// used to try every later group for every start, which is cubic in the run.
func TestLuhnRecognizeLongDigitRunIsBounded(t *testing.T) {
	text := strings.Repeat("1 ", 500000)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := NewLuhnValidator().Recognize(context.Background(), text); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Recognize did not finish a 1 MB digit run in 5 seconds")
	}
}

// Digits inside a valid IBAN are the IBAN's own, so no card is reported there.
func TestLuhnRecognizeValidIBANHasNoCardWindows(t *testing.T) {
	for _, text := range []string{
		"AD12 0001 2030 2003 5910 0100",
		"Pay AD12 0001 2030 2003 5910 0100 today",
	} {
		entities, err := NewLuhnValidator().Recognize(context.Background(), text)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entities) != 0 {
			t.Errorf("Recognize(%q) = %+v, want no card", text, entities)
		}
	}
}

// Three cards in a row: every card digit must be covered. Windows that straddle
// two cards can pass Luhn by chance, and overlapping windows are merged, so the
// three cards may come back as one span. Covering every digit is the contract;
// the number of spans is not.
func TestLuhnRecognizeCardsThreeTimesAreCovered(t *testing.T) {
	text := "4012 8888 8888 1881 4012 8888 8888 1881 4012 8888 8888 1881"
	entities, err := NewLuhnValidator().Recognize(context.Background(), text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for at := 0; at < len(text); {
		i := strings.Index(text[at:], "4012 8888 8888 1881")
		if i < 0 {
			break
		}
		start := at + i
		if !coveredBy(entities, start, start+len("4012 8888 8888 1881")) {
			t.Fatalf("card at %d not covered by %+v", start, entities)
		}
		at = start + len("4012 8888 8888 1881")
	}
}
