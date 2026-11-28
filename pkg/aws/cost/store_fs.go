package cost

import (
	"os"
	"time"
)

// File operations on the local budget store that wait out transient sharing
// errors (#1031). On Windows, replacing budgets.json by rename fails with
// "Access is denied" while any other handle has it open without
// FILE_SHARE_DELETE. Go's os.Open, and so every concurrent Load, opens it that
// way, and so do antivirus scanners and the search indexer. A file that is
// being replaced, or a lock file that is being removed, refuses new opens the
// same way until the last handle closes. Each of these clears within
// milliseconds, so the operation is retried instead of failing the save. On
// other platforms isTransientFileError is always false and each helper is
// one plain call.

const (
	// transientFileRetryWindow bounds the retrying: long enough to outlast a
	// reader's or a scanner's handle, short enough not to hang a command on a
	// real permission error.
	transientFileRetryWindow = 2 * time.Second

	transientFileFirstWait = time.Millisecond
	transientFileMaxWait   = 50 * time.Millisecond
)

// retryTransient runs op until it succeeds, fails with an error transient
// does not accept, or window has passed, backing off from 1ms to 50ms between
// attempts. It returns op's last error.
func retryTransient(window time.Duration, transient func(error) bool, op func() error) error {
	deadline := time.Now().Add(window)
	wait := transientFileFirstWait
	for {
		err := op()
		if err == nil || !transient(err) || !time.Now().Before(deadline) {
			return err
		}
		time.Sleep(wait)
		if wait < transientFileMaxWait {
			wait *= 2
		}
	}
}

// replaceFile renames from over to, waiting out transient sharing errors.
func replaceFile(from, to string) error {
	return retryTransient(transientFileRetryWindow, isTransientFileError, func() error {
		return os.Rename(from, to)
	})
}

// readStoreFile reads path, waiting out transient sharing errors.
func readStoreFile(path string) ([]byte, error) {
	var data []byte
	err := retryTransient(transientFileRetryWindow, isTransientFileError, func() error {
		var rErr error
		data, rErr = os.ReadFile(path) //nolint:gosec // the store path is derived from home/XDG, not user input
		return rErr
	})
	return data, err
}

// removeStoreFile removes path, waiting out transient sharing errors. A path
// that is already gone is not an error.
func removeStoreFile(path string) error {
	err := retryTransient(transientFileRetryWindow, isTransientFileError, func() error {
		return os.Remove(path)
	})
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
