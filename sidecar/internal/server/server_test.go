package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/u007/dataprotect"
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
