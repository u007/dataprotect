//go:build windows

package nlp

import "os"

// isReaped reports whether no process with pid exists any more. On Windows
// FindProcess opens a handle and fails once the process is gone.
func isReaped(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return true
	}
	p.Release()
	return false
}
