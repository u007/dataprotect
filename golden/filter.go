package golden

import (
	"fmt"
	"sort"
	"strings"

	"github.com/u007/dataprotect/recognizers"
)

// Option configures Score. Options are additive and leave the default behaviour
// untouched when none are supplied.
type Option func(*scoreConfig)

// scoreConfig holds the resolved options for one Score call.
//
// inSchema is nil when no schema was requested, which is the seed-set default:
// every detection counts, exactly as this package has always scored.
type scoreConfig struct {
	inSchema map[recognizers.EntityType]bool
}

// hasSchema reports whether a schema filter was requested.
func (c *scoreConfig) hasSchema() bool { return c.inSchema != nil }

// WithInSchema restricts scoring to the entity types a dataset labels.
//
// This exists because a corpus labels only what it annotates. scorePair credits
// a prediction solely against a gold label of the same type, so a detection of a
// type the corpus never records would be counted as a false positive even though
// the corpus has no opinion on it — inflating FP and depressing precision for no
// defect of the detector.
//
// Predictions of excluded types leave the TP/FP/FN counts entirely and are
// tallied in Report.OutOfSchema instead, so the exclusion is visible rather than
// silent. Gold must be entirely inside the schema; Score returns an error
// otherwise, because gold outside it can never match and would otherwise be
// counted as false negatives for entities the detector was never allowed to
// report.
//
// A nil or empty set means "schema requested but empty", which excludes every
// prediction; omit the option entirely for unfiltered scoring.
func WithInSchema(types []recognizers.EntityType) Option {
	return func(c *scoreConfig) {
		if c.inSchema == nil {
			c.inSchema = map[recognizers.EntityType]bool{}
		}
		for _, t := range types {
			c.inSchema[t] = true
		}
	}
}

// applyOptions resolves options into a config. A Score with no options yields
// a config whose hasSchema is false, so the default path is untouched.
func applyOptions(opts []Option) *scoreConfig {
	c := &scoreConfig{}
	for _, o := range opts {
		o(c)
	}
	return c
}

// checkGoldInsideSchema rejects gold labels whose type the schema excludes.
//
// The failure it prevents is quiet: predictions of that type are filtered out
// before matching, so the gold could never be credited and would surface as a
// false negative. That is a wrong number with no visible cause, so it is an
// error naming the offending type instead.
func (c *scoreConfig) checkGoldInsideSchema(gold []recognizers.Entity, dir string) error {
	if !c.hasSchema() {
		return nil
	}
	for _, g := range gold {
		if !c.inSchema[g.Type] {
			return fmt.Errorf(
				"%s: gold label of type %s is outside in_schema (%s); such a label can "+
					"never match because its predictions are filtered out, so it would be "+
					"counted as a false negative — fix the dataset or its schema",
				dir, g.Type, c.schemaList())
		}
	}
	return nil
}

// splitPredictions partitions predictions into those the schema scores and
// those it does not, counting the excluded ones by type so the omission is
// reported rather than hidden.
func (c *scoreConfig) splitPredictions(pred []Prediction, outOfSchema map[string]int) []Prediction {
	if !c.hasSchema() {
		return pred
	}
	kept := make([]Prediction, 0, len(pred))
	for _, p := range pred {
		if c.inSchema[p.Type] {
			kept = append(kept, p)
			continue
		}
		outOfSchema[string(p.Type)]++
	}
	return kept
}

// schemaList renders the schema for an error message, sorted for a stable
// message across runs.
func (c *scoreConfig) schemaList() string {
	if !c.hasSchema() {
		return "(none)"
	}
	names := make([]string, 0, len(c.inSchema))
	for t := range c.inSchema {
		names = append(names, string(t))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
