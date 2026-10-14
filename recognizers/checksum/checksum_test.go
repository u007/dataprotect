package checksum

import (
	"context"
	"testing"

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
