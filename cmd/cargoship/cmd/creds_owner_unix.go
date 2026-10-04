//go:build unix

package cmd

import (
	"os"
	"syscall"
)

// fileOwnerUID reports the uid owning fi, where the platform exposes it.
//
// Split by build tag because syscall.Stat_t does not exist on Windows — the preflight
// that uses this failed to COMPILE there, which the cross-platform smoke lane caught.
func fileOwnerUID(fi os.FileInfo) (uint32, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint32(st.Uid), true
}
