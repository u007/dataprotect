package convert

import (
	"strings"
	"testing"
	"unicode/utf8"

	"dataprotect/golden"
	"dataprotect/recognizers"
)

// The gretel worked example: "café 4111 1111 1111 1111" — é is two bytes, so
// every later character index is one byte further from its byte index than a
// naive reader assumes. char 5..24 is byte 6..25.
func TestBuildLabelsTranslatesCharToByte(t *testing.T) {
	const text = "café 4111 1111 1111 1111"
	spans := []Span{{Start: 5, End: 24, Label: "credit_debit_card"}}

	labels, unmapped, err := BuildLabels(text, spans, NemotronLabels)
	if err != nil {
		t.Fatalf("BuildLabels: unexpected error: %v", err)
	}
	if len(unmapped) != 0 {
		t.Errorf("unmapped = %v, want empty", unmapped)
	}
	if len(labels) != 1 {
		t.Fatalf("got %d labels, want 1", len(labels))
	}
	l := labels[0]
	if l.Type != recognizers.EntityCreditCard {
		t.Errorf("Type = %q, want CREDIT_CARD", l.Type)
	}
	if l.Start == nil || l.End == nil {
		t.Fatalf("offsets = %v/%v, want both set", l.Start, l.End)
	}
	if *l.Start != 6 || *l.End != 25 {
		t.Errorf("offsets = %d..%d, want 6..25 (char 5..24)", *l.Start, *l.End)
	}
	if l.Value != "4111 1111 1111 1111" {
		t.Errorf("Value = %q, want %q", l.Value, "4111 1111 1111 1111")
	}
	// The bug this guards: slicing the BYTES at the character indices gives
	// shifted text. If the test ever produces that, the translation is gone.
	if naive := text[5:24]; naive == l.Value {
		t.Errorf("byte slice at the char indices produced the right value, so the shift is not exercised")
	}
	if strings.Contains(l.Value, " 4111") || !strings.Contains(l.Value, "1111") {
		t.Errorf("Value = %q looks byte-shifted", l.Value)
	}
}

// An unmapped corpus label is dropped from gold but recorded, so the schema can
// state its own scope rather than leaving a reader to guess which of the 55
// corpus labels the measurement covers.
func TestBuildLabelsDropsUnmappedLabels(t *testing.T) {
	const text = "Ada Lovelace"
	spans := []Span{
		{Start: 0, End: 3, Label: "first_name"}, // not in NemotronLabels
		{Start: 4, End: 12, Label: "last_name"}, // not in NemotronLabels
	}
	labels, unmapped, err := BuildLabels(text, spans, NemotronLabels)
	if err != nil {
		t.Fatalf("BuildLabels: unexpected error: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("labels = %+v, want none — an unmapped label must not become gold", labels)
	}
	if len(unmapped) != 2 {
		t.Fatalf("unmapped = %v, want both dropped labels", unmapped)
	}
	seen := map[string]bool{}
	for _, u := range unmapped {
		seen[u] = true
	}
	if !seen["first_name"] || !seen["last_name"] {
		t.Errorf("unmapped = %v, want first_name and last_name", unmapped)
	}
}

// The spec's fail-fast list replaces "non-rune boundary" (unreachable — see the
// ledger ruling): CharToByte only ever returns rune-start positions, so this
// asserts the property structurally instead of testing an unreachable error.
func TestBuildLabelsOffsetsLandOnRuneBoundaries(t *testing.T) {
	for _, text := range []string{
		"café 4111 1111 1111 1111",
		"日本語 555-123-4567",
		"aéb@example.com",
		"plain ascii@example.com",
	} {
		spans := []Span{{Start: 0, End: len([]rune(text)), Label: "email"}}
		labels, _, err := BuildLabels(text, spans, NemotronLabels)
		if err != nil {
			t.Fatalf("BuildLabels(%q): unexpected error: %v", text, err)
		}
		if len(labels) != 1 {
			t.Fatalf("BuildLabels(%q): got %d labels, want 1", text, len(labels))
		}
		l := labels[0]
		if *l.Start > 0 && !utf8.RuneStart(text[*l.Start]) {
			t.Errorf("%q: start byte %d is not a rune boundary", text, *l.Start)
		}
		if *l.End < len(text) && !utf8.RuneStart(text[*l.End]) {
			t.Errorf("%q: end byte %d is not a rune boundary", text, *l.End)
		}
		if text[*l.Start:*l.End] != l.Value {
			t.Errorf("%q: offsets slice to %q but Value is %q", text, text[*l.Start:*l.End], l.Value)
		}
	}
}

// Each fail-fast clause from spec §5.2 must name the span it rejected, so the
// operator running a conversion knows which annotation to look at.
func TestBuildLabelsFailsFast(t *testing.T) {
	const text = "café 4111"
	for _, tc := range []struct {
		name    string
		text    string
		spans   []Span
		wantMsg string
	}{
		{
			name:    "out of bounds",
			text:    text,
			spans:   []Span{{Start: 0, End: 999, Label: "credit_debit_card"}},
			wantMsg: "span 0",
		},
		{
			name:    "zero length",
			text:    text,
			spans:   []Span{{Start: 4, End: 4, Label: "credit_debit_card"}},
			wantMsg: "span 0",
		},
		{
			name:    "non UTF-8 text",
			text:    "caf\xe9latte 4111",
			spans:   []Span{{Start: 0, End: 4, Label: "credit_debit_card"}},
			wantMsg: "UTF-8",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := BuildLabels(tc.text, tc.spans, NemotronLabels)
			if err == nil {
				t.Fatal("BuildLabels: want an error, got nil")
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

// A label map naming a type the detector cannot emit is a configuration bug,
// not something to skip: the measurement would silently exclude a real type.
func TestBuildLabelsRejectsUnknownMappedType(t *testing.T) {
	bad := map[string]recognizers.EntityType{"email": "NOT_A_TYPE"}
	if _, _, err := BuildLabels("a@b.com", []Span{{Start: 0, End: 7, Label: "email"}}, bad); err == nil {
		t.Fatal("BuildLabels: want an error for an unknown mapped type, got nil")
	} else if !strings.Contains(err.Error(), "NOT_A_TYPE") {
		t.Errorf("error %q should name the bad type", err.Error())
	}
}

// Unmapped labels are reported as a set, so two records dropping the same label
// must not produce duplicates in schema.json.
func TestBuildLabelsDeduplicatesUnmapped(t *testing.T) {
	const text = "Ada"
	spans := []Span{
		{Start: 0, End: 3, Label: "first_name"},
		{Start: 0, End: 3, Label: "first_name"},
	}
	_, unmapped, err := BuildLabels(text, spans, NemotronLabels)
	if err != nil {
		t.Fatalf("BuildLabels: unexpected error: %v", err)
	}
	if len(unmapped) != 1 || unmapped[0] != "first_name" {
		t.Errorf("unmapped = %v, want a single deduplicated entry", unmapped)
	}
}

// Golden.Label with nil offsets would fall back to substring matching, which
// rejects repeated values — the exact case Task 3 exists to fix. Converted
// labels must always carry offsets.
func TestBuildLabelsAlwaysSetsOffsets(t *testing.T) {
	const text = "card 4111 1111 1111 1111 and 4111 1111 1111 1111"
	spans := []Span{
		{Start: 5, End: 24, Label: "credit_debit_card"},
		{Start: 29, End: 48, Label: "credit_debit_card"},
	}
	labels, _, err := BuildLabels(text, spans, NemotronLabels)
	if err != nil {
		t.Fatalf("BuildLabels: unexpected error: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("got %d labels, want 2", len(labels))
	}
	for i, l := range labels {
		if l.Start == nil || l.End == nil {
			t.Fatalf("labels[%d] offsets are nil; the substring fallback would reject this repeated value", i)
		}
	}
	if labels[0].Value != labels[1].Value {
		t.Errorf("values differ: %q vs %q", labels[0].Value, labels[1].Value)
	}
	if *labels[0].Start == *labels[1].Start {
		t.Errorf("both labels start at %d; they must be distinct occurrences", *labels[0].Start)
	}
	_ = golden.Case{} // package import used by the type under test
}

// Three spans that all overlap each other are three pairs. The schema reports
// this count, so an undercount would misstate how much nesting the corpus holds.
func TestCountOverlappingPairsCountsEveryPair(t *testing.T) {
	cases := []struct {
		name  string
		spans [][2]int
		want  int
	}{
		{"three mutually overlapping", [][2]int{{0, 10}, {1, 20}, {2, 3}}, 3},
		{"touching spans do not overlap", [][2]int{{0, 5}, {5, 10}}, 0},
		{"identical spans are one pair", [][2]int{{4, 9}, {4, 9}}, 1},
		{"nested span counts once", [][2]int{{0, 10}, {3, 4}}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var labels []golden.Label
			for _, s := range tc.spans {
				start, end := s[0], s[1]
				labels = append(labels, golden.Label{Start: &start, End: &end})
			}
			got, err := countOverlappingPairs(labels)
			if err != nil {
				t.Fatalf("countOverlappingPairs: unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("countOverlappingPairs = %d, want %d", got, tc.want)
			}
		})
	}
}

// A label without offsets must fail loudly. The old sweep skipped it and could
// then index an empty slice, panicking on exactly the input it had skipped.
func TestCountOverlappingPairsRejectsMissingOffsets(t *testing.T) {
	start := 0
	labels := []golden.Label{
		{Type: recognizers.EntityEmail, Value: "a@b.co", Start: &start},
		{Type: recognizers.EntityEmail, Value: "a@b.co"},
	}
	if _, err := countOverlappingPairs(labels); err == nil {
		t.Fatal("countOverlappingPairs with a label missing its End: want error, got nil")
	}
	none := []golden.Label{{Type: recognizers.EntityEmail, Value: "a@b.co"}, {Type: recognizers.EntityEmail, Value: "x@y.co"}}
	if _, err := countOverlappingPairs(none); err == nil {
		t.Fatal("countOverlappingPairs with no offsets at all: want error, got nil")
	}
}

// Unmapped spans are kept with byte offsets, because the scorer must bucket a
// detection by position. The sample has a two-byte rune before the span, so the
// character and byte offsets differ and a missing translation would show.
func TestBuildUnlabelledKeepsOnlyUnmappedSpansInBytes(t *testing.T) {
	const text = "Zoë paid 555-867-5309 via account 12345"
	spans := []Span{
		{Start: 9, End: 21, Label: "phone_number"},    // mapped: not unlabelled
		{Start: 34, End: 39, Label: "account_number"}, // unmapped: kept
	}
	got, err := BuildUnlabelled(text, spans, NemotronLabels)
	if err != nil {
		t.Fatalf("BuildUnlabelled: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d unlabelled spans, want 1: %+v", len(got), got)
	}
	u := got[0]
	if u.Label != "account_number" || u.Value != "12345" {
		t.Errorf("span = %q %q, want account_number 12345", u.Label, u.Value)
	}
	if *u.Start != 35 || *u.End != 40 {
		t.Errorf("byte offsets = %d..%d, want 35..40", *u.Start, *u.End)
	}
	if text[*u.Start:*u.End] != u.Value {
		t.Errorf("text[%d:%d] = %q, does not match value %q", *u.Start, *u.End, text[*u.Start:*u.End], u.Value)
	}
}

// An unlabelled span with a bad offset is an error, not a silent drop.
func TestBuildUnlabelledFailsFastOnBadOffset(t *testing.T) {
	spans := []Span{{Start: 0, End: 500, Label: "account_number"}}
	if _, err := BuildUnlabelled("short", spans, NemotronLabels); err == nil {
		t.Fatal("BuildUnlabelled with out-of-range span: want error, got nil")
	}
}
