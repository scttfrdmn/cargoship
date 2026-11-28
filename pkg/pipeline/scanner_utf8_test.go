package pipeline

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// writeRawName writes a file whose name holds raw bytes, skipping the test where
// the filesystem refuses names that are not valid UTF-8 (APFS does; ext4 doesn't).
func writeRawName(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("cargoship-test latin-1 name\n"), 0o600); err != nil {
		t.Skipf("this filesystem refuses the name %q: %v", name, err)
	}
}

// TestScanner_RefusesNonUTF8Names pins the upload side of #1003. The scanner
// chunked a name that isn't valid UTF-8 like any other. The tar entry kept the
// raw bytes, but the manifest's JSON replaced them with U+FFFD, so the upload
// "succeeded" with a file that restore could not find, and two such names
// collapsed to one manifest path. Now the scan leaves them out, names them, goes
// on chunking every other file, and fails, so no final manifest is written.
func TestScanner_RefusesNonUTF8Names(t *testing.T) {
	root := t.TempDir()
	writeRawName(t, root, "caf\xe9.txt")
	writeRawName(t, root, "caf\xe8.txt")
	writePrefixedFiles(t, filepath.Join(root, "good"), "f", 3, 64)

	got, err := scanFiles(t, &ScannerConfig{RootPath: root, Workers: 2}, nil)
	var bad *manifest.NonUTF8PathsError
	require.True(t, errors.As(err, &bad), "want a NonUTF8PathsError, got %v", err)
	assert.Equal(t, []string{"caf\xe8.txt", "caf\xe9.txt"}, bad.Paths)
	assert.Equal(t, []string{"good/f00000.dat", "good/f00001.dat", "good/f00002.dat"}, got,
		"every other file is still chunked, and the refused ones are not")
}

// TestScanner_RefusesNonUTF8Root covers a source path that is itself not valid
// UTF-8: every name under it would be corrupted, so the scan fails before walking.
func TestScanner_RefusesNonUTF8Root(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "caf\xe9")
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Skipf("this filesystem refuses the name: %v", err)
	}
	writePrefixedFiles(t, root, "f", 1, 64)

	got, err := scanFiles(t, &ScannerConfig{RootPath: root, Workers: 2}, nil)
	var bad *manifest.NonUTF8PathsError
	require.True(t, errors.As(err, &bad), "want a NonUTF8PathsError, got %v", err)
	assert.Empty(t, got)
}

// TestScanner_ExcludedNonUTF8NameIsNotAnError keeps the escape hatch: an
// exclusion is applied before the name check.
func TestScanner_ExcludedNonUTF8NameIsNotAnError(t *testing.T) {
	root := t.TempDir()
	writeRawName(t, root, "caf\xe9.txt")
	writePrefixedFiles(t, root, "f", 1, 64)

	got, err := scanFiles(t, &ScannerConfig{RootPath: root, Workers: 2, ExcludePatterns: []string{"caf*"}}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"f00000.dat"}, got)
}
