package manifest

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// singleFileManifest describes one chunk holding filePath, with no per-file
// checksum, so restore takes the streaming (CopyN) path.
func singleFileManifest(filePath string, size int) *Manifest {
	return &Manifest{
		Version: ManifestVersion, Bucket: "b", Prefix: "p", CompressionType: "zstd",
		TotalChunks: 1, TotalFiles: 1,
		Chunks: []ChunkEntry{{ID: 0, S3Key: "chunk-0.tar.zst", FileCount: 1, FilePaths: []string{filePath}}},
		Files:  []FileEntry{{Path: filePath, Size: int64(size), S3Key: "chunk-0.tar.zst"}},
	}
}

func oneFileTarZst(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(content))}))
	_, err := tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	var out bytes.Buffer
	zw, err := zstd.NewWriter(&out)
	require.NoError(t, err)
	_, err = zw.Write(raw.Bytes())
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return out.Bytes()
}

// noTempFiles fails if a restore temporary file was left in dir.
func noTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasPrefix(e.Name(), ".cargoship-restore-"), "temporary file left behind: %s", e.Name())
	}
}

// TestBatchRestore_FailedRestoreKeepsExistingFile pins #907: restore opened the
// destination with O_TRUNC and removed it when the write failed, so restoring
// over an existing file from a truncated chunk emptied the user's file and then
// deleted it. The restore failed AND the local copy was gone.
func TestBatchRestore_FailedRestoreKeepsExistingFile(t *testing.T) {
	filePath := "/abs/src/big.bin"
	content := bytes.Repeat([]byte("cargoship"), 4096)
	chunk := makeTruncatedTarZst(t, filePath, content, 20000)
	client := &mockS3Client{chunks: map[string][]byte{"p/chunk-0.tar.zst": chunk}}

	dest := t.TempDir()
	target := filepath.Join(dest, filePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	precious := []byte("the user's only local copy")
	require.NoError(t, os.WriteFile(target, precious, 0o600))

	se := NewSelectiveExtractor(singleFileManifest(filePath, len(content)), client, 0).SetExisting(ExistingOverwrite)
	stats, err := se.BatchRestore(context.Background(), []string{filePath}, dest)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Failed)

	got, err := os.ReadFile(target)
	require.NoError(t, err, "a failed restore must not delete the existing file")
	assert.Equal(t, precious, got, "a failed restore must not modify the existing file")
	noTempFiles(t, filepath.Dir(target))
}

// TestBatchRestore_OverwriteReplacesCompletely: a successful restore over an
// existing file replaces it with exactly the restored bytes and leaves no
// temporary file.
func TestBatchRestore_OverwriteReplacesCompletely(t *testing.T) {
	filePath := "/abs/src/f.bin"
	content := []byte("restored content")
	client := &mockS3Client{chunks: map[string][]byte{"p/chunk-0.tar.zst": oneFileTarZst(t, filePath, content)}}

	dest := t.TempDir()
	target := filepath.Join(dest, filePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, bytes.Repeat([]byte("x"), 1000), 0o600)) // longer than content

	se := NewSelectiveExtractor(singleFileManifest(filePath, len(content)), client, 0).SetExisting(ExistingOverwrite)
	stats, err := se.BatchRestore(context.Background(), []string{filePath}, dest)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Restored, "reasons: %v", stats.Errors)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, content, got)
	noTempFiles(t, filepath.Dir(target))
}

func TestAtomicFile_AbortAndCommit(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer func() { _ = root.Close() }()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f"), []byte("old"), 0o600))

	a, err := createAtomic(root, "f")
	require.NoError(t, err)
	_, err = a.Write([]byte("partial"))
	require.NoError(t, err)
	a.abort()
	got, err := os.ReadFile(filepath.Join(dir, "f"))
	require.NoError(t, err)
	assert.Equal(t, "old", string(got), "abort must leave the destination untouched")
	noTempFiles(t, dir)

	a, err = createAtomic(root, "f")
	require.NoError(t, err)
	_, err = a.Write([]byte("new"))
	require.NoError(t, err)
	require.NoError(t, a.commit())
	got, err = os.ReadFile(filepath.Join(dir, "f"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
	noTempFiles(t, dir)

	// A failed close (here: the file is already closed) must not commit.
	a, err = createAtomic(root, "f")
	require.NoError(t, err)
	_, err = a.Write([]byte("lost"))
	require.NoError(t, err)
	require.NoError(t, a.f.Close())
	require.Error(t, a.commit())
	got, err = os.ReadFile(filepath.Join(dir, "f"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(got), "a failed commit must leave the destination as it was")
	noTempFiles(t, dir)
}

func TestWriteContained_RefusesSymlinkLeaf(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real"), []byte("keep"), 0o600))
	if err := os.Symlink("real", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer func() { _ = root.Close() }()
	require.Error(t, writeContained(root, "link", []byte("x")))
	got, err := os.ReadFile(filepath.Join(dir, "real"))
	require.NoError(t, err)
	assert.Equal(t, "keep", string(got))
	noTempFiles(t, dir)
}
