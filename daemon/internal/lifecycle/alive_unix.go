//go:build !windows

package lifecycle

import "syscall"

// processAlive reports whether a process with the given pid is running.
// Sending signal 0 performs no signal delivery, just an existence check.
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
