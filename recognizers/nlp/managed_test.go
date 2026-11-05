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
// ignore-term (healthy, ignores SIGTERM), exit (exits 3 before listening).
// -pidfile, when set, receives the process id before the listener opens.
func runFakeSidecar(args []string) int {
	fs := flag.NewFlagSet(fakeSidecarArg, flag.ContinueOnError)
	addr := fs.String("addr", "", "listen address")
	mode := fs.String("mode", "healthy", "fake sidecar behaviour")
	pidfile := fs.String("pidfile", "", "file that receives the process id")
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

	ignoreTerm := false
	switch *mode {
	case "exit":
		fmt.Fprintln(os.Stderr, "fake sidecar: exiting before listening")
		return 3
	case "ignore-term":
		ignoreTerm = true
		signal.Ignore(syscall.SIGTERM)
	case "healthy", "unhealthy":
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
	mux.HandleFunc("/v1/ner", func(w http.ResponseWriter, _ *http.Request) {
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

// fakeArgs returns the argument list for a fake sidecar in the given mode,
// plus the pidfile path where the child records its pid.
func fakeArgs(t *testing.T, mode string) ([]string, string) {
	t.Helper()
	pidfile := filepath.Join(t.TempDir(), "pid")
	return []string{fakeSidecarArg, "-mode", mode, "-pidfile", pidfile}, pidfile
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

// assertReaped fails unless no process with pid exists any more.
func assertReaped(t *testing.T, pid int) {
	t.Helper()
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("pid %d still alive (kill 0 err = %v)", pid, err)
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
	for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	assertReaped(t, pid)
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
