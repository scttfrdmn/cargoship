//go:build unix

package manifest

import "syscall"

// mkfifoIfSupported creates a FIFO, or reports that the platform cannot.
//
// Behind a build tag because syscall.Mkfifo does not EXIST on Windows. A
// runtime.GOOS check cannot help with that — the package fails to compile, which is
// how the first attempt broke the Windows smoke lane. Same lesson as syscall.Stat_t
// in the credentials preflight: absent symbols are a build-tag problem, never an
// if-statement problem.
func mkfifoIfSupported(path string) error {
	return syscall.Mkfifo(path, 0o600)
}
