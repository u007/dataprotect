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
		{"plain", "123456789", 1},
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
