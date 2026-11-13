package nlp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"encoding/json"
)

// TestNLPClientTimeoutMarksUnhealthy verifies that a client-side HTTP timeout
// (a hung sidecar) still goes through fail(): Health() becomes false and the
// error is returned. Only a caller-ctx cancel is exempt.
func TestNLPClientTimeoutMarksUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"entities": []any{}})
	}))
	defer srv.Close()

	r := New(srv.URL, WithTimeout(50*time.Millisecond))
	_, err := r.Recognize(context.Background(), "x")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if r.Health() {
		t.Fatal("health should be false after a client timeout")
	}
}

// Under best-effort a client timeout is still a sidecar failure: logged and
// dropped (nil, nil) but Health() stays false.
func TestNLPClientTimeoutBestEffort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"entities": []any{}})
	}))
	defer srv.Close()

	r := New(srv.URL, WithTimeout(50*time.Millisecond), WithNLPBestEffort())
	got, err := r.Recognize(context.Background(), "x")
	if err != nil || got != nil {
		t.Fatalf("best-effort: got %v, %v; want nil, nil", got, err)
	}
	if r.Health() {
		t.Fatal("health should be false after a client timeout")
	}
	_ = context.Background
}
