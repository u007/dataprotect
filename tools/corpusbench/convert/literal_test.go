package convert

import (
	"strings"
	"testing"
)

// These fixtures are Python's own repr() output for the shapes nemotron's
// `spans` column actually contains, not hand-written approximations. The
// column is a Python repr, not JSON — a json.Unmarshal over it fails outright —
// so testing against the real serializer is what makes the parser trustworthy.

func TestNemotronSpansQuotedText(t *testing.T) {
	// repr(): apostrophe inside a DOUBLE-quoted value, no escaping needed.
	in := `[{'start': 184, 'end': 201, 'text': "bachelor's degree", 'label': 'education_level'}]`
	spans, err := NemotronSpans(in)
	if err != nil {
		t.Fatalf("NemotronSpans: unexpected error: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	want := Span{Start: 184, End: 201, Label: "education_level"}
	if spans[0] != want {
		t.Errorf("span = %+v, want %+v", spans[0], want)
	}
}

// repr() emits an int unquoted for a numeric field, so a value may be either a
// quoted string or a bare integer. Rejecting the int would reject real rows.
func TestNemotronSpansNonStringText(t *testing.T) {
	in := `[{'start': 123, 'end': 125, 'text': 57, 'label': 'age'}]`
	spans, err := NemotronSpans(in)
	if err != nil {
		t.Fatalf("NemotronSpans: unexpected error: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	if spans[0].Label != "age" || spans[0].Start != 123 || spans[0].End != 125 {
		t.Errorf("span = %+v, want {123 125 age}", spans[0])
	}
}

// repr() chooses the quote style per value and escapes the other quote type
// with a backslash: a value holding both quote kinds comes out single-quoted
// with \' inside it. A parser that only strips matching quotes would keep the
// backslash in the text.
func TestNemotronSpansEscapedQuotes(t *testing.T) {
	in := `[{'start': 0, 'end': 5, 'text': 'it\'s "quoted" here', 'label': 'first_name'}]`
	spans, err := NemotronSpans(in)
	if err != nil {
		t.Fatalf("NemotronSpans: unexpected error: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	if spans[0].Label != "first_name" {
		t.Errorf("label = %q, want first_name", spans[0].Label)
	}
	if spans[0].Start != 0 || spans[0].End != 5 {
		t.Errorf("span = %d..%d, want 0..5", spans[0].Start, spans[0].End)
	}
}

// Control characters arrive escaped by repr(). They are inside the `text`
// value, which the converter discards in favour of the offsets, but the parser
// must still get past them to read `label`.
func TestNemotronSpansEscapedControlChars(t *testing.T) {
	for _, tc := range []struct {
		name  string
		in    string
		label string
		start int
		end   int
	}{
		{"newline", `[{'start': 7, 'end': 12, 'text': 'line\nbreak', 'label': 'note'}]`, "note", 7, 12},
		{"tab", `[{'start': 3, 'end': 8, 'text': 'tab\there', 'label': 'x'}]`, "x", 3, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans, err := NemotronSpans(tc.in)
			if err != nil {
				t.Fatalf("NemotronSpans: unexpected error: %v", err)
			}
			if len(spans) != 1 {
				t.Fatalf("got %d spans, want 1", len(spans))
			}
			if spans[0].Label != tc.label {
				t.Errorf("label = %q, want %q", spans[0].Label, tc.label)
			}
			// Offsets are character indices into the RECORD's text column, so
			// decoding escapes inside this span's own `text` value must not
			// shift them: the raw repr is longer than the decoded string, and
			// reconciling the two would corrupt every offset on a record that
			// contains a newline.
			if spans[0].Start != tc.start || spans[0].End != tc.end {
				t.Errorf("span = %d..%d, want %d..%d (offsets are record-relative)",
					spans[0].Start, spans[0].End, tc.start, tc.end)
			}
		})
	}
}

// A record with no annotations is a legitimate negative case, not a failure.
func TestNemotronSpansEmptyList(t *testing.T) {
	spans, err := NemotronSpans(`[]`)
	if err != nil {
		t.Fatalf("NemotronSpans on empty list: unexpected error: %v", err)
	}
	if len(spans) != 0 {
		t.Errorf("got %d spans, want 0", len(spans))
	}
}

// Several spans in one record must come back in file order, because gold
// labels are expected in text order downstream.
func TestNemotronSpansMultipleInOrder(t *testing.T) {
	in := `[{'start': 5, 'end': 17, 'text': '555-123-4567', 'label': 'phone_number'}, ` +
		`{'start': 21, 'end': 28, 'text': 'a@b.com', 'label': 'email'}]`
	spans, err := NemotronSpans(in)
	if err != nil {
		t.Fatalf("NemotronSpans: unexpected error: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("got %d spans, want 2", len(spans))
	}
	if spans[0].Label != "phone_number" || spans[1].Label != "email" {
		t.Errorf("labels = %q, %q; want phone_number, email", spans[0].Label, spans[1].Label)
	}
	if spans[0].Start != 5 || spans[1].Start != 21 {
		t.Errorf("starts = %d, %d; want 5, 21", spans[0].Start, spans[1].Start)
	}
}

// A truncated or hand-mangled literal must fail loudly. Silently returning a
// partial span list would drop gold labels and inflate precision.
func TestNemotronSpansRejectsMalformed(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
	}{
		{"truncated", `[{'start': 184, `},
		{"not a list", `{'start': 1, 'end': 2, 'label': 'x'}`},
		{"missing label", `[{'start': 1, 'end': 2}]`},
		{"string index", `[{'start': 'one', 'end': 2, 'label': 'x'}]`},
		{"trailing junk", `[] garbage`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NemotronSpans(tc.in); err == nil {
				t.Fatalf("NemotronSpans(%q): want an error, got nil", tc.in)
			} else if !strings.Contains(err.Error(), "spans") {
				t.Errorf("error %q should identify the spans field", err.Error())
			}
		})
	}
}

// repr() leaves printable non-ASCII characters raw — measured over 7179
// printable code points in 0x80..0x2000, zero were escaped. The parser must
// therefore pass UTF-8 through untouched rather than treating a high byte as an
// escape introducer.
func TestNemotronSpansKeepsNonASCIIRaw(t *testing.T) {
	for _, tc := range []struct {
		name  string
		in    string
		text  string
		start int
	}{
		{"accented", `[{'start': 0, 'end': 4, 'text': 'café', 'label': 'first_name'}]`, "café", 0},
		{"cjk", `[{'start': 0, 'end': 3, 'text': '日本語', 'label': 'last_name'}]`, "日本語", 0},
		{"emoji", `[{'start': 0, 'end': 2, 'text': '😀!', 'label': 'note'}]`, "😀!", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans, err := NemotronSpans(tc.in)
			if err != nil {
				t.Fatalf("NemotronSpans: unexpected error: %v", err)
			}
			if len(spans) != 1 {
				t.Fatalf("got %d spans, want 1", len(spans))
			}
			if spans[0].Label == "" {
				t.Error("label was dropped or emptied by a raw non-ASCII byte")
			}
			if spans[0].Start != tc.start {
				t.Errorf("start = %d, want %d", spans[0].Start, tc.start)
			}
		})
	}
}

// repr() escapes non-printable characters as \\, \xNN and \uNNNN (and \UNNNNNNNN
// for astral code points). Those can appear inside a value the parser has to
// skip past to reach `label`.
func TestNemotronSpansDecodesEscapes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		in    string
		label string
	}{
		{"backslash", `[{'start': 0, 'end': 3, 'text': 'back\\slash', 'label': 'note'}]`, "note"},
		{"hex byte", `[{'start': 0, 'end': 3, 'text': 'a\x00b', 'label': 'note'}]`, "note"},
		{"unicode bmp", `[{'start': 0, 'end': 3, 'text': 'a\u200bb', 'label': 'note'}]`, "note"},
		{"astral unicode", `[{'start': 0, 'end': 3, 'text': 'a\U0001ffffb', 'label': 'note'}]`, "note"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans, err := NemotronSpans(tc.in)
			if err != nil {
				t.Fatalf("NemotronSpans: unexpected error: %v", err)
			}
			if len(spans) != 1 {
				t.Fatalf("got %d spans, want 1", len(spans))
			}
			if spans[0].Label != tc.label {
				t.Errorf("label = %q, want %q", spans[0].Label, tc.label)
			}
		})
	}
}

// An unterminated string must not silently end the parse at the next quote or
// at end of input: that would truncate the record and drop labels.
func TestNemotronSpansRejectsUnterminatedString(t *testing.T) {
	in := `[{'start': 0, 'end': 3, 'text': 'unterminated, 'label': 'note'}]`
	if _, err := NemotronSpans(in); err == nil {
		t.Fatal("NemotronSpans: want an error for an unterminated string, got nil")
	}
}

// Guessing a meaning for an unknown escape is what the no-fallbacks rule
// forbids: repr never emits one, so its presence means this is not the data
// the parser claims to understand.
func TestNemotronSpansRejectsUnknownEscape(t *testing.T) {
	for _, in := range []string{
		"[{'start': 0, 'end': 3, 'text': 'bad\\qescape', 'label': 'note'}]",
		"[{'start': 0, 'end': 3, 'text': 'short\\x0', 'label': 'note'}]",
		"[{'start': 0, 'end': 3, 'text': 'short\\u12', 'label': 'note'}]",
	} {
		if _, err := NemotronSpans(in); err == nil {
			t.Errorf("NemotronSpans(%q): want an error for a malformed escape, got nil", in)
		}
	}
}

// repr emits None/True/False bare if a field ever holds one. Those are not part
// of this corpus's shape, so they must be rejected rather than guessed at.
func TestNemotronSpansRejectsUnsupportedLiteral(t *testing.T) {
	for _, in := range []string{
		`[{'start': 0, 'end': 3, 'text': None, 'label': 'note'}]`,
		`[{'start': 0, 'end': 3, 'text': True, 'label': 'note'}]`,
	} {
		if _, err := NemotronSpans(in); err == nil {
			t.Errorf("NemotronSpans(%q): want an error for an unsupported literal, got nil", in)
		}
	}
}
