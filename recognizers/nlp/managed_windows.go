//go:build windows

package nlp

import (
	"os"
	"os/exec"
)

// setProcessGroup does nothing on Windows: process groups are not used there.
func setProcessGroup(*exec.Cmd) {}

// terminate fails on Windows, which cannot send SIGTERM to a process.
// See TODO.md, "NLP managed mode".
func terminate(p *os.Process) error { return p.Signal(os.Interrupt) }

// kill ends only the child process on Windows.
func kill(p *os.Process) error { return p.Kill() }
