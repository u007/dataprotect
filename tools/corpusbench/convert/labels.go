package convert

import (
	"fmt"
	"sort"

	"dataprotect/golden"
	"dataprotect/recognizers"
)

// NemotronLabels maps nemotron's corpus labels onto the detector's entity
// types. It is the corpus's declared schema: every type listed here is in
// scope for scoring, and every label not listed here is out of scope and is
// reported rather than scored as a false positive.
//
// The mapping is a reviewed decision, not an inference — see spec §5.5. Two
// entries deserve their note:
//
//   - fax_number -> PHONE. A fax number is a phone number by this taxonomy. If
//     it turns out to depress precision, the map is revisited, not the
//     recognizer: the detector has no FAX type to be wrong about.
//   - url exists only here; gretel has no url label at all.
var NemotronLabels = map[string]recognizers.EntityType{
	"email":             recognizers.EntityEmail,
	"phone_number":      recognizers.EntityPhone,
	"fax_number":        recognizers.EntityPhone,
	"credit_debit_card": recognizers.EntityCreditCard,
	"ssn":               recognizers.EntitySSN,
	"ipv4":              recognizers.EntityIP,
	"ipv6":              recognizers.EntityIP,
	"url":               recognizers.EntityURL,
}

// GretelLabels is gretel's equivalent map. It carries iban, which nemotron
// does not label — that asymmetry is why both corpora are converted rather
// than one. It has no url label, so URL detections on gretel are out of schema.
var GretelLabels = map[string]recognizers.EntityType{
	"email":              recognizers.EntityEmail,
	"phone_number":       recognizers.EntityPhone,
	"credit_card_number": recognizers.EntityCreditCard,
	"ssn":                recognizers.EntitySSN,
	"ipv4":               recognizers.EntityIP,
	"ipv6":               recognizers.EntityIP,
	"iban":               recognizers.EntityIBAN,
}

// BuildLabels turns corpus spans into golden labels.
//
// Translation happens here and nowhere else: CharToByte is the single place
// character indices become byte offsets, so no caller can bypass it. Labels
// whose corpus name is not in labelMap are dropped from gold — the corpus does
// not claim to annotate them, so counting a detection of that type as a false
// positive would be a lie — and are returned for schema.json so the scope is
// visible rather than implicit.
//
// Errors name the span index; the caller adds corpus, file and row.
func BuildLabels(text string, spans []Span, labelMap map[string]recognizers.EntityType) ([]golden.Label, []string, error) {
	// Validate the map on every call rather than caching: a map naming a type
	// the detector cannot emit would silently exclude a real type from scoring,
	// and the cost is a handful of comparisons per record.
	for name, typ := range labelMap {
		if _, err := recognizers.EntityTypeFromString(string(typ)); err != nil {
			return nil, nil, fmt.Errorf("label map maps %q to an unusable entity type: %w", name, err)
		}
	}

	labels := make([]golden.Label, 0, len(spans))
	unmappedSet := map[string]bool{}
	for i, sp := range spans {
		typ, ok := labelMap[sp.Label]
		if !ok {
			unmappedSet[sp.Label] = true
			continue
		}
		byteStart, byteEnd, err := CharToByte(text, sp.Start, sp.End)
		if err != nil {
			return nil, nil, fmt.Errorf("span %d (label %q): %w", i, sp.Label, err)
		}
		labels = append(labels, golden.Label{
			Type:  typ,
			Value: text[byteStart:byteEnd],
			Start: &byteStart,
			End:   &byteEnd,
		})
	}

	unmapped := make([]string, 0, len(unmappedSet))
	for name := range unmappedSet {
		unmapped = append(unmapped, name)
	}
	sort.Strings(unmapped)
	return labels, unmapped, nil
}

// countOverlappingPairs counts gold labels that overlap another.
//
// Overlap is valid annotation nesting and is kept — scorePair handles it, and
// the corpus scoring path never splices text, so Anonymize's overlap rejection
// is not in play. The count exists so schema.json states it rather than leaving
// a reader to wonder whether overlaps were resolved away.
//
// Labels are sorted by start first, so the count is stable regardless of file
// order and a label nested inside an earlier one is counted once.
func countOverlappingPairs(labels []golden.Label) int {
	if len(labels) < 2 {
		return 0
	}
	type span struct{ start, end int }
	seen := make([]span, 0, len(labels))
	for _, l := range labels {
		if l.Start == nil || l.End == nil {
			continue // cannot happen for converted labels; defensive
		}
		seen = append(seen, span{*l.Start, *l.End})
	}
	sort.Slice(seen, func(i, j int) bool {
		if seen[i].start == seen[j].start {
			return seen[i].end < seen[j].end
		}
		return seen[i].start < seen[j].start
	})
	pairs, furthest := 0, seen[0].end
	for i := 1; i < len(seen); i++ {
		if seen[i].start < furthest {
			pairs++
		}
		if seen[i].end > furthest {
			furthest = seen[i].end
		}
	}
	return pairs
}
