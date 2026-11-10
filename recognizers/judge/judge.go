// Package judge provides an LLM-backed Judge that confirms weak candidates
// (e.g. bare 12-digit MyKad with no keyword) against a local OpenAI-compatible
// HTTP endpoint. The root module stays stdlib-only: only net/http and
// encoding/json are used.
//
// The judge is opt-in: FromEnv returns nil, nil when PII_SCRUB_NLP_JUDGE_URL
// is unset. URLs must point at loopback unless PII_SCRUB_NLP_JUDGE_ALLOW_REMOTE
// is set. Results are cached in memory (no persistence) and keyed by model,
// entity type, candidate, and context window.
package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/u007/dataprotect/recognizers"
)

// defaultTimeout is the HTTP request timeout when WithTimeout is not set.
const defaultTimeout = 10 * time.Second

// defaultModel is the model name sent to the endpoint when none is configured.
const defaultModel = "qwen3-0.6b"

// cacheMax is the maximum number of cached judge results before FIFO eviction.
const cacheMax = 1000

// thinkOpen and thinkClose are the Qwen3 thinking-block delimiters. Responses
// may contain reasoning between them; we strip it before parsing the yes/no
// answer.
const thinkOpen = "### Start of Thinking"
const thinkClose = "### End of Thinking"

// Option configures a Judge.
type Option func(*Judge)

// WithClient sets the HTTP client used for requests.
func WithClient(c *http.Client) Option {
	return func(j *Judge) {
		if c != nil {
			j.client = c
		}
	}
}

// WithTimeout sets the request timeout. Only takes effect when WithClient is
// not also used; the client created by New inherits this timeout.
func WithTimeout(d time.Duration) Option {
	return func(j *Judge) {
		if d > 0 {
			j.timeout = d
		}
	}
}

// WithLogger sets the logger used for best-effort logging.
func WithLogger(l *slog.Logger) Option {
	return func(j *Judge) {
		if l != nil {
			j.logger = l
		}
	}
}

// WithModel sets the model name sent in the request body.
func WithModel(name string) Option {
	return func(j *Judge) {
		if name != "" {
			j.model = name
		}
	}
}

// Judge implements analyzer.Judge by calling a local OpenAI-compatible chat
// completions endpoint. It confirms weak candidates before they reach the
// confidence filter.
type Judge struct {
	baseURL string
	client  *http.Client
	timeout time.Duration
	model   string
	logger  *slog.Logger

	cache judgeCache
}

// New creates a Judge targeting the given base URL. The URL must point at
// loopback (127.0.0.1, localhost, ::1) unless PII_SCRUB_NLP_JUDGE_ALLOW_REMOTE
// is set. A non-loopback URL without the escape hatch returns an error.
func New(baseURL string, opts ...Option) (*Judge, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("judge: parse url %q: %w", baseURL, err)
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	if u.Host == "" {
		return nil, fmt.Errorf("judge: url %q has no host", baseURL)
	}
	if !isLoopbackHost(u.Hostname()) && getEnv("PII_SCRUB_NLP_JUDGE_ALLOW_REMOTE") != "1" {
		return nil, fmt.Errorf("judge: url %q is not loopback; set PII_SCRUB_NLP_JUDGE_ALLOW_REMOTE=1 to override", baseURL)
	}

	j := &Judge{
		baseURL: strings.TrimRight(baseURL, "/"),
		timeout: defaultTimeout,
		model:   defaultModel,
		logger:  slog.Default(),
		cache:   newJudgeCache(),
	}
	for _, o := range opts {
		o(j)
	}
	if j.client == nil {
		j.client = &http.Client{Timeout: j.timeout}
	}
	if j.client.Timeout == 0 {
		j.client.Timeout = j.timeout
	}
	// Always refuse redirects, even for a caller-supplied client, so the
	// loopback gate cannot be bypassed via a 30x.
	j.client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return j, nil
}

// FromEnv builds a Judge from PII_SCRUB_NLP_JUDGE_URL. Returns nil, nil when
// the variable is unset or empty (disabled). PII_SCRUB_NLP_JUDGE_MODEL sets
// the model name; PII_SCRUB_NLP_JUDGE_ALLOW_REMOTE=1 permits non-loopback URLs.
func FromEnv() (*Judge, error) {
	url := getEnv("PII_SCRUB_NLP_JUDGE_URL")
	if url == "" {
		return nil, nil
	}
	model := getEnv("PII_SCRUB_NLP_JUDGE_MODEL")
	opts := []Option{}
	if model != "" {
		opts = append(opts, WithModel(model))
	}
	return New(url, opts...)
}

// Judge confirms a weak candidate. It returns true when the candidate is a real
// entity of the given type, false when it is a look-alike. An error aborts the
// caller's Analyze — there is no silent fallback.
func (j *Judge) Judge(ctx context.Context, entityType recognizers.EntityType, candidate, window string) (bool, error) {
	key := cacheKey(entityType, candidate, window, j.model)
	if hit, ok := j.cache.get(key); ok {
		j.logger.Debug("judge cache hit", "confirmed", hit)
		return hit, nil
	}

	confirmed, err := j.request(ctx, entityType, candidate, window)
	if err != nil {
		return false, err
	}
	j.cache.set(key, confirmed)
	return confirmed, nil
}

// request sends the judge prompt to the endpoint and parses the yes/no answer.
// It returns an error on any failure — never a silent false.
func (j *Judge) request(ctx context.Context, entityType recognizers.EntityType, candidate, window string) (bool, error) {
	body := chatRequest{
		Model:          j.model,
		EnableThinking: false,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt()},
			{Role: "user", Content: userPrompt(entityType, candidate, window)},
		},
		Temperature: 0,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return false, fmt.Errorf("judge: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.baseURL+"/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return false, fmt.Errorf("judge: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := j.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("judge: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return false, fmt.Errorf("judge: endpoint returned %s", resp.Status)
	}

	var out chatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return false, fmt.Errorf("judge: decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return false, fmt.Errorf("judge: response has no choices")
	}
	content := stripThinking(out.Choices[0].Message.Content)
	answer := strings.TrimSpace(strings.ToLower(content))
	switch answer {
	case "yes":
		return true, nil
	case "no":
		return false, nil
	default:
		return false, fmt.Errorf("judge: unparseable answer %q", out.Choices[0].Message.Content)
	}
}

// systemPrompt returns the system instruction for the judge.
func systemPrompt() string {
	return "You are a precise PII judge. Answer only yes or no."
}

// userPrompt builds the user message with the candidate and its context window.
// The window is delimited with quotes so the judge cannot confuse it with the
// candidate itself.
func userPrompt(entityType recognizers.EntityType, candidate, window string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Is the following a %s?\n\n", string(entityType)))
	b.WriteString("Candidate: " + candidate + "\n")
	b.WriteString("Context (verbatim, may contain PII): \"" + window + "\"\n")
	b.WriteString("Answer yes or no only.")
	return b.String()
}

// stripThinking removes Qwen3-style thinking blocks from the response content.
// If no block is found, the content is returned unchanged.
func stripThinking(content string) string {
	start := strings.Index(content, thinkOpen)
	if start < 0 {
		return content
	}
	end := strings.Index(content[start:], thinkClose)
	if end < 0 {
		// Unclosed thinking block: drop from the opening tag onward.
		return content[:start]
	}
	return content[:start] + content[start+end+len(thinkClose):]
}

// judgeCacheKey is a comparable cache key for judge results. Using a struct
// (rather than a concatenated string) avoids delimiter collisions between
// candidate and window text.
type judgeCacheKey struct {
	model      string
	entityType recognizers.EntityType
	candidate  string
	window     string
}

// cacheKey builds the cache key from the judge inputs.
func cacheKey(entityType recognizers.EntityType, candidate, window, model string) judgeCacheKey {
	return judgeCacheKey{
		model:      model,
		entityType: entityType,
		candidate:  candidate,
		window:     window,
	}
}

// isLoopbackHost reports whether host is a loopback address.
func isLoopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// getEnv reads an environment variable.
func getEnv(key string) string {
	return os.Getenv(key)
}

// chatRequest is the OpenAI-compatible chat/completions request body.
type chatRequest struct {
	Model          string        `json:"model"`
	EnableThinking bool          `json:"enable_thinking"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
}

// chatMessage is a single chat message.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the OpenAI-compatible chat/completions response body.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// judgeCache is a bounded, FIFO-evicted in-memory cache of judge results.
// It holds no persistent copy — only the current process memory.
type judgeCache struct {
	mu    sync.Mutex
	items map[judgeCacheKey]bool
	order []judgeCacheKey
}

func newJudgeCache() judgeCache {
	return judgeCache{
		items: make(map[judgeCacheKey]bool),
		order: make([]judgeCacheKey, 0, cacheMax),
	}
}

func (c *judgeCache) get(key judgeCacheKey) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.items[key]
	return v, ok
}

func (c *judgeCache) set(key judgeCacheKey, value bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[key]; exists {
		c.items[key] = value
		return
	}
	if len(c.items) >= cacheMax {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
	}
	c.items[key] = value
	c.order = append(c.order, key)
}
