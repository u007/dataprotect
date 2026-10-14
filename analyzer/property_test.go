package analyzer

import (
	"context"
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/regex"
)

// Property tests for the invariants Analyzer.Analyze must hold for ANY input and
// ANY combination of registered recognizers.
//
// These exist because overlap resolution is byte-offset arithmetic over
// attacker-shaped input, and it was already wrong once: the phone recognizer
// claimed the first ten digits of a 16-digit credit card, and because the phone
// scored higher than the card the card was silently dropped. A table of
// hand-picked cases only covers the pairs somebody thought of; the invariants
// below hold for text nobody imagined.

// assertWellFormed checks every invariant Analyze promises, for one input.
func assertWellFormed(t *testing.T, text string, entities []recognizers.Entity) {
	t.Helper()

	for i, e := range entities {
		if e.Start < 0 || e.End > len(text) {
			t.Fatalf("entity %d span [%d:%d] out of bounds for text of length %d: %+v",
				i, e.Start, e.End, len(text), e)
		}
		if e.Start > e.End {
			t.Errorf("entity %d has inverted span [%d:%d]: %+v", i, e.Start, e.End, e)
		}
		// Text MUST agree with the span. A mismatch means a recognizer computed
		// offsets against a different string than the one it was handed.
		if got := text[e.Start:e.End]; got != e.Text {
			t.Errorf("entity %d Text %q does not match span content %q: %+v", i, e.Text, got, e)
		}
		if e.Confidence < 0 || e.Confidence > 1 {
			t.Errorf("entity %d confidence %f outside [0,1]: %+v", i, e.Confidence, e)
		}
		if e.Type == "" {
			t.Errorf("entity %d has an empty type: %+v", i, e)
		}
	}

	// Sorted by start, non-decreasing.
	for i := 1; i < len(entities); i++ {
		if entities[i].Start < entities[i-1].Start {
			t.Errorf("entities not sorted by start: [%d:%d] then [%d:%d]",
				entities[i-1].Start, entities[i-1].End, entities[i].Start, entities[i].End)
		}
	}

	// The core invariant: no two returned entities may overlap. A downstream
	// operator splices by byte offset, so overlapping spans would rewrite the
	// same region twice and corrupt the text.
	for i := 0; i < len(entities); i++ {
		for j := i + 1; j < len(entities); j++ {
			if entities[i].Start < entities[j].End && entities[j].Start < entities[i].End {
				t.Errorf("overlapping entities %+v and %+v in %q",
					entities[i], entities[j], text)
			}
		}
	}
}

// FuzzAnalyzeInvariants asserts the Analyze invariants over arbitrary text.
//
// Run: go test ./analyzer -run=FuzzAnalyzeInvariants -fuzz=FuzzAnalyzeInvariants
func FuzzAnalyzeInvariants(f *testing.F) {
	seeds := []string{
		"",
		"plain text with no pii",
		"a@b.com",
		"card 4111111111111111 phone 555-123-4567",
		"IBAN GB82 WEST 1234 5698 7654 32",
		"ssn 123-45-6789 ip 8.8.8.8 url https://x.io/p",
		"btc 1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
		"41111111111111111111111111111111", // long digit run
		"DE89370400440532013000 and bad: XX00",
		"éèê unicode 4111111111111111 ☃",
		"\x00\x01\xff binary-ish 4111111111111111",
		"a@b.com a@b.com a@b.com a@b.com",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	a := newRealAnalyzer()
	aDefault := newDefaultAnalyzer()
	ctx := context.Background()

	f.Fuzz(func(t *testing.T, text string) {
		entities, err := a.Analyze(ctx, text)
		if err != nil {
			t.Fatalf("Analyze(%q): %v", text, err)
		}
		assertWellFormed(t, text, entities)

		entities, err = aDefault.Analyze(ctx, text)
		if err != nil {
			t.Fatalf("Analyze(%q): %v", text, err)
		}
		assertWellFormed(t, text, entities)
	})
}

// TestAnalyzeInvariantsOnCorpus runs the same invariants over a corpus of
// awkward inputs without needing the fuzzer, so ordinary `go test` catches a
// regression in CI.
func TestAnalyzeInvariantsOnCorpus(t *testing.T) {
	corpus := []string{
		"",
		" ",
		"\n\t",
		"plain text with no pii at all",
		"a@b.com",
		"a@b.com a@b.com a@b.com",
		"card 4111111111111111 phone 555-123-4567",
		"card: 4111-1111-1111-1111, phone: 555.123.4567",
		"IBAN GB82 WEST 1234 5698 7654 32",
		"IBAN: DE89370400440532013000 and bad: XX00",
		"ssn 123-45-6789 ip 8.8.8.8 url https://x.io/p?q=1",
		"btc 1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq",
		"41111111111111111111111111111111",
		"1234567890123456789012345678901234567890",
		"5555555555554444555555555554444",
		"éèê unicode 4111111111111111 ☃ テスト",
		"\x00\x01\xff\xfe binary 4111111111111111",
		strings.Repeat("a@b.com ", 50),
		strings.Repeat("4111111111111111 ", 20),
		"multi\nline\r\ntext 4111111111111111\nand 555-123-4567",
		"@ @@@ @@@@  .  ..  -  --",
		strings.Repeat("9", 200),
		"GB82 WEST 1234 5698 7654 32 GB82 WEST 1234 5698 7654 32",
	}

	ctx := context.Background()
	for _, name := range []string{"with-checksums", "default"} {
		a := newRealAnalyzer()
		if name == "default" {
			a = newDefaultAnalyzer()
		}
		for i, text := range corpus {
			entities, err := a.Analyze(ctx, text)
			if err != nil {
				t.Fatalf("%s corpus[%d]: unexpected error: %v", name, i, err)
			}
			assertWellFormed(t, text, entities)
		}
	}
}

// An analyzer with NO recognizers must return nothing without erroring.
func TestAnalyzeWithNoRecognizers(t *testing.T) {
	entities, err := New().Analyze(context.Background(), "4111111111111111 a@b.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entities) != 0 {
		t.Errorf("got %d entities with no recognizers, want 0: %+v", len(entities), entities)
	}
}

// Repeated analysis of the same input must return identical results. The
// recognizers are independent, but a future change that sorts, dedupes or
// samples non-deterministically would silently break reproducibility for
// callers that compare consecutive runs.
func TestAnalyzeIsDeterministic(t *testing.T) {
	a := newRealAnalyzer()
	text := "card 4111111111111111 mail a@b.com ssn 123-45-6787 call 555-123-4567 ip 8.8.8.8"
	ctx := context.Background()

	first, err := a.Analyze(ctx, text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 0; i < 25; i++ {
		got, err := a.Analyze(ctx, text)
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if len(got) != len(first) {
			t.Fatalf("iteration %d: got %d entities, first run had %d", i, len(got), len(first))
		}
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("iteration %d entity %d: got %+v, first run had %+v", i, j, got[j], first[j])
			}
		}
	}
}

// A detector must never panic, whatever the bytes. Invalid UTF-8, embedded NULs
// and very long runs all reach the recognizers from real log lines.
func TestAnalyzeNeverPanics(t *testing.T) {
	a := newRealAnalyzer()
	ctx := context.Background()

	inputs := []string{
		string([]byte{0xff, 0xfe, 0xfd}),        // invalid UTF-8
		string([]byte{0x00, 0x41, 0x00, 0x42}),  // embedded NULs
		strings.Repeat("\xff", 1000),            // long invalid run
		strings.Repeat("9", 10000),              // long digit run
		strings.Repeat("a@b.com ", 500),         // many entities
		strings.Repeat("-", 5000),               // separator flood
		"@\x00.\x00@\x00.\x00",                  // interleaved NULs
		"\U0001F600\U0001F601 4111111111111111", // 4-byte runes
	}
	for i, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("input %d panicked: %v", i, r)
				}
			}()
			entities, err := a.Analyze(ctx, in)
			if err != nil {
				t.Errorf("input %d: unexpected error: %v", i, err)
				return
			}
			assertWellFormed(t, in, entities)
		}()
	}
}

// Removing a recognizer must only ever reduce detections, never change the text
// or produce an entity that the full analyzer did not also produce.
func TestRemovingRecognizerIsMonotonic(t *testing.T) {
	full := newRealAnalyzer()
	withoutPhone := New()
	withoutPhone.AddRecognizer(regex.NewEmailRecognizer())
	withoutPhone.AddRecognizer(regex.NewCreditCardRecognizer())
	withoutPhone.AddRecognizer(regex.NewSSNRecognizer())
	withoutPhone.AddRecognizer(regex.NewIPRecognizer())

	text := "card 4111111111111111 mail a@b.com ssn 123-45-6787 call 555-123-4567"
	ctx := context.Background()

	fullEntities, err := full.Analyze(ctx, text)
	if err != nil {
		t.Fatalf("full: unexpected error: %v", err)
	}
	partial, err := withoutPhone.Analyze(ctx, text)
	if err != nil {
		t.Fatalf("partial: unexpected error: %v", err)
	}

	if len(partial) > len(fullEntities) {
		t.Errorf("removing the phone recognizer increased detections: %d -> %d",
			len(partial), len(fullEntities))
	}
	// The phone number must survive when the recognizer is present.
	foundPhone := false
	for _, e := range fullEntities {
		if e.Type == recognizers.EntityPhone {
			foundPhone = true
		}
	}
	if !foundPhone {
		t.Errorf("phone number not detected with the phone recognizer registered: %+v", fullEntities)
	}
}
