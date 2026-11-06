//go:build windows

package nlp

import (
	"log/slog"
	"os"
	"os/exec"
	"syscall"
)

// ctrlBreakEvent is CTRL_BREAK_EVENT from the Windows console API. Go's runtime
// maps it to SIGINT, so the sidecar's os.Interrupt handler runs its normal drain.
const ctrlBreakEvent = 1

// generateConsoleCtrlEvent is GenerateConsoleCtrlEvent from kernel32.dll. It is
// not in package syscall, and the root module stays stdlib-only, so it is loaded
// directly.
var generateConsoleCtrlEvent = syscall.NewLazyDLL("kernel32.dll").NewProc("GenerateConsoleCtrlEvent")

// setProcessGroup starts the child in a new process group whose id is the
// child's pid. Console control events can then be addressed to the child's group
// alone, so this process is not signalled. Processes the child starts inherit
// the group and receive the event too.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// terminate asks the child to drain with CTRL_BREAK_EVENT and returns nil. The
// caller then waits its stop timeout and kills the child if it is still running.
//
// Delivery needs the child to share this process's console. A parent with no
// console (a service, or a GUI host) cannot deliver the event, so terminate logs
// a warning and kills the child at once instead. That path has no graceful drain,
// and only the child is killed, not processes it started.
//
// Untested at runtime on Windows; see TODO.md, "NLP managed mode".
func terminate(p *os.Process, logger *slog.Logger) error {
	r1, _, callErr := generateConsoleCtrlEvent.Call(ctrlBreakEvent, uintptr(p.Pid))
	if r1 != 0 {
		return nil
	}
	logger.Warn("nlp sidecar: CTRL_BREAK not delivered (no shared console), killing without drain", "pid", p.Pid, "err", callErr)
	return kill(p)
}

// kill ends only the child process on Windows. Processes the child started are
// not ended, because os.Process.Kill does not walk the process tree.
func kill(p *os.Process) error { return p.Kill() }
