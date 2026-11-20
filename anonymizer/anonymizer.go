// Package anonymizer provides the PII anonymization engine.
package anonymizer

import (
	"context"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"
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

	// A text that already holds a redaction marker could not be numbered
	// unambiguously on restore, so reject it up front, like Redact does.
	if token, ok := redactMarkerCollision(text); ok {
		return "", fmt.Errorf("text already contains redaction marker %s, so numbering would be ambiguous", token)
	}

	// Compute every replacement before splicing anything. RedactOperator
	// renders as a typed numbered marker ([[redacted<type>n]]) so the output
	// says what type was removed; every other operator is called normally.
	replacements := make([]string, len(ordered))
	var redact *redactor
	// Walk left to right so markers are numbered in the order entities appear
	// in text (ordered is right-to-left).
	for i := len(ordered) - 1; i >= 0; i-- {
		e := ordered[i]
		op := a.operatorFor(e.Type)
		if isRedactOperator(op) {
			if redact == nil {
				redact = newRedactor()
			}
			replacements[i] = redact.markerFor(e.Type, e.Text)
			continue
		}
		replacement, err := op.Replace(ctx, e.Text)
		if err != nil {
			return "", fmt.Errorf("operator %q on %s: %w", op.Name(), e.Type, err)
		}
		replacements[i] = replacement
	}

	return spliceRightToLeft(text, ordered, replacements), nil
}

// isRedactOperator reports whether op is a redact operator, so every entity
// type is redacted with a typed numbered marker regardless of configuration.
func isRedactOperator(op operators.Operator) bool {
	_, ok := op.(*operators.RedactOperator)
	return ok
}

// spliceRightToLeft replaces each entity span with its replacement, walking
// right to left so each span stays valid as the string grows or shrinks. All
// replacements are computed before any splice, so a failure returns an empty
// string rather than a partially rewritten document.
func spliceRightToLeft(text string, ordered []recognizers.Entity, replacements []string) string {
	result := text
	for i, e := range ordered {
		result = result[:e.Start] + replacements[i] + result[e.End:]
	}
	return result
}

// Pseudonymize replaces every entity with a numbered placeholder and returns the
// placeholder-to-original map.
//
// Placeholders are named for the entity type: [EMAIL_1], [CREDIT_CARD_2]. The
// numbers count per type from 1, in the order the entities appear in text. The
// same type and the same canonical value share one placeholder, so a repeated
// value reads as one thing. The canonical form (canonicalValue) ignores spacing
// in a card, phone or SSN, case in an email or IBAN, and IP notation. Different
// values, and the same value under two types, get different placeholders. The
// restored text uses the first spelling seen for each placeholder.
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
	p := newPseudonymizer()
	result, err := a.pseudonymizeText(p, text, entities)
	if err != nil {
		return "", nil, err
	}
	return result, p.pseudonyms, nil
}

// PseudonymizeAll pseudonymizes several texts, such as the cells of a table, with
// one mapping. A value that recurs in any of them gets the same placeholder, and
// numbering runs through the texts in order. texts and entities pair up by index.
// The error rules are those of Pseudonymize, and apply to every text.
func (a *Anonymizer) PseudonymizeAll(texts []string, entities [][]recognizers.Entity) ([]string, map[string]string, error) {
	if len(texts) != len(entities) {
		return nil, nil, fmt.Errorf("pseudonymize: %d texts but %d entity lists", len(texts), len(entities))
	}
	p := newPseudonymizer()
	results := make([]string, len(texts))
	for i, text := range texts {
		result, err := a.pseudonymizeText(p, text, entities[i])
		if err != nil {
			return nil, nil, err
		}
		results[i] = result
	}
	return results, p.pseudonyms, nil
}

// pseudonymizer numbers placeholders across one or more texts. It is local to one
// call, so concurrent calls never share it.
type pseudonymizer struct {
	keyed      map[pseudonymKey]string
	counters   map[recognizers.EntityType]int
	pseudonyms map[string]string
}

// pseudonymKey is an entity type and the canonical form of a value. Two spellings
// of one value (a card with and without spaces) have one key.
type pseudonymKey struct {
	entityType recognizers.EntityType
	value      string
}

func newPseudonymizer() *pseudonymizer {
	return &pseudonymizer{
		keyed:      make(map[pseudonymKey]string),
		counters:   make(map[recognizers.EntityType]int),
		pseudonyms: make(map[string]string),
	}
}

// placeholderFor returns the placeholder for a value of the given type, numbering
// a new one. The map keeps the first spelling seen, so restoring a placeholder
// gives the first form of the value, not each occurrence's own spelling.
func (p *pseudonymizer) placeholderFor(entityType recognizers.EntityType, value string) string {
	key := pseudonymKey{entityType, canonicalValue(entityType, value)}
	if placeholder, seen := p.keyed[key]; seen {
		return placeholder
	}
	p.counters[entityType]++
	placeholder := fmt.Sprintf("[%s_%d]", entityType, p.counters[entityType])
	p.keyed[key] = placeholder
	p.pseudonyms[placeholder] = value
	return placeholder
}

// pseudonymizeText replaces the entities of one text with placeholders from p.
func (a *Anonymizer) pseudonymizeText(p *pseudonymizer, text string, entities []recognizers.Entity) (string, error) {
	// The collision check runs even when there are no entities: a shared mapping
	// (PseudonymizeAll) would rewrite a literal token in this text on restore.
	if token, ok := placeholderCollision(text, entities); ok {
		return "", fmt.Errorf("text already contains placeholder-shaped token %s, so the map would be ambiguous", token)
	}
	if len(entities) == 0 {
		return text, nil
	}
	ordered, err := validate(text, entities)
	if err != nil {
		return "", err
	}

	// Number left to right. ordered is right-to-left, so walk it backwards.
	placeholders := make([]string, len(ordered))
	for i := len(ordered) - 1; i >= 0; i-- {
		e := ordered[i]
		placeholders[i] = p.placeholderFor(e.Type, text[e.Start:e.End])
	}

	// Splice right-to-left, so each span is still valid when it is reached.
	return spliceRightToLeft(text, ordered, placeholders), nil
}

// canonicalValue is the form two spellings of one value share. Digits identify a
// card, phone or SSN however they are grouped. An IBAN ignores case and separators.
// An email is case-insensitive in practice. An IP is its netip form. Other values,
// including URLs, are compared exactly.
func canonicalValue(entityType recognizers.EntityType, value string) string {
	switch entityType {
	case recognizers.EntityCreditCard, recognizers.EntityPhone, recognizers.EntitySSN:
		var digits strings.Builder
		for i := 0; i < len(value); i++ {
			if value[i] >= '0' && value[i] <= '9' {
				digits.WriteByte(value[i])
			}
		}
		return digits.String()
	case recognizers.EntityIBAN:
		return strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(value))
	case recognizers.EntityEmail:
		return strings.ToLower(value)
	case recognizers.EntityIP:
		// A value that is not an address keeps its exact form.
		if addr, err := netip.ParseAddr(value); err == nil {
			return addr.String()
		}
		return value
	}
	return value
}

// placeholderShape matches a placeholder-shaped token: [TYPE_n].
var placeholderShape = regexp.MustCompile(`\[([A-Z_]+)_[0-9]+\]`)

// placeholderCollision reports the first placeholder-shaped token in text whose
// type is one Pseudonymize could emit: a known entity type, or a type carried by
// one of the entities. Any other bracketed token is ordinary text.
func placeholderCollision(text string, entities []recognizers.Entity) (string, bool) {
	emittable := make(map[string]bool, len(recognizers.AllEntityTypes)+len(entities))
	for _, t := range recognizers.AllEntityTypes {
		emittable[string(t)] = true
	}
	for _, e := range entities {
		emittable[string(e.Type)] = true
	}
	for _, m := range placeholderShape.FindAllStringSubmatch(text, -1) {
		if emittable[m[1]] {
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

// Redact replaces every entity with a numbered redaction marker such as
// [[redactedperson1]] or [[redactedsocialnumber2]]. Numbers count per type
// from 1, in the order the entities appear in text. The same type and the same
// canonical value share one marker, so a repeated value reads as one thing.
//
// The marker keeps the entity type and a sequence number, so the output says
// what was removed without revealing the value. Unlike Anonymize, Redact does
// not use the configured operators: every entity type is redacted, even ones
// with no operator of their own, so nothing is ever left unredacted.
//
// Numbering is local to the call. It is never stored on the Anonymizer, so
// concurrent calls cannot share it.
//
// On any error the returned string is empty.
func (a *Anonymizer) Redact(ctx context.Context, text string, entities []recognizers.Entity) (string, error) {
	return a.redactText(newRedactor(), text, entities)
}

// redactKey is an entity type and the canonical form of a value. Two spellings
// of one value (a card with and without spaces) have one key and share one
// redaction marker.
type redactKey struct {
	entityType recognizers.EntityType
	value      string
}

type redactor struct {
	keyed    map[redactKey]string
	counters map[recognizers.EntityType]int
}

func newRedactor() *redactor {
	return &redactor{
		keyed:    make(map[redactKey]string),
		counters: make(map[recognizers.EntityType]int),
	}
}

// markerFor returns the redaction marker for a value of the given type,
// numbering a new one. The same type and canonical value reuse the first
// marker issued.
func (r *redactor) markerFor(entityType recognizers.EntityType, value string) string {
	key := redactKey{entityType, canonicalValue(entityType, value)}
	if marker, seen := r.keyed[key]; seen {
		return marker
	}
	r.counters[entityType]++
	marker := fmt.Sprintf("[[redacted%s%d]]", redactTypeName(entityType), r.counters[entityType])
	r.keyed[key] = marker
	return marker
}

// redactTypeName is the lowercase, underscore-free name used inside a redaction
// marker. SSN is rendered as "socialnumber" so the marker reads naturally.
func redactTypeName(t recognizers.EntityType) string {
	switch t {
	case recognizers.EntitySSN:
		return "socialnumber"
	default:
		return strings.ToLower(strings.ReplaceAll(string(t), "_", ""))
	}
}

// redactMarkerShape matches a redaction marker: [[redacted<name>n]].
var redactMarkerShape = regexp.MustCompile(`\[\[redacted[a-z]+[0-9]+\]\]`)

// redactMarkerCollision reports the first redaction-marker-shaped token in text.
// A text that already holds one could not be numbered unambiguously.
func redactMarkerCollision(text string) (string, bool) {
	m := redactMarkerShape.FindStringIndex(text)
	if m == nil {
		return "", false
	}
	return text[m[0]:m[1]], true
}

// redactText replaces the entities of one text with redaction markers from r.
func (a *Anonymizer) redactText(r *redactor, text string, entities []recognizers.Entity) (string, error) {
	if token, ok := redactMarkerCollision(text); ok {
		return "", fmt.Errorf("text already contains redaction marker %s, so numbering would be ambiguous", token)
	}
	if len(entities) == 0 {
		return text, nil
	}
	ordered, err := validate(text, entities)
	if err != nil {
		return "", err
	}

	// Number left to right. ordered is right-to-left, so walk it backwards.
	markers := make([]string, len(ordered))
	for i := len(ordered) - 1; i >= 0; i-- {
		e := ordered[i]
		markers[i] = r.markerFor(e.Type, text[e.Start:e.End])
	}

	// Splice right-to-left, so each span is still valid when it is reached.
	return spliceRightToLeft(text, ordered, markers), nil
}
