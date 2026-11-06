//go:build !windows

package nlp

import (
	"errors"
	"syscall"
)

// isReaped reports whether no process with pid exists any more.
func isReaped(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}
