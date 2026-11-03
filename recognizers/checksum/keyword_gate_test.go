package checksum

import (
	"context"
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// TestLuhnKeywordGatedCard covers the hybrid rule approved 2026-10-20: a
// Luhn-failing window is a card only when a label-style keyword names it, it
// has a valid network prefix and a standard layout, and it is bounded.
func TestLuhnKeywordGatedCard(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string // reported span, empty means no entity
		conf float64
	}{
		{
			name: "card colon label",
			text: "Credit/Debit Card: 4111 1111 1111 1112",
			want: "4111 1111 1111 1112",
			conf: 0.85,
		},
		{
			name: "card number label",
			text: "card number 4111 1111 1111 1112",
			want: "4111 1111 1111 1112",
			conf: 0.85,
		},
		{
			name: "credit card label",
			text: "my credit card is 4111-1111-1111-1112",
			want: "4111-1111-1111-1112",
			conf: 0.85,
		},
		{
			name: "debit card label unseparated",
			text: "debit card 4111111111111112",
			want: "4111111111111112",
			conf: 0.85,
		},
		{
			name: "network name then separator",
			text: "Visa: 4111 1111 1111 1112",
			want: "4111 1111 1111 1112",
			conf: 0.85,
		},
		{
			name: "cc token",
			text: "CC 4111 1111 1111 1112",
			want: "4111 1111 1111 1112",
			conf: 0.85,
		},
		{
			name: "cvv label",
			text: "cvv 4111 1111 1111 1112",
			want: "4111 1111 1111 1112",
			conf: 0.85,
		},
		{
			name: "trailing cvv is not swallowed",
			text: "debit card 4111 1111 1111 1112 123",
			want: "4111 1111 1111 1112",
			conf: 0.85,
		},
		{
			name: "luhn-valid card keeps the high confidence",
			text: "credit card 4111111111111111",
			want: "4111111111111111",
			conf: 0.99,
		},

		// Negatives: the seed cases that the whole-run prototype broke.
		{
			name: "bare card in prose does not gate",
			text: "A mistyped 4111111111111112 fails the checksum and is not a card.",
		},
		{
			name: "bare Card before the number does not gate",
			text: "Card 4111111111111112 123 expired.",
		},
		{
			name: "bank name Credit does not gate",
			text: "Credit Suisse, London/USD/ 4111 1111 1111 1112",
		},
		{
			name: "invalid network prefix does not gate",
			text: "credit card 1748503543012",
		},
		{
			name: "label beyond the window does not gate",
			text: "credit card " + strings.Repeat("x ", 30) + "4111 1111 1111 1112",
		},
		{
			name: "plural cards is not the label",
			text: "credit cards 4111 1111 1111 1112",
		},
		{
			name: "cc glued to a word is not a token",
			text: "xcc 4111 1111 1111 1112",
		},
		{
			name: "network name glued to a word is not a token",
			text: "VisaX: 4111 1111 1111 1112",
		},
	}

	v := NewLuhnValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := v.Recognize(context.Background(), tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.want == "" {
				if len(entities) != 0 {
					t.Fatalf("got %d entities, want none: %+v", len(entities), entities)
				}
				return
			}
			if len(entities) != 1 {
				t.Fatalf("got %d entities, want 1 %q: %+v", len(entities), tt.want, entities)
			}
			got := entities[0]
			if got.Type != recognizers.EntityCreditCard {
				t.Errorf("type = %s, want CREDIT_CARD", got.Type)
			}
			if got.Text != tt.want {
				t.Errorf("text = %q, want %q", got.Text, tt.want)
			}
			if got.Confidence != tt.conf {
				t.Errorf("confidence = %v, want %v", got.Confidence, tt.conf)
			}
		})
	}
}

// TestLuhnKeywordGateDoesNotDuplicateALuhnCard guards the merge step: a
// Luhn-passing whole run must be reported once, not once at 0.99 and again at
// 0.85 by the gated branch.
func TestLuhnKeywordGateDoesNotDuplicateALuhnCard(t *testing.T) {
	v := NewLuhnValidator()
	entities, err := v.Recognize(context.Background(), "credit card 4111 1111 1111 1111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("got %d entities, want 1: %+v", len(entities), entities)
	}
	if entities[0].Confidence != 0.99 {
		t.Errorf("confidence = %v, want the Luhn 0.99", entities[0].Confidence)
	}
}

// TestLuhnKeywordGatedCardLanguages verifies the opt-in language sets: a
// local-language label gates a Luhn-failing card only when the language was
// requested, and English remains the default.
func TestLuhnKeywordGatedCardLanguages(t *testing.T) {
	const num = "4111 1111 1111 1112"

	tests := []struct {
		name string
		lang []Language
		text string
		want bool
	}{
		{"english default does not gate french", nil, "carte de crédit : " + num, false},
		{"french requested gates", []Language{LanguageFR}, "carte de crédit : " + num, true},
		{"italian requested gates", []Language{LanguageIT}, "carta di credito: " + num, true},
		{"spanish requested gates", []Language{LanguageES}, "Número de Tarjeta: " + num, true},
		{"german requested gates", []Language{LanguageDE}, "Kreditkarte: " + num, true},
		{"dutch requested gates", []Language{LanguageNL}, "Creditcardnummer: " + num, true},
		{"english default does not gate swedish", nil, "Kortnummer: " + num, false},
		{"swedish requested gates", []Language{LanguageSV}, "Kortnummer: " + num, true},
		{"english still gates", []Language{LanguageFR}, "credit card " + num, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewLuhnValidatorWithLanguages(tt.lang...)
			entities, err := v.Recognize(context.Background(), tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(entities) == 1; got != tt.want {
				t.Fatalf("got %d entities (want gated=%v): %+v", len(entities), tt.want, entities)
			}
		})
	}
}
