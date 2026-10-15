package anonymizer

import (
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// restore substitutes each placeholder in out back to its original value. It is
// the inverse the map promises: restore(out, map) must equal the input.
func restore(out string, pseudonyms map[string]string) string {
	var pairs []string
	for placeholder, original := range pseudonyms {
		pairs = append(pairs, placeholder, original)
	}
	return strings.NewReplacer(pairs...).Replace(out)
}

func TestPseudonymizeNumbersLeftToRight(t *testing.T) {
	text := "a x@a.com b y@b.com c"
	entities := []recognizers.Entity{
		span(text, "x@a.com", recognizers.EntityEmail),
		span(text, "y@b.com", recognizers.EntityEmail),
	}
	out, pseudonyms, err := New().Pseudonymize(text, entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "a [EMAIL_1] b [EMAIL_2] c"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if pseudonyms["[EMAIL_1]"] != "x@a.com" || pseudonyms["[EMAIL_2]"] != "y@b.com" || len(pseudonyms) != 2 {
		t.Errorf("map = %v", pseudonyms)
	}
}

func TestPseudonymizeRepeatedValueReusesPlaceholder(t *testing.T) {
	text := "x@a.com and x@a.com and y@b.com"
	entities := []recognizers.Entity{
		{Type: recognizers.EntityEmail, Start: 0, End: 7, Text: "x@a.com", Confidence: 0.99},
		{Type: recognizers.EntityEmail, Start: 12, End: 19, Text: "x@a.com", Confidence: 0.99},
		span(text, "y@b.com", recognizers.EntityEmail),
	}
	out, pseudonyms, err := New().Pseudonymize(text, entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[EMAIL_1] and [EMAIL_1] and [EMAIL_2]"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if len(pseudonyms) != 2 {
		t.Errorf("map has %d entries, want 2: %v", len(pseudonyms), pseudonyms)
	}
}

func TestPseudonymizeCountersArePerType(t *testing.T) {
	text := "x@a.com 4111111111111111 y@b.com"
	entities := []recognizers.Entity{
		span(text, "x@a.com", recognizers.EntityEmail),
		span(text, "4111111111111111", recognizers.EntityCreditCard),
		span(text, "y@b.com", recognizers.EntityEmail),
	}
	out, _, err := New().Pseudonymize(text, entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[EMAIL_1] [CREDIT_CARD_1] [EMAIL_2]"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

func TestPseudonymizeSameValueUnderTwoTypesGetsTwoPlaceholders(t *testing.T) {
	text := "12345 and 12345"
	entities := []recognizers.Entity{
		{Type: recognizers.EntitySSN, Start: 0, End: 5, Text: "12345", Confidence: 0.99},
		{Type: recognizers.EntityPhone, Start: 10, End: 15, Text: "12345", Confidence: 0.99},
	}
	out, pseudonyms, err := New().Pseudonymize(text, entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[SSN_1] and [PHONE_1]"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if pseudonyms["[SSN_1]"] != "12345" || pseudonyms["[PHONE_1]"] != "12345" {
		t.Errorf("map = %v", pseudonyms)
	}
}

// Pseudonymize ignores the configured operators: every entity is replaced.
func TestPseudonymizeIgnoresOperators(t *testing.T) {
	text := "x@a.com"
	a := New()
	a.SetOperator(recognizers.EntityEmail, nil)
	out, _, err := a.Pseudonymize(text, []recognizers.Entity{span(text, "x@a.com", recognizers.EntityEmail)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "[EMAIL_1]" {
		t.Errorf("out = %q, want [EMAIL_1]", out)
	}
}

func TestPseudonymizeNoEntitiesReturnsEmptyNonNilMap(t *testing.T) {
	out, pseudonyms, err := New().Pseudonymize("nothing here", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "nothing here" {
		t.Errorf("out = %q", out)
	}
	if pseudonyms == nil || len(pseudonyms) != 0 {
		t.Errorf("map = %v, want empty and non-nil", pseudonyms)
	}
}

// Every error returns an empty string and a nil map, so no partial result can be
// mistaken for a usable one.
func TestPseudonymizeErrorsReturnEmptyAndNil(t *testing.T) {
	text := "x@a.com and y@b.com"
	tests := []struct {
		name     string
		text     string
		entities []recognizers.Entity
	}{
		{"out of bounds", "short", []recognizers.Entity{
			{Type: recognizers.EntityEmail, Start: 0, End: 999, Text: "short", Confidence: 0.99}}},
		{"overlap", text, []recognizers.Entity{
			{Type: recognizers.EntityEmail, Start: 0, End: 7, Text: "x@a.com", Confidence: 0.99},
			{Type: recognizers.EntityEmail, Start: 4, End: 12, Text: "and y@b", Confidence: 0.99}}},
		{"placeholder collision", "see [EMAIL_1] x@a.com", []recognizers.Entity{
			span("see [EMAIL_1] x@a.com", "x@a.com", recognizers.EntityEmail)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, pseudonyms, err := New().Pseudonymize(tt.text, tt.entities)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if out != "" || pseudonyms != nil {
				t.Errorf("got out=%q map=%v, want empty string and nil map", out, pseudonyms)
			}
		})
	}
}

// An error names the token, never the original value.
func TestPseudonymizeCollisionErrorDoesNotLeakValue(t *testing.T) {
	text := "see [EMAIL_1] x@a.com"
	_, _, err := New().Pseudonymize(text, []recognizers.Entity{span(text, "x@a.com", recognizers.EntityEmail)})
	if err == nil {
		t.Fatal("expected a collision error")
	}
	if !strings.Contains(err.Error(), "[EMAIL_1]") {
		t.Errorf("error %q does not name the token", err)
	}
	if strings.Contains(err.Error(), "x@a.com") {
		t.Errorf("error %q leaks the original value", err)
	}
}

// A bracketed token whose type is not a known entity is ordinary text.
func TestPseudonymizeUnknownBracketTokenIsNotACollision(t *testing.T) {
	text := "[FOO_1] x@a.com"
	out, pseudonyms, err := New().Pseudonymize(text, []recognizers.Entity{span(text, "x@a.com", recognizers.EntityEmail)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "[FOO_1] [EMAIL_1]" || pseudonyms["[EMAIL_1]"] != "x@a.com" {
		t.Errorf("out=%q map=%v", out, pseudonyms)
	}
}

// The round trip: restoring every placeholder reproduces the input exactly. The
// text has a repeated card and a repeated email, so placeholders are shared.
func TestPseudonymizeRoundTrip(t *testing.T) {
	text := "x@a.com 4111111111111111 x@a.com ünïcode y@b.com 4111111111111111"
	var entities []recognizers.Entity
	for _, hit := range []struct {
		value string
		typ   recognizers.EntityType
	}{
		{"x@a.com", recognizers.EntityEmail},
		{"4111111111111111", recognizers.EntityCreditCard},
		{"x@a.com", recognizers.EntityEmail},
		{"y@b.com", recognizers.EntityEmail},
		{"4111111111111111", recognizers.EntityCreditCard},
	} {
		start := strings.Index(text, hit.value)
		entities = append(entities, recognizers.Entity{Type: hit.typ, Start: start, End: start + len(hit.value), Text: hit.value, Confidence: 0.99})
		text = text[:start] + strings.Repeat(" ", len(hit.value)) + text[start+len(hit.value):]
	}
	original := "x@a.com 4111111111111111 x@a.com ünïcode y@b.com 4111111111111111"
	out, pseudonyms, err := New().Pseudonymize(original, entities)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := restore(out, pseudonyms); got != original {
		t.Errorf("restore(out) = %q, want %q", got, original)
	}
}
