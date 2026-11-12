package nlp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/u007/dataprotect/recognizers"
)

func TestNLPDisabled(t *testing.T) {
	r := New("")
	got, err := r.Recognize(context.Background(), "John Smith")
	if err != nil || got != nil {
		t.Fatalf("disabled: got %v, %v; want nil, nil", got, err)
	}
	if r.Name() != "nlp" {
		t.Fatalf("name: %s", r.Name())
	}
	if r.Health() {
		t.Fatal("disabled should not be healthy")
	}
}

func TestNLPSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"entities": []map[string]any{
				{"type": "PERSON", "start": 0, "end": 10, "text": "John Smith", "confidence": 0.85},
				{"type": "LOCATION", "start": 20, "end": 28, "text": "New York", "confidence": 0.85},
			},
		})
	}))
	defer srv.Close()

	r := New(srv.URL)
	got, err := r.Recognize(context.Background(), "John Smith in New York")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len: %d", len(got))
	}
	if got[0].Type != recognizers.EntityPerson || got[0].Start != 0 {
		t.Fatalf("got[0]: %+v", got[0])
	}
	if got[1].Type != recognizers.EntityLocation {
		t.Fatalf("got[1]: %+v", got[1])
	}
	if !r.Health() {
		t.Fatal("should be healthy")
	}
	if r.LastError() != nil {
		t.Fatalf("lastErr: %v", r.LastError())
	}
}

func TestNLPSortedByStart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"entities": []map[string]any{
				{"type": "LOCATION", "start": 20, "end": 28, "text": "New York", "confidence": 0.85},
				{"type": "PERSON", "start": 0, "end": 10, "text": "John Smith", "confidence": 0.85},
			},
		})
	}))
	defer srv.Close()
	r := New(srv.URL)
	got, err := r.Recognize(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Start != 0 || got[1].Start != 20 {
		t.Fatalf("not sorted: %+v", got)
	}
}

func TestNLP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("boom"))
	}))
	defer srv.Close()
	r := New(srv.URL)
	_, err := r.Recognize(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if r.Health() {
		t.Fatal("should be unhealthy")
	}
}

func TestNLPMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{not json"))
	}))
	defer srv.Close()
	r := New(srv.URL)
	_, err := r.Recognize(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNLPTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]any{"entities": []any{}})
	}))
	defer srv.Close()
	r := New(srv.URL, WithTimeout(10*time.Millisecond))
	_, err := r.Recognize(context.Background(), "x")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestNLPCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]any{"entities": []any{}})
	}))
	defer srv.Close()
	r := New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.Recognize(ctx, "x")
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestNLPBestEffort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	r := New(srv.URL, WithNLPBestEffort())
	got, err := r.Recognize(context.Background(), "x")
	if err != nil || got != nil {
		t.Fatalf("best-effort: got %v, %v; want nil, nil", got, err)
	}
	// adapter wrapper also swallows
	wrapped := NewBestEffort(New(srv.URL))
	got2, err2 := wrapped.Recognize(context.Background(), "x")
	if err2 != nil || got2 != nil {
		t.Fatalf("wrapped: got %v, %v", got2, err2)
	}
}

func TestNLPFullURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ner" {
			t.Errorf("path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"entities": []any{}})
	}))
	defer srv.Close()
	r := New(srv.URL + "/v1/ner")
	_, err := r.Recognize(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNLPUnknownTypeSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"entities": []map[string]any{
				{"type": "FOO", "start": 0, "end": 1, "text": "x", "confidence": 0.85},
				{"type": "PERSON", "start": 2, "end": 6, "text": "John", "confidence": 0.85},
			},
		})
	}))
	defer srv.Close()
	r := New(srv.URL)
	got, err := r.Recognize(context.Background(), "x John")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len: %d", len(got))
	}
}

func TestManagedFromEnvRequiresBinary(t *testing.T) {
	t.Setenv("PII_SCRUB_NLP_URL", "")
	t.Setenv("PII_SCRUB_NLP_BINARY", "")
	rec, err := ManagedFromEnv(context.Background())
	if err == nil || rec != nil {
		t.Fatalf("unset binary: got %v, %v; want error", rec, err)
	}
	if !strings.Contains(err.Error(), "PII_SCRUB_NLP_BINARY is not set") {
		t.Fatalf("error: %v", err)
	}
}

func TestManagedFromEnvRejectsURLAndBinary(t *testing.T) {
	t.Setenv("PII_SCRUB_NLP_URL", "http://127.0.0.1:8787")
	t.Setenv("PII_SCRUB_NLP_BINARY", os.Args[0])
	rec, err := ManagedFromEnv(context.Background())
	if err == nil || rec != nil {
		t.Fatalf("both set: got %v, %v; want error and no child", rec, err)
	}
	if !strings.Contains(err.Error(), "both set") {
		t.Fatalf("error: %v", err)
	}
}

// TestManagedFromEnvStartsChild wraps the fake sidecar in a shell script,
// because ManagedFromEnv passes no arguments: the script re-execs the test
// binary in fake-sidecar mode and the library appends -addr.
func TestManagedFromEnvStartsChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a /bin/sh wrapper script")
	}
	self, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	args, pidfile := fakeArgs(t, "healthy")
	script := filepath.Join(t.TempDir(), "sidecar.sh")
	body := "#!/bin/sh\nexec '" + self + "'"
	for _, a := range args {
		body += " '" + a + "'"
	}
	body += " \"$@\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}

	t.Setenv("PII_SCRUB_NLP_URL", "")
	t.Setenv("PII_SCRUB_NLP_BINARY", script)
	rec, err := ManagedFromEnv(context.Background())
	if err != nil {
		t.Fatalf("ManagedFromEnv: %v", err)
	}
	if !rec.Health() {
		t.Fatal("recognizer should be healthy after start")
	}
	got, err := rec.Recognize(context.Background(), "John Smith")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(got) != 1 || got[0].Text != "John" {
		t.Fatalf("Recognize: got %+v", got)
	}
	pid := readPid(t, pidfile)
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertReaped(t, pid)
}

func TestFromEnvRejectsBinary(t *testing.T) {
	t.Setenv("PII_SCRUB_NLP_URL", "")
	t.Setenv("PII_SCRUB_NLP_BINARY", "/bin/true")
	if r, err := FromEnv(); err == nil || r != nil {
		t.Fatalf("FromEnv = (%v, %v), want error pointing at ManagedFromEnv", r, err)
	}
}

func TestFromEnvURL(t *testing.T) {
	t.Setenv("PII_SCRUB_NLP_URL", "http://127.0.0.1:1")
	t.Setenv("PII_SCRUB_NLP_BINARY", "")
	r, err := FromEnv()
	if err != nil || r == nil {
		t.Fatalf("FromEnv = (%v, %v), want recognizer", r, err)
	}
}


// TestManagedFromEnvForwardsOptions checks that options passed to
// ManagedFromEnv reach StartManaged: a WithBestEffort recognizer swallows a
// transient transport error and reports nil, nil instead of the raw error.
func TestManagedFromEnvForwardsOptions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a /bin/sh wrapper script")
	}
	self, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	args, pidfile := fakeArgs(t, "crash-on-ner")
	script := filepath.Join(t.TempDir(), "sidecar.sh")
	body := "#!/bin/sh\nexec '" + self + "'"
	for _, a := range args {
		body += " '" + a + "'"
	}
	body += " \"$@\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}

	t.Setenv("PII_SCRUB_NLP_URL", "")
	t.Setenv("PII_SCRUB_NLP_BINARY", script)
	rec, err := ManagedFromEnv(context.Background(), WithBestEffort())
	if err != nil {
		t.Fatalf("ManagedFromEnv: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	waitUntil(t, "sidecar healthy", func() bool { return rec.Health() })

	// WithBestEffort swallowed: a transport error becomes nil, nil.
	if _, err := rec.Recognize(context.Background(), "John Smith"); err != nil {
		t.Fatalf("Recognize under best-effort: got %v, want nil", err)
	}

	pid := readPid(t, pidfile)
	_ = rec.Close()
	assertReaped(t, pid)
}
