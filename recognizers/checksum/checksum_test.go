package checksum

import (
	"context"
	"testing"
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
