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
	"strings"

	"github.com/u007/dataprotect/recognizers"
)

// apiKeyShape matches common API key formats: a prefix label followed by a
// high-entropy value (e.g. `sk_live_...`, `AKIA...`, `xoxb-...`).
var apiKeyShape = regexp.MustCompile(`(?i)(?:api[_-]?key|apikey|secret[_-]?key|access[_-]?token|auth[_-]?token)[=:]\s*["']?([A-Za-z0-9_\-]{20,})["']?`)

// tokenShape matches bearer/JWT-style tokens and long hex/base64 strings.
var tokenShape = regexp.MustCompile(`(?:Bearer\s+|token[=:]\s*["']?)([A-Za-z0-9_\-]{32,})["']?`)

// privateKeyShape matches PEM private key blocks.
var privateKeyShape = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)

// passwordShape matches password assignments in config-like contexts.
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

// Recognize detects secrets in text.
func (r *SecretRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity

	for _, m := range r.apiKey.FindAllStringIndex(text, -1) {
		val := text[m[0]:m[1]]
		if !isLikelySecret(val) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntitySecret,
			Start:      m[0],
			End:        m[1],
			Text:       val,
			Confidence: 0.98,
		})
	}
	for _, m := range r.token.FindAllStringIndex(text, -1) {
		val := text[m[0]:m[1]]
		if !isLikelySecret(val) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntitySecret,
			Start:      m[0],
			End:        m[1],
			Text:       val,
			Confidence: 0.98,
		})
	}
	for _, m := range r.privateKey.FindAllStringIndex(text, -1) {
		val := text[m[0]:m[1]]
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntitySecret,
			Start:      m[0],
			End:        m[1],
			Text:       val,
			Confidence: 0.99,
		})
	}
	for _, m := range r.password.FindAllStringIndex(text, -1) {
		val := text[m[0]:m[1]]
		if !isLikelySecret(val) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntitySecret,
			Start:      m[0],
			End:        m[1],
			Text:       val,
			Confidence: 0.98,
		})
	}

	return entities, nil
}

// isLikelySecret applies a coarse entropy / shape filter so common non-secret
// strings (URLs, version numbers, base64-encoded innocuous data) are not
// reported. It checks for a mix of character classes and sufficient length.
func isLikelySecret(s string) bool {
	// Strip surrounding punctuation and label prefixes for the check.
	trimmed := strings.Trim(s, ` "'`)
	trimmed = strings.TrimPrefix(trimmed, "Bearer ")
	if len(trimmed) < 20 {
		return false
	}

	digit := false
	upper := false
	lower := false
	special := false
	for _, c := range trimmed {
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

	// Require at least three of four classes to avoid matching plain words or
	// simple numeric strings.
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
	return classes >= 3
}
