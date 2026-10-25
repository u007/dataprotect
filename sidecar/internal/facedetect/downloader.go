package facedetect

import (
	"compress/bzip2"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Downloader manages download-on-first-use for face detection models.
//
// Models are downloaded lazily: on the first request, each missing model
// is fetched from its source URL, decompressed from .bz2, and stored in
// the OS user cache directory. Subsequent requests reuse the cached file.
//
// All methods are safe for concurrent use. Downloads are serialized per
// model via sync.Once; a failed download is retried on the next request.
type Downloader struct {
	mu       sync.Mutex
	done     map[string]struct{} // models successfully downloaded
	failed   map[string]error    // last error per model
	inFlight map[string]*downloadState
}

type downloadState struct {
	done   chan struct{}
	result error
}

// NewDownloader creates a Downloader with no models downloaded yet.
func NewDownloader() *Downloader {
	return &Downloader{
		done:     make(map[string]struct{}),
		failed:   make(map[string]error),
		inFlight: make(map[string]*downloadState),
	}
}

// EnsureModel blocks until the named model is available in the cache.
// If the model is already cached, it returns immediately. Otherwise it
// downloads and decompresses the model from its configured URL.
//
// ctx can be used to cancel an in-progress download. If ctx is cancelled
// while another goroutine is downloading the same model, this call waits
// for that download to finish and then returns its result (or error).
func (d *Downloader) EnsureModel(ctx context.Context, name string) error {
	// Fast path: already downloaded successfully.
	d.mu.Lock()
	if _, ok := d.done[name]; ok {
		d.mu.Unlock()
		return nil
	}
	// Fast path: a previous download failed; retry.
	if _, ok := d.failed[name]; ok {
		delete(d.failed, name)
		d.mu.Unlock()
	} else if st, ok := d.inFlight[name]; ok {
		// Another goroutine is downloading this model. Wait for it.
		d.mu.Unlock()
		select {
		case <-st.done:
			d.mu.Lock()
			err := st.result
			d.mu.Unlock()
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	} else {
		// We are the downloader. Register in-flight state.
		st := &downloadState{done: make(chan struct{})}
		d.inFlight[name] = st
		d.mu.Unlock()

		err := d.download(ctx, name)

		d.mu.Lock()
		delete(d.inFlight, name)
		if err != nil {
			d.failed[name] = err
		} else {
			d.done[name] = struct{}{}
		}
		close(st.done)
		d.mu.Unlock()
		return err
	}

	// We lost the race to retry; another goroutine may be downloading.
	d.mu.Lock()
	if st, ok := d.inFlight[name]; ok {
		d.mu.Unlock()
		select {
		case <-st.done:
			d.mu.Lock()
			err := st.result
			d.mu.Unlock()
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	// No in-flight download; proceed to download.
	st := &downloadState{done: make(chan struct{})}
	d.inFlight[name] = st
	d.mu.Unlock()

	err := d.download(ctx, name)

	d.mu.Lock()
	delete(d.inFlight, name)
	if err != nil {
		d.failed[name] = err
	} else {
		d.done[name] = struct{}{}
	}
	close(st.done)
	d.mu.Unlock()
	return err
}

// download performs the actual download and decompression for one model.
func (d *Downloader) download(ctx context.Context, name string) error {
	var m *Model
	for i := range Models {
		if Models[i].Name == name {
			m = &Models[i]
			break
		}
	}
	if m == nil {
		return errors.New("facedetect: unknown model " + name)
	}

	log := slog.Default()

	// Check if already cached (another goroutine may have finished while
	// we were waiting for the lock).
	exists, err := modelExists(m.Path)
	if err != nil {
		log.Warn("facedetect: failed to check model cache", "model", m.Name, "error", err)
	}
	if exists {
		log.Info("facedetect: model already cached", "model", m.Name)
		return nil
	}

	// Resolve the cache directory.
	cacheDir, err := cacheDir()
	if err != nil {
		return err
	}

	// Download to a temp file, then decompress into the final path.
	tmpPath := filepath.Join(cacheDir, m.Path+".tmp")
	finalPath := filepath.Join(cacheDir, m.Path)

	var lastErr error
	for attempt := 1; attempt <= MaxRetries; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}

		err = d.downloadOnce(ctx, m, tmpPath)
		if err != nil {
			lastErr = err
			log.Warn("facedetect: download attempt failed",
				"model", m.Name, "attempt", attempt, "error", err)
			continue
		}

		// Decompress .bz2 into final path.
		err = d.decompress(tmpPath, finalPath)
		if err != nil {
			lastErr = err
			log.Warn("facedetect: decompression failed",
				"model", m.Name, "attempt", attempt, "error", err)
			os.Remove(tmpPath)
			continue
		}

		os.Remove(tmpPath)

		info, statErr := os.Stat(finalPath)
		if statErr != nil {
			lastErr = statErr
			log.Warn("facedetect: failed to stat downloaded model",
				"model", m.Name, "error", statErr)
			continue
		}

		log.Info("facedetect: model downloaded",
			"model", m.Name, "path", finalPath, "bytes", info.Size())
		return nil
	}

	// Clean up temp file on failure.
	os.Remove(tmpPath)

	return errors.New("facedetect: failed to download model " + m.Name +
		" after " + itoa(MaxRetries) + " attempts: " + lastErr.Error())
}

// downloadOnce performs a single download attempt with a context timeout.
// It writes the raw .bz2 bytes to dst without decompressing; the caller
// is responsible for decompression via decompress.
func (d *Downloader) downloadOnce(ctx context.Context, m *Model, dst string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", m.URL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: DownloadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("server returned " + resp.Status)
	}

	// Write raw .bz2 bytes to the destination file. Decompression happens
	// separately in decompress, so the temp file holds compressed data.
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// decompress reads a .bz2 file from src and writes the decompressed data
// to dst.
func (d *Downloader) decompress(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	bzr := bzip2.NewReader(in)
	_, err = io.Copy(out, bzr)
	return err
}

// itoa formats a non-negative integer as a string (avoids strconv import
// for a single use).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf []byte
	for n > 0 {
		buf = append([]byte{'0' + byte(n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// EnsureAll downloads all configured models. Returns the first error, if
// any; models that fail are still retried on subsequent EnsureModel calls.
func (d *Downloader) EnsureAll(ctx context.Context) error {
	var firstErr error
	for _, m := range Models {
		if err := d.EnsureModel(ctx, m.Name); err != nil {
			slog.Default().Warn("facedetect: model download failed",
				"model", m.Name, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
