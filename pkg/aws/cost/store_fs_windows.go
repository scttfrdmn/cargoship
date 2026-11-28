//go:build windows

package cost

import (
	"errors"
	"syscall"
)

// Windows error codes the syscall package does not name.
const (
	errSharingViolation syscall.Errno = 32 // ERROR_SHARING_VIOLATION
	errLockViolation    syscall.Errno = 33 // ERROR_LOCK_VIOLATION
)

// isTransientFileError reports whether err is a sharing error that clears once
// another handle on the file closes: ERROR_ACCESS_DENIED (rename over, or open
// of, a file another handle holds without FILE_SHARE_DELETE, or one pending
// deletion), ERROR_SHARING_VIOLATION or ERROR_LOCK_VIOLATION.
func isTransientFileError(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	switch errno {
	case syscall.ERROR_ACCESS_DENIED, errSharingViolation, errLockViolation:
		return true
	}
	return false
}
