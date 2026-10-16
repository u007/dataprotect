package golden_test

import (
	"testing"

	"github.com/u007/dataprotect/golden"
)

// TestPseudonymRoundTrip covers the alignment rules: exact text outside the
// placeholders, and spelling allowed only inside a value.
func TestPseudonymRoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		original  string
		pseudo    string
		pseudonym map[string]string
		want      bool
	}{
		{
			name:      "exact restore",
			original:  "mail a@b.com now",
			pseudo:    "mail [EMAIL_1] now",
			pseudonym: map[string]string{"[EMAIL_1]": "a@b.com"},
			want:      true,
		},
		{
			name:      "value spelled differently from the first seen",
			original:  "card 4111 1111 1111 1111 ok",
			pseudo:    "card [CREDIT_CARD_1] ok",
			pseudonym: map[string]string{"[CREDIT_CARD_1]": "4111111111111111"},
			want:      true,
		},
		{
			name:      "prose changed by spacing is rejected",
			original:  "Hello, world a@b.com",
			pseudo:    "Hello world [EMAIL_1]",
			pseudonym: map[string]string{"[EMAIL_1]": "a@b.com"},
			want:      false,
		},
		{
			name:      "wrong value is rejected",
			original:  "a@b.com",
			pseudo:    "[EMAIL_1]",
			pseudonym: map[string]string{"[EMAIL_1]": "x@y.org"},
			want:      false,
		},
		{
			name:      "trailing separators in the value are matched by extent",
			original:  "x -",
			pseudo:    "[TOKEN_1]",
			pseudonym: map[string]string{"[TOKEN_1]": "x"},
			want:      true,
		},
		{
			name:      "placeholder-shaped text that is not a key is literal",
			original:  "see [NOTE_1] a@b.com",
			pseudo:    "see [NOTE_1] [EMAIL_1]",
			pseudonym: map[string]string{"[EMAIL_1]": "a@b.com"},
			want:      true,
		},
		{
			name:      "extra text after the last placeholder is rejected",
			original:  "a@b.com and more",
			pseudo:    "[EMAIL_1]",
			pseudonym: map[string]string{"[EMAIL_1]": "a@b.com"},
			want:      false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := golden.PseudonymRoundTrip(c.original, c.pseudo, c.pseudonym); got != c.want {
				t.Errorf("PseudonymRoundTrip(%q, %q) = %v, want %v", c.original, c.pseudo, got, c.want)
			}
		})
	}
}
