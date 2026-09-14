package pipeline

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOpenContainedSourceFile_RejectsSymlinkSwap is the CSH-SEC-001 regression:
// after the scanner validates a regular file, a path swapped to a symlink must
// NOT be followed at consume time. Both an out-of-tree target (exfiltration) and
// an in-tree target (leaf-symlink policy) are refused; a genuine regular file
// still opens.
func TestOpenContainedSourceFile_RejectsSymlinkSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privilege on Windows; the containment path is exercised on Unix")
	}
	srcDir := t.TempDir()
	outsideDir := t.TempDir()
	secret := filepath.Join(outsideDir, "secret.key")
	require.NoError(t, os.WriteFile(secret, []byte("PRIVATE KEY"), 0o600))

	// 1. A genuine regular file opens and reads.
	good := filepath.Join(srcDir, "data.bin")
	require.NoError(t, os.WriteFile(good, []byte("real data"), 0o644))
	f, err := openContainedSourceFile(srcDir, good)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// 2. Swap the path to a symlink pointing OUTSIDE the source tree (the
	// exfiltration scenario) — must be refused.
	require.NoError(t, os.Remove(good))
	require.NoError(t, os.Symlink(secret, good))
	_, err = openContainedSourceFile(srcDir, good)
	require.Error(t, err, "a symlink to an out-of-tree secret must be refused")

	// 3. Swap to a symlink pointing to another IN-tree file — also refused (the
	// scanner's FollowSymlinks==false policy, enforced at use time).
	inTree := filepath.Join(srcDir, "other.bin")
	require.NoError(t, os.WriteFile(inTree, []byte("other"), 0o644))
	require.NoError(t, os.Remove(good))
	require.NoError(t, os.Symlink(inTree, good))
	_, err = openContainedSourceFile(srcDir, good)
	require.Error(t, err, "a leaf symlink must be refused even when it targets an in-tree file")
}

// TestOpenContainedSourceFile_RejectsNonRegular ensures a directory or device
// path is refused (defense in depth; the body must be a regular file).
func TestOpenContainedSourceFile_RejectsNonRegular(t *testing.T) {
	srcDir := t.TempDir()
	sub := filepath.Join(srcDir, "adir")
	require.NoError(t, os.Mkdir(sub, 0o755))

	_, err := openContainedSourceFile(srcDir, sub)
	require.Error(t, err, "a directory must not be opened as a source file")
}

// TestOpenContainedSourceFile_RejectsEscape ensures a path outside the source
// root is refused even before any filesystem swap.
func TestOpenContainedSourceFile_RejectsEscape(t *testing.T) {
	srcDir := t.TempDir()

	_, err := openContainedSourceFile(srcDir, filepath.Join(srcDir, "..", "escape.txt"))
	require.Error(t, err, "a path escaping the source root must be refused")
}
