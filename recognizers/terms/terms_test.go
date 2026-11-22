package terms

import (
	"context"
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

func run(t *testing.T, r *Recognizer, text string) []recognizers.Entity {
	t.Helper()
	ents, err := r.Recognize(context.Background(), text)
	if err != nil {
		t.Fatalf("Recognize(%q): %v", text, err)
	}
	for _, e := range ents {
		if text[e.Start:e.End] != e.Text {
			t.Errorf("span [%d:%d] is %q but Text is %q", e.Start, e.End, text[e.Start:e.End], e.Text)
		}
		if e.Confidence != Score {
			t.Errorf("confidence %v, want %v", e.Confidence, Score)
		}
	}
	return ents
}

func mustNew(t *testing.T, typ recognizers.EntityType, terms ...string) *Recognizer {
	t.Helper()
	r, err := New(typ, terms...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func texts(ents []recognizers.Entity) []string {
	out := make([]string, len(ents))
	for i, e := range ents {
		out[i] = e.Text
	}
	return out
}

func TestNewRejectsEmptyTerms(t *testing.T) {
	for _, bad := range []string{"", "   ", "\t\n"} {
		if _, err := New(recognizers.EntityCustomTerm, "Acme", bad); err == nil {
			t.Errorf("New with term %q: want error", bad)
		}
	}
	if _, err := New(""); err == nil {
		t.Error("New with empty entity type: want error")
	}
}

func TestCaseInsensitiveAndOriginalText(t *testing.T) {
	r := mustNew(t, recognizers.EntityCustomTerm, "Acme Holdings")
	text := "Invoice to ACME holdings and acme HOLDINGS."
	ents := run(t, r, text)
	got := texts(ents)
	want := []string{"ACME holdings", "acme HOLDINGS"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", got, want)
	}
	if ents[0].Type != recognizers.EntityCustomTerm {
		t.Errorf("type %s", ents[0].Type)
	}
}

func TestWholeTermOnly(t *testing.T) {
	r := mustNew(t, recognizers.EntityCustomTerm, "Ann", "WA-22")
	for _, text := range []string{"Anna wrote", "Joann", "Ann1", "XWA-22", "WA-221"} {
		// "WA-22" inside "WA-221" is glued to a digit; "WA-22" inside "XWA-22"
		// is glued to a letter.
		if ents := run(t, r, text); len(ents) != 0 {
			t.Errorf("%q: want no match, got %q", text, texts(ents))
		}
	}
	if got := texts(run(t, r, "Ann, and (Ann).")); len(got) != 2 {
		t.Errorf("punctuation boundary: got %q", got)
	}
}

func TestWhitespaceRunMatchesAcrossLineBreak(t *testing.T) {
	r := mustNew(t, recognizers.EntityPerson, "Tan Ah Kow")
	text := "Client: Tan\n  Ah\tKow signed."
	got := texts(run(t, r, text))
	if len(got) != 1 || got[0] != "Tan\n  Ah\tKow" {
		t.Fatalf("got %q", got)
	}
}

func TestLongestTermWins(t *testing.T) {
	r := mustNew(t, recognizers.EntityCustomTerm, "Tan", "Tan Ah Kow")
	got := texts(run(t, r, "Tan Ah Kow and Tan alone"))
	if strings.Join(got, "|") != "Tan Ah Kow|Tan" {
		t.Fatalf("got %q", got)
	}
}

func TestSelfOverlapNotMissed(t *testing.T) {
	r := mustNew(t, recognizers.EntityCustomTerm, "aba")
	// "aba" at 0 is glued to a following "b"? No: "ababa" has "aba" at 0 and 2.
	// Neither is whole in "ababa", so nothing is reported; "aba aba" has two.
	if got := run(t, r, "ababa"); len(got) != 0 {
		t.Errorf("ababa: got %q", texts(got))
	}
	if got := texts(run(t, r, "x aba aba")); len(got) != 2 {
		t.Errorf("two whole occurrences: got %q", got)
	}
}

func TestUnicodeBoundaryAndOffsets(t *testing.T) {
	r := mustNew(t, recognizers.EntityCustomTerm, "Müller")
	text := "Herr Müller, nicht Müllers."
	ents := run(t, r, text)
	if len(ents) != 1 {
		t.Fatalf("got %q, want one whole match", texts(ents))
	}
	if text[ents[0].Start:ents[0].End] != "Müller" {
		t.Errorf("span %q", text[ents[0].Start:ents[0].End])
	}
}

func TestDuplicatesAndNoTerms(t *testing.T) {
	r := mustNew(t, recognizers.EntityCustomTerm, "Acme", "acme", "Acme")
	if got := texts(run(t, r, "Acme")); len(got) != 1 {
		t.Errorf("duplicate terms reported twice: %q", got)
	}
	empty := mustNew(t, recognizers.EntityCustomTerm)
	if got := run(t, empty, "anything"); len(got) != 0 {
		t.Errorf("no terms reported %q", texts(got))
	}
}

func TestRegexMetacharactersAreLiteral(t *testing.T) {
	r := mustNew(t, recognizers.EntityCustomTerm, "Deal (A+B).*")
	if got := texts(run(t, r, "see Deal (A+B).* now and Dealxxx")); len(got) != 1 {
		t.Errorf("got %q", got)
	}
}

// A long run of a term's own characters must not make matching quadratic: every
// occurrence is glued to its neighbours, so nothing is reported, and the call
// returns after one pass over the restarts.
func TestPathologicalInputStaysLinear(t *testing.T) {
	r := mustNew(t, recognizers.EntityCustomTerm, "aa")
	text := strings.Repeat("a", 200000)
	if got := run(t, r, text); len(got) != 0 {
		t.Errorf("glued run reported %d entities", len(got))
	}
	text2 := strings.Repeat("aa ", 50000)
	if got := run(t, r, text2); len(got) != 50000 {
		t.Errorf("got %d whole occurrences, want 50000", len(got))
	}
}
