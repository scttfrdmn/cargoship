//go:build !windows

package cost

// isTransientFileError is always false off Windows: POSIX rename replaces a
// file whatever handles are open on it, and opens are never refused for
// sharing.
func isTransientFileError(error) bool { return false }
