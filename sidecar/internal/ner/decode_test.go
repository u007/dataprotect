package ner

import (
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

func TestDecodeBasicPerson(t *testing.T) {
	text := "Hello John Smith today"
	tokens := []Token{
		{Start: 0, End: 5, WordStart: true},
		{Start: 6, End: 10, WordStart: true},
		{Start: 11, End: 16, WordStart: true},
		{Start: 17, End: 22, WordStart: true},
	}
	labels := []string{"O", "B-PER", "I-PER", "O"}
	logits := [][]float32{
		{0, 10},
		{0, 10},
		{0, 10},
		{0, 10},
	}
	labels = []string{"O", "B-PERSON", "I-PERSON"}
	logits = [][]float32{
		{10, 0, 0},
		{0, 10, 0},
		{0, 0, 10},
		{10, 0, 0},
	}
	got, err := Decode(text, tokens, logits, labels)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entities, want 1: %+v", len(got), got)
	}
	if got[0].Type != recognizers.EntityPerson || got[0].Start != 6 || got[0].End != 16 {
		t.Fatalf("got %+v", got[0])
	}
	if got[0].Confidence != 0.80 {
		t.Fatalf("confidence = %v", got[0].Confidence)
	}
}

func TestDecodeMismatchedLengths(t *testing.T) {
	_, err := Decode("hi", []Token{{Start: 0, End: 2, WordStart: true}}, [][]float32{{1, 2}, {3, 4}}, []string{"O", "B-PERSON"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeUnknownLabelDropped(t *testing.T) {
	text := "Hello"
	tokens := []Token{{Start: 0, End: 5, WordStart: true}}
	labels := []string{"O", "B-MISC"}
	logits := [][]float32{{0, 10}}
	got, err := Decode(text, tokens, logits, labels)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d, want 0", len(got))
	}
}

func TestDecodeSpecialTokenDropped(t *testing.T) {
	text := "Hi John"
	tokens := []Token{
		{Start: 0, End: 2, WordStart: true},
		{Start: 2, End: 3, Special: true},
		{Start: 3, End: 7, WordStart: true},
	}
	labels := []string{"O", "B-PERSON"}
	logits := [][]float32{
		{10, 0},
		{0, 10},
		{0, 10},
	}
	got, err := Decode(text, tokens, logits, labels)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
}

func TestDecodeWhitespaceOnlySpanDropped(t *testing.T) {
	text := "   "
	tokens := []Token{{Start: 0, End: 3, WordStart: true}}
	labels := []string{"O", "B-PERSON"}
	logits := [][]float32{{0, 10}}
	got, err := Decode(text, tokens, logits, labels)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d, want 0", len(got))
	}
}

func TestDecodeLogitLabelMismatch(t *testing.T) {
	text := "Hi"
	tokens := []Token{{Start: 0, End: 2, WordStart: true}}
	labels := []string{"O", "B-PERSON"}
	logits := [][]float32{{1, 2, 3}}
	_, err := Decode(text, tokens, logits, labels)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeBIOES(t *testing.T) {
	text := "John"
	tokens := []Token{{Start: 0, End: 4, WordStart: true}}
	labels := []string{"O", "S-PERSON"}
	logits := [][]float32{{0, 10}}
	got, err := Decode(text, tokens, logits, labels)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Start != 0 || got[0].End != 4 {
		t.Fatalf("got %+v", got)
	}
}

func TestDecodeMidWordStartDropped(t *testing.T) {
	text := "xJohn"
	tokens := []Token{{Start: 1, End: 5, WordStart: false}}
	labels := []string{"O", "B-PERSON"}
	logits := [][]float32{{0, 10}}
	got, err := Decode(text, tokens, logits, labels)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d, want 0", len(got))
	}
}

func TestDecodeOverlapResolution(t *testing.T) {
	text := "John Smith"
	tokens := []Token{
		{Start: 0, End: 4, WordStart: true},
		{Start: 5, End: 10, WordStart: true},
	}
	labels := []string{"O", "B-PERSON", "I-PERSON"}
	// Two overlapping spans: "John" (prob low) and "John Smith" (prob high).
	logits := [][]float32{
		{0, 10, 0},
		{0, 0, 10},
	}
	got, err := Decode(text, tokens, logits, labels)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Start != 0 || got[0].End != 10 {
		t.Fatalf("got %+v", got)
	}
}

func TestDecodeDedupIdenticalSpans(t *testing.T) {
	text := "John"
	tokens := []Token{{Start: 0, End: 4, WordStart: true}}
	labels := []string{"O", "B-PERSON"}
	logits := [][]float32{{0, 10}}
	got, err := Decode(text, tokens, logits, labels)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
}

func TestDecodeSixteenTokenLimit(t *testing.T) {
	text := "a b c d e f g h i j k l m n o p" // 16 tokens
	tokens := make([]Token, 0, 16)
	for i := 0; i < 16; i++ {
		tokens = append(tokens, Token{Start: i * 2, End: i*2 + 1, WordStart: true})
	}
	labels := []string{"O", "B-PERSON", "I-PERSON"}
	logits := make([][]float32, len(tokens))
	for i := range logits {
		if i == 0 {
			logits[i] = []float32{0, 10, 0}
		} else {
			logits[i] = []float32{0, 0, 10}
		}
	}
	got, err := Decode(text, tokens, logits, labels)
	if err != nil {
		t.Fatal(err)
	}
	// 16 tokens is allowed (<=16).
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
}

func TestDecodeSeventeenTokenLimitDropped(t *testing.T) {
	text := "a b c d e f g h i j k l m n o p q" // 17 tokens
	tokens := make([]Token, 0, 17)
	for i := 0; i < 17; i++ {
		tokens = append(tokens, Token{Start: i * 2, End: i*2 + 1, WordStart: true})
	}
	labels := []string{"O", "B-PERSON", "I-PERSON"}
	logits := make([][]float32, len(tokens))
	for i := range logits {
		if i == 0 {
			logits[i] = []float32{0, 10, 0}
		} else {
			logits[i] = []float32{0, 0, 10}
		}
	}
	got, err := Decode(text, tokens, logits, labels)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d, want 0 (17 tokens exceeds limit)", len(got))
	}
}
