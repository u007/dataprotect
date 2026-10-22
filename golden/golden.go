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
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/u007/dataprotect/recognizers"
)

// isRuneBoundary reports whether byte offset i begins a rune in s.
//
// It walks the runes instead of testing s[i] alone: testing s[i] only says the
// byte is not a UTF-8 continuation byte, which for malformed input can be true
// of a position that is not a boundary at all. Walking cannot be fooled, and
// this runs once per label rather than in a loop.
func isRuneBoundary(s string, i int) bool {
	if i < 0 || i > len(s) {
		return false
	}
	for off := 0; off < i; {
		_, size := utf8.DecodeRuneInString(s[off:])
		if size < 1 {
			return false
		}
		off += size
		if off > i {
			return false
		}
	}
	return true
}

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
	// Unlabelled lists spans the source corpus annotates with a label this
	// dataset does not map to an entity type. They are not gold: the corpus
	// makes no claim about them. A detection that lands on one, and touches no
	// mapped gold, is bucketed out of the false positives rather than counted
	// as wrong. See Report.Unlabelled. Hand-labelled cases omit the field.
	Unlabelled []UnlabelledSpan `json:"unlabelled,omitempty"`
}

// UnlabelledSpan is one span a corpus annotates with a label outside its type
// map. Both offsets are required: unlike a Label there is no substring fallback,
// because a span that cannot be located exactly cannot be bucketed exactly.
type UnlabelledSpan struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Start *int   `json:"start"`
	End   *int   `json:"end"`
}

// Label is one expected entity, identified by the exact text it covers.
type Label struct {
	Type  recognizers.EntityType `json:"type"`
	Value string                 `json:"value"`
	// Start and End are optional byte offsets into Case.Text. Both nil means
	// "locate Value by substring", which is how every hand-authored case works
	// and remains unchanged. Both set means "use these directly", which is what
	// converted corpus cases need: 13.9% of nemotron positives and 14.9% of
	// gretel positives repeat a value, and a repeated value cannot be located
	// unambiguously by substring.
	//
	// Pointers rather than ints with -1 as a sentinel, so a span at byte 0 is
	// not confused with "no offsets given". Giving exactly one is an error.
	Start *int `json:"start,omitempty"`
	End   *int `json:"end,omitempty"`
}

// Prediction is what a detector produced. It reuses recognizers.Entity rather
// than declaring a parallel span type, so a detector cannot drift out of sync
// with the scorer's idea of a span.
type Prediction = recognizers.Entity

// offsetDesc renders an optional offset for a diagnostic. fmt's %v on a *int
// prints the pointer address, which tells a dataset author nothing they can act
// on; they need the value, or the word "nil" for the side that was omitted.
func offsetDesc(p *int) string {
	if p == nil {
		return "nil"
	}
	return strconv.Itoa(*p)
}

// resolve converts labels into concrete spans against the case text.
func (c Case) resolve() ([]recognizers.Entity, error) {
	cursor := 0
	out := make([]recognizers.Entity, 0, len(c.Entities))
	for i, l := range c.Entities {
		if l.Value == "" {
			return nil, fmt.Errorf("case %q label %d: empty value", c.ID, i)
		}
		// Exactly one offset given: a malformed label. Treating the nil as zero
		// would silently move the span rather than report the mistake.
		if (l.Start == nil) != (l.End == nil) {
			return nil, fmt.Errorf(
				"case %q label %d: half-specified offsets (start=%s end=%s); give both or "+
					"neither — exactly one would move the span instead of locating it",
				c.ID, i, offsetDesc(l.Start), offsetDesc(l.End))
		}
		// Both offsets given: they are authoritative and bypass substring
		// matching entirely, which is the point — a repeated value is located
		// by position, not by uniqueness.
		if l.Start != nil {
			start, end := *l.Start, *l.End
			if start < 0 || end > len(c.Text) || start > len(c.Text) {
				return nil, fmt.Errorf(
					"case %q label %d: offsets %d..%d out of bounds for text of %d bytes",
					c.ID, i, start, end, len(c.Text))
			}
			if start >= end {
				return nil, fmt.Errorf(
					"case %q label %d: empty or inverted span %d..%d", c.ID, i, start, end)
			}
			// Corpus offsets are character indices. A byte slice taken at a
			// mid-rune offset yields invalid UTF-8, which would then be reported
			// as the entity's text and copied into any downstream document.
			if !isRuneBoundary(c.Text, start) || !isRuneBoundary(c.Text, end) {
				return nil, fmt.Errorf(
					"case %q label %d: offsets %d..%d do not land on a rune boundary in %q; "+
						"corpus offsets are character indices and must be translated to bytes",
					c.ID, i, start, end, c.Text)
			}
			if got := c.Text[start:end]; got != l.Value {
				return nil, fmt.Errorf(
					"case %q label %d: value %q does not match text[%d:%d] = %q; the offsets "+
						"and the value disagree, so the label cannot be trusted",
					c.ID, i, l.Value, start, end, got)
			}
			out = append(out, recognizers.Entity{
				Type:       l.Type,
				Start:      start,
				End:        end,
				Text:       l.Value,
				Confidence: 1.0,
			})
			continue
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

// unlabelledSpan is an unlabelled span resolved to byte offsets.
type unlabelledSpan struct {
	label      string
	start, end int
}

// resolveUnlabelled checks each unlabelled span against the case text with the
// same rules a label gets: bounds, a non-empty span, rune boundaries, and a value
// that matches the text at its offsets. A mismatch means a broken span, and it
// is reported rather than bucketed.
func (c Case) resolveUnlabelled() ([]unlabelledSpan, error) {
	out := make([]unlabelledSpan, 0, len(c.Unlabelled))
	for i, u := range c.Unlabelled {
		if u.Label == "" {
			return nil, fmt.Errorf("case %q unlabelled span %d: empty label name", c.ID, i)
		}
		if u.Start == nil || u.End == nil {
			return nil, fmt.Errorf("case %q unlabelled span %d (%s): both start and end are required",
				c.ID, i, u.Label)
		}
		start, end := *u.Start, *u.End
		if start < 0 || end > len(c.Text) || start >= end {
			return nil, fmt.Errorf("case %q unlabelled span %d (%s): offsets %d..%d out of bounds or empty for text of %d bytes",
				c.ID, i, u.Label, start, end, len(c.Text))
		}
		if !isRuneBoundary(c.Text, start) || !isRuneBoundary(c.Text, end) {
			return nil, fmt.Errorf("case %q unlabelled span %d (%s): offsets %d..%d do not land on a rune boundary",
				c.ID, i, u.Label, start, end)
		}
		if got := c.Text[start:end]; got != u.Value {
			return nil, fmt.Errorf("case %q unlabelled span %d (%s): value %q does not match text[%d:%d] = %q",
				c.ID, i, u.Label, u.Value, start, end, got)
		}
		out = append(out, unlabelledSpan{label: u.Label, start: start, end: end})
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
	// Cases is the number of cases SCORED. It is not the number of cases
	// loaded: WithCaseLimit scores a prefix, and a report that said 2000
	// when 100 were scored would read as a full-corpus figure.
	Cases int `json:"cases"`
	// Errors collects broken labels and predictor errors. Non-empty means the
	// dataset or the detector failed to produce an answer, which is distinct
	// from producing a wrong answer.
	Errors []string `json:"errors,omitempty"`
	// OutOfSchema counts predictions excluded by WithInSchema, keyed by entity
	// type. It is empty when no schema is in force. Reporting the exclusion
	// separately is what keeps it from being mistaken for a clean run: those
	// detections were never scored, not scored and found correct.
	OutOfSchema map[string]int `json:"out_of_schema,omitempty"`
	// Unlabelled is nil unless the dataset declares unlabelled spans (a corpus
	// schema that records them) or a case carries one. It sets aside false
	// positives that land on an unlabelled span, and it never changes Exact or
	// Overlap; it reports what the headline precision would be without them.
	Unlabelled *UnlabelledReport `json:"unlabelled,omitempty"`
}

// UnlabelledReport is the bucket of detections on unlabelled spans, and the
// precision that results from setting them aside. The precision is an UPPER
// BOUND on accuracy, not accuracy: a detection on an unlabelled span may be real
// PII the corpus did not label, or it may be a genuine error. The bucket cannot
// tell which, and the headline precision counts all of them as wrong.
type UnlabelledReport struct {
	// ExactRemoved and OverlapRemoved count false positives set aside under each
	// matching policy.
	ExactRemoved   int `json:"exact_removed"`
	OverlapRemoved int `json:"overlap_removed"`
	// ExactUpperBoundP and OverlapUpperBoundP are the precisions with the bucket
	// set aside: TP / (TP + FP - removed).
	ExactUpperBoundP   float64 `json:"exact_upper_bound_precision"`
	OverlapUpperBoundP float64 `json:"overlap_upper_bound_precision"`
	// ByLabel counts the exact-policy bucket by "TYPE on source_label", so a
	// reader can tell a scope gap (tax_id) from a detector defect (account_number).
	ByLabel map[string]int `json:"by_label"`
}

// unlabelledKey renders one bucket entry, e.g. "PHONE on account_number".
func unlabelledKey(typ recognizers.EntityType, label string) string {
	return string(typ) + " on " + label
}

// bucket counts the unmatched predictions that land on an unlabelled span and
// touch no mapped gold. Precedence, in order: a true positive is never here; a
// detection touching any mapped gold stays a false positive, because a wrong
// type or a boundary miss is an error and not a scope question; a detection on
// an unlabelled span goes in the bucket; anything else is a plain false positive.
// byLabel, when non-nil, collects the bucket by "TYPE on label".
func bucket(unmatched []Prediction, gold []recognizers.Entity, spans []unlabelledSpan, byLabel map[string]int) int {
	removed := 0
	for _, p := range unmatched {
		touchesGold := false
		for _, g := range gold {
			if overlapPolicy(p, g) {
				touchesGold = true
				break
			}
		}
		if touchesGold {
			continue
		}
		for _, u := range spans {
			if p.Start < u.end && u.start < p.End {
				removed++
				if byLabel != nil {
					byLabel[unlabelledKey(p.Type, u.label)]++
				}
				break
			}
		}
	}
	return removed
}

// schemaDeclaresUnlabelled reports whether dir holds a corpus schema that records
// unlabelled spans. A schema that lacks the field was written before the field
// existed, so its bucket would read zero for a reason that has nothing to do with
// the detector. That is refused, not scored, and the error names the fix.
func schemaDeclaresUnlabelled(dir string) (bool, error) {
	_, err := os.Stat(filepath.Join(dir, SchemaFilename))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking schema in %s: %w", dir, err)
	}
	sch, err := LoadSchema(dir)
	if err != nil {
		return false, err
	}
	if !sch.UnlabelledRecorded {
		return false, fmt.Errorf("dataset %s: schema.json does not record unlabelled spans "+
			"(unlabelled_recorded is absent or false); re-run make corpus-convert so detections "+
			"on unlabelled spans are bucketed rather than counted with an empty bucket", dir)
	}
	return true, nil
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
	c, _ := scorePairDetail(gold, pred, policy)
	return c
}

// scorePairDetail is scorePair plus the unmatched predictions, which are the
// false positives. Scoring is unchanged; the list lets the unlabelled bucket
// decide which of them to set aside.
func scorePairDetail(gold, pred []recognizers.Entity, policy matchPolicy) (Counts, []Prediction) {
	var c Counts
	var unmatched []Prediction
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
			unmatched = append(unmatched, p)
		}
	}
	for _, ok := range used {
		if !ok {
			c.FN++
		}
	}
	return c, unmatched
}

// Score runs every case through predict and aggregates the results.
//
// A predictor error is recorded in Report.Errors and the case is skipped, not
// counted as a failure: a detector that cannot process an input has produced no
// answer rather than a wrong one, and conflating the two hides both.
func Score(ctx context.Context, dir string, predict func(context.Context, string) ([]Prediction, error), opts ...Option) (*Report, error) {
	cfg := applyOptions(opts)
	cases, err := Load(dir)
	if err != nil {
		return nil, err
	}
	hasSchema, err := schemaDeclaresUnlabelled(dir)
	if err != nil {
		return nil, err
	}
	if cfg.caseLimit > 0 && cfg.caseLimit < len(cases) {
		cases = cases[:cfg.caseLimit]
	}
	rep := &Report{ByType: map[string]Counts{}, OutOfSchema: map[string]int{}}
	rep.Cases = len(cases)
	var unl *UnlabelledReport
	if hasSchema {
		unl = &UnlabelledReport{ByLabel: map[string]int{}}
	}

	var exactTotal, overlapTotal Counts
	negTotal, negClean := 0, 0

	for _, c := range cases {
		gold, err := c.resolve()
		if err != nil {
			rep.Errors = append(rep.Errors, err.Error())
			continue
		}
		unlSpans, err := c.resolveUnlabelled()
		if err != nil {
			rep.Errors = append(rep.Errors, err.Error())
			continue
		}
		if len(unlSpans) > 0 && unl == nil {
			unl = &UnlabelledReport{ByLabel: map[string]int{}}
		}
		// Gold outside the schema can never match, because its predictions are
		// filtered out below. Scoring it would quietly count false negatives,
		// so it is an error naming the type rather than a wrong number.
		if err := cfg.checkGoldInsideSchema(gold, c.ID); err != nil {
			return nil, err
		}
		pred, err := predict(ctx, c.Text)
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("case %q: predictor error: %v", c.ID, err))
			continue
		}
		// Excluded before exact matching, overlap matching and the per-type
		// loop alike: filtering in only one place would let the two
		// granularities disagree for a reason that is not a boundary bug.
		pred = cfg.splitPredictions(pred, rep.OutOfSchema)

		e, eFP := scorePairDetail(gold, pred, exactPolicy)
		o, oFP := scorePairDetail(gold, pred, overlapPolicy)
		exactTotal.TP += e.TP
		exactTotal.FP += e.FP
		exactTotal.FN += e.FN
		overlapTotal.TP += o.TP
		overlapTotal.FP += o.FP
		overlapTotal.FN += o.FN

		if unl != nil {
			unl.ExactRemoved += bucket(eFP, gold, unlSpans, unl.ByLabel)
			unl.OverlapRemoved += bucket(oFP, gold, unlSpans, nil)
		}

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
	if unl != nil {
		unl.ExactUpperBoundP = ratio(exactTotal.TP, exactTotal.TP+exactTotal.FP-unl.ExactRemoved)
		unl.OverlapUpperBoundP = ratio(overlapTotal.TP, overlapTotal.TP+overlapTotal.FP-unl.OverlapRemoved)
		rep.Unlabelled = unl
	}
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

// sortedOutOfSchemaKeys returns the out-of-schema types in sorted order so a
// report renders identically across runs.
func (r *Report) sortedOutOfSchemaKeys() []string {
	keys := make([]string, 0, len(r.OutOfSchema))
	for k := range r.OutOfSchema {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
	// Printed only when non-empty: a schema-free run must look exactly as it
	// always has, and a line saying "none excluded" would be noise.
	if len(r.OutOfSchema) > 0 {
		b.WriteString("out-of-schema (not scored):")
		for _, k := range r.sortedOutOfSchemaKeys() {
			fmt.Fprintf(&b, " %s=%d", k, r.OutOfSchema[k])
		}
		b.WriteString("\n")
	}
	if u := r.Unlabelled; u != nil {
		fmt.Fprintf(&b, "excluding unlabelled spans (upper bound, not accuracy): exact P=%.4f overlap P=%.4f\n",
			u.ExactUpperBoundP, u.OverlapUpperBoundP)
		fmt.Fprintf(&b, "detections on unlabelled spans, set aside from FP: exact %d, overlap %d\n",
			u.ExactRemoved, u.OverlapRemoved)
		keys := make([]string, 0, len(u.ByLabel))
		for k := range u.ByLabel {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if u.ByLabel[keys[i]] != u.ByLabel[keys[j]] {
				return u.ByLabel[keys[i]] > u.ByLabel[keys[j]]
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			fmt.Fprintf(&b, "  %5d  %s\n", u.ByLabel[k], k)
		}
	}
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
