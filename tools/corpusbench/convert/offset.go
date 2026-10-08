// Package convert turns corpus records into golden.Case datasets.
//
// The conversion is where a measurement can silently become wrong: corpora
// annotate with Python character offsets, while Go slices bytes, so an
// untranslated offset selects plausible-looking but incorrect text. Every
// function here therefore validates its input rather than trusting it.
package convert

import (
	"fmt"
	"unicode/utf8"
)

// Span is one corpus annotation, in the corpus's own units.
//
// Start and End are CHARACTER indices — the Nth rune of the record's text, as
// Python counts them. They are deliberately not bytes: a corpus written with
// byte offsets in mind would be a different adapter, and conflating the two is
// the failure this type exists to prevent.
type Span struct {
	// Start is the character index of the first character of the annotation.
	Start int
	// End is the character index just past the last character (half-open).
	End int
	// Label is the corpus's own label name, e.g. "email" or "iban". Mapping
	// it to an Entity type is the label map's job, not this type's.
	Label string
}

// CharToByte translates a character-indexed span into byte offsets.
//
// Corpus offsets are Python character indices. Slicing a Go string with them
// yields the wrong text whenever the record contains a multi-byte character:
// in the measured corpora 3614 of 6712 gretel spans differ between the two
// interpretations, and 132 land on an invalid UTF-8 boundary. nemotron's text
// is ASCII, where the two coincide — so the defect hides in one corpus and
// corrupts the other.
//
// It rejects rather than approximates:
//
//   - text that is not valid UTF-8, because counting characters in damaged
//     input depends on where the damage is rather than on the text's meaning;
//   - indices outside [0, runeCount], which is also how a byte index passed
//     where a character index belongs is caught, since a byte index exceeds the
//     rune count on multi-byte text;
//   - a span whose start is not strictly before its end.
//
// The returned offsets are rune boundaries by construction: they are taken
// from the byte position at which each rune begins, so no returned value can
// split a character.
func CharToByte(text string, charStart, charEnd int) (byteStart, byteEnd int, err error) {
	if !utf8.ValidString(text) {
		return 0, 0, fmt.Errorf(
			"text is not valid UTF-8; character offsets cannot be counted reliably")
	}
	runeCount := utf8.RuneCountInString(text)
	if charStart < 0 || charEnd < 0 || charStart > runeCount || charEnd > runeCount {
		return 0, 0, fmt.Errorf(
			"character offsets %d..%d out of range: text has %d characters "+
				"(indices are character offsets, not bytes)",
			charStart, charEnd, runeCount)
	}
	if charStart >= charEnd {
		return 0, 0, fmt.Errorf("empty or inverted character span %d..%d", charStart, charEnd)
	}

	// Single pass: record the byte offset at which each requested character
	// index begins. An index equal to runeCount has no rune of its own, so it
	// resolves to the end of the text.
	start, end := -1, -1
	charIndex := 0
	for byteOff := range text {
		if charIndex == charStart {
			start = byteOff
		}
		if charIndex == charEnd {
			end = byteOff
			break
		}
		charIndex++
	}
	if start < 0 {
		start = len(text)
	}
	if end < 0 {
		end = len(text)
	}
	return start, end, nil
}
