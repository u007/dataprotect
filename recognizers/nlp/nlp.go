// Package nlp provides a Recognizer that delegates named-entity recognition
// to an external NLP sidecar over HTTP. The root module stays stdlib-only;
// this client uses only net/http and encoding/json.
package nlp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/u007/dataprotect/recognizers"
)

// Option configures an NLPRecognizer.
type Option func(*NLPRecognizer)

// WithClient sets the HTTP client used for requests.
func WithClient(c *http.Client) Option {
	return func(r *NLPRecognizer) {
		r.client = c
	}
}

// WithHTTPClient is an alias for WithClient.
func WithHTTPClient(c *http.Client) Option {
	return WithClient(c)
}

// WithTimeout sets the request timeout.
func WithTimeout(d time.Duration) Option {
	return func(r *NLPRecognizer) {
		r.timeout = d
	}
}

// WithLogger sets the logger used for best-effort logging.
func WithLogger(l *slog.Logger) Option {
	return func(r *NLPRecognizer) {
		if l != nil {
			r.logger = l
		}
	}
}

// WithBestEffort enables best-effort mode: errors are logged (rate-limited)
// and dropped so the rest of the pipeline keeps running.
func WithBestEffort() Option {
	return func(r *NLPRecognizer) {
		r.bestEffort = true
	}
}

// WithNLPBestEffort is an alias for WithBestEffort.
func WithNLPBestEffort() Option {
	return WithBestEffort()
}

// NLPRecognizer is a Recognizer that calls an external sidecar's /v1/ner
// endpoint. An empty URL disables it (Recognize returns nil, nil).
type NLPRecognizer struct {
	url        string
	client     *http.Client
	timeout    time.Duration
	bestEffort bool
	logger     *slog.Logger

	healthy atomic.Bool

	lastErr atomic.Pointer[error]

	lastLog     atomic.Int64
	logInterval time.Duration

	mu sync.Mutex
}

// New creates an NLPRecognizer targeting the given base URL.
func New(url string, opts ...Option) *NLPRecognizer {
	r := &NLPRecognizer{
		url:         strings.TrimRight(url, "/"),
		client:      &http.Client{Timeout: 10 * time.Second},
		timeout:     10 * time.Second,
		logger:      slog.Default(),
		logInterval: 10 * time.Second,
	}
	for _, o := range opts {
		o(r)
	}
	if r.client == nil {
		r.client = &http.Client{Timeout: r.timeout}
	}
	if r.logger == nil {
		r.logger = slog.Default()
	}
	if r.timeout > 0 {
		r.client.Timeout = r.timeout
	}
	r.healthy.Store(r.url != "")
	return r
}

// Name implements recognizers.Recognizer.
func (r *NLPRecognizer) Name() string { return "nlp" }

// Disabled reports whether the recognizer has no configured URL.
func (r *NLPRecognizer) Disabled() bool { return r.url == "" }

// Health reports the last known health state.
func (r *NLPRecognizer) Health() bool { return r.healthy.Load() }

// LastError returns the last error encountered, if any.
func (r *NLPRecognizer) LastError() error {
	if p := r.lastErr.Load(); p != nil {
		return *p
	}
	return nil
}

// Recognize implements recognizers.Recognizer.
func (r *NLPRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	if r.url == "" {
		return nil, nil
	}

	endpoint := r.url
	if !strings.HasSuffix(endpoint, "/v1/ner") {
		endpoint = endpoint + "/v1/ner"
	}

	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return r.fail(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return r.fail(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return r.fail(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		err := fmt.Errorf("nlp sidecar: %s: %s", resp.Status, strings.TrimSpace(string(b)))
		return r.fail(err)
	}

	var out struct {
		Entities []struct {
			Type       string  `json:"type"`
			Start      int     `json:"start"`
			End        int     `json:"end"`
			Text       string  `json:"text"`
			Confidence float64 `json:"confidence"`
		} `json:"entities"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return r.fail(err)
	}

	entities := make([]recognizers.Entity, 0, len(out.Entities))
	for _, e := range out.Entities {
		et, err := recognizers.EntityTypeFromString(e.Type)
		if err != nil {
			// Unknown type: skip rather than fail the whole request.
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       et,
			Start:      e.Start,
			End:        e.End,
			Text:       e.Text,
			Confidence: e.Confidence,
		})
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Start < entities[j].Start
	})

	r.healthy.Store(true)
	r.lastErr.Store(nil)
	return entities, nil
}

func (r *NLPRecognizer) fail(err error) ([]recognizers.Entity, error) {
	r.healthy.Store(false)
	r.lastErr.Store(&err)
	if r.bestEffort {
		r.log(err)
		return nil, nil
	}
	return nil, err
}

func (r *NLPRecognizer) log(err error) {
	now := time.Now().UnixNano()
	last := r.lastLog.Load()
	if last != 0 && now-last < r.logInterval.Nanoseconds() {
		return
	}
	if !r.lastLog.CompareAndSwap(last, now) {
		return
	}
	r.logger.Warn("nlp sidecar request failed", "err", err)
}

// FromEnv builds an NLPRecognizer from PII_SCRUB_NLP_URL. Returns a disabled
// recognizer (nil-safe) when the env var is unset/empty.
func FromEnv() *NLPRecognizer {
	// Avoid importing os in the root? stdlib os is fine.
	return New(getEnv("PII_SCRUB_NLP_URL"))
}

func getEnv(key string) string {
	// imported via os
	return os.Getenv(key)
}
