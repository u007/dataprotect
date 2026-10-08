package convert

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The gretel worked example from the spec: span {851,867} char-slices to
// "31 dicembre 2023" but byte-slices to "o al 31 dicembre". A converter that
// trusts byte offsets here produces confidently wrong label text on every
// non-ASCII record — and nemotron's English text is pure ASCII, so the defect
// would be invisible in half the corpus.
func TestCharToByteNonASCIIShiftsOffsets(t *testing.T) {
	// "…" is 3 bytes / 1 rune, so every later index shifts by 2.
	const text = "…31 dicembre 2023…"
	const want = "31 dicembre 2023"

	byteStart, byteEnd, err := CharToByte(text, 1, 17)
	if err != nil {
		t.Fatalf("CharToByte: unexpected error: %v", err)
	}
	if byteStart != 3 || byteEnd != 19 {
		t.Fatalf("CharToByte(1,17) = %d..%d, want 3..19", byteStart, byteEnd)
	}
	if got := text[byteStart:byteEnd]; got != want {
		t.Errorf("text[%d:%d] = %q, want %q", byteStart, byteEnd, got, want)
	}

	// The byte-slicing the converter must NOT do, so the test documents the
	// difference rather than only asserting the right answer.
	if got := text[1:17]; got == want {
		t.Errorf("this test is broken: byte slicing produced the right text, so the shift is not exercised")
	}
}

// A byte index passed where a character index belongs is the actual reachable
// failure. For multi-byte text it exceeds the rune count and must be rejected
// rather than silently reinterpreted.
func TestCharToByteRejectsByteIndexPastRuneCount(t *testing.T) {
	const text = "日本語" // 3 runes, 9 bytes
	// What matters is that the error names the count it rejected the index
	// against, so the message tells a reader why 9 is wrong here.
	if _, _, err := CharToByte(text, 0, 9); err == nil {
		t.Error("CharToByte(0,9) on 3-rune text: want an error for a byte index, got nil")
	} else if !strings.Contains(err.Error(), "3 characters") ||
		!strings.Contains(err.Error(), "out of range") {
		t.Errorf("error %q should name the character count it was checked against", err.Error())
	}
}

func TestCharToByteRejectsOutOfBounds(t *testing.T) {
	const text = "日本語"
	for _, tc := range []struct {
		name      string
		charStart int
		charEnd   int
		wantInMsg string
	}{
		{"end past rune count", 0, 4, "out of range"},
		{"negative start", -1, 3, "out of range"},
		{"end negative", 0, -1, "out of range"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := CharToByte(text, tc.charStart, tc.charEnd)
			if err == nil {
				t.Fatalf("CharToByte(%d,%d): want an error, got nil", tc.charStart, tc.charEnd)
			}
			if !strings.Contains(err.Error(), tc.wantInMsg) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantInMsg)
			}
		})
	}
}

func TestCharToByteRejectsEmptySpan(t *testing.T) {
	const text = "日本語"
	for _, tc := range []struct {
		name      string
		charStart int
		charEnd   int
	}{
		{"zero length", 1, 1},
		{"inverted", 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := CharToByte(text, tc.charStart, tc.charEnd); err == nil {
				t.Fatalf("CharToByte(%d,%d): want an error for an empty span, got nil",
					tc.charStart, tc.charEnd)
			}
		})
	}
}

// Counting runes in invalid UTF-8 is ambiguous: DecodeRuneInString reports
// RuneError with size 1 for a bad byte, so a "rune count" would depend on where
// the damage is rather than on the text's meaning. Reject it outright.
func TestCharToByteRejectsNonUTF8Text(t *testing.T) {
	const text = "caf\xe9latte" // 0xe9 is a stray continuation byte
	if utf8.ValidString(text) {
		t.Fatal("test is broken: the fixture is valid UTF-8")
	}
	if _, _, err := CharToByte(text, 0, 1); err == nil {
		t.Error("CharToByte on invalid UTF-8: want an error, got nil")
	} else if !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("error %q should name the encoding problem", err.Error())
	}
}

// The invariant the whole converter rests on: whatever the input, the returned
// offsets slice on rune boundaries. If a future change broke the mapping, this
// plus the fuzz target is what catches it before a corpus does.
func TestCharToByteResultAlwaysOnRuneBoundaries(t *testing.T) {
	for _, text := range []string{
		"…31 dicembre 2023…",
		"日本語",
		"aéb",
		"plain ascii",
		"",
	} {
		for start := 0; start <= len([]rune(text)); start++ {
			for end := start; end <= len([]rune(text)); end++ {
				if start == end {
					continue
				}
				bs, be, err := CharToByte(text, start, end)
				if err != nil {
					continue // rejected, which is also fine
				}
				if bs < 0 || be > len(text) || bs > be {
					t.Fatalf("%q [%d,%d) -> %d..%d out of text bounds", text, start, end, bs, be)
				}
				// Guard BEFORE indexing: && is left-to-right, so writing the
				// boundary test before the length test panics at be == len(text).
				if bs > 0 && !utf8.RuneStart(text[bs]) {
					t.Errorf("%q start offset %d is not a rune boundary", text, bs)
				}
				if be < len(text) && !utf8.RuneStart(text[be]) {
					t.Errorf("%q end offset %d is not a rune boundary", text, be)
				}
			}
		}
	}
}

// The round-trip property: converting char offsets to bytes and reading the
// bytes gives the same characters the char slice would have given.
func TestCharToByteRoundTripsOnValidUTF8(t *testing.T) {
	const text = "…31 dicembre 2023…"
	runes := []rune(text)
	for start := 0; start < len(runes); start++ {
		for end := start + 1; end <= len(runes); end++ {
			bs, be, err := CharToByte(text, start, end)
			if err != nil {
				t.Fatalf("CharToByte(%d,%d): unexpected error: %v", start, end, err)
			}
			if got := string(runes[start:end]); got != text[bs:be] {
				t.Errorf("CharToByte(%d,%d) -> %d..%d gives %q, want %q",
					start, end, bs, be, text[bs:be], got)
			}
		}
	}
}

func FuzzCharToByte(f *testing.F) {
	for _, seed := range []string{
		"…31 dicembre 2023…",
		"日本語",
		"aéb",
		"plain ascii",
		"",
		"\xff\xfe invalid",
	} {
		f.Add(seed, 0, 1)
	}
	f.Fuzz(func(t *testing.T, text string, charStart, charEnd int) {
		// Bound the indices before the call: any value is legal input to the
		// fuzz target, and the function must reject rather than panic.
		if charStart < 0 || charEnd < 0 {
			charStart, charEnd = 0, 1
		}
		if charStart > len(text)+1 {
			charStart = 0
		}
		if charEnd > len(text)+1 {
			charEnd = charStart + 1
		}
		bs, be, err := CharToByte(text, charStart, charEnd)
		if err != nil {
			return // rejection is a valid answer, and must never panic
		}
		if !utf8.ValidString(text) {
			t.Fatalf("accepted invalid UTF-8 text %q", text)
		}
		if bs < 0 || be < bs || be > len(text) {
			t.Fatalf("offsets %d..%d out of bounds for %d bytes", bs, be, len(text))
		}
		if bs > 0 && !utf8.RuneStart(text[bs]) {
			t.Fatalf("start %d lands mid-rune in %q", bs, text)
		}
		if be < len(text) && !utf8.RuneStart(text[be]) {
			t.Fatalf("end %d lands mid-rune in %q", be, text)
		}
		// Round-trip: byte slice must equal the rune slice.
		if string([]rune(text)[charStart:charEnd]) != text[bs:be] {
			t.Fatalf("round trip failed for %q [%d,%d) -> %d..%d", text, charStart, charEnd, bs, be)
		}
	})
}
