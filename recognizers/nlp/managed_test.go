package nlp

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeSidecarArg makes the test binary act as a fake sidecar instead of
// running tests. The managed tests re-exec os.Args[0] with it as the first
// argument, so no sidecar binary or ONNX model is needed.
const fakeSidecarArg = "fake-sidecar"

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == fakeSidecarArg {
		os.Exit(runFakeSidecar(os.Args[2:]))
	}
	os.Exit(m.Run())
}

// runFakeSidecar serves /healthz and /v1/ner on -addr. Modes:
// healthy (200 on /healthz, stops on SIGTERM), unhealthy (503 on /healthz),
// ignore-term (healthy, ignores SIGTERM), exit (exits 3 before listening),
// bind-fail (exits 1 with a bind error on every attempt), bind-fail-once (bind
// error on the first attempt only), crash-on-ner (exits 4 on the first /v1/ner
// request), slow-ner (holds /v1/ner until the client goes away or SIGTERM).
// -pidfile, when set, receives the process id before the listener opens.
// -statefile, when set, gets one line per attempt holding the -addr it was given.
func runFakeSidecar(args []string) int {
	fs := flag.NewFlagSet(fakeSidecarArg, flag.ContinueOnError)
	addr := fs.String("addr", "", "listen address")
	mode := fs.String("mode", "healthy", "fake sidecar behaviour")
	pidfile := fs.String("pidfile", "", "file that receives the process id")
	statefile := fs.String("statefile", "", "file that records each attempt's address")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	if *pidfile != "" {
		if err := os.WriteFile(*pidfile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "write pidfile:", err)
			return 2
		}
	}

	attempts := 0
	if *statefile != "" {
		n, err := recordAttempt(*statefile, *addr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "record attempt:", err)
			return 2
		}
		attempts = n
	}

	ignoreTerm := false
	switch *mode {
	case "exit":
		fmt.Fprintln(os.Stderr, "fake sidecar: exiting before listening")
		return 3
	case "bind-fail":
		return bindFailure(*addr)
	case "bind-fail-once":
		if attempts == 1 {
			return bindFailure(*addr)
		}
	case "ignore-term":
		ignoreTerm = true
		signal.Ignore(syscall.SIGTERM)
	case "healthy", "unhealthy", "crash-on-ner", "slow-ner":
	default:
		fmt.Fprintln(os.Stderr, "fake sidecar: unknown mode", *mode)
		return 2
	}

	l, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		return 2
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if *mode == "unhealthy" {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, `{"status":"ok","backend":"fake"}`)
	})
	mux.HandleFunc("/v1/ner", func(w http.ResponseWriter, r *http.Request) {
		switch *mode {
		case "crash-on-ner":
			fmt.Fprintln(os.Stderr, "fake sidecar: fatal crash")
			os.Exit(4)
		case "slow-ner":
			fmt.Fprintln(os.Stderr, "fake sidecar: request in flight")
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"entities":[{"type":"PERSON","start":0,"end":4,"text":"John","confidence":0.80}]}`)
	})
	srv := &http.Server{Handler: mux}

	if !ignoreTerm {
		term := make(chan os.Signal, 1)
		signal.Notify(term, syscall.SIGTERM)
		go func() {
			<-term
			_ = srv.Close()
		}()
	}
	if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return 1
	}
	return 0
}

// bindFailure prints the error a real sidecar prints when its port is taken.
func bindFailure(addr string) int {
	fmt.Fprintf(os.Stderr, "listen tcp %s: bind: address already in use\n", addr)
	return 1
}

// recordAttempt appends addr to path and returns how many attempts are recorded.
func recordAttempt(path, addr string) (int, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	if _, err := fmt.Fprintln(f, addr); err != nil {
		_ = f.Close()
		return 0, err
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strings.Count(string(b), "\n"), nil
}

// fakeArgs returns the argument list for a fake sidecar in the given mode,
// plus the pidfile path where the child records its pid.
func fakeArgs(t *testing.T, mode string) ([]string, string) {
	t.Helper()
	pidfile := filepath.Join(t.TempDir(), "pid")
	return []string{fakeSidecarArg, "-mode", mode, "-pidfile", pidfile}, pidfile
}

// attemptArgs is fakeArgs plus a statefile that records each attempt's address.
func attemptArgs(t *testing.T, mode string) ([]string, string) {
	t.Helper()
	dir := t.TempDir()
	pidfile := filepath.Join(dir, "pid")
	statefile := filepath.Join(dir, "attempts")
	return []string{fakeSidecarArg, "-mode", mode, "-pidfile", pidfile, "-statefile", statefile}, statefile
}

func readPid(t *testing.T, pidfile string) int {
	t.Helper()
	b, err := os.ReadFile(pidfile)
	if err != nil {
		t.Fatalf("read pidfile: %v", err)
	}
	pid, err := strconv.Atoi(string(b))
	if err != nil {
		t.Fatalf("parse pid %q: %v", b, err)
	}
	return pid
}

func readFields(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.Fields(string(b))
}

// assertReaped fails unless no process with pid exists any more.
func assertReaped(t *testing.T, pid int) {
	t.Helper()
	if !isReaped(pid) {
		t.Fatalf("pid %d still alive", pid)
	}
}

func isDone(p *managedProcess) bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// waitUntil polls cond until it holds, and fails the test after 5 seconds.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartManagedBecomesHealthy(t *testing.T) {
	args, _ := fakeArgs(t, "healthy")
	rec, err := startManaged(context.Background(), os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	if !rec.Health() {
		t.Fatal("recognizer should be healthy after start")
	}
	resp, err := http.Get(rec.url + "/healthz")
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status: %s", resp.Status)
	}
}

func TestManagedRecognizeUsesChildPort(t *testing.T) {
	args, _ := fakeArgs(t, "healthy")
	rec, err := startManaged(context.Background(), os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	got, err := rec.Recognize(context.Background(), "John Smith")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(got) != 1 || got[0].Text != "John" {
		t.Fatalf("Recognize: got %+v", got)
	}
}

func TestStartManagedHealthTimeoutKillsChild(t *testing.T) {
	args, pidfile := fakeArgs(t, "unhealthy")
	_, err := startManaged(context.Background(), os.Args[0], args, 2*time.Second, 5*time.Second, nil)
	if err == nil {
		t.Fatal("expected health timeout error")
	}
	if !strings.Contains(err.Error(), "not healthy after") {
		t.Fatalf("error does not name the health timeout: %v", err)
	}
	assertReaped(t, readPid(t, pidfile))
}

func TestStartManagedEarlyExitFailsFast(t *testing.T) {
	args, _ := fakeArgs(t, "exit")
	start := time.Now()
	_, err := startManaged(context.Background(), os.Args[0], args, 30*time.Second, 5*time.Second, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected early-exit error")
	}
	if !strings.Contains(err.Error(), "exited before becoming healthy") {
		t.Fatalf("error does not report early exit: %v", err)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("early exit took %s; should fail fast, not wait for the health timeout", elapsed)
	}
}

// The fake sidecar's exit mode writes a known line to stderr before exiting. A
// failed startup must carry that line, so the cause is in the error.
func TestStartManagedErrorIncludesChildStderr(t *testing.T) {
	args, _ := fakeArgs(t, "exit")
	_, err := startManaged(context.Background(), os.Args[0], args, 30*time.Second, 5*time.Second, nil)
	if err == nil {
		t.Fatal("expected early-exit error")
	}
	if !strings.Contains(err.Error(), "sidecar stderr (tail): fake sidecar: exiting before listening") {
		t.Fatalf("error does not carry the child's stderr: %v", err)
	}
}

// A child that exits for a reason other than a taken port is not retried.
func TestStartManagedDoesNotRetryOtherExits(t *testing.T) {
	args, statefile := attemptArgs(t, "exit")
	_, err := startManaged(context.Background(), os.Args[0], args, 30*time.Second, 5*time.Second, nil)
	if err == nil {
		t.Fatal("expected early-exit error")
	}
	if n := len(readFields(t, statefile)); n != 1 {
		t.Fatalf("attempts = %d; want 1, a non-bind exit is not retried", n)
	}
}

// A child that cannot bind its port is retried on a fresh port.
func TestStartManagedRetriesWhenPortIsTaken(t *testing.T) {
	args, statefile := attemptArgs(t, "bind-fail-once")
	rec, err := startManaged(context.Background(), os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	if !rec.Health() {
		t.Fatal("recognizer should be healthy after the retry")
	}
	attempts := readFields(t, statefile)
	if len(attempts) != 2 {
		t.Fatalf("attempts = %v; want 2 (the bind failure, then the retry)", attempts)
	}
	if attempts[0] == attempts[1] {
		t.Fatalf("retry reused port %s; want a fresh port", attempts[0])
	}
}

// When every attempt loses the port, startup gives up after the bounded count.
func TestStartManagedGivesUpAfterMaxAttempts(t *testing.T) {
	args, statefile := attemptArgs(t, "bind-fail")
	_, err := startManaged(context.Background(), os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err == nil {
		t.Fatal("expected failure when every attempt loses the port")
	}
	if want := fmt.Sprintf("in %d attempts", managedStartAttempts); !strings.Contains(err.Error(), want) {
		t.Fatalf("error does not name the attempt bound %q: %v", want, err)
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("error does not carry the child's bind failure: %v", err)
	}
	if n := len(readFields(t, statefile)); n != managedStartAttempts {
		t.Fatalf("attempts = %d; want %d", n, managedStartAttempts)
	}
}

func TestTailBufferKeepsLastBytes(t *testing.T) {
	tb := &tailBuffer{max: 8}
	if _, err := tb.Write([]byte("abcdef")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := tb.Write([]byte("ghijkl")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := tb.String(); got != "efghijkl" {
		t.Fatalf("String() = %q; want the last 8 bytes %q", got, "efghijkl")
	}
	if _, err := tb.Write([]byte("0123456789")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := tb.String(); got != "23456789" {
		t.Fatalf("String() after an oversized write = %q; want %q", got, "23456789")
	}
}

func TestStartManagedContextCancelKillsChild(t *testing.T) {
	args, pidfile := fakeArgs(t, "unhealthy")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := startManaged(ctx, os.Args[0], args, 30*time.Second, 5*time.Second, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v; want context deadline exceeded", err)
	}
	assertReaped(t, readPid(t, pidfile))
}

func TestStartManagedRejectsBadArgs(t *testing.T) {
	tests := []struct {
		name   string
		binary string
		args   []string
		want   string
	}{
		{name: "empty binary", binary: "", args: nil, want: "binary path is empty"},
		{name: "addr flag", binary: os.Args[0], args: []string{"-addr", "127.0.0.1:1"}, want: "must not set -addr"},
		{name: "addr flag with value", binary: os.Args[0], args: []string{"--addr=127.0.0.1:1"}, want: "must not set -addr"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := StartManaged(context.Background(), tt.binary, tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v; want containing %q", err, tt.want)
			}
		})
	}
}

func TestManagedCloseStopsChild(t *testing.T) {
	args, pidfile := fakeArgs(t, "healthy")
	rec, err := startManaged(context.Background(), os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	url := rec.url
	pid := readPid(t, pidfile)

	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertReaped(t, pid)

	resp, err := http.Get(url + "/healthz")
	if err == nil {
		resp.Body.Close()
		t.Fatal("healthz still answers after Close")
	}
}

func TestManagedCloseKillsChildIgnoringSIGTERM(t *testing.T) {
	args, pidfile := fakeArgs(t, "ignore-term")
	const stopTimeout = 200 * time.Millisecond
	rec, err := startManaged(context.Background(), os.Args[0], args, 10*time.Second, stopTimeout, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	pid := readPid(t, pidfile)

	start := time.Now()
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if elapsed := time.Since(start); elapsed < stopTimeout {
		t.Fatalf("Close returned after %s; want at least the SIGTERM grace period %s", elapsed, stopTimeout)
	}
	assertReaped(t, pid)
}

func TestManagedCloseIdempotent(t *testing.T) {
	args, _ := fakeArgs(t, "healthy")
	rec, err := startManaged(context.Background(), os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// A wrapper script as the binary starts the sidecar as its own child. Close
// must stop that grandchild too, which only works when the whole process group
// is signalled.
func TestManagedCloseStopsWrappedChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a /bin/sh wrapper script")
	}
	script := filepath.Join(t.TempDir(), "wrap.sh")
	body := "#!/bin/sh\n\"" + os.Args[0] + "\" \"$@\" &\nwait\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}
	args, pidfile := fakeArgs(t, "healthy")
	rec, err := startManaged(context.Background(), script, args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	pid := readPid(t, pidfile)
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// The grandchild is reparented to init, which reaps it a moment later.
	deadline := time.Now().Add(3 * time.Second)
	for !isReaped(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	assertReaped(t, pid)
}

// A parent that never calls Close must not leak the child: cancelling the
// context passed to StartManaged stops the sidecar.
func TestManagedContextCancelStopsChildWithoutClose(t *testing.T) {
	args, pidfile := fakeArgs(t, "healthy")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec, err := startManaged(ctx, os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })
	pid := readPid(t, pidfile)

	cancel()
	waitUntil(t, "child to be reaped after context cancel", func() bool { return isReaped(pid) })

	_, err = rec.Recognize(context.Background(), "John Smith")
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("Recognize after context cancel: err = %v; want a closed error", err)
	}
}

func TestManagedRecognizeAfterCloseReturnsClosed(t *testing.T) {
	args, _ := fakeArgs(t, "healthy")
	rec, err := startManaged(context.Background(), os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err = rec.Recognize(context.Background(), "John Smith")
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("Recognize after Close: err = %v; want a closed error", err)
	}
	if rec.Health() {
		t.Fatal("recognizer should report unhealthy after Close")
	}
}

// A request in flight when Close runs must fail with the closed error, not with
// the transport error the cut connection produces.
func TestManagedRecognizeInFlightDuringCloseReturnsClosed(t *testing.T) {
	args, _ := fakeArgs(t, "slow-ner")
	rec, err := startManaged(context.Background(), os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	result := make(chan error, 1)
	go func() {
		_, err := rec.Recognize(context.Background(), "John Smith")
		result <- err
	}()
	waitUntil(t, "request to reach the sidecar", func() bool {
		return strings.Contains(rec.proc.stderr.String(), "request in flight")
	})

	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "closed") {
			t.Fatalf("in-flight Recognize: err = %v; want a closed error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight Recognize did not return after Close")
	}
}

// A child that dies after start is reported by later Recognize calls with its
// exit status and the stderr tail, not as a raw connection error.
func TestManagedRecognizeReportsChildExit(t *testing.T) {
	args, _ := fakeArgs(t, "crash-on-ner")
	rec, err := startManaged(context.Background(), os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	if _, err := rec.Recognize(context.Background(), "John Smith"); err == nil {
		t.Fatal("Recognize against a crashing sidecar must fail")
	}
	waitUntil(t, "child to exit", func() bool { return isDone(rec.proc) })

	_, err = rec.Recognize(context.Background(), "John Smith")
	for _, want := range []string{"sidecar process exited", "sidecar stderr (tail): fake sidecar: fatal crash"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("Recognize after child exit: err = %v; want containing %q", err, want)
		}
	}
	if rec.Health() {
		t.Fatal("recognizer should report unhealthy after the child exits")
	}
}

// Another local service that answers 200 on /healthz is not the sidecar.
func TestProbeHealthzRejectsForeignBody(t *testing.T) {
	foreign := http.NewServeMux()
	foreign.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ok") })
	srv := httptest.NewServer(foreign)
	t.Cleanup(srv.Close)
	if err := probeHealthz(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("a plain-text 200 on /healthz must not count as the sidecar")
	}

	sidecar := http.NewServeMux()
	sidecar.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `{"status":"ok","backend":"x"}`)
	})
	srv2 := httptest.NewServer(sidecar)
	t.Cleanup(srv2.Close)
	if err := probeHealthz(context.Background(), srv2.Client(), srv2.URL); err != nil {
		t.Fatalf("the sidecar's body must pass: %v", err)
	}
}
