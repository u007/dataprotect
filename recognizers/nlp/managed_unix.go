//go:build !windows

package nlp

import (
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
func terminate(p *os.Process) error { return syscall.Kill(-p.Pid, syscall.SIGTERM) }

// kill ends the child's whole process group.
func kill(p *os.Process) error { return syscall.Kill(-p.Pid, syscall.SIGKILL) }
