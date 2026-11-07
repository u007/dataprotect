//go:build !windows

package nlp

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child in its own process group, so that a wrapper
// script given as the binary and the sidecar it starts are signalled together.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminate asks the child's whole process group to exit.
func terminate(p *os.Process, _ *slog.Logger) error {
	return groupSignal(p.Pid, syscall.SIGTERM)
}

// kill ends the child's whole process group.
func kill(p *os.Process) error { return groupSignal(p.Pid, syscall.SIGKILL) }

// groupSignal signals the process group led by pid. A group that is already
// gone is reported as os.ErrProcessDone, which stop treats as a finished child.
func groupSignal(pid int, sig syscall.Signal) error {
	err := syscall.Kill(-pid, sig)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
