package convert

import (
	"strings"
	"testing"
)

// gretel's pii_spans column IS valid JSON, unlike nemotron's Python repr, so
// this adapter can use the standard decoder. The split between two parsers is
// deliberate: forcing one parser over both corpora would mean writing a
// lenient JSON reader, and lenient readers are how malformed data gets in.

func TestGretelSpansJSON(t *testing.T) {
	in := `[{"start": 131, "end": 139, "label": "employee_id"}, {"start": 218, "end": 226, "label": "employee_id"}]`
	spans, err := GretelSpans(in)
	if err != nil {
		t.Fatalf("GretelSpans: unexpected error: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("got %d spans, want 2", len(spans))
	}
	if spans[0] != (Span{Start: 131, End: 139, Label: "employee_id"}) {
		t.Errorf("span[0] = %+v, want {131 139 employee_id}", spans[0])
	}
	if spans[1].Start != 218 || spans[1].End != 226 {
		t.Errorf("span[1] = %d..%d, want 218..226", spans[1].Start, spans[1].End)
	}
}

// A record with no annotations is a negative case, not a failure.
func TestGretelSpansEmptyList(t *testing.T) {
	spans, err := GretelSpans(`[]`)
	if err != nil {
		t.Fatalf("GretelSpans on empty list: unexpected error: %v", err)
	}
	if len(spans) != 0 {
		t.Errorf("got %d spans, want 0", len(spans))
	}
}

// The gold-label order downstream depends on the adapter preserving file order.
func TestGretelSpansPreservesOrder(t *testing.T) {
	in := `[{"start": 5, "end": 17, "label": "phone_number"}, {"start": 21, "end": 28, "label": "email"}]`
	spans, err := GretelSpans(in)
	if err != nil {
		t.Fatalf("GretelSpans: unexpected error: %v", err)
	}
	if len(spans) != 2 || spans[0].Label != "phone_number" || spans[1].Label != "email" {
		t.Fatalf("spans = %+v, want phone_number then email", spans)
	}
}

// Rejecting malformed input is the whole point: a silently truncated span list
// would drop gold labels, and every dropped label is a false positive that
// never gets counted.
func TestGretelSpansRejectsMalformed(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
	}{
		{"truncated", `[{"start": 131, `},
		{"not a list", `{"start": 131, "end": 139, "label": "x"}`},
		{"missing label", `[{"start": 1, "end": 2}]`},
		{"string index", `[{"start": "one", "end": 2, "label": "x"}]`},
		{"trailing junk", `[] garbage`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := GretelSpans(tc.in); err == nil {
				t.Fatalf("GretelSpans(%q): want an error, got nil", tc.in)
			} else if !strings.Contains(err.Error(), "spans") {
				t.Errorf("error %q should identify the spans field", err.Error())
			}
		})
	}
}

func TestGretelLangRejectsUnknownLanguage(t *testing.T) {
	if _, _, err := GretelLang("", 0, "Klingon"); err == nil {
		t.Fatal("expected an error for an unknown gretel language")
	}
}
