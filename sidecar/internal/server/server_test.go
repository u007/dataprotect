package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/u007/dataprotect/sidecar/internal/ner"
)

func TestHealthz(t *testing.T) {
	srv := New("127.0.0.1:0", ner.NewProseBackend())
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
	srv := New("127.0.0.1:0", ner.NewProseBackend())
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
	srv := New("127.0.0.1:0", ner.NewProseBackend())
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
	srv := New("127.0.0.1:0", ner.NewProseBackend())
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/ner", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: %d", rr.Code)
	}
}
