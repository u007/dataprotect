package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// fakeJudgeServer is an httptest server that answers yes/no based on a handler.
type fakeJudgeServer struct {
	*httptest.Server
	mu       sync.Mutex
	calls    int
	answers  map[string]bool // candidate -> answer
	garbage  bool            // return unparseable content
	fail500  bool            // return HTTP 500
	thinkTag bool            // wrap yes inside Qwen3 think tags
}

func newFakeJudgeServer(t *testing.T, handler http.HandlerFunc) *fakeJudgeServer {
	srv := &fakeJudgeServer{
		Server:  httptest.NewServer(handler),
		answers: make(map[string]bool),
	}
	t.Cleanup(srv.Server.Close)
	return srv
}

func (s *fakeJudgeServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestJudgeConfirmed(t *testing.T) {
	srv := newFakeJudgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"content": "yes",
					},
				},
			},
		})
	})

	j, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	confirmed, err := j.Judge(context.Background(), recognizers.EntityMYIC, "900515103421", "MyKad number on the form")
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if !confirmed {
		t.Errorf("confirmed = false, want true")
	}
}

func TestJudgeRejected(t *testing.T) {
	srv := newFakeJudgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"content": "no",
					},
				},
			},
		})
	})

	j, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	confirmed, err := j.Judge(context.Background(), recognizers.EntityMYIC, "123456789012", "order number shipped")
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if confirmed {
		t.Errorf("confirmed = true, want false")
	}
}

func TestJudgeCaseInsensitive(t *testing.T) {
	srv := newFakeJudgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"content": "YES",
					},
				},
			},
		})
	})

	j, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	confirmed, err := j.Judge(context.Background(), recognizers.EntityMYIC, "900515103421", "MyKad")
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if !confirmed {
		t.Errorf("confirmed = false, want true (case-insensitive YES)")
	}
}

func TestJudgeWithThinkTags(t *testing.T) {
	// Build think-tagged content via concatenation so the XML-like tags
	// survive the edit tool.
	thinkOpen := "### Start of Thinking"
	thinkClose := "### End of Thinking"
	content := thinkOpen + thinkClose + "\nyes"

	srv := newFakeJudgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"content": content,
					},
				},
			},
		})
	})

	j, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	confirmed, err := j.Judge(context.Background(), recognizers.EntityMYIC, "900515103421", "MyKad")
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if !confirmed {
		t.Errorf("confirmed = false, want true (think tags stripped)")
	}
}

func TestStripThinking(t *testing.T) {
	thinkOpen := "### Start of Thinking"
	thinkClose := "### End of Thinking"

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "no tags", input: "yes", want: "yes"},
		{name: "think block", input: "reasoning here " + thinkOpen + thinkClose + "\nyes", want: "reasoning here \nyes"},
		{name: "unclosed think", input: "reasoning " + thinkOpen + " never ends", want: "reasoning "},
		{name: "empty think", input: "before" + thinkOpen + thinkClose + "after", want: "beforeafter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripThinking(tt.input)
			if got != tt.want {
				t.Errorf("stripThinking(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestJudgeUnparseableAnswer(t *testing.T) {
	srv := newFakeJudgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"content": "maybe",
					},
				},
			},
		})
	})

	j, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = j.Judge(context.Background(), recognizers.EntityMYIC, "900515103421", "MyKad")
	if err == nil {
		t.Fatal("expected error for unparseable answer, got nil")
	}
	if !strings.Contains(err.Error(), "unparseable answer") {
		t.Errorf("error = %q, want unparseable answer", err)
	}
}

func TestJudgeServer500(t *testing.T) {
	srv := newFakeJudgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	})

	j, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = j.Judge(context.Background(), recognizers.EntityMYIC, "900515103421", "MyKad")
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

func TestJudgeCacheHit(t *testing.T) {
	var calls int
	srv := newFakeJudgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"content": "yes",
					},
				},
			},
		})
	})

	j, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	confirmed1, err := j.Judge(context.Background(), recognizers.EntityMYIC, "900515103421", "MyKad")
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	confirmed2, err := j.Judge(context.Background(), recognizers.EntityMYIC, "900515103421", "MyKad")
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}

	if !confirmed1 || !confirmed2 {
		t.Errorf("both calls should confirm")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (cache hit on second call)", calls)
	}
}

func TestJudgeCacheKeyedByModel(t *testing.T) {
	var calls int
	srv := newFakeJudgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"content": "yes",
					},
				},
			},
		})
	})

	j1, err := New(srv.URL, WithModel("model-a"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	j2, err := New(srv.URL, WithModel("model-b"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := j1.Judge(context.Background(), recognizers.EntityMYIC, "900515103421", "MyKad"); err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if _, err := j2.Judge(context.Background(), recognizers.EntityMYIC, "900515103421", "MyKad"); err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (different models = separate cache keys)", calls)
	}
}

func TestJudgeRejectsNonLoopback(t *testing.T) {
	_, err := New("https://remote.example.com/v1")
	if err == nil {
		t.Fatal("expected error for non-loopback URL, got nil")
	}
	if !strings.Contains(err.Error(), "not loopback") {
		t.Errorf("error = %q, want not loopback", err)
	}
}

func TestJudgeRejectsNonLoopbackLocalhostRemote(t *testing.T) {
	_, err := New("http://example.com:8080")
	if err == nil {
		t.Fatal("expected error for remote URL, got nil")
	}
}

func TestJudgeAllowsLoopbackVariants(t *testing.T) {
	for _, url := range []string{
		"http://127.0.0.1:8787",
		"http://localhost:8787",
		"http://[::1]:8787",
		"http://127.0.0.1:1",
	} {
		j, err := New(url)
		if err != nil {
			t.Errorf("New(%q) = %v, want success", url, err)
			continue
		}
		if j.baseURL == "" {
			t.Errorf("New(%q) produced empty baseURL", url)
		}
		_ = j
	}
}

func TestJudgeFromEnvDisabled(t *testing.T) {
	t.Setenv("PII_SCRUB_NLP_JUDGE_URL", "")
	j, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if j != nil {
		t.Errorf("FromEnv = %v, want nil (disabled)", j)
	}
}

func TestJudgeFromEnvURL(t *testing.T) {
	srv := newFakeJudgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"content": "yes",
					},
				},
			},
		})
	})

	t.Setenv("PII_SCRUB_NLP_JUDGE_URL", srv.URL)
	t.Setenv("PII_SCRUB_NLP_JUDGE_MODEL", "test-model")

	j, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if j == nil {
		t.Fatal("FromEnv = nil, want non-nil")
	}
	if j.model != "test-model" {
		t.Errorf("model = %q, want test-model", j.model)
	}

	confirmed, err := j.Judge(context.Background(), recognizers.EntityMYIC, "900515103421", "MyKad")
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if !confirmed {
		t.Errorf("confirmed = false, want true")
	}
}

func TestJudgeConcurrent(t *testing.T) {
	srv := newFakeJudgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"content": "yes",
					},
				},
			},
		})
	})

	j, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan bool, 100)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			candidate := string(rune('0'+i%10)) + "900515103421"
			confirmed, err := j.Judge(context.Background(), recognizers.EntityMYIC, candidate, "MyKad")
			if err != nil {
				t.Errorf("Judge goroutine %d: %v", i, err)
			}
			results <- confirmed
		}(i)
	}
	wg.Wait()
	close(results)

	count := 0
	for range results {
		count++
	}
	if count != 50 {
		t.Errorf("got %d results, want 50", count)
	}
}

// TestJudgeCacheKeyNoDelimiterCollision ensures distinct (candidate, window)
// pairs that would have collided under the old "|" concatenation are cached
// separately.
func TestJudgeCacheKeyNoDelimiterCollision(t *testing.T) {
	var calls int
	srv := newFakeJudgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{"content": "yes"},
				},
			},
		})
	})

	j, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := j.Judge(context.Background(), recognizers.EntityMYIC, "x", "y|z"); err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if _, err := j.Judge(context.Background(), recognizers.EntityMYIC, "x|y", "z"); err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (no cache collision)", calls)
	}
}

// TestJudgeAllowsRemoteWhenAllowed verifies that PII_SCRUB_NLP_JUDGE_ALLOW_REMOTE=1
// overrides the loopback-only restriction. This is an explicit opt-in escape hatch,
// not the default.
func TestJudgeAllowsRemoteWhenAllowed(t *testing.T) {
	t.Setenv("PII_SCRUB_NLP_JUDGE_ALLOW_REMOTE", "1")
	if _, err := New("http://example.local:8080/judge"); err != nil {
		t.Fatalf("expected ALLOW_REMOTE to permit non-loopback: %v", err)
	}
}

// TestJudgeFromEnvNilWhenUnset verifies that FromEnv returns nil, nil when
// PII_SCRUB_NLP_JUDGE_URL is unset, so the pass is disabled by default.
func TestJudgeFromEnvNilWhenUnset(t *testing.T) {
	t.Setenv("PII_SCRUB_NLP_JUDGE_URL", "")

	j, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if j != nil {
		t.Errorf("FromEnv = %v, want nil (judge disabled when URL unset)", j)
	}
}

// TestJudgeRedirectRefused verifies that a redirect response is refused even
// when the handler redirects within loopback, so the loopback gate cannot be
// bypassed via a 30x to an arbitrary host.
func TestJudgeRedirectRefused(t *testing.T) {
	srv := newFakeJudgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://evil.example/steal", http.StatusFound)
	})

	j, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = j.Judge(context.Background(), recognizers.EntityMYIC, "900515103421", "MyKad")
	if err == nil {
		t.Fatal("expected error for redirected request, got nil")
	}
	if !strings.Contains(err.Error(), "302") && !strings.Contains(err.Error(), "Found") {
		t.Errorf("error = %q, want redirect-refusal (302 refused)", err)
	}
}

// TestJudgeCacheEviction verifies the bounded FIFO cache evicts old entries at
// cacheMax so repeated distinct candidates do not grow without bound.
func TestJudgeCacheEviction(t *testing.T) {
	var callsMu sync.Mutex
	var calls int
	srv := newFakeJudgeServer(t, func(w http.ResponseWriter, r *http.Request) {
		callsMu.Lock()
		calls++
		callsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"message": map[string]interface{}{
						"content": "yes",
					},
				},
			},
		})
	})

	j, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Fill the cache past cacheMax with distinct keys.
	for i := 0; i < cacheMax+50; i++ {
		candidate := fmt.Sprintf("candidate-%05d", i)
		if _, err := j.Judge(context.Background(), recognizers.EntityMYIC, candidate, "window"); err != nil {
			t.Fatalf("Judge(%d): %v", i, err)
		}
	}
	callsMu.Lock()
	firstCalls := calls
	callsMu.Unlock()

	// "candidate-00000" was inserted first and should have been evicted once
	// the cache passed cacheMax: the judge must call the endpoint again.
	if _, err := j.Judge(context.Background(), recognizers.EntityMYIC, "candidate-00000", "window"); err != nil {
		t.Fatalf("Judge(evicted): %v", err)
	}
	callsMu.Lock()
	afterCalls := calls
	callsMu.Unlock()
	if afterCalls <= firstCalls {
		t.Errorf("server calls = %d before re-query, %d after; want an increase (entry was evicted)", firstCalls, afterCalls)
	}
}
