package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/u007/dataprotect/sidecar/internal/ner"
)

const maxBodyBytes = 1 << 20

type Server struct {
	addr    string
	backend ner.NERBackend
}

func New(addr string, backend ner.NERBackend) *Server {
	return &Server{addr: addr, backend: backend}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/ner", s.handleNER)
	return mux
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

func (s *Server) Run(ctx context.Context) error {
	hs := &http.Server{
		Addr:              s.addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(shutdownCtx)
	}()
	log.Printf("piiscrub-sidecar listening on %s", s.addr)
	if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen: %w", err)
	}
	return nil
}
