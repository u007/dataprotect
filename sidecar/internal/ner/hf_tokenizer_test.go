//go:build onnx

package ner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type parityToken struct {
	Token     string `json:"token"`
	ID        int64  `json:"id"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
	WordStart bool   `json:"word_start"`
	Special   bool   `json:"special"`
}

type paritySentence struct {
	Text   string        `json:"text"`
	Tokens []parityToken `json:"tokens"`
}

type parityFixture struct {
	TokenizerSHA256 string           `json:"tokenizer_sha256"`
	Sentences       []paritySentence `json:"sentences"`
}

// loadVocab reads the tokenizer.json's WordPiece vocab so the test can compare
// token strings, not just ids: the seam carries ids, and the fixture's contract is
// token strings plus byte offsets.
func loadVocab(t *testing.T, path string) map[int64]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Model struct {
			Vocab map[string]int64 `json:"vocab"`
		} `json:"model"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	vocab := make(map[int64]string, len(doc.Model.Vocab))
	for token, id := range doc.Model.Vocab {
		vocab[id] = token
	}
	return vocab
}

func TestHFTokenizerParity(t *testing.T) {
	dir := filepath.Join("testdata")
	tokenizerPath := filepath.Join(dir, "tokenizer.json")

	data, err := os.ReadFile(tokenizerPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	gotSum := hex.EncodeToString(sum[:])

	fixtureData, err := os.ReadFile(filepath.Join(dir, "tokenizer_parity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture parityFixture
	if err := json.Unmarshal(fixtureData, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.TokenizerSHA256 != gotSum {
		t.Fatalf("tokenizer.json sha256 = %s, fixture pins %s", gotSum, fixture.TokenizerSHA256)
	}

	tok, err := NewHFTokenizer(tokenizerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tok.(*hfTokenizer).Close()
	vocab := loadVocab(t, tokenizerPath)

	for _, sentence := range fixture.Sentences {
		t.Run(sentence.Text, func(t *testing.T) {
			got, err := tok.Tokenize(sentence.Text)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(sentence.Tokens) {
				t.Fatalf("got %d tokens, want %d: %+v", len(got), len(sentence.Tokens), got)
			}
			for i, want := range sentence.Tokens {
				g := got[i]
				if g.ID != want.ID {
					t.Errorf("token %d: id %d, want %d", i, g.ID, want.ID)
				}
				if name := vocab[g.ID]; name != want.Token {
					t.Errorf("token %d: string %q, want %q", i, name, want.Token)
				}
				if g.Start != want.Start || g.End != want.End {
					t.Errorf("token %d (%q): offsets [%d,%d], want [%d,%d]",
						i, want.Token, g.Start, g.End, want.Start, want.End)
				}
				if g.WordStart != want.WordStart {
					t.Errorf("token %d (%q): wordStart %v, want %v", i, want.Token, g.WordStart, want.WordStart)
				}
				if g.Special != want.Special {
					t.Errorf("token %d (%q): special %v, want %v", i, want.Token, g.Special, want.Special)
				}
			}
		})
	}
}

func TestHFTokenizerEmptyText(t *testing.T) {
	tok, err := NewHFTokenizer(filepath.Join("testdata", "tokenizer.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer tok.(*hfTokenizer).Close()

	got, err := tok.Tokenize("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d tokens for empty text, want 0", len(got))
	}
}

func TestHFTokenizerMissingFile(t *testing.T) {
	if _, err := NewHFTokenizer(filepath.Join("testdata", "does-not-exist.json")); err == nil {
		t.Fatal("expected an error for a missing tokenizer file")
	}
}

func TestTokenOffsets(t *testing.T) {
	valid := []struct {
		textLen int
		start   uint
		end     uint
		wantS   int
		wantE   int
	}{
		{textLen: 0, start: 0, end: 0, wantS: 0, wantE: 0},
		{textLen: 11, start: 6, end: 11, wantS: 6, wantE: 11},
		{textLen: 4, start: 0, end: 4, wantS: 0, wantE: 4},
	}
	for _, tt := range valid {
		s, e, err := tokenOffsets(tt.textLen, 0, tt.start, tt.end)
		if err != nil {
			t.Fatalf("textLen=%d offsets=[%d,%d]: unexpected error: %v", tt.textLen, tt.start, tt.end, err)
		}
		if s != tt.wantS || e != tt.wantE {
			t.Fatalf("textLen=%d offsets=[%d,%d]: got [%d,%d], want [%d,%d]",
				tt.textLen, tt.start, tt.end, s, e, tt.wantS, tt.wantE)
		}
	}

	invalid := []struct {
		name    string
		textLen int
		start   uint
		end     uint
	}{
		{name: "end past the text", textLen: 10, start: 9, end: 11},
		{name: "start past the text", textLen: 10, start: 11, end: 11},
		{name: "end before start", textLen: 10, start: 5, end: 4},
		{name: "non-empty span in empty text", textLen: 0, start: 0, end: 1},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := tokenOffsets(tt.textLen, 3, tt.start, tt.end); err == nil {
				t.Fatal("expected an error, not a clamped span")
			}
		})
	}
}

// checkWordStarts asserts wordStart for every token of one sequence.
func checkWordStarts(t *testing.T, text string, pieces []string, tokens []Token, want []bool) {
	t.Helper()
	for i := range want {
		if got := wordStart(text, pieces, tokens, i); got != want[i] {
			t.Errorf("wordStart(%q at [%d,%d]) = %v, want %v",
				pieces[i], tokens[i].Start, tokens[i].End, got, want[i])
		}
	}
}

func TestWordStartMarkers(t *testing.T) {
	// "x" starts at 18, one byte after the comma's space, so the whitespace gap
	// makes it a word start even though it carries no marker.
	text := "Berlin!ing world, x"
	pieces := []string{"▁Berlin", "##ing", "Ġworld", ",", "x"}
	tokens := []Token{
		{Start: 0, End: 7}, {Start: 7, End: 10}, {Start: 11, End: 16},
		{Start: 16, End: 17}, {Start: 18, End: 19},
	}
	checkWordStarts(t, text, pieces, tokens, []bool{true, false, true, false, true})
}

func TestWordStartSentencePieceBareMarker(t *testing.T) {
	// A bare "▁" begins a word; the piece after it continues that word.
	t.Run("marker covers the space", func(t *testing.T) {
		checkWordStarts(t, "x 2019",
			[]string{"x", "▁", "2019"},
			[]Token{{Start: 0, End: 1}, {Start: 1, End: 2}, {Start: 2, End: 6}},
			[]bool{true, true, false})
	})
	t.Run("zero-width marker with a gap before the next piece", func(t *testing.T) {
		// Without the bare-marker rule the gap at byte 1 would open a second word.
		checkWordStarts(t, "x 2019",
			[]string{"x", "▁", "2019"},
			[]Token{{Start: 0, End: 1}, {Start: 1, End: 1}, {Start: 2, End: 6}},
			[]bool{true, true, false})
	})
	t.Run("byte-level bare marker", func(t *testing.T) {
		checkWordStarts(t, "a b",
			[]string{"a", "Ġ", "b"},
			[]Token{{Start: 0, End: 1}, {Start: 1, End: 1}, {Start: 2, End: 3}},
			[]bool{true, true, false})
	})
}

func TestWordStartZeroWidth(t *testing.T) {
	// A zero-width piece with no marker occupies no bytes and cannot begin a word,
	// and it must not hide the boundary before the next word.
	checkWordStarts(t, "a b",
		[]string{"a", "<x>", "b"},
		[]Token{{Start: 0, End: 1}, {Start: 1, End: 1}, {Start: 2, End: 3}},
		[]bool{true, false, true})
}

func TestWordStartSpecialToken(t *testing.T) {
	// A special token never begins a word, even at the start of the sequence, and
	// it is skipped when looking for the previous word's bytes.
	checkWordStarts(t, "hello",
		[]string{"<s>", "hello"},
		[]Token{{Start: 0, End: 0, Special: true}, {Start: 0, End: 5}},
		[]bool{false, true})
}

func TestWordStartNonWhitespaceGap(t *testing.T) {
	// A zero-width-space gap (U+200B, bytes 2..4) is not whitespace, so the piece
	// after it continues the word instead of opening one.
	checkWordStarts(t, "ab​cd",
		[]string{"ab", "cd"},
		[]Token{{Start: 0, End: 2}, {Start: 5, End: 7}},
		[]bool{true, false})
}

func TestWordStartOverlapAndTouch(t *testing.T) {
	// Touching or overlapping offsets never open a word.
	t.Run("touching", func(t *testing.T) {
		checkWordStarts(t, "abcd",
			[]string{"ab", "cd"},
			[]Token{{Start: 0, End: 2}, {Start: 2, End: 4}},
			[]bool{true, false})
	})
	t.Run("overlapping", func(t *testing.T) {
		checkWordStarts(t, "abcd",
			[]string{"ab", "cd"},
			[]Token{{Start: 0, End: 2}, {Start: 1, End: 4}},
			[]bool{true, false})
	})
}
