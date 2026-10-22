package piiscrub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/nlp"
)

// nlpCombinationConfidence mirrors sidecar/internal/ner.DefaultConfidence. The
// root module cannot import that constant (the sidecar module imports the root
// module), so the value is repeated here. TestNLPConfidenceWindow below fails
// if it leaves the usable window, and the sidecar's own
// TestDefaultConfidenceWindow guards the same number from the other side.
const nlpCombinationConfidence = 0.80

type fakeNEREntity struct {
	Type       string  `json:"type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
}

type fakeNERResponse struct {
	Entities []fakeNEREntity `json:"entities"`
}

// fakeNERServer serves a sidecar-shaped /v1/ner endpoint. Each spec names a
// substring of the request text; the handler locates it and returns that span
// so the offsets are always correct for the text under test.
func fakeNERServer(t *testing.T, specs ...fakeNEREntity) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ner" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		out := fakeNERResponse{Entities: make([]fakeNEREntity, 0, len(specs))}
		for _, spec := range specs {
			start := strings.Index(req.Text, spec.Text)
			if start < 0 {
				continue
			}
			out.Entities = append(out.Entities, fakeNEREntity{
				Type:       spec.Type,
				Start:      start,
				End:        start + len(spec.Text),
				Text:       spec.Text,
				Confidence: spec.Confidence,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(out); err != nil {
			t.Errorf("encoding fake NER response: %v", err)
		}
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func analyzeWithNLP(t *testing.T, url, text string) []recognizers.Entity {
	t.Helper()
	s := NewScrubberWithRecognizers(nlp.New(url))
	entities, err := s.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return entities
}

// TestNLPConfidenceWindow asserts the documented NLP score is usable with the
// default pipeline: above the 0.70 floor and strictly below the 0.85 lowest
// built-in recognizer.
func TestNLPConfidenceWindow(t *testing.T) {
	const (
		defaultScrubberFloor = 0.70
		lowestBuiltin        = 0.85
	)
	if nlpCombinationConfidence <= defaultScrubberFloor {
		t.Fatalf("NLP confidence %v must be above the default scrubber floor %v", nlpCombinationConfidence, defaultScrubberFloor)
	}
	if nlpCombinationConfidence >= lowestBuiltin {
		t.Fatalf("NLP confidence %v must be below the lowest built-in recognizer %v", nlpCombinationConfidence, lowestBuiltin)
	}
}

// TestNLPCombinationConfidenceOrdering is the combination gate for the NLP
// path: NLP entities must survive the default floor where the traditional
// pipeline has nothing, and must lose to a typed detection when they overlap.
func TestNLPCombinationConfidenceOrdering(t *testing.T) {
	t.Run("NLP fills a gap the traditional pipeline misses", func(t *testing.T) {
		srv := fakeNERServer(t, fakeNEREntity{
			Type: "PERSON", Text: "John Smith", Confidence: nlpCombinationConfidence,
		})

		entities := analyzeWithNLP(t, srv.URL, "John Smith lives here")

		if len(entities) != 1 {
			t.Fatalf("got %d entities, want 1: %+v", len(entities), entities)
		}
		if entities[0].Type != recognizers.EntityPerson {
			t.Errorf("type = %s, want PERSON", entities[0].Type)
		}
		if entities[0].Text != "John Smith" {
			t.Errorf("text = %q, want %q", entities[0].Text, "John Smith")
		}
	})

	t.Run("NLP does not override a typed EMAIL on overlap", func(t *testing.T) {
		// The fake PERSON starts one byte into the email address so the two
		// spans overlap without sharing a start; the analyzer sorts by start,
		// and share-a-start ties would depend on sort.Slice's unstable order.
		srv := fakeNERServer(t, fakeNEREntity{
			Type: "PERSON", Text: "ohn@example.com", Confidence: nlpCombinationConfidence,
		})

		entities := analyzeWithNLP(t, srv.URL, "contact john@example.com now")

		if len(entities) != 1 {
			t.Fatalf("got %d entities, want 1: %+v", len(entities), entities)
		}
		if entities[0].Type != recognizers.EntityEmail {
			t.Errorf("type = %s, want EMAIL (typed detection must win)", entities[0].Type)
		}
	})

	t.Run("NLP below the default floor is dropped", func(t *testing.T) {
		srv := fakeNERServer(t, fakeNEREntity{
			Type: "PERSON", Text: "John Smith", Confidence: 0.69,
		})

		entities := analyzeWithNLP(t, srv.URL, "John Smith lives here")

		if len(entities) != 0 {
			t.Fatalf("got %d entities, want 0 (0.69 is below the 0.70 floor): %+v", len(entities), entities)
		}
	})

	t.Run("NLP exactly at the floor is kept", func(t *testing.T) {
		srv := fakeNERServer(t, fakeNEREntity{
			Type: "PERSON", Text: "John Smith", Confidence: 0.70,
		})

		entities := analyzeWithNLP(t, srv.URL, "John Smith lives here")

		if len(entities) != 1 || entities[0].Type != recognizers.EntityPerson {
			t.Fatalf("got %+v, want one PERSON (the filter keeps confidence >= 0.70)", entities)
		}
	})

	t.Run("public threshold above the NLP score filters NLP out", func(t *testing.T) {
		srv := fakeNERServer(t, fakeNEREntity{
			Type: "PERSON", Text: "John Smith", Confidence: nlpCombinationConfidence,
		})

		// The documented conservative mode: keep only entities at or above
		// 0.85, which excludes every NLP entity (0.80) and leaves the typed
		// built-ins alone.
		s := NewScrubber(
			WithRecognizer(nlp.New(srv.URL)),
			WithMinConfidence(0.85),
		)
		entities, err := s.Analyze(context.Background(), "John Smith lives here")
		if err != nil {
			t.Fatalf("Analyze: %v", err)
		}
		if len(entities) != 0 {
			t.Fatalf("got %d entities, want 0 (threshold 0.85 > NLP %v): %+v",
				len(entities), nlpCombinationConfidence, entities)
		}
	})
}
