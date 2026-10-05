package structured

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"dataprotect/analyzer"
	"dataprotect/anonymizer"
	"dataprotect/operators"
	"dataprotect/recognizers"
	"dataprotect/recognizers/regex"
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
