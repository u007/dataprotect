package ner

import (
	"context"
	"sync"
	"testing"
)

// TestProseBackendConcurrent checks that the shared model is safe for concurrent
// requests. The model is built once with sync.Once and is read-only during
// inference, so concurrent use must be safe; this test is the proof.
func TestProseBackendConcurrent(t *testing.T) {
	b := NewProseBackend()
	text := "Contact Ingrid Larsen at Northgate Ventures in Copenhagen about invoice 12345."
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				ents, err := b.Recognize(context.Background(), text)
				if err != nil {
					t.Error(err)
					return
				}
				if len(ents) != 3 {
					t.Errorf("got %d entities, want 3", len(ents))
					return
				}
			}
		}()
	}
	wg.Wait()
}
