package anonymizer

import (
	"context"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"dataprotect/operators"
	"dataprotect/recognizers"
)

// Property tests for Anonymizer.Anonymize.
//
// Anonymize is byte-offset arithmetic: it walks entities right-to-left,
// splicing each span out and a replacement in, so that the offsets of the
// entities still to come stay valid. Get the direction or the span bounds
// wrong and it silently corrupts the document — or panics on a rune boundary.
// The analyzer's fuzz target covers the spans that reach Anonymize; this one
// covers the splice itself, over arbitrary text, arbitrary entity layouts and
// arbitrary replacements.
//
// The invariants:
//   - For a well-formed entity list (in-bounds, non-overlapping), Anonymize
//     never errors and the output is exactly the text with each span replaced.
//     The oracle builds that left-to-right; Anonymize builds it right-to-left,
//     so a bug in the direction shows up as a mismatch rather than matching the
//     oracle by construction.
//   - For an arbitrary entity list, Anonymize never panics. It either errors
//     (returning the empty string) or returns a string. Overlapping spans are
//     rejected with an error, so the malformed-input fuzz below expects the
//     error path for them.
//   - Anonymize never mutates the caller's entity slice.
//   - The operator is applied to e.Text, not to text[e.Start:e.End]; the
//     recognizers always make them equal, so this documents the trust boundary.
//
// The decoders drop overlapping spans when building well-formed input, so the
// ORACLE never sees an overlap — overlap handling is covered by the explicit
// error test in anonymizer_test.go instead.

// oracleSplice is the specification Anonymize must implement, written the
// obvious left-to-right way. Entities must be ascending and non-overlapping.
func oracleSplice(text string, entities []recognizers.Entity, repl func(recognizers.Entity) string) string {
	var b strings.Builder
	prev := 0
	for _, e := range entities {
		b.WriteString(text[prev:e.Start])
		b.WriteString(repl(e))
		prev = e.End
	}
	b.WriteString(text[prev:])
	return b.String()
}

// decodeValidEntities turns raw bytes into a WELL-FORMED entity list: each pair
// of bytes is a start and a length scaled into the text, then the list is
// sorted and any span overlapping the previous kept one is dropped. The result
// is guaranteed in-bounds, ascending and non-overlapping, with Text matching
// the span — the input contract Anonymize documents.
func decodeValidEntities(text string, raw []byte) []recognizers.Entity {
	if len(text) == 0 {
		return nil
	}
	type bounds struct{ start, end int }
	var spans []bounds
	for i := 0; i+1 < len(raw); i += 2 {
		start := int(raw[i]) * len(text) / 256
		end := start + int(raw[i+1])*len(text)/256
		if end > len(text) {
			end = len(text)
		}
		spans = append(spans, bounds{start, end})
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end < spans[j].end
	})

	var out []recognizers.Entity
	lastEnd := 0
	for _, s := range spans {
		if s.start < lastEnd {
			continue // overlaps the previously kept span
		}
		out = append(out, recognizers.Entity{
			Type:       recognizers.EntityType("T"),
			Start:      s.start,
			End:        s.end,
			Text:       text[s.start:s.end],
			Confidence: 0.9,
		})
		lastEnd = s.end
	}
	return out
}

// decodeRawEntities turns raw bytes into a possibly-MALFORMED entity list:
// offsets are byte-derived and may be negative, inverted or out of range. It is
// used only to prove Anonymize never panics on bad input.
func decodeRawEntities(text string, raw []byte) []recognizers.Entity {
	var out []recognizers.Entity
	for i := 0; i+1 < len(raw); i += 2 {
		start := int(raw[i]) - 128
		end := int(raw[i+1]) - 128
		e := recognizers.Entity{Type: recognizers.EntityType("T"), Start: start, End: end, Confidence: 0.9}
		if start >= 0 && end >= start && end <= len(text) {
			e.Text = text[start:end]
		}
		out = append(out, e)
	}
	return out
}

func copyEntities(entities []recognizers.Entity) []recognizers.Entity {
	out := make([]recognizers.Entity, len(entities))
	copy(out, entities)
	return out
}

// sortedAscending returns a copy of entities in ascending (start, end) order.
// oracleSplice requires that order; Anonymize sorts internally, so an unsorted
// but otherwise valid input is legal and must be sorted before being handed to
// the oracle.
func sortedAscending(entities []recognizers.Entity) []recognizers.Entity {
	out := copyEntities(entities)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		return out[i].End < out[j].End
	})
	return out
}

// TestAnonymizeInvariantsOnCorpus runs the splice oracle over a corpus without
// needing the fuzzer, for several operators (so replacement lengths vary). It
// reuses the package's span helper so no byte offset is hand-counted.
func TestAnonymizeInvariantsOnCorpus(t *testing.T) {
	ctx := context.Background()
	email := recognizers.EntityEmail
	phone := recognizers.EntityPhone
	card := recognizers.EntityCreditCard

	corpus := []struct {
		text     string
		entities []recognizers.Entity
	}{
		{"", nil},
		{"plain text with no entities", nil},
		{"a@b.com", []recognizers.Entity{span("a@b.com", "a@b.com", email)}},
		{
			"mail a@b.com or call 5551234567 now",
			[]recognizers.Entity{
				span("mail a@b.com or call 5551234567 now", "a@b.com", email),
				span("mail a@b.com or call 5551234567 now", "5551234567", phone),
			},
		},
		{
			"adjacent a@b.comc@d.com end",
			[]recognizers.Entity{
				span("adjacent a@b.comc@d.com end", "a@b.com", email),
				span("adjacent a@b.comc@d.com end", "c@d.com", email),
			},
		},
		{
			"unicode é☃ 4111111111111111 tail",
			[]recognizers.Entity{
				span("unicode é☃ 4111111111111111 tail", "é☃", recognizers.EntityType("T")),
				span("unicode é☃ 4111111111111111 tail", "4111111111111111", card),
			},
		},
		{
			"empty span here",
			[]recognizers.Entity{span("empty span here", "", recognizers.EntityType("T"))},
		},
		{
			// Deliberately UNSORTED: Anonymize sorts internally, so descending
			// input is legal and must produce the same splice as ascending.
			"two entities out of order",
			[]recognizers.Entity{
				span("two entities out of order", "order", recognizers.EntityType("T")),
				span("two entities out of order", "two", recognizers.EntityType("T")),
			},
		},
	}

	ops := []operators.Operator{
		operators.NewReplaceOperator(""),
		operators.NewReplaceOperator("<X>"),
		operators.NewReplaceOperator("a much longer replacement than the entity"),
		operators.NewRedactOperator(),
		operators.NewHashOperator(8),
		operators.NewMaskOperator("****-****-****-####"),
	}

	for _, tc := range corpus {
		for _, op := range ops {
			a := New(WithDefaultOperator(op))
			before := copyEntities(tc.entities)

			got, err := a.Anonymize(ctx, tc.text, tc.entities)
			if err != nil {
				t.Fatalf("%s on %q: unexpected error: %v", op.Name(), tc.text, err)
			}
			want := oracleSplice(tc.text, sortedAscending(tc.entities), func(e recognizers.Entity) string {
				out, err := op.Replace(ctx, e.Text)
				if err != nil {
					t.Fatalf("%s.Replace(%q): %v", op.Name(), e.Text, err)
				}
				return out
			})
			if got != want {
				t.Errorf("%s on %q:\n got  %q\n want %q", op.Name(), tc.text, got, want)
			}
			for i := range before {
				if before[i] != tc.entities[i] {
					t.Errorf("%s mutated caller's entity %d: %+v -> %+v", op.Name(), i, before[i], tc.entities[i])
				}
			}
		}
	}
}

// TestAnonymizePerTypeRoutingOracle covers the two paths the single-default
// corpus loop never reaches: per-type operators set with SetOperator, and a
// type with neither an operator nor a default, which must pass through
// untouched.
func TestAnonymizePerTypeRoutingOracle(t *testing.T) {
	ctx := context.Background()
	text := "mail a@b.com call 5551234567"
	entities := []recognizers.Entity{
		span(text, "a@b.com", recognizers.EntityEmail),
		span(text, "5551234567", recognizers.EntityPhone),
	}

	// Email and phone route to DIFFERENT operators, so a bug that ignored the
	// type map (or applied one operator to everything) would be caught.
	a := New()
	a.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))
	a.SetOperator(recognizers.EntityPhone, operators.NewMaskOperator("***-***-####"))

	got, err := a.Anonymize(ctx, text, entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := oracleSplice(text, entities, func(e recognizers.Entity) string {
		op := a.operatorFor(e.Type)
		if op == nil {
			t.Fatalf("no operator for %s", e.Type)
		}
		out, err := op.Replace(ctx, e.Text)
		if err != nil {
			t.Fatalf("%s.Replace(%q): %v", op.Name(), e.Text, err)
		}
		return out
	})
	if got != want {
		t.Errorf("per-type routing:\n got  %q\n want %q", got, want)
	}

	// No operator for either type and no default: both pass through untouched.
	untouched := New()
	got, err = untouched.Anonymize(ctx, text, entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != text {
		t.Errorf("unconfigured entities were altered: got %q, want %q", got, text)
	}
}

// TestAnonymizeUsesEntityText pins the trust boundary: the operator is applied
// to e.Text, NOT to text[e.Start:e.End]. The recognizers always make the two
// equal (the analyzer's invariants assert it), so this is a deliberately lying
// entity; it documents that Anonymize believes Text.
func TestAnonymizeUsesEntityText(t *testing.T) {
	ctx := context.Background()
	text := "value=4111111111111111"
	// Span content is the card, but Text claims a different number.
	e := recognizers.Entity{
		Type: recognizers.EntityCreditCard, Start: 6, End: 22, Text: "9999999999999999", Confidence: 1,
	}
	a := New(WithDefaultOperator(operators.NewMaskOperator("****-****-****-####")))

	got, err := a.Anonymize(ctx, text, []recognizers.Entity{e})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Masking Text keeps its last four (9999), not the span's (1111).
	const want = "value=****-****-****-9999"
	if got != want {
		t.Errorf("operator did not use e.Text: got %q, want %q", got, want)
	}
}

// TestAnonymizeMaskOnMultibyteEntity pins that a rune-aware mask composes with
// the byte-offset splice: the output stays valid UTF-8 and the entity's runes are
// masked one slot per character, not per byte.
func TestAnonymizeMaskOnMultibyteEntity(t *testing.T) {
	ctx := context.Background()
	text := "name: José ☃"
	e := span(text, "José ☃", recognizers.EntityType("T"))
	a := New(WithDefaultOperator(operators.NewMaskOperator("#***")))

	got, err := a.Anonymize(ctx, text, []recognizers.Entity{e})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !utf8.ValidString(got) {
		t.Errorf("output is not valid UTF-8: %q", got)
	}
	// "#***" keeps the first significant rune (J) and masks the next four
	// (o, s, é, ☃); the space is a separator and is dropped. One '*' per rune.
	const want = "name: J****"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// wellFormed reports whether entities satisfy Anonymize's input contract:
// in-bounds, Text matching the span, and non-overlapping. Order does not
// matter (Anonymize sorts).
func wellFormed(text string, entities []recognizers.Entity) bool {
	for _, e := range entities {
		if e.Start < 0 || e.Start > e.End || e.End > len(text) || e.Text != text[e.Start:e.End] {
			return false
		}
	}
	sorted := copyEntities(entities)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Start < sorted[i-1].End {
			return false
		}
	}
	return true
}

// FuzzAnonymizeInvariants asserts the splice oracle for well-formed entities,
// over arbitrary text, replacements and entity layouts.
//
// Run: go test ./anonymizer -run=FuzzAnonymizeInvariants -fuzz=FuzzAnonymizeInvariants
func FuzzAnonymizeInvariants(f *testing.F) {
	f.Add("", "<X>", []byte{})
	f.Add("abcd", "<X>", []byte{0, 64, 64, 64})
	f.Add("a@b.com", "[REDACTED]", []byte{0, 128})
	f.Add("é☃", "*", []byte{0, 128, 64, 128})
	f.Add("card 4111111111111111 phone 555-123-4567", "###", []byte{0, 64, 64, 64, 128, 64})

	f.Fuzz(func(t *testing.T, text, marker string, raw []byte) {
		entities := decodeValidEntities(text, raw)
		a := New(WithDefaultOperator(operators.NewReplaceOperator(marker)))
		before := copyEntities(entities)

		got, err := a.Anonymize(context.Background(), text, entities)
		if err != nil {
			t.Fatalf("valid entities errored: %v\ntext=%q marker=%q entities=%+v", err, text, marker, entities)
		}
		want := oracleSplice(text, entities, func(recognizers.Entity) string { return marker })
		if got != want {
			t.Fatalf("splice mismatch:\ntext=%q marker=%q entities=%+v\n got  %q\n want %q",
				text, marker, entities, got, want)
		}
		for i := range before {
			if before[i] != entities[i] {
				t.Fatalf("Anonymize mutated caller's entity %d: %+v -> %+v", i, before[i], entities[i])
			}
		}
	})
}

// FuzzAnonymizeNeverPanics feeds Anonymize deliberately malformed entity lists
// (negative, inverted, out-of-range, overlapping). It may return an error; it
// must not panic, and a failed call must not return a half-spliced string.
//
// Run: go test ./anonymizer -run=FuzzAnonymizeNeverPanics -fuzz=FuzzAnonymizeNeverPanics
func FuzzAnonymizeNeverPanics(f *testing.F) {
	f.Add("abcd", []byte{0, 200, 50, 200, 255, 255})
	f.Add("", []byte{1, 2, 3, 4})
	f.Add("text", []byte{255, 0, 128, 128})
	// The input that first panicked the oracle: valid but UNSORTED entities
	// ([5,5) then [4,5) over "00000"). Kept as a seed so plain `go test` covers
	// the unsorted-valid path.
	f.Add("00000", []byte{0x85, 0x85, 0x84, 0x85})
	// A valid, sorted, non-overlapping layout, so the "oracle when well-formed"
	// branch runs on every `go test`, not only when random bytes happen to
	// decode to a valid list.
	f.Add("abcd", []byte{0x80, 0x81, 0x81, 0x81})

	f.Fuzz(func(t *testing.T, text string, raw []byte) {
		entities := decodeRawEntities(text, raw)
		a := New(WithDefaultOperator(operators.NewReplaceOperator("<X>")))
		got, err := a.Anonymize(context.Background(), text, entities)

		if err != nil {
			if got != "" {
				t.Fatalf("error path returned a partial result %q (err=%v)", got, err)
			}
			return
		}
		// If the byte-decoded list happens to satisfy the input contract, the
		// output must be the exact splice. Without this the target is only a
		// no-panic check and could pass while returning wrong output.
		if wellFormed(text, entities) {
			want := oracleSplice(text, sortedAscending(entities), func(recognizers.Entity) string { return "<X>" })
			if got != want {
				t.Fatalf("valid layout, wrong splice:\ntext=%q entities=%+v\n got  %q\n want %q",
					text, entities, got, want)
			}
		}
	})
}
