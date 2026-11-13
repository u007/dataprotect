//go:build windows

package nlp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"testing"
	"time"
)

// A group that is already gone must read as a finished child, not as a signal
// failure: stop races with the child exiting on its own.
func TestTerminateAndKillReportGoneGroupAsDone(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit", "0")
	setProcessGroup(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	proc := &os.Process{Pid: cmd.Process.Pid}
	if err := terminate(proc, slog.New(slog.NewTextHandler(io.Discard, nil))); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("terminate on gone group = %v, want os.ErrProcessDone", err)
	}
	if err := kill(proc); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("kill on gone group = %v, want os.ErrProcessDone", err)
	}
}

// A grandchild that outlives the child and holds its stderr open must not keep
// the process from being reported as exited.
func TestSpawnManagedDoneClosesWhenGrandchildHoldsStderr(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p, err := spawnManaged(logger, time.Second, os.Args[0], []string{fakeSidecarArg, "-mode", "healthy"})
	if err != nil {
		t.Fatalf("spawnManaged: %v", err)
	}
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		t.Fatal("done did not close while a grandchild held stderr")
	}
	// Reap the stray grandchild's group.
	_ = kill(p.cmd.Process)
}

// TestManagedCloseStopsChildWindows exercises the Windows stop path: the child
// is started with CREATE_NEW_PROCESS_GROUP and Close sends CTRL_BREAK_EVENT,
// falling back to an immediate kill when there is no shared console.
//
// This test cannot run on Linux; it is compile-checked with
// GOOS=windows go test -c and exercised by the windows-nlp CI job.
func TestManagedCloseStopsChildWindows(t *testing.T) {
	args, pidfile := fakeArgs(t, "healthy")
	rec, err := startManaged(context.Background(), os.Args[0], args, 10*time.Second, 5*time.Second, nil)
	if err != nil {
		t.Fatalf("startManaged: %v", err)
	}
	pid := readPid(t, pidfile)

	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertReaped(t, pid)
}
