// Package anonymizer provides the PII anonymization engine.
package anonymizer

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"unicode/utf8"

	"github.com/u007/dataprotect/operators"
	"github.com/u007/dataprotect/recognizers"
)

// Anonymizer orchestrates operators to transform detected PII entities.
type Anonymizer struct {
	operators map[recognizers.EntityType]operators.Operator
	defaultOp operators.Operator
}

// Option configures an Anonymizer.
type Option func(*Anonymizer)

// WithDefaultOperator sets the default operator for unconfigured entity types.
// Without one, Anonymize returns an error for an entity whose type has no
// operator of its own.
func WithDefaultOperator(op operators.Operator) Option {
	return func(a *Anonymizer) {
		a.defaultOp = op
	}
}

// New creates a new Anonymizer.
func New(opts ...Option) *Anonymizer {
	a := &Anonymizer{
		operators: make(map[recognizers.EntityType]operators.Operator),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// SetOperator sets the operator for a specific entity type.
func (a *Anonymizer) SetOperator(entityType recognizers.EntityType, op operators.Operator) {
	a.operators[entityType] = op
}

// Anonymize transforms detected entities in text.
//
// Every operator rewrites the string, so byte offsets from the original text go
// stale as soon as the first one runs. Each entity is therefore replaced
// individually, right-to-left, so an entity's own span is still valid when it is
// reached: later entities sit at higher offsets, which earlier (rightward)
// replacements cannot disturb. Doing this per entity — rather than grouping by
// type and handing each operator a batch — also makes the result independent of
// map iteration order.
//
// Entities are checked by validate before anything is spliced. On any error the
// returned string is empty: a partially spliced document would be worse than
// none, so a caller can never mistake a failure for a usable result.
//
// An entity whose type has no operator and no default is an error, so it is
// never returned unredacted. This is checked before any splice.
func (a *Anonymizer) Anonymize(ctx context.Context, text string, entities []recognizers.Entity) (string, error) {
	if len(entities) == 0 {
		return text, nil
	}
	ordered, err := validate(text, entities)
	if err != nil {
		return "", err
	}

	// Check every type before splicing anything, so an unconfigured type fails
	// the call rather than leaving its span in the output.
	for _, e := range ordered {
		if a.operatorFor(e.Type) == nil {
			return "", fmt.Errorf("no operator or default for entity type %s [%d:%d)",
				e.Type, e.Start, e.End)
		}
	}

	result := text
	for _, e := range ordered {
		op := a.operatorFor(e.Type)
		replacement, err := op.Replace(ctx, e.Text)
		if err != nil {
			return "", fmt.Errorf("operator %q on %s: %w", op.Name(), e.Type, err)
		}
		result = result[:e.Start] + replacement + result[e.End:]
	}

	return result, nil
}

// Pseudonymize replaces every entity with a numbered placeholder and returns the
// placeholder-to-original map.
//
// Placeholders are named for the entity type: [EMAIL_1], [CREDIT_CARD_2]. The
// numbers count per type from 1, in the order the entities appear in text. The
// same type and the same exact value share one placeholder, so a repeated value
// reads as one thing. Different values, and the same value under two types, get
// different placeholders.
//
// Numbering is local to the call. It is never stored on the Anonymizer, so
// concurrent calls cannot share it.
//
// Pseudonymize does not use the configured operators. Every entity is replaced,
// whatever its type. The returned map holds the original values in plain text,
// so the caller must protect it as it would the input.
//
// On any error the returned string is empty and the map is nil. An error never
// includes an original value. A text that already holds a placeholder-shaped
// token for a known type is an error, because the map could not be reversed
// reliably: the token would be indistinguishable from a placeholder.
func (a *Anonymizer) Pseudonymize(text string, entities []recognizers.Entity) (string, map[string]string, error) {
	pseudonyms := make(map[string]string)
	if len(entities) == 0 {
		return text, pseudonyms, nil
	}
	ordered, err := validate(text, entities)
	if err != nil {
		return "", nil, err
	}
	if token, ok := placeholderCollision(text); ok {
		return "", nil, fmt.Errorf("text already contains placeholder-shaped token %s, so the map would be ambiguous", token)
	}

	// Number left to right. ordered is right-to-left, so walk it backwards.
	type pseudonymKey struct {
		entityType recognizers.EntityType
		value      string
	}
	placeholders := make([]string, len(ordered))
	keyed := make(map[pseudonymKey]string)
	counters := make(map[recognizers.EntityType]int)
	for i := len(ordered) - 1; i >= 0; i-- {
		e := ordered[i]
		value := text[e.Start:e.End]
		key := pseudonymKey{e.Type, value}
		placeholder, seen := keyed[key]
		if !seen {
			counters[e.Type]++
			placeholder = fmt.Sprintf("[%s_%d]", e.Type, counters[e.Type])
			keyed[key] = placeholder
			pseudonyms[placeholder] = value
		}
		placeholders[i] = placeholder
	}

	// Splice right-to-left, so each span is still valid when it is reached.
	result := text
	for i, e := range ordered {
		result = result[:e.Start] + placeholders[i] + result[e.End:]
	}
	return result, pseudonyms, nil
}

// placeholderShape matches a placeholder-shaped token: [TYPE_n].
var placeholderShape = regexp.MustCompile(`\[([A-Z_]+)_[0-9]+\]`)

// placeholderCollision reports the first placeholder-shaped token in text whose
// type is a known entity type.
func placeholderCollision(text string) (string, bool) {
	known := make(map[string]bool, len(recognizers.AllEntityTypes))
	for _, t := range recognizers.AllEntityTypes {
		known[string(t)] = true
	}
	for _, m := range placeholderShape.FindAllStringSubmatch(text, -1) {
		if known[m[1]] {
			return m[0], true
		}
	}
	return "", false
}

// validate checks entities against text and returns a right-to-left copy. It
// rejects an out-of-bounds span, a span whose start or end falls inside a rune,
// and overlapping spans. Nothing is spliced until it has passed.
func validate(text string, entities []recognizers.Entity) ([]recognizers.Entity, error) {
	ordered := make([]recognizers.Entity, len(entities))
	copy(ordered, entities)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Start != ordered[j].Start {
			return ordered[i].Start > ordered[j].Start
		}
		return ordered[i].End > ordered[j].End
	})

	// Check every span against the original text before splicing anything. A
	// span that starts or ends inside a multi-byte rune would splice invalid
	// UTF-8 whatever the operator does, so it is rejected here.
	for _, e := range ordered {
		if e.Start < 0 || e.End > len(text) || e.Start > e.End {
			return nil, fmt.Errorf("entity %s span [%d:%d] out of bounds for length %d",
				e.Type, e.Start, e.End, len(text))
		}
		if !isRuneBoundary(text, e.Start) || !isRuneBoundary(text, e.End) {
			return nil, fmt.Errorf("entity %s span [%d:%d] is not on a UTF-8 rune boundary",
				e.Type, e.Start, e.End)
		}
	}

	// Reject overlaps before splicing anything. In this descending order the
	// next entity starts at or before the previous one, so it overlaps iff it
	// ends after the previous start. Checking first (rather than mid-loop) means
	// a rejected input never yields a partially spliced result.
	for i := 1; i < len(ordered); i++ {
		if ordered[i].End > ordered[i-1].Start {
			return nil, fmt.Errorf("overlapping entities: [%d:%d) and [%d:%d)",
				ordered[i].Start, ordered[i].End, ordered[i-1].Start, ordered[i-1].End)
		}
	}
	return ordered, nil
}

// isRuneBoundary reports whether byte offset i of s falls between two runes.
// Offsets 0 and len(s) are always boundaries. The caller guarantees 0 <= i <= len(s).
func isRuneBoundary(s string, i int) bool {
	return i == 0 || i == len(s) || utf8.RuneStart(s[i])
}

// operatorFor returns the operator configured for an entity type, or the
// default operator when the type has none. It returns nil when the type is
// unconfigured and there is no default; Anonymize turns that into an error.
func (a *Anonymizer) operatorFor(t recognizers.EntityType) operators.Operator {
	if op, ok := a.operators[t]; ok {
		return op
	}
	return a.defaultOp
}
