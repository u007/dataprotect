package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/u007/dataprotect"
	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/sidecar/internal/ner"
)

func newTestServer() *Server {
	return New("127.0.0.1:0", ner.NewProseBackend(), piiscrub.NewScrubber())
}

func TestHealthz(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body["status"] != "ok" || body["backend"] != "prose" {
		t.Fatalf("body: %v", body)
	}
}

func TestNEREndpoint(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()
	payload := []byte(`{"text":"John Smith works at Acme Corp in New York."}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/ner", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Entities []struct {
			Type  string `json:"type"`
			Start int    `json:"start"`
			End   int    `json:"end"`
			Text  string `json:"text"`
		} `json:"entities"`
	}
	json.Unmarshal(rr.Body.Bytes(), &out)
	if len(out.Entities) == 0 {
		t.Fatal("no entities")
	}
	foundPerson := false
	for _, e := range out.Entities {
		if e.Type == "PERSON" && e.Text == "John Smith" {
			foundPerson = true
		}
	}
	if !foundPerson {
		t.Fatalf("PERSON not found: %+v", out.Entities)
	}
}

func TestNERMaxBytes(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()
	big := bytes.Repeat([]byte("a"), (1<<20)+100)
	payload := append([]byte(`{"text":"`), big...)
	payload = append(payload, []byte(`"}`)...)
	req := httptest.NewRequest(http.MethodPost, "/v1/ner", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code == http.StatusOK {
		t.Fatal("expected error for oversized body")
	}
}

func TestNERMethodNotAllowed(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/ner", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestScrubEndpoint(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()
	payload := []byte(`{"text":"Contact John Smith at john.smith@example.com or +1-202-555-0123."}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/scrub", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Text string `json:"text"`
	}
	json.Unmarshal(rr.Body.Bytes(), &out)
	if strings.Contains(out.Text, "john.smith@example.com") {
		t.Fatalf("email not redacted: %s", out.Text)
	}
	if strings.Contains(out.Text, "+1-202-555-0123") {
		t.Fatalf("phone not redacted: %s", out.Text)
	}
}

func TestScrubMethodNotAllowed(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/scrub", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestPseudonymizeEndpoint(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()
	payload := []byte(`{"text":"Contact John Smith at john.smith@example.com."}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/pseudonymize", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Text    string            `json:"text"`
		Mapping map[string]string `json:"mapping"`
	}
	json.Unmarshal(rr.Body.Bytes(), &out)
	if strings.Contains(out.Text, "john.smith@example.com") {
		t.Fatalf("email not pseudonymized: %s", out.Text)
	}
	if len(out.Mapping) == 0 {
		t.Fatal("no mapping entries")
	}
}

func TestPseudonymizeMethodNotAllowed(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/pseudonymize", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: %d", rr.Code)
	}
}

type blockingBackend struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingBackend) Name() string { return "blocking" }

func (b *blockingBackend) Recognize(context.Context, string) ([]recognizers.Entity, error) {
	close(b.started)
	<-b.release
	return nil, nil
}

func TestHandlerWaitsForInflightBeforeDrain(t *testing.T) {
	b := &blockingBackend{started: make(chan struct{}), release: make(chan struct{})}
	srv := New("127.0.0.1:0", b, piiscrub.NewScrubber())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqDone := make(chan struct{})
	go func() {
		defer close(reqDone)
		resp, err := http.Post(ts.URL+"/v1/ner", "application/json", strings.NewReader(`{"text":"x"}`))
		if err != nil {
			t.Errorf("post: %v", err)
			return
		}
		resp.Body.Close()
	}()

	<-b.started

	drained := make(chan struct{})
	go func() {
		srv.inflight.Wait()
		close(drained)
	}()

	select {
	case <-drained:
		t.Fatal("drain returned while a handler was still running")
	case <-time.After(50 * time.Millisecond):
	}

	close(b.release)
	<-reqDone
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("drain did not return after the handler finished")
	}
}

// stuckBackend blocks in Recognize and ignores ctx until release is closed.
type stuckBackend struct {
	started chan struct{}
	release chan struct{}
}

func (b *stuckBackend) Name() string { return "stuck" }

func (b *stuckBackend) Recognize(context.Context, string) ([]recognizers.Entity, error) {
	close(b.started)
	<-b.release
	return nil, nil
}

// freeAddr returns a loopback address with no listener, for Run to bind.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("probe close: %v", err)
	}
	return addr
}

func TestRunReturnsWhenInflightHandlerIgnoresContext(t *testing.T) {
	b := &stuckBackend{started: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() { close(b.release) })
	addr := freeAddr(t)
	srv := New(addr, b, piiscrub.NewScrubber())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- srv.Run(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	go func() {
		// The error is expected: the server force-closes this connection on shutdown.
		resp, err := http.Post("http://"+addr+"/v1/ner", "application/json", strings.NewReader(`{"text":"x"}`))
		if err == nil {
			resp.Body.Close()
		}
	}()
	select {
	case <-b.started:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never started")
	}

	cancel()
	// Shutdown waits shutdownTimeout, then the in-flight wait waits another
	// shutdownTimeout before Run abandons the stuck handler.
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2*shutdownTimeout + 5*time.Second):
		t.Fatal("Run did not return; a handler ignoring ctx blocked shutdown")
	}
}
