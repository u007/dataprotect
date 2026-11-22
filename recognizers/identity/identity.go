// Package identity detects personal and tax identifiers that have no checksum
// or no universal shape the detector can use on its own. Each rule is
// keyword-gated, like the rules in recognizers/finance: the value is reported
// only when a label for it (for example "tax id", "passport number", "driver's
// license" or "date of birth") sits directly before it, and the reported span is
// the value alone, never the label. Every rule in this package scores 0.85.
//
// Overlap with the other recognizers is decided in analyzer.winsOverlap, not
// here: these types yield to SSN, PH_TIN, IN_GSTIN and the MY and SG passports,
// they beat PHONE, and they lose to CREDIT_CARD.
package identity

import (
	"context"
	"regexp"

	"github.com/u007/dataprotect/internal/keyword"
	"github.com/u007/dataprotect/recognizers"
)

// keywordScore is the confidence for every keyword-gated identity rule.
const keywordScore = keyword.Score

// labelRecognizer reports, for each match of pattern, the value capture group
// as a span over the value alone.
type labelRecognizer struct {
	name    string
	typ     recognizers.EntityType
	pattern *regexp.Regexp
}

// Name returns the recognizer name.
func (r *labelRecognizer) Name() string { return r.name }

// Recognize reports every value that follows the rule's label. Only the value
// is reported.
func (r *labelRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
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
