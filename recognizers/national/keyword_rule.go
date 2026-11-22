package national

import (
	"context"
	"regexp"

	"github.com/u007/dataprotect/internal/keyword"
	"github.com/u007/dataprotect/recognizers"
)

// keywordScore is the confidence for every keyword-gated national rule.
const keywordScore = keyword.Score

// keywordRule reports, for each match of pattern, the value capture group as a
// span over the value alone, the same way recognizers/identity does.
type keywordRule struct {
	name    string
	typ     recognizers.EntityType
	pattern *regexp.Regexp
}

// Name returns the recognizer name.
func (r *keywordRule) Name() string { return r.name }

// Recognize reports every value that follows the rule's label, and only the value.
func (r *keywordRule) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringSubmatchIndex(text, -1) {
		entities = append(entities, recognizers.Entity{
			Type:       r.typ,
			Start:      m[2],
			End:        m[3],
			Text:       text[m[2]:m[3]],
			Confidence: keywordScore,
		})
	}
	return entities, nil
}
