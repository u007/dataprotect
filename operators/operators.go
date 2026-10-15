// Package operators provides anonymization operators for PII entities.
package operators

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"
)

// Operator transforms a single detected entity's text into its masked form.
//
// Operate on one value at a time: the caller owns string splicing, because a
// replacement changes the length of the text and invalidates the byte offsets
// of every entity that has not been rewritten yet. Applying operators to
// already-spliced text would require re-deriving all remaining spans after each
// replacement.
type Operator interface {
	Name() string
	Replace(ctx context.Context, value string) (string, error)
}

// ReplaceOperator replaces entities with a fixed string.
type ReplaceOperator struct {
	replacement string
}

// NewReplaceOperator creates a new replace operator.
func NewReplaceOperator(replacement string) *ReplaceOperator {
	return &ReplaceOperator{replacement: replacement}
}

// Name returns the operator name.
func (o *ReplaceOperator) Name() string { return "replace" }

// Replace returns the replacement string for any value.
func (o *ReplaceOperator) Replace(ctx context.Context, value string) (string, error) {
	return o.replacement, nil
}

// MaskOperator partially masks entities.
type MaskOperator struct {
	maskPattern string
}

// NewMaskOperator creates a new mask operator.
// maskPattern uses '#' for characters to keep and '*' for characters to mask.
// Example: "****-****-####" keeps last 4 digits of a credit card.
//
// The pattern is interpreted as UTF-8; an invalid byte in the PATTERN is read as
// U+FFFD (patterns are developer constants, so this is a degenerate case). This
// is separate from the input handling, which is byte-faithful for invalid bytes.
func NewMaskOperator(maskPattern string) *MaskOperator {
	return &MaskOperator{maskPattern: maskPattern}
}

// TailMaskOperator masks a value like MaskOperator, except that a value longer
// than the pattern is aligned to the pattern's END: the value's last characters
// fill the slots, and its leading characters are masked. So a '#' keeps the
// value's last digits whatever its length. A value that fits the pattern is
// masked exactly as MaskOperator masks it.
//
// MaskOperator fills slots from the front, so a 13-digit phone number showed
// digits from its middle. This operator is the default for phone and card.
type TailMaskOperator struct {
	maskPattern string
}

// NewTailMaskOperator creates a tail-aligned mask operator. The pattern uses the
// same slots as NewMaskOperator: '#' keeps a character and '*' masks it.
func NewTailMaskOperator(maskPattern string) *TailMaskOperator {
	return &TailMaskOperator{maskPattern: maskPattern}
}

// Name returns the operator name.
func (o *TailMaskOperator) Name() string { return "tail_mask" }

// Replace masks a single value according to the mask pattern, aligned to its end.
func (o *TailMaskOperator) Replace(ctx context.Context, value string) (string, error) {
	return o.applyMask(value), nil
}

// applyMask aligns the value's significant characters to the pattern's last slot
// when there are more of them than slots, and otherwise defers to MaskOperator.
func (o *TailMaskOperator) applyMask(s string) string {
	slots := 0
	for _, r := range o.maskPattern {
		if isPatternSlot(r) {
			slots++
		}
	}
	sig := significantChars(s)
	if slots == 0 || len(sig) <= slots {
		return NewMaskOperator(o.maskPattern).applyMask(s)
	}
	next := len(sig) - slots
	var result strings.Builder
	for _, r := range o.maskPattern {
		if !isPatternSlot(r) {
			result.WriteRune(r)
			continue
		}
		if r == '#' {
			result.WriteString(sig[next])
		} else {
			result.WriteByte('*')
		}
		next++
	}
	return result.String()
}

// significantChars returns the raw text of each significant character of s: every
// character that is not a separator, with invalid bytes kept as one unit each.
// It follows the same rule as MaskOperator.applyMask.
func significantChars(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if (r != utf8.RuneError || size > 1) && isMaskSeparator(r) {
			i += size
			continue
		}
		out = append(out, s[i:i+size])
		i += size
	}
	return out
}

// Name returns the operator name.
func (o *MaskOperator) Name() string { return "mask" }

// Replace masks a single value according to the mask pattern.
func (o *MaskOperator) Replace(ctx context.Context, value string) (string, error) {
	return o.applyMask(value), nil
}

// isMaskSeparator reports whether r is punctuation that a mask pattern treats
// as structure rather than data. Separators in the INPUT are dropped, and
// separators in the PATTERN are emitted in their place.
func isMaskSeparator(r rune) bool {
	return r == ' ' || r == '-' || r == '.'
}

// applyMask masks s against the configured pattern.
//
// The pattern is applied to the input's SIGNIFICANT RUNES (every Unicode
// character that is not a space, '-' or '.'), so the pattern describes the
// masked value's SHAPE, not its byte offsets:
//
//	'#' keeps the corresponding input rune
//	'*' masks the corresponding input rune
//	any other rune is a separator, emitted from the pattern and consuming no
//	  input rune
//
// Rune semantics matter: one slot is one CHARACTER, so a multi-byte rune is
// never split. "é" with "#*" yields "é*", not the invalid "\xC3*" that a
// byte-wise mask produced.
//
// That also makes "****-****-****-####" keep the last four digits of a card
// whether it was written "4111111111111111" or "4111 1111 1111 1111" — both
// mask to "****-****-****-1111".
//
// If the input has more significant runes than the pattern has slots the extras
// are masked; if it has fewer, the remaining slots are masked too.
//
// Input that is not valid UTF-8 is handled byte-faithfully: an invalid byte is
// one significant unit and, when kept by '#', is written back verbatim rather
// than replaced with U+FFFD. A PII scrubber must not rewrite bytes it was told
// to keep.
func (o *MaskOperator) applyMask(s string) string {
	if o.maskPattern == "" {
		// Mask everything, one '*' per rune (separators included).
		return strings.Repeat("*", utf8.RuneCountInString(s))
	}

	pattern := []rune(o.maskPattern)

	var result strings.Builder
	maskIdx := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		raw := s[i : i+size]
		// A valid rune that is a separator is dropped. An invalid byte (RuneError
		// with size 1) is NOT a separator — it is significant data.
		if (r != utf8.RuneError || size > 1) && isMaskSeparator(r) {
			i += size
			continue
		}
		// Emit the pattern's separators up to the next data slot.
		for maskIdx < len(pattern) && !isPatternSlot(pattern[maskIdx]) {
			result.WriteRune(pattern[maskIdx])
			maskIdx++
		}
		if maskIdx < len(pattern) && pattern[maskIdx] == '#' {
			result.WriteString(raw)
		} else {
			result.WriteByte('*')
		}
		maskIdx++
		i += size
	}

	// Fill any slots the input did not reach, then any trailing separators.
	for ; maskIdx < len(pattern); maskIdx++ {
		if !isPatternSlot(pattern[maskIdx]) {
			result.WriteRune(pattern[maskIdx])
		} else {
			result.WriteByte('*')
		}
	}
	return result.String()
}

// isPatternSlot reports whether r consumes an input character.
func isPatternSlot(r rune) bool {
	return r == '*' || r == '#'
}

// HashOperator replaces entities with their SHA-256 hash.
type HashOperator struct {
	truncate int // number of hash characters to keep (0 = full)
}

// NewHashOperator creates a new hash operator.
func NewHashOperator(truncate int) *HashOperator {
	return &HashOperator{truncate: truncate}
}

// Name returns the operator name.
func (o *HashOperator) Name() string { return "hash" }

// Replace returns the SHA-256 hex digest of the value, truncated to the
// configured length. The same value always produces the same hash, which keeps
// referential integrity across a dataset.
func (o *HashOperator) Replace(ctx context.Context, value string) (string, error) {
	sum := sha256.Sum256([]byte(value))
	digest := hex.EncodeToString(sum[:])
	if o.truncate > 0 && o.truncate < len(digest) {
		digest = digest[:o.truncate]
	}
	return digest, nil
}

// RedactOperator removes entities entirely.
type RedactOperator struct {
	replacement string
}

// NewRedactOperator creates a new redact operator.
func NewRedactOperator() *RedactOperator {
	return &RedactOperator{replacement: "[REDACTED]"}
}

// Name returns the operator name.
func (o *RedactOperator) Name() string { return "redact" }

// Replace returns the redaction marker for any value.
func (o *RedactOperator) Replace(ctx context.Context, value string) (string, error) {
	return o.replacement, nil
}

// Compile-time checks
var _ Operator = (*ReplaceOperator)(nil)
var _ Operator = (*MaskOperator)(nil)
var _ Operator = (*HashOperator)(nil)
var _ Operator = (*RedactOperator)(nil)
