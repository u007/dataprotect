package facedetect

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestStatsEmptyCache reports no models and all missing on a fresh cache.
func TestStatsEmptyCache(t *testing.T) {
	origCacheDir := cacheDir
	defer func() { cacheDirFunc = origCacheDir }()

	tmp := t.TempDir()
	cacheDirFunc = func() (string, error) { return tmp, nil }

	stats, err := Stats()
	if err != nil {
		t.Fatalf("Stats() returned error: %v", err)
	}

	if stats.Dir != tmp {
		t.Errorf("Stats().Dir = %q, want %q", stats.Dir, tmp)
	}
	if len(stats.Models) != 0 {
		t.Errorf("Stats().Models = %v, want empty", stats.Models)
	}
	if len(stats.Missing) != len(Models) {
		t.Errorf("Stats().Missing has %d entries, want %d", len(stats.Missing), len(Models))
	}
	if stats.TotalSize != 0 {
		t.Errorf("Stats().TotalSize = %d, want 0", stats.TotalSize)
	}
}

// TestModelExists tracks creation and deletion of a model file.
func TestModelExists(t *testing.T) {
	origCacheDir := cacheDir
	defer func() { cacheDirFunc = origCacheDir }()

	tmp := t.TempDir()
	cacheDirFunc = func() (string, error) { return tmp, nil }

	name := Models[0].Path
	exists, err := modelExists(name)
	if err != nil {
		t.Fatalf("modelExists() returned error: %v", err)
	}
	if exists {
		t.Error("modelExists() = true, want false (no models cached yet)")
	}

	// Create the file.
	path := filepath.Join(tmp, name)
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() returned error: %v", err)
	}

	exists, err = modelExists(name)
	if err != nil {
		t.Fatalf("modelExists() returned error: %v", err)
	}
	if !exists {
		t.Error("modelExists() = false, want true (file created)")
	}

	// Remove it.
	if err := removeModel(name); err != nil {
		t.Fatalf("removeModel() returned error: %v", err)
	}

	exists, err = modelExists(name)
	if err != nil {
		t.Fatalf("modelExists() returned error: %v", err)
	}
	if exists {
		t.Error("modelExists() = true, want false (file removed)")
	}
}

// TestDownloaderConcurrentSameModel ensures only one download happens
// when multiple goroutines request the same missing model.
func TestDownloaderConcurrentSameModel(t *testing.T) {
	origCacheDir := cacheDir
	defer func() { cacheDirFunc = origCacheDir }()

	tmp := t.TempDir()
	cacheDirFunc = func() (string, error) { return tmp, nil }

	// HTTP server that serves one model, tracking request count.
	var mu sync.Mutex
	requests := 0
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++

		mu.Unlock()

		// Block until released, so concurrent requests pile up.
		<-release

		// Serve valid bzip2-compressed data ("test-model-payload").
		w.Header().Set("Content-Type", "application/x-bzip2")
		w.Write([]byte{
			0x42, 0x5a, 0x68, 0x39, 0x31, 0x41, 0x59, 0x26, 0x53, 0x59,
			0xbc, 0x83, 0x6a, 0x4a, 0x00, 0x00, 0x08, 0x11, 0x80, 0x00,
			0x02, 0x26, 0x06, 0xcc, 0x20, 0x20, 0x00, 0x31, 0x00, 0xd3,
			0x4d, 0x04, 0x03, 0x4c, 0x86, 0x82, 0xa0, 0x60, 0x3c, 0x96,
			0xcd, 0xf1, 0x5e, 0x2e, 0xe4, 0x8a, 0x70, 0xa1, 0x21, 0x79,
			0x06, 0xd4, 0x94,
		})
	}))
	defer srv.Close()

	// Override model URL to point at our test server.
	origModels := Models
	Models = []Model{
		{Name: "test-model", URL: srv.URL, Path: "test-model.dat"},
	}
	defer func() { Models = origModels }()

	dl := NewDownloader()
	ctx := context.Background()

	var wg sync.WaitGroup
	errCh := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- dl.EnsureModel(ctx, "test-model")
		}()
	}

	close(release)
	wg.Wait()
	close(errCh)

	// All calls should succeed.
	for err := range errCh {
		if err != nil {
			t.Errorf("EnsureModel() returned error: %v", err)
		}
	}

	// Only one HTTP request should have been made.
	mu.Lock()
	count := requests
	mu.Unlock()
	if count != 1 {
		t.Errorf("download requests = %d, want 1 (serialized per model)", count)
	}

	// Model file should exist.
	exists, err := modelExists(Models[0].Path)
	if err != nil {
		t.Fatalf("modelExists() returned error: %v", err)
	}
	if !exists {
		t.Error("model file not found after download")
	}
}

// TestDownloaderRetryOnFailure retries after a failing server.
func TestDownloaderRetryOnFailure(t *testing.T) {
	origCacheDir := cacheDir
	defer func() { cacheDirFunc = origCacheDir }()

	tmp := t.TempDir()
	cacheDirFunc = func() (string, error) { return tmp, nil }

	// Server that always fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	origModels := Models
	Models = []Model{
		{Name: "fail-model", URL: srv.URL, Path: "fail-model.dat"},
	}
	defer func() { Models = origModels }()

	dl := NewDownloader()
	ctx := context.Background()

	err := dl.EnsureModel(ctx, "fail-model")
	if err == nil {
		t.Error("EnsureModel() = nil, want error (server always fails)")
	}
	if !strings.Contains(err.Error(), "fail-model") {
		t.Errorf("error %q does not mention model name", err)
	}

	// Model file should not exist.
	exists, err := modelExists(Models[0].Path)
	if err != nil {
		t.Fatalf("modelExists() returned error: %v", err)
	}
	if exists {
		t.Error("model file exists after failed download")
	}
}

// TestDownloaderContextCancel verifies context cancellation propagates.
func TestDownloaderContextCancel(t *testing.T) {
	origCacheDir := cacheDir
	defer func() { cacheDirFunc = origCacheDir }()

	tmp := t.TempDir()
	cacheDirFunc = func() (string, error) { return tmp, nil }

	// Server that blocks until the client disconnects.
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block // never unblocks
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{0x42, 0x5A, 0x68}) // not valid bzip2, but we cancel before reading
	}))
	defer srv.Close()
	defer close(block)

	origModels := Models
	Models = []Model{
		{Name: "cancel-model", URL: srv.URL, Path: "cancel-model.dat"},
	}
	defer func() { Models = origModels }()

	dl := NewDownloader()
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		// Cancel after a short delay.
		cancel()
	}()

	err := dl.EnsureModel(ctx, "cancel-model")
	if err == nil {
		t.Error("EnsureModel() = nil, want error (context cancelled)")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

// TestDownloadOnceStatusNotOK checks that non-200 responses produce errors.
func TestDownloadOnceStatusNotOK(t *testing.T) {
	origCacheDir := cacheDir
	defer func() { cacheDirFunc = origCacheDir }()

	tmp := t.TempDir()
	cacheDirFunc = func() (string, error) { return tmp, nil }

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "not found")
	}))
	defer srv.Close()

	origModels := Models
	Models = []Model{
		{Name: "notfound", URL: srv.URL, Path: "notfound.dat"},
	}
	defer func() { Models = origModels }()

	dl := NewDownloader()
	ctx := context.Background()

	err := dl.EnsureModel(ctx, "notfound")
	if err == nil {
		t.Error("EnsureModel() = nil, want error (404)")
	}
	if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q does not reflect 404", err)
	}
}

// TestNewDetectorReturnsStub verifies NewDetector returns a StubDetector.
func TestNewDetectorReturnsStub(t *testing.T) {
	d := NewDetector()
	if _, ok := d.(StubDetector); !ok {
		t.Errorf("NewDetector() = %T, want StubDetector", d)
	}
}

// TestStubDetectorReturnsErrDlibUnavailable verifies the stub always errors.
func TestStubDetectorReturnsErrDlibUnavailable(t *testing.T) {
	d := StubDetector{}
	_, err := d.DetectFaces(nil)
	if err == nil {
		t.Error("DetectFaces() = nil error, want ErrDlibUnavailable")
	}
	if !errors.Is(err, ErrDlibUnavailable) {
		t.Errorf("error = %v, want ErrDlibUnavailable", err)
	}
}
