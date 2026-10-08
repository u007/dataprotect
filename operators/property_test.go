package operators

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

// Property tests for the Operator contract.
//
// These exist because an operator is the last thing to touch a value before it
// leaves the library, and it is handed whatever the recognizer matched — which
// is attacker-shaped: arbitrary bytes, embedded separators, multi-byte runes,
// empty strings, very long runs. A table of hand-picked values only covers the
// shapes somebody thought of. The invariants below hold for every value and
// every mask pattern.
//
// The contract the Anonymizer relies on:
//   - Replace never returns an error and never panics (it has no failure mode
//     to report; an operator that errors would abort a whole document).
//   - Replace is deterministic: the same value always maps to the same output,
//     which is what keeps referential integrity across a dataset.

// operatorCorpus is a shared set of awkward values. It is used by both the
// corpus test (so ordinary `go test` catches a regression) and the fuzz seeds.
var operatorCorpus = []string{
	"",
	" ",
	"4111111111111111",
	"4111 1111 1111 1111",
	"GB82 WEST 1234 5698 7654 32",
	"é",                // 2-byte rune
	"☃",                // 3-byte rune
	"\x00\x01\xff\xfe", // non-UTF-8 bytes
	"a-b.c d",
	strings.Repeat("9", 300),
}

// maskCorpus is a set of mask patterns: well-formed ones plus the degenerate
// shapes (empty, no slots, only separators, non-slot letters) that the pattern
// grammar does not forbid.
var maskCorpus = []string{
	"",
	"****-****-****-####",
	"###",
	"*",
	"**-**",
	"  -  ",
	"xx", // letters are neither slots nor separators
}

// assertOperatorTotal checks that one operator is total (never errors) and
// deterministic on one value.
func assertOperatorTotal(t *testing.T, op Operator, value string) {
	t.Helper()
	ctx := context.Background()

	first, err := op.Replace(ctx, value)
	if err != nil {
		t.Errorf("%s.Replace(%q) returned an error: %v", op.Name(), value, err)
		return
	}
	second, err := op.Replace(ctx, value)
	if err != nil {
		t.Errorf("%s.Replace(%q) errored on the second call: %v", op.Name(), value, err)
		return
	}
	if first != second {
		t.Errorf("%s.Replace(%q) is not deterministic: %q then %q", op.Name(), value, first, second)
	}
}

// TestOperatorInvariantsOnCorpus runs the totality/determinism invariants over
// the corpus without needing the fuzzer.
func TestOperatorInvariantsOnCorpus(t *testing.T) {
	for _, value := range operatorCorpus {
		assertOperatorTotal(t, NewReplaceOperator("<X>"), value)
		assertOperatorTotal(t, NewRedactOperator(), value)
		assertOperatorTotal(t, NewHashOperator(0), value)
		assertOperatorTotal(t, NewHashOperator(8), value)
		for _, pattern := range maskCorpus {
			assertOperatorTotal(t, NewMaskOperator(pattern), value)
		}
	}
}

// TestMaskOutputLength pins the mask arithmetic as a single RUNE-count identity
// that holds for every non-empty pattern:
//
//	runeCount(out) == nonSlotPatternRunes + max(significantInputRunes, slots)
//
// The reasoning: maskIdx only ever increases, so each non-slot pattern rune is
// emitted exactly once (in the inner loop while the input lasts, or in the tail
// fill); and one data rune is emitted per significant input rune, plus one '*'
// per slot the input did not reach. This is a closed form even for patterns
// containing literal letters — a letter is a non-slot rune and is emitted like
// any other separator, so it needs no special case.
//
// The identity is in RUNES, not bytes: masking is one slot per character, so a
// multi-byte rune contributes 1, not its byte length.
//
// It is stated for VALID UTF-8 input. An invalid byte is also one unit, but
// dropping a separator between two invalid bytes can let the kept bytes join
// into one valid rune, so the rune count of the output can be lower than the
// identity predicts — that case is out of scope here.
//
// The EMPTY pattern is excluded: applyMask short-circuits it to one '*' per
// input RUNE (separators included), which is a different contract pinned by
// TestMaskEmptyPatternMasksEverything.
func TestMaskOutputLength(t *testing.T) {
	patterns := []string{"****-****-****-####", "###", "*", "**-**", "  -  ", "xx", "x#x", "a#b#c"}
	values := []string{"", "123", "4111111111111111", "4111 1111 1111 1111", "a-b.c d", "é☃", "a-é☃-b"}

	for _, pattern := range patterns {
		nonSlot, slots := 0, 0
		for _, r := range pattern {
			if isPatternSlot(r) {
				slots++
			} else {
				nonSlot++
			}
		}
		for _, value := range values {
			sig := 0
			for _, r := range value {
				if !isMaskSeparator(r) {
					sig++
				}
			}
			want := nonSlot + sig
			if slots > sig {
				want = nonSlot + slots
			}

			out, err := NewMaskOperator(pattern).Replace(context.Background(), value)
			if err != nil {
				t.Fatalf("mask %q %q: unexpected error: %v", pattern, value, err)
			}
			if got := utf8.RuneCountInString(out); got != want {
				t.Errorf("mask %q on %q = %q: %d runes, want %d (nonSlot=%d sig=%d slots=%d)",
					pattern, value, out, got, want, nonSlot, sig, slots)
			}
		}
	}
}

// TestMaskEmptyPatternMasksEverything pins the documented degenerate case: an
// empty pattern masks every RUNE, including separators, so the output is one '*'
// per input character (not per byte).
func TestMaskEmptyPatternMasksEverything(t *testing.T) {
	for _, value := range operatorCorpus {
		out, err := NewMaskOperator("").Replace(context.Background(), value)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := strings.Repeat("*", utf8.RuneCountInString(value)); out != want {
			t.Errorf("empty pattern on %q: got %q, want %q", value, out, want)
		}
	}
}

// TestMaskDoesNotLeakInputData is the invariant that actually matters for a PII
// library: when the pattern contains no '#' slot, NO input data byte may appear
// in the output. Every non-separator output byte must be either '*' or a byte
// that is a literal in the pattern (a pattern byte that is not a slot). A
// regression that turned '#' handling into a pass-through, or that emitted the
// input on an unfilled slot, would leak the value and fail here.
//
// The other operators are constants or a hash, so their outputs cannot leak by
// construction; the mask operator is the only one with a data path.
func TestMaskDoesNotLeakInputData(t *testing.T) {
	patterns := []string{"", "****-****-****-####", "**-**", "  -  ", "xx", "*"}
	values := []string{"", "4111111111111111", "4111 1111 1111 1111", "a-b.c d", "é☃"}

	isLiteral := func(pattern string, r rune) bool {
		if r == '*' || r == '#' {
			return false // slots are never emitted as themselves
		}
		return strings.ContainsRune(pattern, r)
	}

	for _, pattern := range patterns {
		if strings.ContainsRune(pattern, '#') {
			continue // a '#' slot is SUPPOSED to keep the input rune
		}
		for _, value := range values {
			out, err := NewMaskOperator(pattern).Replace(context.Background(), value)
			if err != nil {
				t.Fatalf("mask %q %q: unexpected error: %v", pattern, value, err)
			}
			for _, r := range out {
				if isMaskSeparator(r) {
					continue
				}
				if r == '*' || isLiteral(pattern, r) {
					continue
				}
				t.Errorf("mask %q on %q = %q leaked input rune %q",
					pattern, value, out, r)
			}
		}
	}
}

// TestMaskIsIndependentOfMaskedInput is a strictly stronger form of the leak
// invariant. With no '#' slot, the output depends only on the pattern and the
// SHAPE of the input (its significant-RUNE count), never on the input's values
// or separator positions: two such values must mask identically. A leak that
// happened to land on a rune equal to '*' or to a pattern literal would pass the
// scan in TestMaskDoesNotLeakInputData but fails here.
func TestMaskIsIndependentOfMaskedInput(t *testing.T) {
	// The EMPTY pattern is excluded: it masks every rune INCLUDING separators,
	// so its output length is the input's rune count and it is not shape-
	// independent. That special case is pinned by
	// TestMaskEmptyPatternMasksEverything.
	patterns := []string{"****-****-****-####", "**-**", "  -  ", "xx", "*"}
	pairs := [][2]string{
		{"4111111111111111", "4222222222222222"},
		{"abc", "xyz"},
		{"a-b.c d", "e-f.g h"},
		{"ab-cd", "a-b.c d"}, // same significant count (4), different separators
		{"é☃", "☃é"},         // multi-byte runes: 2 significant runes each
	}
	sigCount := func(s string) int {
		n := 0
		for _, r := range s {
			if !isMaskSeparator(r) {
				n++
			}
		}
		return n
	}
	// expectedShape is the independent closed form for a no-'#' pattern: every
	// slot becomes '*', non-slot runes are emitted as themselves, and input
	// beyond the last slot adds one '*' each. Comparing against it (not just
	// against another output) is what stops the test passing when BOTH inputs
	// leak identically.
	expectedShape := func(pattern string, sig int) string {
		var b strings.Builder
		slots := 0
		for _, r := range pattern {
			if isPatternSlot(r) {
				slots++
				b.WriteByte('*')
			} else {
				b.WriteRune(r)
			}
		}
		if sig > slots {
			b.WriteString(strings.Repeat("*", sig-slots))
		}
		return b.String()
	}

	for _, pattern := range patterns {
		if strings.ContainsRune(pattern, '#') {
			continue // a '#' slot is SUPPOSED to keep the input rune
		}
		for _, pair := range pairs {
			if sigCount(pair[0]) != sigCount(pair[1]) {
				continue
			}
			o1, err := NewMaskOperator(pattern).Replace(context.Background(), pair[0])
			if err != nil {
				t.Fatalf("mask %q %q: %v", pattern, pair[0], err)
			}
			o2, err := NewMaskOperator(pattern).Replace(context.Background(), pair[1])
			if err != nil {
				t.Fatalf("mask %q %q: %v", pattern, pair[1], err)
			}
			if o1 != o2 {
				t.Errorf("mask %q is input-dependent: %q -> %q but %q -> %q",
					pattern, pair[0], o1, pair[1], o2)
			}
			if want := expectedShape(pattern, sigCount(pair[0])); o1 != want {
				t.Errorf("mask %q on %q = %q, want shape %q", pattern, pair[0], o1, want)
			}
		}
	}
}

// TestHashOutputLength pins the documented hash length: the full SHA-256 hex
// digest is 64 characters; a positive truncation shorter than that wins, and a
// truncation at or beyond 64 (or zero) yields the full digest.
func TestHashOutputLength(t *testing.T) {
	cases := []struct {
		truncate int
		want     int
	}{
		{0, 64},
		{1, 1},
		{8, 8},
		{63, 63},
		{64, 64},
		{100, 64},
		{-1, 64},
	}
	for _, c := range cases {
		out, err := NewHashOperator(c.truncate).Replace(context.Background(), "4111111111111111")
		if err != nil {
			t.Fatalf("truncate=%d: unexpected error: %v", c.truncate, err)
		}
		if len(out) != c.want {
			t.Errorf("truncate=%d: length %d, want %d", c.truncate, len(out), c.want)
		}
	}
}

// FuzzOperatorReplace asserts the totality and determinism invariants for
// arbitrary values and arbitrary mask patterns.
//
// Run: go test ./operators -run=FuzzOperatorReplace -fuzz=FuzzOperatorReplace
func FuzzOperatorReplace(f *testing.F) {
	f.Add("4111111111111111", "****-****-****-####")
	f.Add("", "")
	f.Add("é☃", "###")
	f.Add("\x00\xff", "*")
	f.Add("a-b.c d", "  -  ")
	f.Add("é☃-a", "**-##")
	f.Add("日本語", "#*")

	f.Fuzz(func(t *testing.T, value, pattern string) {
		assertOperatorTotal(t, NewReplaceOperator("<X>"), value)
		assertOperatorTotal(t, NewRedactOperator(), value)
		assertOperatorTotal(t, NewHashOperator(0), value)
		assertOperatorTotal(t, NewHashOperator(8), value)

		mask := NewMaskOperator(pattern)
		assertOperatorTotal(t, mask, value)

		out, err := mask.Replace(context.Background(), value)
		if err != nil {
			t.Fatalf("mask %q %q: %v", pattern, value, err)
		}
		// Masking valid UTF-8 yields valid UTF-8: it keeps whole runes or writes
		// '*'/pattern runes, never a partial rune.
		if utf8.ValidString(value) && !utf8.ValidString(out) {
			t.Fatalf("mask %q on valid input %q produced invalid UTF-8 %q", pattern, value, out)
		}

		// The empty-pattern contract must hold for any value: one '*' per rune.
		if pattern == "" {
			if want := strings.Repeat("*", utf8.RuneCountInString(value)); out != want {
				t.Errorf("empty pattern on %q: got %q, want %q", value, out, want)
			}
		}
	})
}
