// Package anonymizer provides the PII anonymization engine.
package anonymizer

import (
	"context"
	"fmt"
	"sort"

	"dataprotect/operators"
	"dataprotect/recognizers"
)

// Anonymizer orchestrates operators to transform detected PII entities.
type Anonymizer struct {
	operators map[recognizers.EntityType]operators.Operator
	defaultOp operators.Operator
}

// Option configures an Anonymizer.
type Option func(*Anonymizer)

// WithDefaultOperator sets the default operator for unconfigured entity types.
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
func (a *Anonymizer) Anonymize(ctx context.Context, text string, entities []recognizers.Entity) (string, error) {
	if len(entities) == 0 {
		return text, nil
	}

	// Replace right-to-left so each span stays valid when reached.
	ordered := make([]recognizers.Entity, len(entities))
	copy(ordered, entities)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Start != ordered[j].Start {
			return ordered[i].Start > ordered[j].Start
		}
		return ordered[i].End > ordered[j].End
	})

	result := text
	for _, e := range ordered {
		op := a.operatorFor(e.Type)
		if op == nil {
			continue
		}
		replacement, err := op.Replace(ctx, e.Text)
		if err != nil {
			return "", fmt.Errorf("operator %q on %s: %w", op.Name(), e.Type, err)
		}
		if e.Start < 0 || e.End > len(result) || e.Start > e.End {
			// A span outside the working string cannot be applied safely.
			return "", fmt.Errorf("operator %q: entity %s span [%d:%d] out of bounds for length %d",
				op.Name(), e.Type, e.Start, e.End, len(result))
		}
		result = result[:e.Start] + replacement + result[e.End:]
	}

	return result, nil
}

// operatorFor returns the operator configured for an entity type, or the
// default operator when the type has none. It returns nil when the type is
// unconfigured and there is no default, which leaves the entity untouched.
func (a *Anonymizer) operatorFor(t recognizers.EntityType) operators.Operator {
	if op, ok := a.operators[t]; ok {
		return op
	}
	return a.defaultOp
}
