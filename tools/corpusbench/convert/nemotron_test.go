package convert

import (
	"fmt"
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// Nemotron's spans are Python reprs, so these fixtures use single quotes. The
// label-level behaviour is pinned in labels_test.go; these tests drive the
// whole Nemotron conversion so the wiring between the label map, the offsets
// and the schema is covered end to end.

// Every nemotron label the map carries must reach its entity type through the
// full conversion. A typo in the map would otherwise drop the label silently
// into the unlabelled bucket.
func TestNemotronMapsEachLabelToItsType(t *testing.T) {
	cases := []struct {
		label string
		want  recognizers.EntityType
	}{
		{"email", recognizers.EntityEmail},
		{"phone_number", recognizers.EntityPhone},
		{"fax_number", recognizers.EntityPhone},
		{"credit_debit_card", recognizers.EntityCreditCard},
		{"ssn", recognizers.EntitySSN},
		{"ipv4", recognizers.EntityIP},
		{"ipv6", recognizers.EntityIP},
		{"url", recognizers.EntityURL},
		{"api_key", recognizers.EntitySecret},
		{"http_cookie", recognizers.EntitySecret},
		{"first_name", recognizers.EntityPerson},
		{"last_name", recognizers.EntityPerson},
		{"user_name", recognizers.EntityPerson},
		{"company_name", recognizers.EntityOrganization},
		{"city", recognizers.EntityLocation},
		{"country", recognizers.EntityLocation},
		{"state", recognizers.EntityLocation},
		{"county", recognizers.EntityLocation},
		{"postcode", recognizers.EntityLocation},
		{"street_address", recognizers.EntityLocation},
		{"swift_bic", recognizers.EntitySWIFTBIC},
		{"bank_routing_number", recognizers.EntityUSRouting},
		{"account_number", recognizers.EntityBankAccount},
		{"cvv", recognizers.EntityCardCVV},
		{"pin", recognizers.EntityPIN},
		{"tax_id", recognizers.EntityTaxID},
		{"date_of_birth", recognizers.EntityDateOfBirth},
	}

	const prefix = "contact "
	const value = "fixture-value"
	var rows []nemotronRow
	for _, c := range cases {
		end := len(prefix) + len(value)
		rows = append(rows, nemotronRow{
			Text: prefix + value + " now",
			Spans: fmt.Sprintf("[{'start': %d, 'end': %d, 'text': '%s', 'label': '%s'}]",
				len(prefix), end, value, c.label),
		})
	}

	root := t.TempDir()
	writeNemotronFixture(t, root, rows)

	got, schema, err := Nemotron(root, 0)
	if err != nil {
		t.Fatalf("Nemotron: unexpected error: %v", err)
	}
	if len(got) != len(cases) {
		t.Fatalf("got %d cases, want %d", len(got), len(cases))
	}
	for i, c := range cases {
		if len(got[i].Entities) != 1 {
			t.Errorf("label %q: got %d entities, want 1", c.label, len(got[i].Entities))
			continue
		}
		if got[i].Entities[0].Type != c.want {
			t.Errorf("label %q: type = %q, want %q", c.label, got[i].Entities[0].Type, c.want)
		}
	}

	// The schema declares the scope: one entry per distinct entity type in the
	// map, so the scorer knows which detections are in scope.
	wantInSchema := map[recognizers.EntityType]bool{}
	for _, c := range cases {
		wantInSchema[c.want] = true
	}
	if len(schema.InSchema) != len(wantInSchema) {
		t.Errorf("InSchema = %v, want the %d distinct types of the map", schema.InSchema, len(wantInSchema))
	}
	for _, typ := range schema.InSchema {
		if !wantInSchema[typ] {
			t.Errorf("InSchema contains %q, which no mapped label produces", typ)
		}
	}
}

// Spans are character indices in nemotron, and gold must be byte offsets. Each
// label's byte span has to cover exactly its value, including when multi-byte
// text sits before it, or the labels would be mis-sized and score as misses.
func TestNemotronTranslatesOffsetsForNonASCIIText(t *testing.T) {
	const text = "Zoë 555-123-4567 日本 123-45-6789"
	// Character indices: "Zoë " is 4 runes, the phone is 12 runes, then " 日本 "
	// is 4 runes, so the SSN starts at rune 20.
	root := t.TempDir()
	writeNemotronFixture(t, root, []nemotronRow{{
		Text: text,
		Spans: `[{'start': 4, 'end': 16, 'text': '555-123-4567', 'label': 'phone_number'}, ` +
			`{'start': 20, 'end': 31, 'text': '123-45-6789', 'label': 'ssn'}]`,
	}})

	cases, _, err := Nemotron(root, 0)
	if err != nil {
		t.Fatalf("Nemotron: unexpected error: %v", err)
	}
	if len(cases) != 1 || len(cases[0].Entities) != 2 {
		t.Fatalf("got %d cases with labels %+v, want 1 case with 2 labels", len(cases), cases)
	}

	// strings.Index returns byte offsets, so it is an independent oracle for
	// where each value sits in the text.
	wantPhoneStart := strings.Index(text, "555-123-4567")
	wantSSNStart := strings.Index(text, "123-45-6789")
	for i, want := range []struct {
		value string
		start int
	}{
		{"555-123-4567", wantPhoneStart},
		{"123-45-6789", wantSSNStart},
	} {
		l := cases[0].Entities[i]
		if l.Start == nil || l.End == nil {
			t.Fatalf("label %d has no byte offsets: %+v", i, l)
		}
		if *l.Start != want.start || *l.End != want.start+len(want.value) {
			t.Errorf("label %d offsets = %d..%d, want %d..%d",
				i, *l.Start, *l.End, want.start, want.start+len(want.value))
		}
		if text[*l.Start:*l.End] != want.value {
			t.Errorf("label %d covers %q, want %q", i, text[*l.Start:*l.End], want.value)
		}
	}
}

// A label the map does not carry is out of scope. It must be recorded in the
// schema and in the case's unlabelled spans, and must never become gold.
//
// "age" is the unmapped example, not "first_name": the person labels are
// mapped as of 2026-10-20, when the NLP sidecar made them detectable, so
// "first_name" no longer demonstrates an unmapped label.
func TestNemotronUnmappedLabelIsUnlabelledNotGold(t *testing.T) {
	const text = "Ana 555-123-4567"
	root := t.TempDir()
	writeNemotronFixture(t, root, []nemotronRow{{
		Text: text,
		Spans: `[{'start': 0, 'end': 3, 'text': 'Ana', 'label': 'age'}, ` +
			`{'start': 4, 'end': 16, 'text': '555-123-4567', 'label': 'phone_number'}]`,
	}})

	cases, schema, err := Nemotron(root, 0)
	if err != nil {
		t.Fatalf("Nemotron: unexpected error: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("got %d cases, want 1", len(cases))
	}
	if len(cases[0].Entities) != 1 || cases[0].Entities[0].Type != recognizers.EntityPhone {
		t.Errorf("gold = %+v, want only the PHONE label", cases[0].Entities)
	}
	if len(cases[0].Unlabelled) != 1 {
		t.Fatalf("unlabelled = %+v, want one first_name span", cases[0].Unlabelled)
	}
	if u := cases[0].Unlabelled[0]; u.Label != "age" || u.Value != "Ana" {
		t.Errorf("unlabelled span = %+v, want age covering \"Ana\"", u)
	}
	if len(schema.UnmappedLabels) != 1 || schema.UnmappedLabels[0] != "age" {
		t.Errorf("UnmappedLabels = %v, want [age]", schema.UnmappedLabels)
	}
}

// A span that points past the end of its record cannot be located, so the
// record is an error, naming the corpus, the file and the row. Dropping the
// record would silently remove its gold labels.
func TestNemotronFailsFastOnOutOfRangeSpan(t *testing.T) {
	root := t.TempDir()
	writeNemotronFixture(t, root, []nemotronRow{
		{Text: "fine 555-123-4567", Spans: `[{'start': 5, 'end': 17, 'text': '555-123-4567', 'label': 'phone_number'}]`},
		{Text: "short", Spans: `[{'start': 0, 'end': 99, 'text': 'short', 'label': 'email'}]`},
	})

	_, _, err := Nemotron(root, 0)
	if err == nil {
		t.Fatal("Nemotron: want an error for an out-of-range span, got nil")
	}
	for _, want := range []string{"nemotron-pii", "row 1", "span 0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should contain %q", err.Error(), want)
		}
	}
}
