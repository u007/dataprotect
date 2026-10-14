package anonymizer

import (
	"context"
	"strings"
	"testing"

	"github.com/u007/dataprotect/operators"
	"github.com/u007/dataprotect/recognizers"
)

// span builds an entity for the first occurrence of value in text. Hand-counting
// byte offsets in test literals is error-prone and silently produces spans that
// slice the surrounding prose.
func span(text, value string, entityType recognizers.EntityType) recognizers.Entity {
	start := strings.Index(text, value)
	if start < 0 {
		panic("test value not present in text: " + value)
	}
	return recognizers.Entity{
		Type:       entityType,
		Start:      start,
		End:        start + len(value),
		Text:       value,
		Confidence: 0.99,
	}
}

func TestAnonymizeEmpty(t *testing.T) {
	a := New()
	got, err := a.Anonymize(context.Background(), "text", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "text" {
		t.Errorf("got %q, want %q", got, "text")
	}
}

func TestAnonymizePerType(t *testing.T) {
	a := New()
	a.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))
	a.SetOperator(recognizers.EntityPhone, operators.NewMaskOperator("***-***-####"))

	text := "mail a@b.com or call 5551234567"
	entities := []recognizers.Entity{
		span(text, "a@b.com", recognizers.EntityEmail),
		span(text, "5551234567", recognizers.EntityPhone),
	}

	got, err := a.Anonymize(context.Background(), text, entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "mail <EMAIL> or call ***-***-4567"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// An entity type with no configured operator and no default must fail the call,
// never pass through unredacted. The IP here is the leftmost entity, so the
// configured email to its right would be spliced first; the result must still
// be empty, not a partial document with the IP left in.
func TestAnonymizeUnconfiguredTypeIsAnError(t *testing.T) {
	a := New()
	a.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))

	text := "ip 8.8.8.8 mail a@b.com"
	entities := []recognizers.Entity{
		span(text, "8.8.8.8", recognizers.EntityIP),
		span(text, "a@b.com", recognizers.EntityEmail),
	}

	got, err := a.Anonymize(context.Background(), text, entities)
	if err == nil {
		t.Fatalf("expected an error for an unconfigured type, got %q", got)
	}
	if !strings.Contains(err.Error(), "IP_ADDRESS") {
		t.Errorf("error %q does not name the unconfigured type", err)
	}
	if got != "" {
		t.Errorf("error path returned a partial result %q", got)
	}
}

func TestAnonymizeDefaultOperator(t *testing.T) {
	a := New(WithDefaultOperator(operators.NewRedactOperator()))
	a.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))

	text := "mail a@b.com ip 8.8.8.8"
	entities := []recognizers.Entity{
		span(text, "a@b.com", recognizers.EntityEmail),
		span(text, "8.8.8.8", recognizers.EntityIP),
	}

	got, err := a.Anonymize(context.Background(), text, entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "mail <EMAIL> ip [REDACTED]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Two operators rewriting the same string must both land. Each replacement
// changes the text's length, so the engine must splice right-to-left rather than
// replaying offsets from the original string.
func TestAnonymizeMultipleTypesAdjustsSpans(t *testing.T) {
	a := New()
	a.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))
	a.SetOperator(recognizers.EntitySSN, operators.NewMaskOperator("***-**-####"))

	text := "a@b.com 123456789"
	entities := []recognizers.Entity{
		span(text, "a@b.com", recognizers.EntityEmail),
		span(text, "123456789", recognizers.EntitySSN),
	}

	got, err := a.Anonymize(context.Background(), text, entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "<EMAIL> ***-**-6789"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Input order must not matter: the engine sorts by offset before splicing.
func TestAnonymizeIsOrderIndependent(t *testing.T) {
	a := New()
	a.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))
	a.SetOperator(recognizers.EntityPhone, operators.NewMaskOperator("***-***-####"))

	text := "a@b.com then 5551234567"
	forward := []recognizers.Entity{
		span(text, "a@b.com", recognizers.EntityEmail),
		span(text, "5551234567", recognizers.EntityPhone),
	}
	reversed := []recognizers.Entity{forward[1], forward[0]}

	want := "<EMAIL> then ***-***-4567"
	g1, err := a.Anonymize(context.Background(), text, forward)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g2, err := a.Anonymize(context.Background(), text, reversed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g1 != want {
		t.Errorf("forward order: got %q, want %q", g1, want)
	}
	if g2 != want {
		t.Errorf("reversed order: got %q, want %q", g2, want)
	}
}

// A replacement that grows the text must not corrupt a later entity's span.
func TestAnonymizeGrowthReplacement(t *testing.T) {
	a := New()
	a.SetOperator(recognizers.EntityIP, operators.NewReplaceOperator("<A-VERY-LONG-IP-PLACEHOLDER>"))
	a.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))

	text := "ip 8.8.8.8 and email a@b.com"
	entities := []recognizers.Entity{
		span(text, "8.8.8.8", recognizers.EntityIP),
		span(text, "a@b.com", recognizers.EntityEmail),
	}
	// The email sits to the right of the IP, so it is spliced first. The IP
	// replacement then grows the string, and the email must stay intact.
	got, err := a.Anonymize(context.Background(), text, entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "ip <A-VERY-LONG-IP-PLACEHOLDER> and email <EMAIL>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAnonymizeRejectsOutOfBoundsSpan(t *testing.T) {
	a := New()
	a.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))

	_, err := a.Anonymize(context.Background(), "short", []recognizers.Entity{
		{Type: recognizers.EntityEmail, Start: 0, End: 999, Text: "short", Confidence: 0.99},
	})
	if err == nil {
		t.Fatal("expected an error for an out-of-bounds span, got nil")
	}
	if !strings.Contains(err.Error(), "out of bounds") {
		t.Errorf("error %q does not explain the out-of-bounds span", err)
	}
}

// TestAnonymizeRejectsSpanInsideRune pins the rune-boundary guard. A span whose
// start or end lands inside a multi-byte rune would splice invalid UTF-8 whatever
// the operator does, so it must fail before any splice. A span that lands on rune
// edges is still accepted.
func TestAnonymizeRejectsSpanInsideRune(t *testing.T) {
	a := New(WithDefaultOperator(operators.NewReplaceOperator("<X>")))

	// "€" is three bytes (offsets 1..4): start 2 is inside it.
	_, err := a.Anonymize(context.Background(), "a€b", []recognizers.Entity{
		{Type: recognizers.EntityType("T"), Start: 2, End: 4, Text: "\x82\xac", Confidence: 0.99},
	})
	if err == nil {
		t.Fatal("expected an error for a span starting inside a rune, got nil")
	}
	if !strings.Contains(err.Error(), "rune boundary") {
		t.Errorf("error %q does not explain the rune-boundary span", err)
	}

	// End 3 is inside "€" as well.
	_, err = a.Anonymize(context.Background(), "a€b", []recognizers.Entity{
		{Type: recognizers.EntityType("T"), Start: 1, End: 3, Text: "\xe2\x82", Confidence: 0.99},
	})
	if err == nil {
		t.Fatal("expected an error for a span ending inside a rune, got nil")
	}

	out, err := a.Anonymize(context.Background(), "a€b", []recognizers.Entity{
		{Type: recognizers.EntityType("T"), Start: 1, End: 4, Text: "€", Confidence: 0.99},
	})
	if err != nil {
		t.Fatalf("span on rune edges rejected: %v", err)
	}
	if out != "a<X>b" {
		t.Errorf("got %q, want %q", out, "a<X>b")
	}
}

// TestAnonymizeRejectsOverlappingSpans pins the overlap guard. Overlapping spans
// cannot be spliced independently — replacing the rightward one leaves the
// leftward one's offsets stale — and before this guard the result was silently
// corrupt (e.g. "01<X>>9" for [2,7)+[5,9) over "0123456789") with no error. In a
// PII scrubber that can emit partly-unredacted text, so it must fail loudly.
func TestAnonymizeRejectsOverlappingSpans(t *testing.T) {
	a := New(WithDefaultOperator(operators.NewReplaceOperator("<X>")))

	_, err := a.Anonymize(context.Background(), "0123456789", []recognizers.Entity{
		{Type: recognizers.EntityType("T"), Start: 2, End: 7, Text: "23456", Confidence: 0.99},
		{Type: recognizers.EntityType("T"), Start: 5, End: 9, Text: "5678", Confidence: 0.99},
	})
	if err == nil {
		t.Fatal("expected an error for overlapping spans, got nil")
	}
	if !strings.Contains(err.Error(), "overlapping") {
		t.Errorf("error %q does not explain the overlap", err)
	}

	// Adjacent spans (end == next start) and an empty span at a boundary are
	// NOT overlaps and must still be accepted.
	if _, err := a.Anonymize(context.Background(), "0123456789", []recognizers.Entity{
		{Type: recognizers.EntityType("T"), Start: 2, End: 5, Text: "234", Confidence: 0.99},
		{Type: recognizers.EntityType("T"), Start: 5, End: 9, Text: "5678", Confidence: 0.99},
	}); err != nil {
		t.Errorf("adjacent spans wrongly rejected: %v", err)
	}
	if _, err := a.Anonymize(context.Background(), "0123456789", []recognizers.Entity{
		{Type: recognizers.EntityType("T"), Start: 4, End: 4, Text: "", Confidence: 0.99},
		{Type: recognizers.EntityType("T"), Start: 4, End: 7, Text: "456", Confidence: 0.99},
	}); err != nil {
		t.Errorf("empty span at a boundary wrongly rejected: %v", err)
	}
}

// TestAnonymizeManyEntitiesIsDeterministic is the regression guard for the
// original defect: entities were grouped into a map by type and the groups were
// processed in random order, each replaying byte offsets computed against the
// ORIGINAL string. Whichever type the runtime happened to iterate first shifted
// the text and corrupted every later span, so the output differed run to run.
//
// Every entity below has a different replacement LENGTH (a 7-char email
// placeholder, a 12-char phone mask, a 10-char SSN mask, a 10-char redaction), so
// a single pass in the wrong order is enough to corrupt the result. Run with
// -count=50: map iteration order is randomised per run, so one pass cannot
// distinguish a correct implementation from a lucky one.
func TestAnonymizeManyEntitiesIsDeterministic(t *testing.T) {
	a := New()
	a.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))
	a.SetOperator(recognizers.EntityPhone, operators.NewMaskOperator("***-***-####"))
	a.SetOperator(recognizers.EntitySSN, operators.NewMaskOperator("***-**-####"))
	a.SetOperator(recognizers.EntityIP, operators.NewRedactOperator())

	text := "aaa a@b.com bbb 5551234567 ccc 123-45-6789 ddd 8.8.8.8 eee"
	entities := []recognizers.Entity{
		span(text, "a@b.com", recognizers.EntityEmail),
		span(text, "5551234567", recognizers.EntityPhone),
		span(text, "123-45-6789", recognizers.EntitySSN),
		span(text, "8.8.8.8", recognizers.EntityIP),
	}
	want := "aaa <EMAIL> bbb ***-***-4567 ccc ***-**-6789 ddd [REDACTED] eee"

	ctx := context.Background()
	for i := 0; i < 50; i++ {
		got, err := a.Anonymize(ctx, text, entities)
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if got != want {
			t.Fatalf("iteration %d:\n got %q\nwant %q", i, got, want)
		}
	}
}

// Adjacent entities with no separator between them: the right-hand span starts
// exactly where the left-hand one ends, which is where an off-by-one in the
// splice would truncate or duplicate a character.
func TestAnonymizeAdjacentEntities(t *testing.T) {
	a := New()
	a.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<E>"))
	a.SetOperator(recognizers.EntityIP, operators.NewReplaceOperator("<IP>"))

	text := "a@b.com8.8.8.8"
	entities := []recognizers.Entity{
		span(text, "a@b.com", recognizers.EntityEmail),
		span(text, "8.8.8.8", recognizers.EntityIP),
	}
	if entities[0].End != entities[1].Start {
		t.Fatalf("test setup wrong: spans are not adjacent (%d, %d)", entities[0].End, entities[1].Start)
	}

	got, err := a.Anonymize(context.Background(), text, entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "<E><IP>" {
		t.Errorf("got %q, want %q", got, "<E><IP>")
	}
}

// Two entities of the same type must both be rewritten, including when one
// replacement is a different length from the other.
func TestAnonymizeRepeatedTypeDifferentLengths(t *testing.T) {
	a := New()
	a.SetOperator(recognizers.EntityEmail, operators.NewHashOperator(8))

	text := "a@x.com and b@yyyyyyyy.com"
	entities := []recognizers.Entity{
		span(text, "a@x.com", recognizers.EntityEmail),
		span(text, "b@yyyyyyyy.com", recognizers.EntityEmail),
	}

	got, err := a.Anonymize(context.Background(), text, entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// SHA-256 prefixes of "a@x.com" and "b@yyyyyyyy.com".
	want := "478abec7 and bb9819d3"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
