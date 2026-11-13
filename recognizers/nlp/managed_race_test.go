//go:build !windows

package nlp

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// A request in flight when the StartManaged ctx is cancelled must fail rather
// than hang. Mirrors TestManagedRecognizeInFlightDuringCloseReturnsClosed.
func TestManagedRecognizeInFlightDuringCtxCancelReturnsClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	args, _ := fakeArgs(t, "slow-ner")
	rec, err := startManaged(ctx, os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	t.Cleanup(func() { cancel(); _ = rec.Close() })

	result := make(chan error, 1)
	go func() {
		_, err := rec.Recognize(context.Background(), "John Smith")
		result <- err
	}()
	waitUntil(t, "request to reach the sidecar", func() bool {
		return strings.Contains(rec.proc.stderr.String(), "request in flight")
	})

	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatalf("in-flight Recognize: err = nil; want closed/exited error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight Recognize did not return after ctx cancel")
	}
}

// Cancelling the per-call context must abort Recognize with context.Canceled
// while leaving the recognizer usable for later calls.
func TestManagedRecognizePerCallCtxCancelReturnsCanceled(t *testing.T) {
	args, _ := fakeArgs(t, "slow-ner")
	rec, err := startManaged(context.Background(), os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := rec.Recognize(ctx, "John Smith")
		result <- err
	}()
	waitUntil(t, "request to reach the sidecar", func() bool {
		return strings.Contains(rec.proc.stderr.String(), "request in flight")
	})
	cancel()

	select {
	case err := <-result:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("per-call cancel: err = %v; want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("per-call cancel did not return")
	}

	if !rec.Health() {
		t.Fatalf("recognizer reported unhealthy after a client-side cancel")
	}
}

// Many concurrent Recognize calls racing a Close must all return promptly
// without hanging or panicking. Run under -race for contention coverage.
func TestManagedRecognizeConcurrentCloseNoHang(t *testing.T) {
	args, _ := fakeArgs(t, "slow-ner")
	rec, err := startManaged(context.Background(), os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}

	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := rec.Recognize(context.Background(), "John Smith")
			errs <- err
		}()
	}
	waitUntil(t, "requests to reach the sidecar", func() bool {
		return strings.Contains(rec.proc.stderr.String(), "request in flight")
	})
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	for i := 0; i < n; i++ {
		select {
		case <-errs:
		case <-time.After(10 * time.Second):
			t.Fatalf("goroutine %d did not return after Close", i)
		}
	}
}
