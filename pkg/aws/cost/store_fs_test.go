package cost

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errBusy  = errors.New("busy")
	errFatal = errors.New("fatal")
)

func isBusy(err error) bool { return errors.Is(err, errBusy) }

func TestRetryTransient(t *testing.T) {
	t.Run("retries a transient error until it clears", func(t *testing.T) {
		calls := 0
		err := retryTransient(time.Second, isBusy, func() error {
			calls++
			if calls < 4 {
				return errBusy
			}
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 4, calls)
	})
	t.Run("returns any other error at once", func(t *testing.T) {
		calls := 0
		err := retryTransient(time.Second, isBusy, func() error { calls++; return errFatal })
		assert.ErrorIs(t, err, errFatal)
		assert.Equal(t, 1, calls)
	})
	t.Run("gives up after the window with the last error", func(t *testing.T) {
		start := time.Now()
		err := retryTransient(30*time.Millisecond, isBusy, func() error { return errBusy })
		assert.ErrorIs(t, err, errBusy)
		assert.Less(t, time.Since(start), time.Second)
	})
}

func TestIsTransientFileError_NotForOrdinaryErrors(t *testing.T) {
	assert.False(t, isTransientFileError(nil))
	assert.False(t, isTransientFileError(os.ErrNotExist))
	assert.False(t, isTransientFileError(errFatal))
}

func TestRemoveStoreFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	require.NoError(t, os.WriteFile(p, []byte("1\n"), 0600))
	require.NoError(t, removeStoreFile(p))
	_, err := os.Stat(p)
	assert.True(t, os.IsNotExist(err))
	assert.NoError(t, removeStoreFile(p), "already gone is not an error")
}
