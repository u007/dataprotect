package piiscrub

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/terms"
)

func redactText(t *testing.T, s *Scrubber, text string) string {
	t.Helper()
	out, err := s.Redact(context.Background(), text)
	if err != nil {
		t.Fatalf("Redact(%q): %v", text, err)
	}
	return out
}

// MONEY is on by default, and a multi-byte symbol must be replaced exactly.
func TestMoneyRedactsExactSpanWithMultibyteSymbols(t *testing.T) {
	got := redactText(t, NewScrubber(), "Pay €12.50 and £7 now, then ¥300.")
	want := "Pay [[redactedmoney1]] and [[redactedmoney2]] now, then [[redactedmoney3]]."
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestMoneyDoesNotTouchBareNumbers(t *testing.T) {
	text := "Invoice 4410 has 12 lines and total 1,250 units."
	if got := redactText(t, NewScrubber(), text); got != text {
		t.Errorf("bare numbers changed: %q", got)
	}
}

// WithoutEntityTypes(EntityMoney) turns the recognizer off: the amount survives.
func TestWithoutEntityTypesMoneyLeavesAmountsUntouched(t *testing.T) {
	text := "Fee of $1,250.00 and RM 900 due."
	off := NewScrubber(WithoutEntityTypes(recognizers.EntityMoney))
	if got := redactText(t, off, text); got != text {
		t.Errorf("with MONEY off, amounts changed: %q", got)
	}
	if got := redactText(t, NewScrubber(), text); got == text {
		t.Errorf("with MONEY on, the amounts were not redacted")
	}
}

// Case numbers are on by default, and a suit number and its surrounding party
// name are handled together.
func TestCaseNumberOnByDefault(t *testing.T) {
	got := redactText(t, NewScrubber(), "Suit No: 77/2020 filed by the plaintiff.")
	if want := "Suit No: [[redactedcasenumber1]] filed by the plaintiff."; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := redactText(t, NewScrubber(WithoutEntityTypes(recognizers.EntityCaseNumber)), "Case 23-CV-0001-JFH"); got != "Case 23-CV-0001-JFH" {
		t.Errorf("CASE_NUMBER off: got %q", got)
	}
}

func TestNeutralCitationsSurviveRedaction(t *testing.T) {
	text := "See [2021] SGHC 123 and [2020] 1 MLJ 123 and [2021] SGCA 5."
	if got := redactText(t, NewScrubber(), text); got != text {
		t.Errorf("neutral citation redacted: %q", got)
	}
}

// A caller's term is redacted with its own marker, and spellings that differ in
// case or whitespace share one marker.
func TestCustomTermRedactsWithSharedMarker(t *testing.T) {
	r, err := terms.New(recognizers.EntityCustomTerm, "Project Falcon", "Tan Ah Kow")
	if err != nil {
		t.Fatal(err)
	}
	s := NewScrubber(WithRecognizer(r))
	text := "Project Falcon, then project   falcon again; Tan Ah\nKow signed."
	got := redactText(t, s, text)
	want := "[[redactedcustomterm1]], then [[redactedcustomterm1]] again; [[redactedcustomterm2]] signed."
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

// A CUSTOM_TERM wins an overlap with a card, so a term that contains digits is
// redacted whole and no part of it leaks.
func TestCustomTermWinsOverCardOnOverlap(t *testing.T) {
	r, err := terms.New(recognizers.EntityCustomTerm, "Ref 4111 1111 1111 1111")
	if err != nil {
		t.Fatal(err)
	}
	s := NewScrubber(WithRecognizer(r))
	got := redactText(t, s, "Matter Ref 4111 1111 1111 1111 open.")
	if want := "Matter [[redactedcustomterm1]] open."; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCustomTermEmptyRejectedAtConstruction(t *testing.T) {
	if _, err := terms.New(recognizers.EntityCustomTerm, "  "); err == nil {
		t.Fatal("blank term accepted")
	}
}
