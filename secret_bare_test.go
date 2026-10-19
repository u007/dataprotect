package piiscrub

import (
	"context"
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// A provider token inside a URL is handled by the URL, not the secret. The URL
// recognizer scores 0.99 and the secret recognizer 0.98, so on overlap the URL
// span wins and the whole URL is redacted. These tests pin that outcome for a
// token in the query string, in the path and in the userinfo, and check that
// the Scrubber leaves no part of the URL or the token visible.
func TestBareProviderTokenInsideURLIsRedactedWhole(t *testing.T) {
	const ghToken = "ghp_Q7rT2vN9bLx4Wc8Hm1Pd6Ys3Kf0Zj5Gu8Ea2"
	const awsKey = "AKIAIOSFODNN7EXAMPLE"

	tests := []struct {
		name string
		text string
		url  string
	}{
		{"token in query string", "see https://deploy.example.com/hook?token=" + ghToken + "&x=1 for details", "https://deploy.example.com/hook?token=" + ghToken + "&x=1"},
		{"token in the path", "fetch https://keys.example.com/" + awsKey + "/list today", "https://keys.example.com/" + awsKey + "/list"},
		{"token in the userinfo", "clone https://" + ghToken + "@github.example.com/acme/app.git now", "https://" + ghToken + "@github.example.com/acme/app.git"},
	}

	s := NewScrubber()
	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents, err := s.Analyze(ctx, tt.text)
			if err != nil {
				t.Fatalf("analyze: %v", err)
			}
			var urls, secrets []recognizers.Entity
			for _, e := range ents {
				switch e.Type {
				case recognizers.EntityURL:
					urls = append(urls, e)
				case recognizers.EntitySecret:
					secrets = append(secrets, e)
				}
			}
			if len(urls) != 1 || urls[0].Text != tt.url {
				t.Fatalf("URL entities = %+v, want one span %q", urls, tt.url)
			}
			if len(secrets) != 0 {
				t.Fatalf("secret entities = %+v, want none inside the URL", secrets)
			}

			out, err := s.Anonymize(ctx, tt.text)
			if err != nil {
				t.Fatalf("anonymize: %v", err)
			}
			for _, leak := range []string{ghToken, awsKey, "example.com", "/hook", "/list", "acme/app"} {
				if strings.Contains(out, leak) {
					t.Fatalf("output %q still shows %q", out, leak)
				}
			}
		})
	}
}
