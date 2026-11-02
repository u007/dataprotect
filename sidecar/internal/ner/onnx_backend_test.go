//go:build onnx

package ner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// fakeTokenizer reports the tokens the test built, so chunking and decoding can
// be exercised without a real tokenizer.
type fakeTokenizer struct {
	tokens []Token
	err    error
}

func (f fakeTokenizer) Tokenize(string) ([]Token, error) { return f.tokens, f.err }

// fakeSession answers each token from its id. B-PER is channel 1 and I-PER is
// channel 2, so a token's id alone decides its label.
type fakeSession struct {
	inputNames []string
	ids        map[int64]int // token id -> argmax channel
	err        error
	calls      int
}

func (f *fakeSession) InputNames() []string { return f.inputNames }

func (f *fakeSession) Run(_ context.Context, feeds map[string][]int64) ([][]float32, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	ids := feeds["input_ids"]
	rows := make([][]float32, len(ids))
	for i, id := range ids {
		row := make([]float32, 3)
		row[f.ids[id]] = 10
		rows[i] = row
	}
	return rows, nil
}

// words returns n one-byte tokens separated by spaces, each a word start with a
// global offset. Token i starts at byte 2*i.
func words(n int) (string, []Token) {
	var b strings.Builder
	tokens := make([]Token, n)
	for i := 0; i < n; i++ {
		b.WriteString("a")
		if i != n-1 {
			b.WriteString(" ")
		}
		tokens[i] = Token{ID: int64(i), Start: 2 * i, End: 2*i + 1, WordStart: true}
	}
	return b.String(), tokens
}

func TestONNXBackendRecognizeSingleChunk(t *testing.T) {
	text, tokens := words(5)
	sess := &fakeSession{
		inputNames: []string{"input_ids", "attention_mask"},
		ids:        map[int64]int{1: 1, 2: 2},
	}
	b := &ONNXBackend{
		tokenizer: fakeTokenizer{tokens: tokens},
		session:   sess,
		labels:    []string{"O", "B-PER", "I-PER"},
	}

	got, err := b.Recognize(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entities, want 1: %+v", len(got), got)
	}
	if got[0].Type != recognizers.EntityPerson || got[0].Start != 2 || got[0].End != 5 {
		t.Fatalf("got %+v, want PERSON at [2,5]", got[0])
	}
	if got[0].Confidence != 0.80 {
		t.Fatalf("confidence = %v, want 0.80", got[0].Confidence)
	}
}

func TestONNXBackendRecognizeDedupsAcrossChunks(t *testing.T) {
	text, tokens := words(450)
	sess := &fakeSession{
		inputNames: []string{"input_ids", "attention_mask"},
		ids:        map[int64]int{340: 1, 341: 2},
	}
	b := &ONNXBackend{
		tokenizer: fakeTokenizer{tokens: tokens},
		session:   sess,
		labels:    []string{"O", "B-PER", "I-PER"},
	}

	got, err := b.Recognize(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if sess.calls != 2 {
		t.Fatalf("ran %d chunks, want 2 (400-token chunks with a 64-token stride)", sess.calls)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entities, want 1 after dedup: %+v", len(got), got)
	}
	if got[0].Start != 680 || got[0].End != 683 {
		t.Fatalf("got [%d,%d], want the document-global [680,683]", got[0].Start, got[0].End)
	}
}

func TestONNXBackendRecognizeKeepsStraddlingSpanWhole(t *testing.T) {
	text, tokens := words(450)
	// Tokens 398, 399 and 400 straddle the first chunk's boundary at 400.
	// Only the second chunk sees token 400, so only it can produce the whole
	// span; the merge must prefer it over the truncated one.
	sess := &fakeSession{
		inputNames: []string{"input_ids", "attention_mask"},
		ids:        map[int64]int{398: 1, 399: 2, 400: 2},
	}
	b := &ONNXBackend{
		tokenizer: fakeTokenizer{tokens: tokens},
		session:   sess,
		labels:    []string{"O", "B-PER", "I-PER"},
	}

	got, err := b.Recognize(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entities, want 1: %+v", len(got), got)
	}
	if got[0].Start != 796 || got[0].End != 801 {
		t.Fatalf("got [%d,%d], want the whole span [796,801]", got[0].Start, got[0].End)
	}
}

func TestONNXBackendRecognizeEmptyText(t *testing.T) {
	b := &ONNXBackend{
		tokenizer: fakeTokenizer{},
		session:   &fakeSession{inputNames: []string{"input_ids", "attention_mask"}},
		labels:    []string{"O"},
	}
	got, err := b.Recognize(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d entities for empty text", len(got))
	}
}

func TestONNXBackendRecognizeReturnsNoPartialEntitiesOnError(t *testing.T) {
	text, tokens := words(450)
	sess := &fakeSession{
		inputNames: []string{"input_ids", "attention_mask"},
		ids:        map[int64]int{},
		err:        errors.New("runtime exploded"),
	}
	b := &ONNXBackend{
		tokenizer: fakeTokenizer{tokens: tokens},
		session:   sess,
		labels:    []string{"O", "B-PER", "I-PER"},
	}

	got, err := b.Recognize(context.Background(), text)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got != nil {
		t.Fatalf("got %d entities on error, want none", len(got))
	}
}

func TestONNXBackendRecognizePropagatesTokenizerError(t *testing.T) {
	b := &ONNXBackend{
		tokenizer: fakeTokenizer{err: errors.New("bad tokenizer")},
		session:   &fakeSession{inputNames: []string{"input_ids", "attention_mask"}},
		labels:    []string{"O"},
	}
	if _, err := b.Recognize(context.Background(), "hello"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestNewONNXBackendValidatesConfig(t *testing.T) {
	if _, err := NewONNXBackend(ONNXConfig{}); err == nil {
		t.Fatal("expected an error for an empty config")
	}
}

// closingTokenizer counts Close calls, standing in for the native tokenizer.
type closingTokenizer struct {
	fakeTokenizer
	closes int
	err    error
}

func (c *closingTokenizer) Close() error {
	c.closes++
	return c.err
}

// closingSession counts Close calls, standing in for the native session.
type closingSession struct {
	fakeSession
	closes int
}

func (c *closingSession) Close() error {
	c.closes++
	return nil
}

func TestONNXBackendCloseReleasesSessionAndTokenizer(t *testing.T) {
	tok := &closingTokenizer{}
	sess := &closingSession{fakeSession: fakeSession{inputNames: []string{"input_ids"}}}
	b := &ONNXBackend{tokenizer: tok, session: sess, labels: []string{"O"}}

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if tok.closes != 1 || sess.closes != 1 {
		t.Fatalf("closes: tokenizer=%d session=%d, want 1 and 1", tok.closes, sess.closes)
	}
}

func TestONNXBackendCloseReportsTokenizerErrorAndStillClosesSession(t *testing.T) {
	closeErr := errors.New("tokenizer close failed")
	tok := &closingTokenizer{err: closeErr}
	sess := &closingSession{fakeSession: fakeSession{inputNames: []string{"input_ids"}}}
	b := &ONNXBackend{tokenizer: tok, session: sess, labels: []string{"O"}}

	if err := b.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close error = %v, want %v", err, closeErr)
	}
	if sess.closes != 1 {
		t.Fatalf("session closes = %d, want 1 after a tokenizer close error", sess.closes)
	}
}
