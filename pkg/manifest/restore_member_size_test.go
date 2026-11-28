package manifest

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hostileSizeChunk returns a .tar.zst whose only member's header claims size
// bytes but carries a few bytes of data, the shape of a corrupt or crafted
// chunk. The header is written without the data a tar.Writer would demand.
func hostileSizeChunk(t *testing.T, name string, size int64) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: size, Typeflag: tar.TypeReg}))
	raw.WriteString("cargoship-hostile-size")
	raw.Write(make([]byte, 2048)) // a zero trailer, so the reader sees an end
	var out bytes.Buffer
	zw, err := zstd.NewWriter(&out)
	require.NoError(t, err)
	_, err = zw.Write(raw.Bytes())
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return out.Bytes()
}

// TestExtractFromChunkData_RejectsHostileMemberSize pins #985: the verified
// path allocated a buffer of the tar header's size before comparing it with
// anything, so a chunk claiming an enormous member panicked (or exhausted
// memory) instead of failing the file. The header size is now checked against
// the manifest's before any allocation.
func TestExtractFromChunkData_RejectsHostileMemberSize(t *testing.T) {
	content := []byte("cargoship-hostile-size")
	sum := sha256.Sum256(content)
	for _, size := range []int64{1 << 55, int64(len(content)) + 1} {
		entry := &FileEntry{Path: "/src/f.bin", Size: int64(len(content)), S3Key: "chunk-0.tar.zst", Checksum: hex.EncodeToString(sum[:])}
		m := &Manifest{Version: ManifestVersion, Bucket: "b", Prefix: "p", CompressionType: "zstd",
			ChecksumAlgorithm: ChecksumAlgorithmSHA256, SourcePath: "/src",
			Chunks: []ChunkEntry{{ID: 0, S3Key: "chunk-0.tar.zst"}}, Files: []FileEntry{*entry}}
		se := NewSelectiveExtractor(m, &mockS3Client{}, 0)
		dest := t.TempDir()
		root, err := os.OpenRoot(dest)
		require.NoError(t, err)

		var n int
		assert.NotPanics(t, func() {
			n, _, err = se.extractFromChunkData(hostileSizeChunk(t, entry.Path, size), []*FileEntry{&m.Files[0]}, root, dest)
		}, "header size %d", size)
		_ = root.Close()
		assert.Zero(t, n, "header size %d", size)
		require.Error(t, err, "header size %d", size)
		assert.Contains(t, err.Error(), "the archive holds", "header size %d: %v", size, err)
		noTempFiles(t, dest)
	}
}
