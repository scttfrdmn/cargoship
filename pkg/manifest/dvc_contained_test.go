package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateDVCFiles_StaysInsideOutputDir pins #916: entries were joined onto
// the output directory as recorded. A ".." entry overwrote a .dvc file outside
// it, and an absolute entry (upload records absolute paths) built a nested
// <out>/Users/... tree, inside the user's data for --generate-dvc-files.
func TestGenerateDVCFiles_StaysInsideOutputDir(t *testing.T) {
	base := t.TempDir()
	out := filepath.Join(base, "out")
	outside := filepath.Join(base, "tracked.txt.dvc")
	require.NoError(t, os.WriteFile(outside, []byte("remote: origin\n"), 0o600))

	src := filepath.Join(base, "src")
	m := &Manifest{UploadID: "u", Bucket: "b", SourcePath: src, Files: []FileEntry{
		{Path: filepath.Join(src, "a", "x.csv"), Size: 1, ContentHash: "h1", S3Key: "k"},
	}}
	n, err := m.GenerateDVCFiles(out, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	_, err = os.Stat(filepath.Join(out, "a", "x.csv.dvc"))
	assert.NoError(t, err, "the sidecar goes at the path relative to the source")
	entries, err := os.ReadDir(out)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no nested absolute tree")
	assert.Equal(t, "a", entries[0].Name())

	escaping := &Manifest{UploadID: "u", Files: []FileEntry{{Path: "../tracked.txt", Size: 1, ContentHash: "h2"}}}
	_, err = escaping.GenerateDVCFiles(out, nil)
	require.Error(t, err, "an entry leaving the output directory must be refused")
	got, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "remote: origin\n", string(got), "a file outside the output directory must be untouched")
}

func TestDVCRelPath(t *testing.T) {
	for _, tc := range []struct {
		source, entry, want string
		wantErr             bool
	}{
		{"/src", "/src/a/b.txt", "a/b.txt", false},
		{"", "a/./b.txt", "a/b.txt", false},
		{"", "/abs/b.txt", "abs/b.txt", false},
		{"", "a/../../x", "", true},
		{"", "/", "", true},
	} {
		got, err := dvcRelPath(tc.source, tc.entry)
		if tc.wantErr {
			assert.Error(t, err, tc.entry)
			continue
		}
		assert.NoError(t, err, tc.entry)
		assert.Equal(t, tc.want, got, tc.entry)
	}
}
