// Package golden scores a detector against hand-labelled cases.
//
// The purpose is to make the accuracy claims in SPEC.md checkable instead of
// aspirational. Three design choices matter more than the metric arithmetic:
//
//  1. Cases label entities by SUBSTRING, never by byte offset. A label then
//     records something a person can read in the text and verify by eye;
//     locating it is the runner's job.
//
//  2. A label must be unambiguous. A value occurring more than once in the text
//     is rejected, and a value not occurring at all is rejected. Either is a
//     labelling bug, and reporting it as a detector failure would be a lie that
//     hides the real problem.
//
//  3. Cases with an empty entity list are NEGATIVE cases: text containing no
//     PII. They are what make the precision number mean anything. A suite of
//     positive cases alone measures recall and reports precision as a
//     tautology.
//
// Matching granularity is deliberate and reported twice. Exact-span matching
// requires identical start/end and is the strict measure. Overlap matching
// accepts any shared byte for the same type and is lenient about boundaries.
// Comparing the two localises a recall loss: a small gap means the detector
// found the entity but mis-sized it, a large gap means it missed it entirely.
package golden

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dataprotect/recognizers"
)

// Case is one hand-labelled input/output pair.
//
// Entities must be listed in the order they appear in the text. The runner
// matches each label to the next occurrence at or after the previous match, so a
// case that lists its labels out of order is rejected rather than scored oddly.
//
// A case with an empty Entities list IS a negative case; there is deliberately
// no "negative" flag. An explicit flag could disagree with the entity list —
// a case labelled negative while carrying entities would silently be scored as
// a positive case, and the flag would win or lose by accident. Deriving
// negatives from the labels means a dataset cannot express that contradiction,
// because it cannot express it.
type Case struct {
	ID       string  `json:"id"`
	Note     string  `json:"note,omitempty"`
	Text     string  `json:"text"`
	Entities []Label `json:"entities"`
}

// Label is one expected entity, identified by the exact text it covers.
type Label struct {
	Type  recognizers.EntityType `json:"type"`
	Value string                 `json:"value"`
}

// Prediction is what a detector produced. It reuses recognizers.Entity rather
// than declaring a parallel span type, so a detector cannot drift out of sync
// with the scorer's idea of a span.
type Prediction = recognizers.Entity

// resolve converts labels into concrete spans against the case text.
func (c Case) resolve() ([]recognizers.Entity, error) {
	cursor := 0
	out := make([]recognizers.Entity, 0, len(c.Entities))
	for i, l := range c.Entities {
		if l.Value == "" {
			return nil, fmt.Errorf("case %q label %d: empty value", c.ID, i)
		}
		// Reject ambiguity rather than silently taking the first match. Counting
		// the whole text is what makes a repeated value an error: taking the
		// first of several occurrences is exactly the kind of silent wrong
		// answer this package exists to prevent.
		if n := strings.Count(c.Text, l.Value); n != 1 {
			return nil, fmt.Errorf(
				"case %q label %d: value %q occurs %d times in the text, want exactly 1; "+
					"a repeated value cannot be located unambiguously, so reword the case or "+
					"make the labelled spans distinct",
				c.ID, i, l.Value, n)
		}
		rel := strings.Index(c.Text[cursor:], l.Value)
		if rel < 0 {
			return nil, fmt.Errorf(
				"case %q label %d: value %q occurs in the text but before byte %d, so the "+
					"labels are not listed in text order",
				c.ID, i, l.Value, cursor)
		}
		start := cursor + rel
		out = append(out, recognizers.Entity{
			Type:       l.Type,
			Start:      start,
			End:        start + len(l.Value),
			Text:       l.Value,
			Confidence: 1.0,
		})
		cursor = start + len(l.Value)
	}
	return out, nil
}

// Counts holds true positives, false positives and false negatives under one
// matching policy.
type Counts struct {
	TP int `json:"tp"`
	FP int `json:"fp"`
	FN int `json:"fn"`
}

// Metrics is a report for one matching policy.
//
// A zero denominator yields 1, not 0. Detecting nothing in a text that contains
// no PII is perfect precision, and dropping that case as a failure would make a
// detector look worse the more thoroughly it avoids false positives.
type Metrics struct {
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
	// Negatives counts negative cases and NegativesClean how many produced zero
	// detections. A false-positive regression shows up here even when aggregate
	// F1 barely moves, which is why it is reported separately.
	Negatives      int `json:"negatives"`
	NegativesClean int `json:"negatives_clean"`
}

// P returns precision.
func (c Counts) P() float64 { return ratio(c.TP, c.TP+c.FP) }

// R returns recall.
func (c Counts) R() float64 { return ratio(c.TP, c.TP+c.FN) }

// F1 returns the harmonic mean of P and R.
func (c Counts) F1() float64 {
	p, r := c.P(), c.R()
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}

func ratio(num, den int) float64 {
	if den == 0 {
		return 1
	}
	return float64(num) / float64(den)
}

// Report is the full result of scoring a detector.
type Report struct {
	// Exact requires the same type and identical start/end.
	Exact Metrics `json:"exact"`
	// Overlap requires the same type and any shared byte, so it tolerates
	// boundary errors.
	Overlap Metrics `json:"overlap"`
	// ByType holds exact-match counts keyed by entity type.
	ByType map[string]Counts `json:"by_type"`
	// Errors collects broken labels and predictor errors. Non-empty means the
	// dataset or the detector failed to produce an answer, which is distinct
	// from producing a wrong answer.
	Errors []string `json:"errors,omitempty"`
}

// matchPolicy decides whether a prediction found a gold entity.
type matchPolicy func(pred Prediction, gold recognizers.Entity) bool

// exactPolicy demands identical boundaries.
func exactPolicy(pred Prediction, gold recognizers.Entity) bool {
	return pred.Start == gold.Start && pred.End == gold.End
}

// overlapPolicy accepts any shared byte.
func overlapPolicy(pred Prediction, gold recognizers.Entity) bool {
	return pred.Start < gold.End && gold.Start < pred.End
}

// scorePair counts one case under one policy.
//
// A gold entity can be credited at most once, so two overlapping predictions
// cannot both count as finding the same thing. Matching is greedy in gold order;
// under exact matching that is unambiguous, and under overlap matching a
// prediction shared between two golds is credited to the first, which is
// deliberate and deterministic rather than optimal.
func scorePair(gold, pred []recognizers.Entity, policy matchPolicy) Counts {
	var c Counts
	used := make([]bool, len(gold))
	for _, p := range pred {
		hit := -1
		for i, g := range gold {
			if used[i] || p.Type != g.Type {
				continue
			}
			if policy(p, g) {
				hit = i
				break
			}
		}
		if hit >= 0 {
			used[hit] = true
			c.TP++
		} else {
			c.FP++
		}
	}
	for _, ok := range used {
		if !ok {
			c.FN++
		}
	}
	return c
}

// Score runs every case through predict and aggregates the results.
//
// A predictor error is recorded in Report.Errors and the case is skipped, not
// counted as a failure: a detector that cannot process an input has produced no
// answer rather than a wrong one, and conflating the two hides both.
func Score(ctx context.Context, dir string, predict func(context.Context, string) ([]Prediction, error)) (*Report, error) {
	cases, err := Load(dir)
	if err != nil {
		return nil, err
	}
	rep := &Report{ByType: map[string]Counts{}}

	var exactTotal, overlapTotal Counts
	negTotal, negClean := 0, 0

	for _, c := range cases {
		gold, err := c.resolve()
		if err != nil {
			rep.Errors = append(rep.Errors, err.Error())
			continue
		}
		pred, err := predict(ctx, c.Text)
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("case %q: predictor error: %v", c.ID, err))
			continue
		}

		e := scorePair(gold, pred, exactPolicy)
		o := scorePair(gold, pred, overlapPolicy)
		exactTotal.TP += e.TP
		exactTotal.FP += e.FP
		exactTotal.FN += e.FN
		overlapTotal.TP += o.TP
		overlapTotal.FP += o.FP
		overlapTotal.FN += o.FN

		if len(gold) == 0 {
			negTotal++
			if len(pred) == 0 {
				negClean++
			}
		}

		// Per-type counts must cover types seen in the PREDICTIONS as well as in
		// the gold labels. Taking keys from gold alone would report P=1.0 for a
		// type the detector hallucinated and nobody labelled, hiding the false
		// positive that is the entire reason the metric exists.
		for _, typ := range allTypes(gold, pred) {
			tc := scorePair(filterType(gold, typ), filterType(pred, typ), exactPolicy)
			cur := rep.ByType[typ]
			cur.TP += tc.TP
			cur.FP += tc.FP
			cur.FN += tc.FN
			rep.ByType[typ] = cur
		}
	}

	rep.Exact = metricsOf(exactTotal, negTotal, negClean)
	rep.Overlap = metricsOf(overlapTotal, negTotal, negClean)
	return rep, nil
}

func metricsOf(c Counts, negTotal, negClean int) Metrics {
	return Metrics{
		Precision:      c.P(),
		Recall:         c.R(),
		F1:             c.F1(),
		TP:             c.TP,
		FP:             c.FP,
		FN:             c.FN,
		Negatives:      negTotal,
		NegativesClean: negClean,
	}
}

// allTypes returns the sorted union of types present in gold and in pred.
func allTypes(gold, pred []recognizers.Entity) []string {
	seen := map[string]bool{}
	for _, e := range gold {
		seen[string(e.Type)] = true
	}
	for _, e := range pred {
		seen[string(e.Type)] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func filterType(in []recognizers.Entity, t string) []recognizers.Entity {
	var out []recognizers.Entity
	for _, e := range in {
		if string(e.Type) == t {
			out = append(out, e)
		}
	}
	return out
}

// Load reads every *.json case file in dir, in sorted filename order so runs are
// reproducible regardless of filesystem enumeration order.
//
// This is a local fixture load, not a list endpoint, so there is no pagination:
// the corpus is a fixed set of files that must all be scored, and silently
// loading only the first page would understate every metric.
//
// Unknown JSON fields are rejected. The runner derives negative cases from an
// empty entity list, so a hand-written "negative": true would otherwise be
// silently discarded — and a dataset author who wrote it would reasonably
// believe it did something. Rejecting the field turns that mistake into a loud
// error naming the field.
//
// A dataset of zero cases is also an error. With no cases there are no true
// positives and no denominators, so every metric collapses to the
// zero-denominator value of 1 and an empty or misplaced directory would report
// perfect accuracy. That is the exact failure this package exists to make
// impossible, so it is refused here rather than rendered as a passing score.
func Load(dir string) ([]Case, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading golden dir %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		// The schema sidecar is metadata, not a case file. Load sees every
		// *.json in the directory, so without this skip a corpus dataset would
		// fail wholesale with "cannot unmarshal object into []golden.Case".
		if e.Name() == SchemaFilename {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("no *.json cases found in %s", dir)
	}

	var cases []Case
	seen := map[string]bool{}
	for _, n := range names {
		path := filepath.Join(dir, n)
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		var loaded []Case
		dec := json.NewDecoder(bytes.NewReader(raw))
		// Unknown fields are a labelling mistake, not a forward-compatible
		// extension: a case file is written by hand, so every field in it was
		// meant to be read. "negative" in particular would look like it
		// controls negative-case handling and would do nothing.
		dec.DisallowUnknownFields()
		if err := dec.Decode(&loaded); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		if len(loaded) == 0 {
			return nil, fmt.Errorf("%s parsed to zero cases", path)
		}
		for _, c := range loaded {
			if c.ID == "" {
				return nil, fmt.Errorf("%s: a case has no id", path)
			}
			if seen[c.ID] {
				return nil, fmt.Errorf("%s: duplicate case id %q", path, c.ID)
			}
			seen[c.ID] = true
			if c.Text == "" {
				return nil, fmt.Errorf("case %q in %s has no text", c.ID, path)
			}
			for i, l := range c.Entities {
				if _, err := recognizers.EntityTypeFromString(string(l.Type)); err != nil {
					return nil, fmt.Errorf("case %q label %d in %s: %w", c.ID, i, path, err)
				}
			}
			cases = append(cases, c)
		}
	}
	return cases, nil
}

// Summary renders the report for a test failure message.
func (r *Report) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "exact-span match: P=%.4f R=%.4f F1=%.4f  (tp=%d fp=%d fn=%d)\n",
		r.Exact.Precision, r.Exact.Recall, r.Exact.F1, r.Exact.TP, r.Exact.FP, r.Exact.FN)
	fmt.Fprintf(&b, "overlap match:    P=%.4f R=%.4f F1=%.4f  (tp=%d fp=%d fn=%d)\n",
		r.Overlap.Precision, r.Overlap.Recall, r.Overlap.F1, r.Overlap.TP, r.Overlap.FP, r.Overlap.FN)
	fmt.Fprintf(&b, "negative cases:   %d clean of %d\n",
		r.Exact.NegativesClean, r.Exact.Negatives)
	b.WriteString("\nby type (exact):\n")
	for _, k := range r.sortedTypeKeys() {
		c := r.ByType[k]
		fmt.Fprintf(&b, "  %-12s P=%.4f R=%.4f F1=%.4f  (tp=%d fp=%d fn=%d)\n",
			k, c.P(), c.R(), c.F1(), c.TP, c.FP, c.FN)
	}
	if len(r.Errors) > 0 {
		fmt.Fprintf(&b, "\nLABEL/PREDICT ERRORS (%d):\n", len(r.Errors))
		for _, e := range r.Errors {
			fmt.Fprintf(&b, "  %s\n", e)
		}
	}
	return b.String()
}

func (r *Report) sortedTypeKeys() []string {
	keys := make([]string, 0, len(r.ByType))
	for k := range r.ByType {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
