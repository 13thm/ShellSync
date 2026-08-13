//go:build windows

package lifecycle

import "golang.org/x/sys/windows"

// stillActive is the exit code returned for a running process.
const stillActive = 259

// processAlive reports whether a process with the given pid is running.
func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
