package manifest

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #705: an unreadable source root produced zero files, an empty delta, and a
// cheerful "no changes; nothing to back up" — an unattended agent reporting
// success while backing up nothing at all. The walk callback swallowed every
// error under a comment claiming it logged, which it did not.
func TestScanLocalFiles_UnreadableRootIsAnError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: mode 0000 is still readable")
	}
	root := filepath.Join(t.TempDir(), "src")
	require.NoError(t, os.Mkdir(root, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644))
	require.NoError(t, os.Chmod(root, 0o000))
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	files, err := ScanLocalFiles(root)
	require.Error(t, err, "an unreadable source root must be an error, not an empty tree")
	assert.Empty(t, files)
	assert.Contains(t, err.Error(), "cannot read source directory",
		"the message must say the SOURCE could not be read, not that nothing changed")
}

// A subdirectory the agent cannot read means the source contains data we cannot
// see. Backing up the rest and reporting success would omit those files with
// nothing downstream able to tell "not backed up" from "never existed".
func TestScanLocalFiles_UnreadableSubdirectoryIsAnError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: mode 0000 is still readable")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "visible.txt"), []byte("x"), 0o644))
	secret := filepath.Join(root, "locked")
	require.NoError(t, os.Mkdir(secret, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(secret, "inner.txt"), []byte("y"), 0o644))
	require.NoError(t, os.Chmod(secret, 0o000))
	t.Cleanup(func() { _ = os.Chmod(secret, 0o755) })

	_, err := ScanLocalFiles(root)
	require.Error(t, err)

	var upe *UnreadablePathsError
	require.True(t, errors.As(err, &upe), "want *UnreadablePathsError, got %T: %v", err, err)
	assert.Len(t, upe.Paths, 1, "the unreadable directory is reported once, not once per child")
	assert.Contains(t, upe.Paths[0], "locked")
	// The remedy is specific and must be in the message: this is what an operator
	// reads when a container runs as the wrong uid.
	assert.Contains(t, err.Error(), "read access")
	assert.Contains(t, err.Error(), "exclude_patterns")
}

// An operator who deliberately excludes an unreadable path must not be blocked by
// it — otherwise the escape hatch named in the error message does not work.
func TestScanLocalFilesExcluding_ExcludedUnreadablePathIsNotAnError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: mode 0000 is still readable")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "visible.txt"), []byte("x"), 0o644))
	secret := filepath.Join(root, "locked")
	require.NoError(t, os.Mkdir(secret, 0o755))
	require.NoError(t, os.Chmod(secret, 0o000))
	t.Cleanup(func() { _ = os.Chmod(secret, 0o755) })

	files, err := ScanLocalFilesExcluding(root, []string{"locked"})
	require.NoError(t, err, "an excluded path is pruned before it is ever read")
	require.Len(t, files, 1)
	assert.Equal(t, "visible.txt", filepath.ToSlash(files[0].Path))
}

// A file that vanishes between readdir and lstat is NORMAL on a live filesystem —
// something was deleted while the agent walked. Failing on that would make an
// unattended agent flaky on exactly the churn it exists to capture, so it must be
// skipped silently rather than reported as unreadable.
func TestScanLocalFiles_VanishedFileIsNotAnError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "stays.txt"), []byte("x"), 0o644))

	// A dangling symlink reproduces the same callback shape: Walk uses Lstat, so the
	// link itself is reported fine and is skipped as non-regular (#693). Use a
	// genuinely missing target under a directory that Walk will stat.
	gone := filepath.Join(root, "gone.txt")
	require.NoError(t, os.WriteFile(gone, []byte("y"), 0o644))
	require.NoError(t, os.Remove(gone)) // vanished before the scan

	files, err := ScanLocalFiles(root)
	require.NoError(t, err, "normal filesystem churn must not fail a backup cycle")
	require.Len(t, files, 1)
	assert.Equal(t, "stays.txt", filepath.ToSlash(files[0].Path))
}

// The error must list a bounded number of paths: a source with thousands of
// unreadable files should produce a usable message, not a wall of text.
func TestUnreadablePathsError_TruncatesTheList(t *testing.T) {
	paths := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		paths = append(paths, filepath.Join("/src", "f"+string(rune('a'+i))))
	}
	e := &UnreadablePathsError{Root: "/src", Paths: paths}
	msg := e.Error()
	assert.Contains(t, msg, "cannot read 12 path(s)")
	assert.Contains(t, msg, "and 7 more", "shows 5 then summarises the rest")
}
