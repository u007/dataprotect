package secret

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// Every token below is fictional. AKIAIOSFODNN7EXAMPLE is the AWS documentation
// example. The rest are random-looking strings of the documented length, so a
// secret scanner such as gitleaks flags them by design.
const (
	fakeAWSKey      = "AKIAIOSFODNN7EXAMPLE"
	fakeGHTail      = "Q7rT2vN9bLx4Wc8Hm1Pd6Ys3Kf0Zj5Gu8Ea2"
	fakeGHToken     = "ghp_" + fakeGHTail
	fakeFineGrained = "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"
	fakeSlackBot    = "xoxb-1234567890-1234567890-Q7rT2vN9bLx4Wc8Hm1Pd6Ys3"
	fakeStripeLive  = "sk_live_Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys"
	fakeGoogleKey   = "AIzaSyD7kP2mQ9xR4vL8nB1cT6wF3hJ0zG5aEQx"
	fakeGeminiAuth  = "AQ.Ab8R4xN6ig1shM8Q9xUooLyPtHooh8X2azsxiburzg4TZgQ7cW"
	fakeNPMToken    = "npm_Q7rT2vN9bLx4Wc8Hm1Pd6Ys3Kf0Zj5Gu8EaK"
	fakeSendGrid    = "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"
)

// TestBareProviderTokenSpan checks that each provider format with no label is
// reported over the whole token, and nothing else.
func TestBareProviderTokenSpan(t *testing.T) {
	r := NewSecretRecognizer()
	ctx := context.Background()

	tests := []struct {
		name string
		text string
		want string
	}{
		{"aws access key id", "Rotate the key " + fakeAWSKey + " before Friday.", fakeAWSKey},
		{"aws key id glued to an underscore", "key_" + fakeAWSKey + " in the config", fakeAWSKey},
		{"aws key id glued to a letter", "key" + fakeAWSKey + " in the config", fakeAWSKey},
		{"aws temporary key id", "session ASIAZ7KQ3WM5RT2PXN8D expires soon", "ASIAZ7KQ3WM5RT2PXN8D"},
		{"github classic token", "clone with " + fakeGHToken + " for now", fakeGHToken},
		{"github token one character long", fakeGHToken + "x", fakeGHToken + "x"},
		{"github token glued to an underscore", "token_ghp_" + fakeGHTail + " end", "ghp_" + fakeGHTail},
		{"github oauth token", "gho_" + fakeGHTail + " in the callback", "gho_" + fakeGHTail},
		{"github user-to-server token", "ghu_" + fakeGHTail, "ghu_" + fakeGHTail},
		{"github server-to-server token", "ghs_" + fakeGHTail + " from the app", "ghs_" + fakeGHTail},
		{"github refresh token", "ghr_" + fakeGHTail + " kept", "ghr_" + fakeGHTail},
		{"github fine-grained token", "use " + fakeFineGrained + " here", fakeFineGrained},
		{"slack bot token", "bot " + fakeSlackBot + " posts", fakeSlackBot},
		{"slack user token", "xoxp-1234567890-1234567890-1234567890-Q7rT2vN9bLx4Wc8Hm1Pd6Ys3Kf0Zj5G in the env", "xoxp-1234567890-1234567890-1234567890-Q7rT2vN9bLx4Wc8Hm1Pd6Ys3Kf0Zj5G"},
		{"stripe live secret key", "charge with " + fakeStripeLive, fakeStripeLive},
		{"stripe live restricted key", "rk_live_Hm1Pd6Ys3Kf0Zj5Gu8Ea2Bq7 for refunds", "rk_live_Hm1Pd6Ys3Kf0Zj5Gu8Ea2Bq7"},
		{"stripe test secret key", "charge with sk_test_Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys", "sk_test_Zq7Rt2vN9bLx4Wc8Hm1Pd6Ys"},
		{"google api key", "maps " + fakeGoogleKey + " in the page", fakeGoogleKey},
		{"google gemini auth key", "gemini " + fakeGeminiAuth + " in the env", fakeGeminiAuth},
		{"gemini auth key glued to an underscore", "key_" + fakeGeminiAuth + " end", fakeGeminiAuth},
		{"gemini auth key one character long", fakeGeminiAuth + "x", fakeGeminiAuth + "x"},
		{"npm access token", "publish with " + fakeNPMToken, fakeNPMToken},
		{"sendgrid api key", fakeSendGrid + " in the mailer", fakeSendGrid},
		{"bare jwt", "the session is " + testJWT + " for now", testJWT},
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
			if ents[0].Text != tt.want {
				t.Fatalf("span = %q, want %q", ents[0].Text, tt.want)
			}
			if ents[0].Type != recognizers.EntitySecret {
				t.Fatalf("type = %v, want SECRET", ents[0].Type)
			}
		})
	}
}

// TestBareProviderTokenRejected covers the negatives: a wrong length, a prefix
// with no token, a lower-case variant and a prefix mentioned in prose must not
// report anything.
func TestBareProviderTokenRejected(t *testing.T) {
	r := NewSecretRecognizer()
	ctx := context.Background()

	tests := []struct {
		name string
		text string
	}{
		{"aws key id one character short", "AKIAIOSFODNN7EXAMPL is cut"},
		{"aws key id one character long", "AKIAIOSFODNN7EXAMPLEX is glued"},
		{"aws key id lower-case", "akiaiosfodnn7example"},
		{"aws key id lower-case tail", "AKIAiosfodnn7example"},
		{"aws key id uniform", "AKIAAAAAAAAAAAAAAAAA"},
		{"aws prefix named in prose", "AWS key ids start with AKIA, so grep for it."},
		{"aws key id cut one character short after a glued underscore", "key_AKIAIOSFODNN7EXAMPL end"},
		{"github token one character short", "ghp_" + fakeGHTail[:35] + " end"},
		{"github token upper-case prefix", "GHP_" + fakeGHTail},
		{"github prefix not in the list", "ghx_" + fakeGHTail},
		{"github fine-grained one character short", fakeFineGrained[:len(fakeFineGrained)-1] + " end"},
		{"stripe live key one character short", fakeStripeLive[:len(fakeStripeLive)-1] + " end"},
		{"google key one character short", "AIzaSyD7kP2mQ9xR4vL8nB1cT6wF3hJ0zG5aE"},
		{"google key one character long", fakeGoogleKey + "x"},
		{"gemini auth key body one character below the floor", "AQ.Ab8R4xN6ig1shM8Q9xUooLyPtHooh8X2az end"},
		{"gemini prefix named in prose", "New Gemini keys start with AQ. instead of AIza."},
		{"npm token one character short", "npm_Q7rT2vN9bLx4Wc8Hm1Pd6Ys3Kf0Zj5Gu8Ea"},
		{"sendgrid key short second part", "SG.Q7rT2vN9bLx4Wc8Hm1Pd6Y.Ys3Kf0Zj5Gu8Ea2Bq7Rt4Lm9Nv1Cx6Wp3Hd8Ks0Jf"},
		{"slack bot token digits too few", "xoxb-123-456-Q7rT2vN9bLx4Wc8Hm1Pd6Ys3"},
		{"jwt with no signature", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJrYXRlIn0"},
		{"jwt with a non-jwt payload", "eyJhbGciOiJIUzI1NiJ9.c3ViamVjdHNhbWU.dozjgNtPnIxz"},
		{"jwt with a fourth segment", testJWT + ".extra"},
		{"jwt upper-case header prefix", "EYJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJrYXRlIn0.dozjgNtPnIxz"},
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

// TestBareProviderTokenOneSpanWithLabel checks that a token inside a labelled
// assignment gives one span. The labelled span is kept, since it is the wider one.
func TestBareProviderTokenOneSpanWithLabel(t *testing.T) {
	r := NewSecretRecognizer()
	ctx := context.Background()

	tests := []struct {
		name string
		text string
		want string
	}{
		// The bare AKIA span is the same as the label's, so the two are
		// identical and deduplicated by uniqueSpans.
		{"aws key under access_token label", "access_token=" + fakeAWSKey, fakeAWSKey},
		// The password value takes the `;`, and the bare token stops before it.
		// The bare span overlaps the password span, so only the password is kept.
		{"github token under a password label with a semicolon", "password=" + fakeGHToken + "; next", fakeGHToken + ";"},
		// The api_key value takes a dotted tail, and the bare AKIA stops before
		// the dot. The bare span is inside the label's, so only the label is kept.
		{"aws key under api_key label with a dotted tail", "api_key=" + fakeAWSKey + ".foo", fakeAWSKey + ".foo"},
		// A JSON-quoted value: the label span is the value inside the quotes, the
		// same span as the bare match.
		{"aws key in a json-quoted api_key", `{"api_key": "` + fakeAWSKey + `"}`, fakeAWSKey},
		{"github token under a token label", "token=" + fakeGHToken, fakeGHToken},
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
			if ents[0].Text != tt.want {
				t.Fatalf("span = %q, want %q", ents[0].Text, tt.want)
			}
		})
	}
}
