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
// recognizer (nil-safe) when the env var is unset/empty. It fails when
// PII_SCRUB_NLP_BINARY is set, because that variable starts a managed child,
// which FromEnv cannot do; call ManagedFromEnv instead.
func FromEnv() (*NLPRecognizer, error) {
	if getEnv("PII_SCRUB_NLP_BINARY") != "" {
		return nil, fmt.Errorf("nlp: PII_SCRUB_NLP_BINARY is set; FromEnv only reads PII_SCRUB_NLP_URL, call ManagedFromEnv to start the managed sidecar")
	}
	return New(getEnv("PII_SCRUB_NLP_URL")), nil
}

// ManagedFromEnv starts the sidecar binary named by PII_SCRUB_NLP_BINARY with
// StartManaged and returns it. The binary path is the opt-in, so one variable
// carries both the switch and the path. It fails when that variable is unset,
// and when PII_SCRUB_NLP_URL is also set, because the URL selects an external
// sidecar and the binary starts a managed child. The caller must Close the
// result. ctx also bounds the child's lifetime: when it ends, the child stops.
//
// opts are forwarded to StartManaged, so callers can configure the recognizer
// (for example WithBestEffort, WithTimeout, WithLogger) the same way they would
// with StartManaged directly.
func ManagedFromEnv(ctx context.Context, opts ...Option) (*ManagedRecognizer, error) {
	url := getEnv("PII_SCRUB_NLP_URL")
	binary := getEnv("PII_SCRUB_NLP_BINARY")
	if binary == "" {
		return nil, fmt.Errorf("nlp managed: PII_SCRUB_NLP_BINARY is not set")
	}
	if url != "" {
		return nil, fmt.Errorf("nlp managed: PII_SCRUB_NLP_URL and PII_SCRUB_NLP_BINARY are both set; set only one (URL uses an external sidecar, BINARY starts a managed child)")
	}
	return StartManaged(ctx, binary, nil, opts...)
}

func getEnv(key string) string {
	// imported via os
	return os.Getenv(key)
}
