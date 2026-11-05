package nlp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/u007/dataprotect/recognizers"
)

const (
	managedStartTimeout = 30 * time.Second
	managedStopTimeout  = 5 * time.Second
	managedPollInterval = 50 * time.Millisecond
	managedProbeTimeout = time.Second
)

var _ recognizers.Recognizer = (*ManagedRecognizer)(nil)

// ManagedRecognizer is an NLPRecognizer whose sidecar process is started by
// this library. Close stops the process; the embedded recognizer keeps the
// Recognizer behaviour of NLPRecognizer.
type ManagedRecognizer struct {
	*NLPRecognizer
	proc      *managedProcess
	closeOnce sync.Once
	closeErr  error
}

// StartManaged starts the sidecar binary at binary with args, waits until its
// /healthz answers 200, and returns a recognizer that talks to it. The library
// picks a free loopback port and passes it as -addr, so args must not set
// -addr. The caller must Close the result to stop the process. On Unix the child
// runs in its own process group and shutdown sends SIGTERM to the group; on
// Windows only the child is signalled and Close returns an error (see TODO.md).
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
		return nil, errors.Join(fmt.Errorf("nlp managed: %s: %w", binary, err), proc.stop())
	}
	// A child that lost the port race exits at once while another service on
	// that port may answer the probe, so a child that is already gone means the
	// answer did not come from it.
	select {
	case <-proc.done:
		return nil, fmt.Errorf("nlp managed: %s exited right after the health check passed (wait: %v)", binary, proc.waitErr)
	default:
	}

	rec.logger.Info("nlp sidecar started", "binary", binary, "pid", proc.cmd.Process.Pid, "addr", addr)
	return &ManagedRecognizer{NLPRecognizer: rec, proc: proc}, nil
}

// Close stops the sidecar: SIGTERM first, then Kill once the stop timeout has
// passed. It is idempotent; every call returns the result of the first.
func (m *ManagedRecognizer) Close() error {
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
	done        chan struct{}
	waitErr     error // written before done is closed; read only after <-done
}

// spawnManaged is the only place a managed sidecar process is spawned. The
// child's stderr goes to this process's stderr, where the sidecar logs.
func spawnManaged(logger *slog.Logger, stopTimeout time.Duration, binary string, argv []string) (*managedProcess, error) {
	cmd := exec.Command(binary, argv...)
	cmd.Stderr = os.Stderr
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &managedProcess{
		cmd:         cmd,
		logger:      logger,
		stopTimeout: stopTimeout,
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

// stop sends SIGTERM, waits up to stopTimeout for the child to exit, then
// kills it. It returns nil once the child has been reaped.
func (p *managedProcess) stop() error {
	select {
	case <-p.done:
		return nil
	default:
	}

	pid := p.cmd.Process.Pid
	if err := terminate(p.cmd.Process); err != nil {
		if !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("send SIGTERM to sidecar pid %d: %w", pid, err)
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

	p.logger.Warn("sidecar ignored SIGTERM, killing", "pid", pid, "timeout", p.stopTimeout)
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
