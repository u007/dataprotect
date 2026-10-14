package regex

import (
	"context"
	"testing"

	"dataprotect/recognizers"
)

func TestEmailRecognizer(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantHits int
		want     string
	}{
		{"simple", "contact john@example.com", 1, "john@example.com"},
		{"with plus", "email john.doe+tag@example.co.uk now", 1, "john.doe+tag@example.co.uk"},
		{"multiple", "a@x.com and b@y.org", 2, ""},
		{"no email", "no email here", 0, ""},
		{"with underscore", "first_last@test.com", 1, "first_last@test.com"},
		{"with hyphen", "my-user@sub.domain.com", 1, "my-user@sub.domain.com"},
	}

	r := NewEmailRecognizer()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entities) != tt.wantHits {
				t.Errorf("got %d entities, want %d: %+v", len(entities), tt.wantHits, entities)
			}
			if tt.want != "" && len(entities) > 0 && entities[0].Text != tt.want {
				t.Errorf("got text %q, want %q", entities[0].Text, tt.want)
			}
			if len(entities) > 0 {
				e := entities[0]
				if e.Type != recognizers.EntityEmail {
					t.Errorf("got type %q, want EMAIL", e.Type)
				}
				if e.Text != tt.text[e.Start:e.End] {
					t.Errorf("Text field %q does not match span %q", e.Text, tt.text[e.Start:e.End])
				}
				if e.Confidence < 0.9 {
					t.Errorf("confidence %f too low for regex match", e.Confidence)
				}
			}
		})
	}
}

func TestPhoneRecognizer(t *testing.T) {
	// The parenthesised and leading-"+" cases assert the WHOLE span, not just
	// the hit count. The count-only version of this table is what let the
	// leading "(" and "+" be silently dropped for so long: the recognizer still
	// reported one PHONE, just one byte too far to the right, so a count
	// assertion could not see it. See the phone anchor note in regex.go.
	tests := []struct {
		name     string
		text     string
		wantHits int
		want     string // first entity's exact text; "" means "do not check"
	}{
		{"us format", "call 555-123-4567", 1, "555-123-4567"},
		{"with parens", "call (555) 123-4567", 1, "(555) 123-4567"},
		{"parens at start", "(555) 867-5309", 1, "(555) 867-5309"},
		{"parens in prose", "Call (555) 867-5309 before noon.", 1, "(555) 867-5309"},
		{"parens no space", "(555)867-5309", 1, "(555)867-5309"},
		{"parens with country code", "call 1 (555) 867-5309 now", 1, "1 (555) 867-5309"},
		{"parens with +country code", "+1 (555) 867-5309", 1, "+1 (555) 867-5309"},
		{"country code needs a boundary", "x1 (555) 867-5309", 1, "(555) 867-5309"},
		{"with spaces", "call 555 123 4567", 1, "555 123 4567"},
		{"international", "call +1-555-123-4567", 1, "+1-555-123-4567"},
		{"international at start", "+44 20 7946 0958", 1, "+44 20 7946 0958"},
		{"international space separator", "+1 555 123 4567", 1, "+1 555 123 4567"},
		{"not inside a digit run", "x555-123-4567", 0, ""},
		{"+ prefix alone is not a phone", "+12345", 0, ""},
		{"arithmetic is not a phone", "1+23456789", 0, ""},
		{"card with a + prefix is not a phone", "+4111111111111111", 0, ""},
		{"not a bare 16-digit card", "4111111111111111", 0, ""},
		{"not a spaced card", "4111 1111 1111 1111", 0, ""},
		{"no phone", "no phone here", 0, ""},
	}

	r := NewPhoneRecognizer()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entities) != tt.wantHits {
				t.Errorf("got %d entities, want %d: %+v", len(entities), tt.wantHits, entities)
			}
			for _, e := range entities {
				if e.Type != recognizers.EntityPhone {
					t.Errorf("got type %q, want PHONE", e.Type)
				}
			}
			if tt.want != "" && len(entities) > 0 {
				if got := entities[0].Text; got != tt.want {
					t.Errorf("got text %q, want %q", got, tt.want)
				}
				if got := tt.text[entities[0].Start:entities[0].End]; got != tt.want {
					t.Errorf("span %q does not match Text field %q", got, tt.want)
				}
			}
		})
	}
}

func TestCreditCardRecognizer(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantHits int
	}{
		{"visa spaced", "4111 1111 1111 1111", 1},
		{"visa hyphen", "4111-1111-1111-1111", 1},
		{"visa plain", "4111111111111111", 1},
		{"amex", "378282246310005", 1},
		{"mastercard", "5555555555554444", 1},
		{"short number", "1234", 0},
	}

	r := NewCreditCardRecognizer()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entities) != tt.wantHits {
				t.Errorf("got %d entities, want %d: %+v", len(entities), tt.wantHits, entities)
			}
		})
	}
}

func TestSSNRecognizer(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantHits int
	}{
		{"dashed", "123-45-6789", 1},
		{"spaced", "123 45 6789", 1},
		{"dotted", "123.45.6789", 1},
		{"mixed separators", "123-45 6789", 1},
		// A bare nine-digit value is an SSN only when it IS the value — a
		// declared field, a CSV cell, a standalone token. In running text the
		// shape is shared by account and document numbers, so it is refused.
		// These three are the exact false positives that cost 449 of 483
		// precision on gretel (P=0.0929) and are why this recognizer's seed
		// coverage could never expose the defect: every real SSN in both
		// corpora (220 of 232) is dashed, and none is bare.
		{"plain", "123456789", 1},
		{"bare in prose - account number", "Account Number: 123456789", 0},
		{"bare in prose - CAS", "CAS No.: 123456789", 0},
		{"bare in prose - passport", "Passport Number: 322996077", 0},
		// One separator only: a decimal number of the right length, seen in the
		// corpus as longitude 177.978032. Both separator positions are required.
		{"single separator", "177.978032", 0},
		{"too short", "12-345-678", 0},
	}

	r := NewSSNRecognizer()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entities) != tt.wantHits {
				t.Errorf("got %d entities, want %d: %+v", len(entities), tt.wantHits, entities)
			}
		})
	}
}

func TestIPRecognizer(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantHits int
	}{
		{"ipv4", "192.168.1.1", 1},
		{"public ip", "8.8.8.8", 1},
		{"in text", "server at 10.0.0.1 is up", 1},
		{"invalid octet", "999.999.999.999", 0},
		{"partial", "192.168.1", 0},
	}

	r := NewIPRecognizer()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entities) != tt.wantHits {
				t.Errorf("got %d entities, want %d: %+v", len(entities), tt.wantHits, entities)
			}
		})
	}
}

func TestURLRecognizer(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantHits int
	}{
		{"https", "visit https://example.com today", 1},
		{"http", "visit http://example.com", 1},
		{"with path", "https://example.com/path?q=1", 1},
		{"no scheme", "example.com", 0},
	}

	r := NewURLRecognizer()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entities) != tt.wantHits {
				t.Errorf("got %d entities, want %d: %+v", len(entities), tt.wantHits, entities)
			}
		})
	}
}

func TestIBANRecognizer(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantHits int
	}{
		{"german", "DE89370400440532013000", 1},
		{"with spaces", "GB82 WEST 1234 5698 7654 32", 1},
		{"too short", "DE89", 0},
	}

	r := NewIBANRecognizer()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entities) != tt.wantHits {
				t.Errorf("got %d entities, want %d: %+v", len(entities), tt.wantHits, entities)
			}
		})
	}
}

func TestBitcoinRecognizer(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantHits int
	}{
		{"p2pkh", "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", 1},
		{"bech32", "bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq", 1},
		{"taproot", "bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqzk5jj0", 1},
		{"taproot-bad-checksum", "bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqzk5jj1", 0},
		{"random", "not a bitcoin address", 0},
	}

	r := NewBitcoinRecognizer()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entities) != tt.wantHits {
				t.Errorf("got %d entities, want %d: %+v", len(entities), tt.wantHits, entities)
			}
		})
	}
}

// TestURLRecognizerExactSpan asserts the matched text, not a hit count. A count
// cannot see a URL that keeps a sentence-final period, so each case pins the
// exact span.
func TestURLRecognizerExactSpan(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"trailing period", "See https://acme.example.com/status.", "https://acme.example.com/status"},
		{"trailing comma", "Visit https://example.com, then reply.", "https://example.com"},
		{"trailing question mark", "Did you read https://example.com/faq?", "https://example.com/faq"},
		{"query string kept whole", "Open https://shop.example.net/orders?id=4821&view=full now", "https://shop.example.net/orders?id=4821&view=full"},
		{"balanced parens kept", "See https://en.wikipedia.org/wiki/Foo_(bar) today", "https://en.wikipedia.org/wiki/Foo_(bar)"},
		{"markdown parens trimmed", "[docs](https://example.com/a) here", "https://example.com/a"},
		{"stacked closer and period", "(see https://x.example.com/a).", "https://x.example.com/a"},
		{"trailing slash kept", "Root is https://example.com/ today", "https://example.com/"},
		{"single quotes trimmed", "the 'https://example.com/q' page", "https://example.com/q"},
		{"bare scheme drops out", "nothing at http://. here", ""},
	}

	r := NewURLRecognizer()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities, err := r.Recognize(ctx, tt.text)
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
				t.Fatalf("got %d entities, want 1: %+v", len(entities), entities)
			}
			if entities[0].Text != tt.want {
				t.Errorf("Text = %q, want %q", entities[0].Text, tt.want)
			}
		})
	}
}
