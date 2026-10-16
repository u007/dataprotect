package structured

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/u007/dataprotect/analyzer"
	"github.com/u007/dataprotect/anonymizer"
	"github.com/u007/dataprotect/operators"
	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/regex"
)

func newProcessor() *BatchProcessor {
	a := analyzer.New()
	a.AddRecognizer(regex.NewEmailRecognizer())
	a.AddRecognizer(regex.NewSSNRecognizer())

	an := anonymizer.New()
	an.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))
	an.SetOperator(recognizers.EntitySSN, operators.NewMaskOperator("***-**-####"))

	return NewBatch(a, an)
}

func TestProcessCSV(t *testing.T) {
	bp := newProcessor()
	bp.SetColumnType("email", recognizers.EntityEmail)
	bp.SetColumnType("ssn", recognizers.EntitySSN)

	in := strings.NewReader("name,email,ssn\nJohn,a@b.com,123456789\nJane,x@y.org,987654321\n")
	var out bytes.Buffer

	ragged, err := bp.ProcessCSV(context.Background(), in, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ragged != 0 {
		t.Errorf("got %d ragged rows, want 0 for well-formed input", ragged)
	}

	got := out.String()
	want := "name,email,ssn\nJohn,<EMAIL>,***-**-6789\nJane,<EMAIL>,***-**-4321\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

// Only configured columns are scrubbed; an unlisted column must survive intact
// even when it holds something the analyzer would otherwise match.
func TestProcessCSVLeavesUnconfiguredColumns(t *testing.T) {
	bp := newProcessor()
	bp.SetColumnType("email", recognizers.EntityEmail)

	in := strings.NewReader("notes,email\ncontact a@b.com,other@x.com\n")
	var out bytes.Buffer

	ragged, err := bp.ProcessCSV(context.Background(), in, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ragged != 0 {
		t.Errorf("got %d ragged rows, want 0 for well-formed input", ragged)
	}

	got := out.String()
	if !strings.Contains(got, "contact a@b.com") {
		t.Errorf("unconfigured column was scrubbed: %q", got)
	}
	if !strings.Contains(got, "<EMAIL>") {
		t.Errorf("configured column was not scrubbed: %q", got)
	}
}

func TestProcessCSVHeaderOnly(t *testing.T) {
	bp := newProcessor()
	bp.SetColumnType("email", recognizers.EntityEmail)

	in := strings.NewReader("email\na@b.com\n")
	var out bytes.Buffer

	ragged, err := bp.ProcessCSV(context.Background(), in, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ragged != 0 {
		t.Errorf("got %d ragged rows, want 0 for well-formed input", ragged)
	}
	if got := out.String(); got != "email\n<EMAIL>\n" {
		t.Errorf("got %q, want %q", got, "email\n<EMAIL>\n")
	}
}

// A short row must not panic on the missing column. It is padded to the
// header width so the output keeps a consistent field count, and reported via
// the ragged count rather than silently accepted.
func TestProcessCSVShortRow(t *testing.T) {
	bp := newProcessor()
	bp.SetColumnType("email", recognizers.EntityEmail)
	bp.SetColumnType("ssn", recognizers.EntitySSN)

	in := strings.NewReader("email,ssn\na@b.com\n")
	var out bytes.Buffer

	ragged, err := bp.ProcessCSV(context.Background(), in, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ragged != 1 {
		t.Errorf("got %d ragged rows, want 1: the short row must be reported, not silently accepted", ragged)
	}
	// Padded to the header width: the missing trailing cell becomes empty.
	if got := out.String(); got != "email,ssn\n<EMAIL>,\n" {
		t.Errorf("got %q, want %q", got, "email,ssn\n<EMAIL>,\n")
	}
}

func TestProcessCSVQuotedFields(t *testing.T) {
	bp := newProcessor()
	bp.SetColumnType("email", recognizers.EntityEmail)

	in := strings.NewReader("email\n\"a@b.com, please reply\"\n")
	var out bytes.Buffer

	ragged, err := bp.ProcessCSV(context.Background(), in, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ragged != 0 {
		t.Errorf("got %d ragged rows, want 0 for well-formed input", ragged)
	}
	if !strings.Contains(out.String(), "<EMAIL>") {
		t.Errorf("quoted field not scrubbed: %q", out.String())
	}
}

// One mapping spans the file: a repeated email gets one placeholder, and the
// returned map restores it.
func TestPseudonymizeCSVSharesPlaceholderAcrossRows(t *testing.T) {
	bp := newProcessor()
	bp.SetColumnType("email", recognizers.EntityEmail)

	in := strings.NewReader("name,email\nJohn,a@b.com\nJane,a@b.com\nKim,x@y.org\n")
	var out bytes.Buffer

	ragged, pseudonyms, err := bp.PseudonymizeCSV(context.Background(), in, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ragged != 0 {
		t.Errorf("got %d ragged rows, want 0 for well-formed input", ragged)
	}
	want := "name,email\nJohn,[EMAIL_1]\nJane,[EMAIL_1]\nKim,[EMAIL_2]\n"
	if got := out.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if pseudonyms["[EMAIL_1]"] != "a@b.com" || pseudonyms["[EMAIL_2]"] != "x@y.org" {
		t.Errorf("mapping %v does not restore the file's emails", pseudonyms)
	}
}

// Only configured columns are pseudonymized, as in ProcessCSV.
func TestPseudonymizeCSVLeavesUnconfiguredColumns(t *testing.T) {
	bp := newProcessor()
	bp.SetColumnType("email", recognizers.EntityEmail)

	in := strings.NewReader("name,email\na@b.com,x@y.org\n")
	var out bytes.Buffer

	if _, _, err := bp.PseudonymizeCSV(context.Background(), in, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := out.String(), "name,email\na@b.com,[EMAIL_1]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A literal placeholder in an unscanned column would be rewritten when the file
// is restored, so the whole file is rejected and nothing is written.
func TestPseudonymizeCSVRejectsLiteralPlaceholderInUnscannedColumn(t *testing.T) {
	bp := newProcessor()
	bp.SetColumnType("email", recognizers.EntityEmail)

	in := strings.NewReader("name,email,note\nJohn,a@b.com,see [EMAIL_1]\n")
	var out bytes.Buffer

	if _, _, err := bp.PseudonymizeCSV(context.Background(), in, &out); err == nil {
		t.Fatal("want an error for a literal placeholder in an unscanned column")
	}
	if out.Len() != 0 {
		t.Errorf("wrote %q before the check failed; want no output", out.String())
	}
}

// A short row is padded to the header width and reported through the count.
func TestPseudonymizeCSVShortRow(t *testing.T) {
	bp := newProcessor()
	bp.SetColumnType("email", recognizers.EntityEmail)

	in := strings.NewReader("name,email\nJohn\nJane,b@c.com\n")
	var out bytes.Buffer

	ragged, _, err := bp.PseudonymizeCSV(context.Background(), in, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ragged != 1 {
		t.Errorf("got %d ragged rows, want 1", ragged)
	}
	if got, want := out.String(), "name,email\nJohn,\nJane,[EMAIL_1]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
