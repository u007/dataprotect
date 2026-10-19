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

// jwtSegment is one base64url segment of a JWT or JWE.
const jwtSegment = `[A-Za-z0-9_\-]+`

// dottedTail is the optional run of dot-joined segments after a value's first
// segment. It needs at least two segments, so a value followed by one dotted word,
// such as the `.Next` in `Bearer <value>.Next step`, stops at the value. It has no
// upper bound, so a token of any segment count is captured to its end. Every
// segment needs at least one character, so a sentence-final `.` is never taken.
const dottedTail = `(?:(?:\.` + jwtSegment + `){2,})?`

// jwtValue matches a JWT or JWE whose header starts with `eyJ`, the base64url of
// `{"` that every JSON header begins with. It has no length floor on the header,
// so a short header such as `eyJhbGciOiJIUzI1NiJ9` (20 characters) matches. The
// base is a three-segment JWS, or a five-segment JWE whose second segment may be
// empty (the `dir` key management form, `header..iv.ciphertext.tag`). The prefix is
// case-sensitive: the shapes are `(?i)`, so the literal is written `(?-i:eyJ)`.
const jwtValue = `(?:(?-i:eyJ)` + jwtSegment + `(?:\.\.` + jwtSegment + `\.` + jwtSegment + `\.` + jwtSegment + `|\.` + jwtSegment + `\.` + jwtSegment + `)` + dottedTail + `)`

// labelSeparator is what may sit between a key name and its `=` or `:`. The key
// name may be followed by a closing quote (JSON `"api_key": ...`), and spaces or
// tabs may precede the separator. A newline does not, so the label stays on its line.
const labelSeparator = `["']?[ \t]*[=:]`

// apiKeyShape matches common API key formats: a prefix label followed by a
// high-entropy value (e.g. `sk_live_...`, `AKIA...`, `xoxb-...`). Group 1 is the
// value: a JWT or JWE (jwtValue), or a bare value of 20 or more characters with
// an optional dotted tail (dottedTail).
var apiKeyShape = regexp.MustCompile(`(?i)(?:api[_-]?key|apikey|secret[_-]?key|access[_-]?token|auth[_-]?token)` + labelSeparator + `\s*["']?(` + jwtValue + `|[A-Za-z0-9_\-]{20,}` + dottedTail + `)["']?`)

// tokenShape matches bearer/JWT-style tokens and long hex/base64 strings. Group 1
// is the value: a JWT or JWE (jwtValue), or a bare value of 32 or more characters
// with an optional dotted tail (dottedTail). The 32-character floor applies only to
// a value that is not a JWT, so a short-header JWT after `Bearer` or `token=` is
// matched.
var tokenShape = regexp.MustCompile(`(?:Bearer\s+|token` + labelSeparator + `\s*["']?)(` + jwtValue + `|[A-Za-z0-9_\-]{32,}` + dottedTail + `)["']?`)

// privateKeyShape matches PEM private key blocks. It has no value part, so the
// whole header is the span.
var privateKeyShape = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)

// passwordShape matches password assignments in config-like contexts. Group 1 is
// the value: at least 8 characters with no space, quote or line break.
var passwordShape = regexp.MustCompile(`(?i)(?:password|passwd|pass|pwd|secret)` + labelSeparator + `\s*["']?([^\s"']{8,})["']?`)

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
// `access_token=` or `auth_token=` value that is a JWT, or 32 or more characters,
// and report the same span, so the later duplicate is dropped and the first in
// emission order is kept. The result is sorted by Start, with ties in emission order.
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

// maxUniformUnit is the longest repeated unit that still marks a value as a
// uniform placeholder. `abab...` (unit 2) and `abcdabcd...` (unit 4) are rejected.
const maxUniformUnit = 4

// isLikelySecret reports whether a captured value is credential material rather
// than a uniform placeholder. It receives the value only, never the key name or
// separator.
//
// It has no character-class rule. Once a shape's length floor is met (apiKey 20,
// token 32, password 8, JWT with no header floor), the value is reported, so a
// lower-case passphrase, an all-digit token and a dotted lower-case password are
// all reported. The only values rejected are uniform ones: every character
// identical (`00000000000000000000`), or one repeated unit of up to
// maxUniformUnit bytes, which needs at least two full repeats (`abababab`).
//
// Known tradeoff, accepted because the scrubber prefers over-redaction: a plain
// word of 8 or more characters after a label, such as `Password: Required`, is
// reported. The label is the only context; the value is not checked for shape.
func isLikelySecret(value string) bool {
	for unit := 1; unit <= maxUniformUnit && 2*unit <= len(value); unit++ {
		if repeatsEvery(value, unit) {
			return false
		}
	}
	return true
}

// repeatsEvery reports whether value is its first unit bytes repeated, with every
// byte after the first unit equal to the byte unit positions earlier.
func repeatsEvery(value string, unit int) bool {
	for i := unit; i < len(value); i++ {
		if value[i] != value[i-unit] {
			return false
		}
	}
	return true
}
