package secret

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

func TestSecretRecognizerAPIKey(t *testing.T) {
	r := NewSecretRecognizer()
	ctx := context.Background()

	tests := []struct {
		name     string
		text     string
		wantHits int
	}{
		{"api key", `api_key=sk_live_51N4mK2eZvKYlo2C5x7yPq9w`, 1},
		{"aws access key", `access_token=AKIAIOSFODNN7EXAMPLE`, 1},
		{"slack token", `token=xoxb-1234567890-1234567890-abcdefghijklmnop`, 1},
		{"not a secret", `just some random text here`, 0},
		{"too short", `key=abc123`, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ents) != tt.wantHits {
				t.Fatalf("got %d entities, want %d: %+v", len(ents), tt.wantHits, ents)
			}
			for _, e := range ents {
				if e.Type != recognizers.EntitySecret {
					t.Errorf("got type %q, want %q", e.Type, recognizers.EntitySecret)
				}
				if e.Confidence < 0.7 {
					t.Errorf("confidence %f below threshold", e.Confidence)
				}
			}
		})
	}
}

func TestSecretRecognizerPrivateKey(t *testing.T) {
	r := NewSecretRecognizer()
	ctx := context.Background()

	text := `-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEA...
-----END RSA PRIVATE KEY-----`
	ents, err := r.Recognize(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("got %d entities, want 1: %+v", len(ents), ents)
	}
	if ents[0].Type != recognizers.EntitySecret {
		t.Errorf("got type %q, want %q", ents[0].Type, recognizers.EntitySecret)
	}
}

func TestSecretRecognizerBearerToken(t *testing.T) {
	r := NewSecretRecognizer()
	ctx := context.Background()

	text := `Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNtPnIxz`
	ents, err := r.Recognize(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("got %d entities, want 1: %+v", len(ents), ents)
	}
	if ents[0].Type != recognizers.EntitySecret {
		t.Errorf("got type %q, want %q", ents[0].Type, recognizers.EntitySecret)
	}
}
