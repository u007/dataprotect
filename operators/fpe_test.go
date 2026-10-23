package operators

import (
	"context"
	"testing"
)

func TestFPEOperatorName(t *testing.T) {
	o, err := NewFPEOperator([]byte("0123456789abcdef"), 10, []byte("tweak"), 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.Name() != "fpe" {
		t.Errorf("Name() = %q, want fpe", o.Name())
	}
}

func TestFPEOperatorRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef") // 16 bytes, AES-128
	tweak := []byte("piiscrub-fpe-tweak")
	o, err := NewFPEOperator(key, 10, tweak, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []string{
		"4111111111111111", // 16-digit card
		"555512345678",     // 12-digit
		"1234567890",       // 10-digit phone
		"00000000",         // all zeros
		"9999999999999999", // all nines
		"12",               // minimum length
		"01",               // leading zero
		"99",               // max two-digit
		"12345",            // odd length
	}

	for _, tc := range cases {
		enc, err := o.Encrypt(tc)
		if err != nil {
			t.Errorf("Encrypt(%q) unexpected error: %v", tc, err)
			continue
		}
		if len(enc) != len(tc) {
			t.Errorf("Encrypt(%q) = %q: length %d, want %d", tc, enc, len(enc), len(tc))
		}
		dec, err := o.Decrypt(enc)
		if err != nil {
			t.Errorf("Decrypt(%q) unexpected error: %v", enc, err)
			continue
		}
		if dec != tc {
			t.Errorf("Decrypt(Encrypt(%q)) = %q, want %q", tc, dec, tc)
		}
		// Format preservation: every output char must be a valid digit in the radix.
		for i := 0; i < len(enc); i++ {
			if _, ok := charToDigit(enc[i], 10); !ok {
				t.Errorf("Encrypt(%q) = %q: byte %#x at position %d not a digit", tc, enc, enc[i], i)
			}
		}
	}
}

func TestFPEOperatorDeterministic(t *testing.T) {
	o, err := NewFPEOperator([]byte("0123456789abcdef"), 10, []byte("tweak"), 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	enc1, err := o.Encrypt("4111111111111111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	enc2, err := o.Encrypt("4111111111111111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enc1 != enc2 {
		t.Errorf("Encrypt is not deterministic: %q vs %q", enc1, enc2)
	}
}

func TestFPEOperatorDifferentKeysDiffer(t *testing.T) {
	key1 := []byte("0123456789abcdef")
	key2 := []byte("fedcba9876543210")
	tweak := []byte("tweak")

	o1, err := NewFPEOperator(key1, 10, tweak, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	o2, err := NewFPEOperator(key2, 10, tweak, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	enc1, _ := o1.Encrypt("4111111111111111")
	enc2, _ := o2.Encrypt("4111111111111111")
	if enc1 == enc2 {
		t.Errorf("different keys produced the same ciphertext: %q", enc1)
	}
}

func TestFPEOperatorDifferentTweaksDiffer(t *testing.T) {
	key := []byte("0123456789abcdef")
	tweak1 := []byte("tweak-one")
	tweak2 := []byte("tweak-two")

	o1, err := NewFPEOperator(key, 10, tweak1, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	o2, err := NewFPEOperator(key, 10, tweak2, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	enc1, _ := o1.Encrypt("4111111111111111")
	enc2, _ := o2.Encrypt("4111111111111111")
	if enc1 == enc2 {
		t.Errorf("different tweaks produced the same ciphertext: %q", enc1)
	}
}

func TestFPEOperatorReplaceSatisfiesInterface(t *testing.T) {
	o, err := NewFPEOperator([]byte("0123456789abcdef"), 10, []byte("tweak"), 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var _ Operator = o
	got, err := o.Replace(context.Background(), "4111111111111111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 16 {
		t.Errorf("Replace returned %q, expected length 16", got)
	}
}

func TestFPEOperatorInvalidKey(t *testing.T) {
	_, err := NewFPEOperator([]byte("short"), 10, nil, 8)
	if err == nil {
		t.Errorf("expected error for short key, got nil")
	}

	_, err = NewFPEOperator([]byte("0123456789abcdef"), 1, nil, 8)
	if err == nil {
		t.Errorf("expected error for radix < 2, got nil")
	}

	_, err = NewFPEOperator([]byte("0123456789abcdef"), 100, nil, 8)
	if err == nil {
		t.Errorf("expected error for radix > 62, got nil")
	}

	// Tweak longer than 64 bytes.
	longTweak := make([]byte, 65)
	_, err = NewFPEOperator([]byte("0123456789abcdef"), 10, longTweak, 8)
	if err == nil {
		t.Errorf("expected error for tweak > 64 bytes, got nil")
	}
}

func TestFPEOperatorInvalidInput(t *testing.T) {
	o, err := NewFPEOperator([]byte("0123456789abcdef"), 10, []byte("tweak"), 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Too short.
	if _, err := o.Encrypt("1"); err == nil {
		t.Errorf("expected error for 1-char input, got nil")
	}

	// Non-digit character in radix 10.
	if _, err := o.Encrypt("4111X11111111111"); err == nil {
		t.Errorf("expected error for non-digit input, got nil")
	}

	// Empty.
	if _, err := o.Encrypt(""); err == nil {
		t.Errorf("expected error for empty input, got nil")
	}
}

func TestFPEOperatorRadix36(t *testing.T) {
	o, err := NewFPEOperator([]byte("0123456789abcdef0123456789abcdef"), 36, []byte("tweak"), 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Alphanumeric lowercase input (0-9, a-z).
	plain := "abc123xyz"
	enc, err := o.Encrypt(plain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(enc) != len(plain) {
		t.Errorf("Encrypt(%q) = %q: length %d, want %d", plain, enc, len(enc), len(plain))
	}
	dec, err := o.Decrypt(enc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != plain {
		t.Errorf("Decrypt(Encrypt(%q)) = %q, want %q", plain, dec, plain)
	}
}

func TestFPEOperatorSingleDigitRounds(t *testing.T) {
	// Fewer than 2 digits is invalid; 2 digits is the minimum.
	o, err := NewFPEOperator([]byte("0123456789abcdef"), 10, []byte("tweak"), 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tc := range []string{"00", "01", "10", "99"} {
		enc, err := o.Encrypt(tc)
		if err != nil {
			t.Errorf("Encrypt(%q) unexpected error: %v", tc, err)
			continue
		}
		dec, err := o.Decrypt(enc)
		if err != nil {
			t.Errorf("Decrypt(%q) unexpected error: %v", enc, err)
			continue
		}
		if dec != tc {
			t.Errorf("round-trip failed for %q: got %q", tc, dec)
		}
	}
}
