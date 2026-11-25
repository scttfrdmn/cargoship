package manifest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBatchRestore_ExistingFilePolicy pins #915: restore replaced existing local
// files with no prompt and no flag, so restoring into a working copy silently
// replaced newer local edits with the backed-up version.
func TestBatchRestore_ExistingFilePolicy(t *testing.T) {
	m, client := sameBasenameManifest(t, "notes/a.txt", "notes/b.txt")
	targets := []string{"notes/a.txt", "notes/b.txt"}
	setup := func(t *testing.T) (string, string) {
		t.Helper()
		dest := t.TempDir()
		existing := filepath.Join(dest, "notes", "a.txt")
		require.NoError(t, os.MkdirAll(filepath.Dir(existing), 0o755))
		require.NoError(t, os.WriteFile(existing, []byte("USER'S NEWER LOCAL EDITS"), 0o600))
		return dest, existing
	}

	t.Run("refused by default, nothing written", func(t *testing.T) {
		dest, existing := setup(t)
		_, err := NewSelectiveExtractor(m, client, 0).BatchRestore(context.Background(), targets, dest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "notes/a.txt")
		assert.Contains(t, err.Error(), "--overwrite")
		got, _ := os.ReadFile(existing)
		assert.Equal(t, "USER'S NEWER LOCAL EDITS", string(got))
		_, statErr := os.Stat(filepath.Join(dest, "notes", "b.txt"))
		assert.True(t, os.IsNotExist(statErr), "a refused restore writes nothing")
	})
	t.Run("skip keeps it and restores the rest", func(t *testing.T) {
		dest, existing := setup(t)
		stats, err := NewSelectiveExtractor(m, client, 0).SetExisting(ExistingSkip).BatchRestore(context.Background(), targets, dest)
		require.NoError(t, err)
		assert.Equal(t, int64(1), stats.Skipped)
		assert.Equal(t, int64(1), stats.Restored)
		got, _ := os.ReadFile(existing)
		assert.Equal(t, "USER'S NEWER LOCAL EDITS", string(got))
		_, statErr := os.Stat(filepath.Join(dest, "notes", "b.txt"))
		assert.NoError(t, statErr)
	})
	t.Run("overwrite replaces it", func(t *testing.T) {
		dest, existing := setup(t)
		stats, err := NewSelectiveExtractor(m, client, 0).SetExisting(ExistingOverwrite).BatchRestore(context.Background(), targets, dest)
		require.NoError(t, err)
		assert.Equal(t, int64(2), stats.Restored)
		got, _ := os.ReadFile(existing)
		assert.NotEqual(t, "USER'S NEWER LOCAL EDITS", string(got))
	})
	t.Run("an empty destination needs no flag", func(t *testing.T) {
		stats, err := NewSelectiveExtractor(m, client, 0).BatchRestore(context.Background(), targets, t.TempDir())
		require.NoError(t, err)
		assert.Equal(t, int64(2), stats.Restored)
	})
}
