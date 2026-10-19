package secret

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// testJWT is the public jwt.io example token: a header, a payload and a signature,
// each base64url. The header is 36 characters, above the token floor.
const testJWT = `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNtPnIxz`

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

// TestSecretRecognizerValueSpan checks that a label-based match is reported over
// the captured value only: the key name, separator, quotes and `Bearer ` scheme
// stay outside the span.
func TestSecretRecognizerValueSpan(t *testing.T) {
	r := NewSecretRecognizer()
	ctx := context.Background()

	tests := []struct {
		name string
		text string
		want string
	}{
		{"api key", `Set api_key=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys on the worker`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys`},
		{"token", `Pipeline variable ci_token=Vb4Nm8Qw2Er6Ty0Ui3Op7As1Df5Gh9Jk2Lz6Xc is masked`, `Vb4Nm8Qw2Er6Ty0Ui3Op7As1Df5Gh9Jk2Lz6Xc`},
		{"bearer token", `Authorization: Bearer Hk2Mw9Qe4Rt7Yu1Io6Pa3Sd8Fg5Hj0Kl2Zx4C`, `Hk2Mw9Qe4Rt7Yu1Io6Pa3Sd8Fg5Hj0Kl2Zx4C`},
		{"password", `DB_HOST=db.internal.example password=Tr0ub4dor&Fixture`, `Tr0ub4dor&Fixture`},
		{"quoted password", `password: "Fixture-Pass-2024!"`, `Fixture-Pass-2024!`},
		{"access token of 32 characters reported once", `access_token=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq`},
		{"auth token of 32 characters reported once", `auth_token=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq`},
		{"jwt bearer token", `Authorization: Bearer ` + testJWT, testJWT},
		{"jwt after token label", `Set token=` + testJWT + ` for the sync job`, testJWT},
		{"jwt access token reported once", `access_token=` + testJWT, testJWT},
		{"dotted prose after a token keeps the bare value", `token=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.Next step`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq`},
		{"dotted prose after an api key keeps the bare value", `api_key=Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq.Next step`, `Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys3Tg0Bn5Kq`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ents) != 1 {
				t.Fatalf("got %d entities, want 1: %+v", len(ents), ents)
			}
			e := ents[0]
			if got := tt.text[e.Start:e.End]; got != tt.want {
				t.Errorf("span text %q, want %q", got, tt.want)
			}
			if e.Text != tt.want {
				t.Errorf("Text %q, want %q", e.Text, tt.want)
			}
		})
	}
}

// TestSecretRecognizerRejectsShortOrUniformValue pins what the value-only rules
// reject. The two short rows fall below the per-shape floors in the regexes. The
// uniform-digit row is the case the old whole-match rule accepted, because the key
// name supplied the lower-case and separator classes.
func TestSecretRecognizerRejectsShortOrUniformValue(t *testing.T) {
	r := NewSecretRecognizer()
	ctx := context.Background()

	tests := []struct {
		name string
		text string
	}{
		{"api key value under 20 characters", `api_key=Zq7Rt2vN9bLx4Wc8Hm1`},
		{"password value under 8 characters", `password=Tr0ub4d`},
		{"uniform digit api key value", `api_key=00000000000000000000`},
		{"uniform letter password value", `password=aaaaaaaaaaaaaaaaaaaaaaaaa`},
		{"bearer followed by short dotted word", `Authorization: Bearer example.config.yaml`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents, err := r.Recognize(ctx, tt.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ents) != 0 {
				t.Fatalf("got %d entities, want 0: %+v", len(ents), ents)
			}
		})
	}
}
