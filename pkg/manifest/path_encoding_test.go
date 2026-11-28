package manifest

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeRawName writes a file whose name holds raw bytes, skipping the test where
// the filesystem refuses names that are not valid UTF-8 (APFS does; ext4 doesn't).
func writeRawName(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("cargoship-test latin-1 name\n"), 0o600); err != nil {
		t.Skipf("this filesystem refuses the name %q: %v", name, err)
	}
}

// TestScanLocalFiles_RefusesNonUTF8Names pins the delta side of #1003. A name
// that isn't valid UTF-8 was returned as-is, then JSON stored it with U+FFFD, so
// the next delta saw it as New forever (and as Deleted with --track-deletes), and
// two such names collapsed to one manifest path. It is now an error naming every
// such file, unless an exclusion covers it.
func TestScanLocalFiles_RefusesNonUTF8Names(t *testing.T) {
	root := t.TempDir()
	writeRawName(t, root, "caf\xe9.txt")
	writeRawName(t, root, "caf\xe8.txt")
	require.NoError(t, os.WriteFile(filepath.Join(root, "ok.txt"), []byte("cargoship-test ok\n"), 0o600))

	files, err := ScanLocalFiles(root)
	var bad *NonUTF8PathsError
	require.True(t, errors.As(err, &bad), "want a NonUTF8PathsError, got files=%v err=%v", files, err)
	assert.Equal(t, []string{"caf\xe8.txt", "caf\xe9.txt"}, bad.Paths)
	assert.Contains(t, err.Error(), `"caf\xe9.txt"`, "the name is shown escaped")

	files, err = ScanLocalFilesExcluding(root, []string{"caf*"})
	require.NoError(t, err, "excluding the files is the escape hatch")
	require.Len(t, files, 1)
	assert.Equal(t, "ok.txt", files[0].Path)
}

func TestNonUTF8PathsError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *NonUTF8PathsError
		want    []string
		notWant []string
	}{
		{
			name: "names escaped",
			err:  &NonUTF8PathsError{Root: "/data", Paths: []string{"docs/caf\xe9.txt"}},
			want: []string{`1 path(s) under "/data"`, `"docs/caf\xe9.txt"`, "exclude_patterns", "issues/1049"},
		},
		{
			name:    "long list truncated",
			err:     &NonUTF8PathsError{Root: "/d", Paths: []string{"a\xff", "b\xff", "c\xff", "d\xff", "e\xff", "f\xff", "g\xff"}},
			want:    []string{"7 path(s)", `"e\xff"`, "(and 2 more)"},
			notWant: []string{`"f\xff"`},
		},
		{
			name: "root itself",
			err:  &NonUTF8PathsError{Root: "/data/caf\xe9"},
			want: []string{`the source path "/data/caf\xe9" is not valid UTF-8`, "issues/1049"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			for _, w := range tt.want {
				assert.Contains(t, msg, w)
			}
			for _, w := range tt.notWant {
				assert.NotContains(t, msg, w)
			}
			assert.True(t, StorablePathName(msg), "the message itself must be valid UTF-8")
		})
	}
}
