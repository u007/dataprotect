package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/u007/dataprotect"
	"github.com/u007/dataprotect/sidecar/internal/ner"
)

const maxBodyBytes = 1 << 20

// shutdownTimeout is the drain budget: Shutdown gets it to finish active
// requests, and Run gets it again to wait for in-flight handlers to return.
const shutdownTimeout = 5 * time.Second

type Server struct {
	addr     string
	backend  ner.NERBackend
	scrubber *piiscrub.Scrubber
	inflight sync.WaitGroup
}

func New(addr string, backend ner.NERBackend, scrubber *piiscrub.Scrubber) *Server {
	return &Server{addr: addr, backend: backend, scrubber: scrubber}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/ner", s.handleNER)
	mux.HandleFunc("/v1/scrub", s.handleScrub)
	mux.HandleFunc("/v1/pseudonymize", s.handlePseudonymize)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.inflight.Add(1)
		defer s.inflight.Done()
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"backend": s.backend.Name(),
	})
}

type nerRequest struct {
	Text string `json:"text"`
}

type nerEntity struct {
	Type       string  `json:"type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
}

type nerResponse struct {
	Entities []nerEntity `json:"entities"`
}

func (s *Server) handleNER(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req nerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	entities, err := s.backend.Recognize(r.Context(), req.Text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := nerResponse{Entities: make([]nerEntity, 0, len(entities))}
	for _, e := range entities {
		resp.Entities = append(resp.Entities, nerEntity{
			Type:       string(e.Type),
			Start:      e.Start,
			End:        e.End,
			Text:       e.Text,
			Confidence: e.Confidence,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type scrubRequest struct {
	Text string `json:"text"`
}

type scrubResponse struct {
	Text string `json:"text"`
}

type pseudonymizeRequest struct {
	Text string `json:"text"`
}

type pseudonymizeResponse struct {
	Text    string            `json:"text"`
	Mapping map[string]string `json:"mapping"`
}

func (s *Server) handleScrub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req scrubRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	redacted, err := s.scrubber.Anonymize(r.Context(), req.Text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := scrubResponse{Text: redacted}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handlePseudonymize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req pseudonymizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	redacted, mapping, err := s.scrubber.Pseudonymize(r.Context(), req.Text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := pseudonymizeResponse{Text: redacted, Mapping: mapping}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) Run(ctx context.Context) error {
	hs := &http.Server{
		Addr:              s.addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := hs.Shutdown(shutdownCtx); err != nil {
			log.Printf("piiscrub-sidecar: shutdown: %v; closing active connections", err)
			if cerr := hs.Close(); cerr != nil {
				log.Printf("piiscrub-sidecar: close: %v", cerr)
			}
		}
	}()
	log.Printf("piiscrub-sidecar listening on %s", s.addr)
	if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen: %w", err)
	}
	// ListenAndServe returns as soon as Shutdown closes the listener, before
	// in-flight handlers have drained. Wait for Shutdown, then for every
	// handler, but only for shutdownTimeout: a handler whose backend ignores
	// ctx would otherwise block here forever. Abandoned handlers are logged
	// and Run returns so the caller can release backend resources.
	<-shutdownDone
	drained := make(chan struct{})
	go func() {
		s.inflight.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(shutdownTimeout):
		log.Printf("piiscrub-sidecar: in-flight handlers did not finish within %s; abandoning them", shutdownTimeout)
	}
	return nil
}
