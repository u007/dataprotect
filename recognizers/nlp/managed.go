package nlp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/u007/dataprotect/recognizers"
)

const (
	managedStartTimeout = 30 * time.Second
	managedStopTimeout  = 5 * time.Second
	managedPollInterval = 50 * time.Millisecond
	managedProbeTimeout = time.Second
	// managedStderrTail bounds the child stderr kept for startup errors.
	managedStderrTail = 4 << 10
	// managedStartAttempts bounds how many fresh ports startManaged tries when the
	// child cannot bind its port. The port is probed and then released before the
	// child binds it, so another process can take it in between.
	managedStartAttempts = 3
	// managedStopWaitDelay bounds how long Wait lingers on the stderr pipe after
	// the child exits, when a grandchild still holds it open.
	managedStopWaitDelay = time.Second
)

// bindFailureMarkers are the stderr fragments a child prints when it cannot bind
// its port: Go's wording on Unix and on Windows.
var bindFailureMarkers = []string{
	"address already in use",
	"Only one usage of each socket address",
}

var (
	// errPortTaken marks a startup in which the child exited because it could not
	// bind its port. startManaged retries these and only these.
	errPortTaken = errors.New("port already in use")
	// errManagedClosed is what Recognize returns once Close has begun.
	errManagedClosed = errors.New("nlp managed: recognizer is closed")
)

var _ recognizers.Recognizer = (*ManagedRecognizer)(nil)

// ManagedRecognizer is an NLPRecognizer whose sidecar process is started by
// this library. Close stops the process; the embedded recognizer keeps the
// Recognizer behaviour of NLPRecognizer.
type ManagedRecognizer struct {
	*NLPRecognizer
	proc      *managedProcess
	closing   atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

// StartManaged starts the sidecar binary at binary with args, waits until its
// /healthz answers 200, and returns a recognizer that talks to it. The library
// picks a free loopback port and passes it as -addr, so args must not set
// -addr. The caller must Close the result to stop the process. On Unix the child
// runs in its own process group and shutdown sends SIGTERM to the group; on
// Windows the child gets CTRL_BREAK_EVENT and is killed after the stop timeout
// if still running. Without a shared console it is killed without a drain. The
// Windows path is untested at runtime (see TODO.md).
//
// ctx also bounds the child's lifetime: when ctx ends before Close is called, the
// sidecar is stopped as Close would stop it, so a caller that never calls Close
// does not leak it. Pass a context that lives as long as the recognizer. If the
// child cannot bind its port, startup is retried on a fresh port, up to
// managedStartAttempts times.
func StartManaged(ctx context.Context, binary string, args []string, opts ...Option) (*ManagedRecognizer, error) {
	return startManaged(ctx, binary, args, managedStartTimeout, managedStopTimeout, opts)
}

func startManaged(ctx context.Context, binary string, args []string, startTimeout, stopTimeout time.Duration, opts []Option) (*ManagedRecognizer, error) {
	if binary == "" {
		return nil, errors.New("nlp managed: binary path is empty")
	}
	for _, a := range args {
		if a == "-addr" || a == "--addr" || strings.HasPrefix(a, "-addr=") || strings.HasPrefix(a, "--addr=") {
			return nil, fmt.Errorf("nlp managed: args must not set -addr, the library chooses the port: %q", a)
		}
	}

	var lastErr error
	for attempt := 1; attempt <= managedStartAttempts; attempt++ {
		m, err := startManagedOnce(ctx, binary, args, startTimeout, stopTimeout, opts)
		if err == nil {
			return m, nil
		}
		if !errors.Is(err, errPortTaken) {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("nlp managed: child could not bind a port in %d attempts: %w", managedStartAttempts, lastErr)
}

// startManagedOnce makes one startup attempt on a freshly chosen loopback port.
// A failure in which the child could not bind wraps errPortTaken.
func startManagedOnce(ctx context.Context, binary string, args []string, startTimeout, stopTimeout time.Duration, opts []Option) (*ManagedRecognizer, error) {
	addr, err := freeLoopbackAddr()
	if err != nil {
		return nil, fmt.Errorf("nlp managed: choose loopback port: %w", err)
	}
	rec := New("http://"+addr, opts...)

	argv := make([]string, 0, len(args)+2)
	argv = append(argv, args...)
	argv = append(argv, "-addr", addr)

	proc, err := spawnManaged(rec.logger, stopTimeout, binary, argv)
	if err != nil {
		return nil, fmt.Errorf("nlp managed: start %s: %w", binary, err)
	}
	if err := proc.awaitHealthy(ctx, rec.url, startTimeout); err != nil {
		stopErr := proc.stop()
		return nil, errors.Join(proc.startFailure(binary, err), stopErr)
	}
	// A child that lost the port race exits at once while another service on
	// that port may answer the probe, so a child that is already gone means the
	// answer did not come from it.
	select {
	case <-proc.done:
		return nil, proc.startFailure(binary, fmt.Errorf("exited right after the health check passed (wait: %v)", proc.waitErr))
	default:
	}

	m := &ManagedRecognizer{NLPRecognizer: rec, proc: proc}
	go m.stopWhenDone(ctx)
	rec.logger.Info("nlp sidecar started", "binary", binary, "pid", proc.cmd.Process.Pid, "addr", addr)
	return m, nil
}

// stopWhenDone ties the child to ctx. If ctx ends before the child exits, the
// sidecar is stopped as Close stops it. Once the child has exited, it returns.
func (m *ManagedRecognizer) stopWhenDone(ctx context.Context) {
	select {
	case <-ctx.Done():
		if err := m.Close(); err != nil {
			m.logger.Warn("nlp sidecar stop after context ended failed", "err", err)
		}
	case <-m.proc.done:
	}
}

// Recognize implements recognizers.Recognizer. Once Close has begun it returns
// a closed error. If the child has exited, it returns the exit status with the
// child's stderr tail. A request that fails while either holds reports that
// state in place of the transport error.
func (m *ManagedRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	if err := m.stateErr(); err != nil {
		return m.fail(err)
	}
	entities, err := m.NLPRecognizer.Recognize(ctx, text)
	if cause := m.stateErr(); cause != nil && (err != nil || m.LastError() != nil) {
		return m.fail(cause)
	}
	return entities, err
}

// stateErr returns why the sidecar can no longer serve requests, or nil while
// it can. Close sets closing before it signals the child, so a request that
// fails after that point always sees the closed state.
func (m *ManagedRecognizer) stateErr() error {
	if m.closing.Load() {
		return errManagedClosed
	}
	select {
	case <-m.proc.done:
		return m.proc.withStderr(fmt.Errorf("nlp managed: sidecar process exited (wait: %v)", m.proc.waitErr))
	default:
		return nil
	}
}

// Close stops the sidecar: SIGTERM first, then Kill once the stop timeout has
// passed. It is idempotent; every call returns the result of the first. Requests
// that run during or after Close fail with a closed error.
func (m *ManagedRecognizer) Close() error {
	m.closing.Store(true)
	m.closeOnce.Do(func() {
		m.closeErr = m.proc.stop()
	})
	return m.closeErr
}

// managedProcess is a sidecar child owned by a ManagedRecognizer.
type managedProcess struct {
	cmd         *exec.Cmd
	logger      *slog.Logger
	stopTimeout time.Duration
	stderr      *tailBuffer
	done        chan struct{}
	waitErr     error // written before done is closed; read only after <-done
}

// spawnManaged is the only place a managed sidecar process is spawned. The
// child's stderr goes to this process's stderr, where the sidecar logs, and
// its last managedStderrTail bytes are kept for startup errors.
func spawnManaged(logger *slog.Logger, stopTimeout time.Duration, binary string, argv []string) (*managedProcess, error) {
	stderr := &tailBuffer{max: managedStderrTail}
	cmd := exec.Command(binary, argv...)
	// The tail goes first and the forward to os.Stderr never fails, so a closed or
	// broken stderr cannot stop the copy and block the child on a full pipe.
	cmd.Stderr = io.MultiWriter(stderr, bestEffortWriter{os.Stderr})
	// A grandchild that outlives the child holds the stderr pipe open, and Wait
	// would block on it. WaitDelay bounds that wait so done always closes.
	cmd.WaitDelay = managedStopWaitDelay
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &managedProcess{
		cmd:         cmd,
		logger:      logger,
		stopTimeout: stopTimeout,
		stderr:      stderr,
		done:        make(chan struct{}),
	}
	go func() {
		p.waitErr = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

// awaitHealthy polls base+"/healthz" until it returns 200. It fails when the
// child exits, the context ends, or timeout passes.
func (p *managedProcess) awaitHealthy(ctx context.Context, base string, timeout time.Duration) error {
	client := &http.Client{Timeout: managedProbeTimeout}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(managedPollInterval)
	defer ticker.Stop()

	for {
		last := probeHealthz(ctx, client, base)
		if last == nil {
			return nil
		}
		select {
		case <-p.done:
			return fmt.Errorf("sidecar exited before becoming healthy (wait: %v)", p.waitErr)
		case <-ctx.Done():
			return fmt.Errorf("waiting for sidecar health: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("sidecar not healthy after %s (last probe: %w)", timeout, last)
		case <-ticker.C:
		}
	}
}

// startFailure wraps a failed startup with the child's recent stderr, so the
// error reports why the child died and not only its exit status. It must be
// called after the child has exited. When the child could not bind its port,
// the result also wraps errPortTaken so startManaged retries on a fresh port.
func (p *managedProcess) startFailure(binary string, cause error) error {
	err := p.withStderr(fmt.Errorf("nlp managed: %s: %w", binary, cause))
	if !p.portTaken() {
		return err
	}
	p.logger.Warn("nlp sidecar could not bind its port", "binary", binary, "err", err)
	return fmt.Errorf("%w: %w", errPortTaken, err)
}

// portTaken reports whether the child has exited with a bind failure on stderr.
// It must be called after the child has exited, so stderr is complete.
func (p *managedProcess) portTaken() bool {
	select {
	case <-p.done:
	default:
		return false
	}
	tail := p.stderr.String()
	for _, marker := range bindFailureMarkers {
		if strings.Contains(tail, marker) {
			return true
		}
	}
	return false
}

// withStderr appends the child's recent stderr to err, so a failed startup
// reports why the child died and not only its exit status.
func (p *managedProcess) withStderr(err error) error {
	tail := strings.TrimSpace(p.stderr.String())
	if tail == "" {
		return err
	}
	return fmt.Errorf("%w; sidecar stderr (tail): %s", err, tail)
}

// tailBuffer is an io.Writer that keeps only the last max bytes written to it.
// The child's stderr copy writes to it while startup errors read it, so access
// is locked.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

// Write appends p and drops the oldest bytes beyond max. It never fails.
func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := len(p)
	if len(p) > t.max {
		p = p[len(p)-t.max:]
	}
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.max; over > 0 {
		copied := copy(t.buf, t.buf[over:])
		t.buf = t.buf[:copied]
	}
	return n, nil
}

// bestEffortWriter forwards to w and reports success even when w fails. The
// child's stderr is also kept in the tail buffer, so a broken forward target
// must not abort the copy.
type bestEffortWriter struct{ w io.Writer }

func (b bestEffortWriter) Write(p []byte) (int, error) {
	// intentionally not logged: logging here would write to the same broken stderr.
	_, _ = b.w.Write(p)
	return len(p), nil
}

// String returns the retained bytes.
func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

func probeHealthz(ctx context.Context, client *http.Client, base string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: %s", resp.Status)
	}
	var body struct {
		Status  string `json:"status"`
		Backend string `json:"backend"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("healthz: not the sidecar's JSON body: %w", err)
	}
	if body.Status != "ok" || body.Backend == "" {
		return fmt.Errorf("healthz: body is not the sidecar's (status %q, backend %q)", body.Status, body.Backend)
	}
	return nil
}

// stop sends the platform stop signal (SIGTERM on Unix), waits up to stopTimeout for the child to exit, then
// kills it. It returns nil once the child has been reaped.
func (p *managedProcess) stop() error {
	select {
	case <-p.done:
		return nil
	default:
	}

	pid := p.cmd.Process.Pid
	if err := terminate(p.cmd.Process, p.logger); err != nil {
		if !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("send stop signal to sidecar pid %d: %w", pid, err)
		}
		<-p.done
		return nil
	}

	timer := time.NewTimer(p.stopTimeout)
	defer timer.Stop()
	select {
	case <-p.done:
		return nil
	case <-timer.C:
	}

	p.logger.Warn("sidecar ignored stop signal, killing", "pid", pid, "timeout", p.stopTimeout)
	if err := kill(p.cmd.Process); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill sidecar pid %d after %s: %w", pid, p.stopTimeout, err)
	}
	<-p.done
	return nil
}

// freeLoopbackAddr returns a loopback address with no listener on it.
func freeLoopbackAddr() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		return "", err
	}
	return addr, nil
}
