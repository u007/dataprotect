// Package operators provides anonymization operators for PII entities.
package operators

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
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
func NewMaskOperator(maskPattern string) *MaskOperator {
	return &MaskOperator{maskPattern: maskPattern}
}

// Name returns the operator name.
func (o *MaskOperator) Name() string { return "mask" }

// Replace masks a single value according to the mask pattern.
func (o *MaskOperator) Replace(ctx context.Context, value string) (string, error) {
	return o.applyMask(value), nil
}

// isMaskSeparator reports whether c is punctuation that a mask pattern treats
// as structure rather than data. Separators in the INPUT are dropped, and
// separators in the PATTERN are emitted in their place.
func isMaskSeparator(c byte) bool {
	return c == ' ' || c == '-' || c == '.'
}

// applyMask masks s against the configured pattern.
//
// The pattern is applied to the input's SIGNIFICANT characters (everything that
// is not a space, '-' or '.'), so the pattern describes the masked value's shape
// rather than its byte offsets:
//
//	'#' keeps the corresponding input character
//	'*' masks the corresponding input character
//	any other character is a separator, emitted from the pattern and consuming
//	  no input character
//
// That makes "****-****-****-####" keep the last four digits of a card whether
// it was written "4111111111111111" or "4111 1111 1111 1111" — both mask to
// "****-****-****-1111".
//
// If the input has more significant characters than the pattern has slots the
// extras are masked; if it has fewer, the remaining slots are masked too.
func (o *MaskOperator) applyMask(s string) string {
	if o.maskPattern == "" {
		return strings.Repeat("*", len(s))
	}

	var result strings.Builder
	maskIdx := 0
	for i := 0; i < len(s); i++ {
		if isMaskSeparator(s[i]) {
			continue
		}
		// Emit the pattern's separators up to the next data slot.
		for maskIdx < len(o.maskPattern) && !isPatternSlot(o.maskPattern[maskIdx]) {
			result.WriteByte(o.maskPattern[maskIdx])
			maskIdx++
		}
		if maskIdx < len(o.maskPattern) && o.maskPattern[maskIdx] == '#' {
			result.WriteByte(s[i])
		} else {
			result.WriteByte('*')
		}
		maskIdx++
	}

	// Fill any slots the input did not reach, then any trailing separators.
	for ; maskIdx < len(o.maskPattern); maskIdx++ {
		if !isPatternSlot(o.maskPattern[maskIdx]) {
			result.WriteByte(o.maskPattern[maskIdx])
		} else {
			result.WriteByte('*')
		}
	}
	return result.String()
}

// isPatternSlot reports whether c consumes an input character.
func isPatternSlot(c byte) bool {
	return c == '*' || c == '#'
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
