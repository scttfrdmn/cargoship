//go:build !unix

package manifest

import "errors"

// mkfifoIfSupported reports that this platform has no FIFOs. The caller logs and moves
// on: the shapes that matter in practice (symlinks, directories) are covered everywhere.
func mkfifoIfSupported(_ string) error {
	return errors.New("FIFOs are not supported on this platform")
}
