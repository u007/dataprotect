// Package secret detects secrets such as API keys, tokens, passwords, private
// keys and other high-entropy credential material. Each recognizer matches a
// shape and keeps only matches that pass its validator, so a shape match alone
// is never reported.
//
// The root module stays stdlib-only: no external entropy or crypto libraries are
// used. Confidence is kept at 0.98 so secrets are reported but can still be
// outranked by a higher-confidence PII type on overlap.
package secret

import (
	"context"
	"regexp"
	"sort"

	"github.com/u007/dataprotect/recognizers"
)

// jwtTail is the optional part of a value that makes it a JWT: exactly two more
// dot-joined base64url segments after the first. It is shared by apiKeyShape and
// tokenShape so that `access_token=<JWT>` gives both shapes the same whole-JWT
// span. Without it the value stops at the first `.`. A value with one dot or
// three or more dots matches the first segment alone, as it always has.
const jwtTail = `(?:\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+)?`

// apiKeyShape matches common API key formats: a prefix label followed by a
// high-entropy value (e.g. `sk_live_...`, `AKIA...`, `xoxb-...`). Group 1 is the
// value, optionally a JWT.
var apiKeyShape = regexp.MustCompile(`(?i)(?:api[_-]?key|apikey|secret[_-]?key|access[_-]?token|auth[_-]?token)[=:]\s*["']?([A-Za-z0-9_\-]{20,}` + jwtTail + `)["']?`)

// tokenShape matches bearer/JWT-style tokens and long hex/base64 strings. Group 1
// is the value, optionally a JWT.
//
// The 32-character floor applies to the first segment of a JWT, which is the
// header. A JWT with a shorter header, such as `eyJhbGciOiJIUzI1NiJ9` (20
// characters), matches neither the Bearer nor the token= form at all. That gap is
// tracked in TODO.md.
var tokenShape = regexp.MustCompile(`(?:Bearer\s+|token[=:]\s*["']?)([A-Za-z0-9_\-]{32,}` + jwtTail + `)["']?`)

// privateKeyShape matches PEM private key blocks. It has no value part, so the
// whole header is the span.
var privateKeyShape = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)

// passwordShape matches password assignments in config-like contexts. Group 1 is
// the value.
var passwordShape = regexp.MustCompile(`(?i)(?:password|passwd|pass|pwd|secret)[=:]\s*["']?([^\s"']{8,})["']?`)

// SecretRecognizer detects secrets in text.
type SecretRecognizer struct {
	apiKey     *regexp.Regexp
	token      *regexp.Regexp
	privateKey *regexp.Regexp
	password   *regexp.Regexp
}

// NewSecretRecognizer creates a secret recognizer.
func NewSecretRecognizer() *SecretRecognizer {
	return &SecretRecognizer{
		apiKey:     apiKeyShape,
		token:      tokenShape,
		privateKey: privateKeyShape,
		password:   passwordShape,
	}
}

// Name returns the recognizer name.
func (r *SecretRecognizer) Name() string { return "secret" }

// Recognize detects secrets in text. A label-based match is reported over the
// captured value only, so the span excludes the key name, separator, quotes and
// the `Bearer ` scheme. The PEM header is reported whole, since it has no value.
//
// Each (Start, End) is reported once. The apiKey and token shapes both match an
// `access_token=` or `auth_token=` value of 32 or more characters and report the
// same span, so the later duplicate is dropped and the first in emission order is
// kept. The result is sorted by Start, with ties in emission order.
func (r *SecretRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity

	entities = append(entities, valueEntities(r.apiKey, text)...)
	entities = append(entities, valueEntities(r.token, text)...)
	for _, m := range r.privateKey.FindAllStringIndex(text, -1) {
		entities = append(entities, secretEntity(text, m[0], m[1], 0.99))
	}
	entities = append(entities, valueEntities(r.password, text)...)

	return uniqueSpans(entities), nil
}

// uniqueSpans keeps the first entity for each (Start, End), in emission order,
// and returns the survivors sorted by Start.
func uniqueSpans(entities []recognizers.Entity) []recognizers.Entity {
	type span struct{ start, end int }
	seen := make(map[span]struct{}, len(entities))
	unique := entities[:0]
	for _, e := range entities {
		key := span{e.Start, e.End}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, e)
	}
	sort.SliceStable(unique, func(i, j int) bool { return unique[i].Start < unique[j].Start })
	return unique
}

// valueEntities returns one entity per match of re whose value (capture group 1)
// passes isLikelySecret. Every shape that uses it has the value as group 1.
func valueEntities(re *regexp.Regexp, text string) []recognizers.Entity {
	var entities []recognizers.Entity
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		start, end := m[2], m[3]
		if !isLikelySecret(text[start:end]) {
			continue
		}
		entities = append(entities, secretEntity(text, start, end, 0.98))
	}
	return entities
}

// secretEntity builds a secret entity over text[start:end].
func secretEntity(text string, start, end int, confidence float64) recognizers.Entity {
	return recognizers.Entity{
		Type:       recognizers.EntitySecret,
		Start:      start,
		End:        end,
		Text:       text[start:end],
		Confidence: confidence,
	}
}

// isLikelySecret reports whether a captured value looks like credential material
// rather than a plain word or a uniform placeholder. It receives the value only,
// never the key name or separator.
//
// It has no length check of its own: each shape's quantifier already sets the
// floor for its value (apiKey 20, token 32, password 8), so a password of 17
// characters passes while an API key of 17 does not.
//
// It requires two of the four character classes. Three would reject real
// credentials that use fewer classes: hex digests (digits and lower case) and AWS
// access key IDs (upper case and digits). One class is what uniform placeholders
// such as `00000000000000000000` or `aaaaaaaaaaaaaaaaaaaa` use, so that is what
// it rejects.
//
// Known tradeoff, accepted because the scrubber prefers over-redaction: form-style
// text such as `Password: Required` (two classes, 8 characters or more) is
// reported.
func isLikelySecret(value string) bool {
	digit := false
	upper := false
	lower := false
	special := false
	for _, c := range value {
		switch {
		case c >= '0' && c <= '9':
			digit = true
		case c >= 'A' && c <= 'Z':
			upper = true
		case c >= 'a' && c <= 'z':
			lower = true
		default:
			special = true
		}
	}

	classes := 0
	if digit {
		classes++
	}
	if upper {
		classes++
	}
	if lower {
		classes++
	}
	if special {
		classes++
	}
	return classes >= 2
}
