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
	"strconv"
	"strings"

	"github.com/u007/dataprotect/recognizers"
)

// jwtSegment is one base64url segment of a JWT or JWE.
const jwtSegment = `[A-Za-z0-9_\-]+`

// dottedTail is the optional run of dot-joined segments after a value's base. Every
// segment needs at least one character, so a sentence-final `.` is never taken. It
// has no upper bound, so a token of any segment count is captured to its end. A
// single trailing segment followed by whitespace is trimmed afterwards by tokenEnd,
// so `Bearer <value>.Next step` keeps only the value.
const dottedTail = `(?:\.` + jwtSegment + `)*`

// jwtBase matches a JWT or JWE whose header starts with `eyJ`, the base64url of
// `{"` that every JSON header begins with. It has no length floor on the header,
// so a short header such as `eyJhbGciOiJIUzI1NiJ9` (20 characters) matches. The
// base is a three-segment JWS, or a five-segment JWE whose second segment may be
// empty (the `dir` key management form, `header..iv.ciphertext.tag`). A JWE with a
// non-empty second segment is a JWS-shaped base followed by the dotted tail. The
// prefix is case-sensitive: the shapes are `(?i)`, so the literal is written
// `(?-i:eyJ)`.
const jwtBase = `(?-i:eyJ)` + jwtSegment + `(?:\.\.` + jwtSegment + `\.` + jwtSegment + `\.` + jwtSegment + `|\.` + jwtSegment + `\.` + jwtSegment + `)`

// tokenValue is the value of a labelled token: a JWT or JWE base (jwtBase), or a
// bare run of at least minLen characters, followed by the dotted tail. Group 1 is
// the whole value and group 2 is its base, which tokenEnd needs to trim a tail.
func tokenValue(minLen int) string {
	return `((` + jwtBase + `|[A-Za-z0-9_\-]{` + strconv.Itoa(minLen) + `,})` + dottedTail + `)`
}

// labelSeparator is what may sit between a key name and its `=` or `:`. The key
// name may be followed by a closing quote (JSON `"api_key": ...`), and spaces or
// tabs may precede the separator. A newline does not, so the label stays on its line.
const labelSeparator = `["']?[ \t]*[=:]`

// apiKeyShape matches labelled API keys: a key-name label followed by a
// high-entropy value (e.g. `api_key=sk_live_...`). A provider token with no label
// is matched by bareShape instead. Group 1 is the value and group 2 its base
// (tokenValue): a JWT or JWE, or a bare value of 20 or more characters, with an
// optional dotted tail.
var apiKeyShape = regexp.MustCompile(`(?i)(?:api[_-]?key|apikey|secret[_-]?key|access[_-]?token|auth[_-]?token)` + labelSeparator + `\s*["']?` + tokenValue(20) + `["']?`)

// tokenShape matches bearer/JWT-style tokens and long hex/base64 strings. Group 1
// is the value and group 2 its base (tokenValue): a JWT or JWE, or a bare value of
// 32 or more characters, with an optional dotted tail. The 32-character floor
// applies only to a value that is not a JWT, so a short-header JWT after `Bearer`
// or `token=` is matched.
var tokenShape = regexp.MustCompile(`(?:Bearer\s+|token` + labelSeparator + `\s*["']?)` + tokenValue(32) + `["']?`)

// privateKeyShape matches PEM private key blocks. It has no value part, so the
// whole header is the span.
var privateKeyShape = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)

// tokenByte is a byte of a provider token's character set, the characters a
// token can continue with. A bare match is kept only when no token byte sits
// directly before or after it, so a match is never a fragment of a longer run.
func tokenByte(b byte) bool {
	return b == '_' || b == '-' || ('0' <= b && b <= '9') || ('A' <= b && b <= 'Z') || ('a' <= b && b <= 'z')
}

// bareShape matches provider token formats with no label. Each alternative has a
// distinctive, case-sensitive prefix and a fixed shape, and the match is the whole
// token. The formats, and the source each was checked against, are in SOURCES.md
// ("Provider token formats"). A match is kept only when bareBounded accepts it.
//
// Each alternative has one capture group over its random body, the part after
// the fixed prefix. bareEntities runs the uniform check on that body, so a
// placeholder such as `AKIAAAAAAAAAAAAAAAAA` is rejected. The group is the
// JWT itself for the JWT alternative, which has no fixed prefix to strip.
//
//   - AWS access key id: `AKIA` or `ASIA`, then 16 upper-case letters or digits.
//   - GitHub token: `ghp_`, `gho_`, `ghu_`, `ghs_` or `ghr_`, then 36 alphanumerics;
//     fine-grained `github_pat_`, then 82 characters of alphanumerics or `_`.
//   - Slack bot and user token: `xoxb-` or `xoxp-`, then 10 to 13 digits per group.
//   - Stripe live secret or restricted key: `sk_live_` or `rk_live_`, then 24 or
//     more alphanumerics.
//   - Google API key: `AIza`, then 35 characters of letters, digits, `_` or `-`.
//   - npm access token: `npm_`, then 36 alphanumerics.
//   - SendGrid API key: `SG.`, 22 characters, `.`, 43 characters.
//   - JWT with no label: an `eyJ` header and an `ey` payload, each at least 8
//     characters, then a signature of at least 8 characters. A JWT is three
//     segments, so a fourth segment after the signature makes the match unbounded
//     and it is dropped.
var bareShape = regexp.MustCompile(strings.Join([]string{
	`(?:AKIA|ASIA)([0-9A-Z]{16})`,
	`gh[pousr]_([0-9A-Za-z]{36})`,
	`github_pat_([0-9A-Za-z_]{82})`,
	`xoxb-[0-9]{10,13}-[0-9]{10,13}-([0-9A-Za-z]{24,})`,
	`xoxp-[0-9]{10,13}-[0-9]{10,13}-[0-9]{10,13}-([0-9A-Za-z]{28,34})`,
	`xox[ars]-([0-9A-Za-z-]{20,})`,
	`(?:sk|rk)_(?:live|test|org)_([0-9A-Za-z]{24,})`,
	`AIza([0-9A-Za-z_\-]{35})`,
	`npm_([0-9A-Za-z]{36})`,
	`SG\.[0-9A-Za-z_\-]{22}\.([0-9A-Za-z_\-]{43})`,
	`(eyJ[0-9A-Za-z_\-]{8,}\.ey[0-9A-Za-z_\-]{8,}\.[0-9A-Za-z_\-]{8,})`,
	`(eyJ[0-9A-Za-z_\-]{8,}\.[0-9A-Za-z_\-]*\.[0-9A-Za-z_\-]{8,}\.[0-9A-Za-z_\-]{8,}\.[0-9A-Za-z_\-]{8,})`,
}, "|"))

// bareBounded reports whether text[start:end] is a whole token. No token byte may
// sit directly before or after it, and a `.` followed by a token byte is not
// allowed after it, so a longer dotted token is not cut short. That last case is
// a JWT with a fourth segment, and an SG key with more segments. Such a match is
// dropped rather than reported as a prefix that leaves the rest visible.
func bareBounded(text string, start, end int) bool {
	if start > 0 && tokenByte(text[start-1]) {
		return false
	}
	if end < len(text) {
		if tokenByte(text[end]) {
			return false
		}
		if text[end] == '.' && end+1 < len(text) && tokenByte(text[end+1]) {
			return false
		}
	}
	return true
}

// passwordShape matches password assignments in config-like contexts. Group 1 is
// the value: at least 8 characters with no space, quote or line break.
//
// The short labels `pass` and `pwd` have a left boundary: the byte before them
// must not be an ASCII letter, or the match is at the start of text. So
// `bypass=kittenmitten` and `Route compass: northeastregion` are not passwords,
// while `db_pass=`, `db-pwd=`, `DB_PASS=` and `"pass": ` still are. The
// boundary is a non-capturing context group inside the alternation, so group 1
// stays the value. `password`, `passwd` and `secret` have no boundary, so
// `dbpassword=` and `mysecret=` match. Accepted cost: a letter directly before
// `pass` or `pwd` (`dbpass=x`, `rootpwd=x`) is no longer matched.
var passwordShape = regexp.MustCompile(`(?i)(?:password|passwd|secret|(?:^|(?-i:[^A-Za-z]))(?:pass|pwd))` + labelSeparator + `\s*["']?([^\s"']{8,})["']?`)

// SecretRecognizer detects secrets in text.
type SecretRecognizer struct {
	apiKey     *regexp.Regexp
	token      *regexp.Regexp
	privateKey *regexp.Regexp
	password   *regexp.Regexp
	bare       *regexp.Regexp
}

// NewSecretRecognizer creates a secret recognizer.
func NewSecretRecognizer() *SecretRecognizer {
	return &SecretRecognizer{
		apiKey:     apiKeyShape,
		token:      tokenShape,
		privateKey: privateKeyShape,
		password:   passwordShape,
		bare:       bareShape,
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
//
// A bare provider token (bareShape) is dropped when it overlaps a labelled span.
// The labelled span is the wider one: it can take a trailing `;` from a password
// or a dotted tail from an api_key, so the token is reported once, with its label.
func (r *SecretRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var labelled []recognizers.Entity

	labelled = append(labelled, valueEntities(r.apiKey, text, tokenEnd)...)
	labelled = append(labelled, valueEntities(r.token, text, tokenEnd)...)
	labelled = append(labelled, valueEntities(r.password, text, passwordEnd)...)

	entities := labelled
	for _, m := range r.privateKey.FindAllStringIndex(text, -1) {
		entities = append(entities, secretEntity(text, m[0], m[1], 0.99))
	}
	for _, e := range bareEntities(r.bare, text) {
		if !overlapsAny(e, labelled) {
			entities = append(entities, e)
		}
	}

	return uniqueSpans(entities), nil
}

// bareEntities returns one entity per bare provider token that is bounded and
// whose body passes isLikelySecret. The span is the whole match, the token with
// its prefix. The body is the capture group of the alternative that matched.
func bareEntities(re *regexp.Regexp, text string) []recognizers.Entity {
	var entities []recognizers.Entity
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		if !bareBounded(text, m[0], m[1]) || !isLikelySecret(bareBody(text, m)) {
			continue
		}
		entities = append(entities, secretEntity(text, m[0], m[1], 0.98))
	}
	return entities
}

// bareBody returns the body of a bareShape match: the text of the one capture
// group that participated, since each alternative has exactly one group.
func bareBody(text string, m []int) string {
	for g := 2; g+1 < len(m); g += 2 {
		if m[g] >= 0 {
			return text[m[g]:m[g+1]]
		}
	}
	// Every bareShape alternative has a group that always participates, so a
	// match with none is a broken pattern, not an input to skip.
	panic("secret: bareShape match has no body group")
}

// overlapsAny reports whether e shares at least one byte with any of others.
func overlapsAny(e recognizers.Entity, others []recognizers.Entity) bool {
	for _, o := range others {
		if e.Start < o.End && o.Start < e.End {
			return true
		}
	}
	return false
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
// passes isLikelySecret. Every shape that uses it has the value as group 1. trim
// gives the value's end from the match's submatch indices, so the sentence
// punctuation that a shape cannot leave out of its match is dropped. The
// uniform check runs on the trimmed value.
func valueEntities(re *regexp.Regexp, text string, trim func(text string, m []int) int) []recognizers.Entity {
	var entities []recognizers.Entity
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		start, end := m[2], trim(text, m)
		if !isLikelySecret(text[start:end]) {
			continue
		}
		entities = append(entities, secretEntity(text, start, end, 0.98))
	}
	return entities
}

// tokenEnd returns the end of a labelled JWT or token value. The tail after the
// base is a run of dotted segments. A tail of exactly one segment followed by
// whitespace is sentence prose (`Bearer <value>.Next step`), so the value ends at
// the base. Every other tail is kept: two or more segments, or one segment directly
// followed by a non-space byte or the end of text. That last case is the
// fail-closed reading of `token=<value>.abc`, which a shape cannot tell from a
// dotted word that ends a sentence.
func tokenEnd(text string, m []int) int {
	baseEnd, end := m[5], m[3]
	if strings.Count(text[baseEnd:end], ".") == 1 && end < len(text) && isSpaceByte(text[end]) {
		return baseEnd
	}
	return end
}

// closerOpener maps each closing bracket that passwordEnd may drop to its opener.
var closerOpener = map[byte]byte{')': '(', ']': '[', '}': '{'}

// passwordEnd returns the end of a password value, when whitespace or the end of
// text follows it. A last byte of `.` is a sentence-final period, which is dropped:
// `password: Rainbow@2025.` reports `Rainbow@2025`. Only one period is dropped, and
// `!`, `?` and `,` are kept because a password may end with them. A last byte that
// closes a bracket is dropped when the value holds no opener for it, so
// `(pass=Hunter2xyz)` reports `Hunter2xyz`, while `Pa(ss)w0rd1` keeps its balanced
// parentheses and `password=abc)def12345` keeps its inner `)`. The 8-character
// floor is checked on the match before this trim, so `password: Abcdefg.` reports
// `Abcdefg`.
func passwordEnd(text string, m []int) int {
	end := m[3]
	last := text[end-1]
	if last == '.' && (end == len(text) || isSpaceByte(text[end])) {
		return end - 1
	}
	if opener, ok := closerOpener[last]; ok {
		if (end == len(text) || isSpaceByte(text[end]) || text[end] == 34 || text[end] == 39) && strings.IndexByte(text[m[2]:end-1], opener) < 0 {
			return end - 1
		}
	}
	return end
}

// isSpaceByte reports whether b is a byte that the regular expression `\s` matches.
func isSpaceByte(b byte) bool {
	return strings.IndexByte(" \t\n\f\r", b) >= 0
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
