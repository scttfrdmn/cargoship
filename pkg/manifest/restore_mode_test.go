package manifest

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// modedFile is one tar member with the mode the archiver recorded for it.
type modedFile struct {
	name    string
	content []byte
	mode    int64
}

// modedTarZst builds a single .tar.zst chunk holding files with their modes.
func modedTarZst(t *testing.T, files []modedFile) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for _, f := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: f.name, Mode: f.mode, Size: int64(len(f.content))}))
		_, err := tw.Write(f.content)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	var out bytes.Buffer
	zw, err := zstd.NewWriter(&out)
	require.NoError(t, err)
	_, err = zw.Write(raw.Bytes())
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return out.Bytes()
}

// modedManifest describes files in one unframed chunk. With checksummed set
// each entry carries its SHA-256, which sends restore down the buffered,
// verified path; without it, down the streaming path.
func modedManifest(files []modedFile, checksummed bool) *Manifest {
	m := &Manifest{
		Version: ManifestVersion, Bucket: "b", Prefix: "p", CompressionType: "zstd",
		ChecksumAlgorithm: ChecksumAlgorithmSHA256, SourcePath: "/src",
		TotalChunks: 1, TotalFiles: int64(len(files)),
		Chunks: []ChunkEntry{{ID: 0, S3Key: "chunk-0.tar.zst", FileCount: len(files)}},
	}
	for _, f := range files {
		e := FileEntry{Path: f.name, Size: int64(len(f.content)), S3Key: "chunk-0.tar.zst"}
		if checksummed {
			sum := sha256.Sum256(f.content)
			e.Checksum = hex.EncodeToString(sum[:])
		}
		m.Chunks[0].FilePaths = append(m.Chunks[0].FilePaths, f.name)
		m.Files = append(m.Files, e)
	}
	return m
}

func skipModesOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on Windows")
	}
}

// fileMode returns the full mode of path, failing the test if it is absent.
func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	require.NoError(t, err)
	return fi.Mode()
}

const specialBits = os.ModeSetuid | os.ModeSetgid | os.ModeSticky

// TestBatchRestore_RestoresArchivedModes pins #974: every restored file came
// back 0644 whatever mode was archived, so a 0600 key became world-readable and
// a 0755 script lost its execute bit. The archived permission bits are now
// applied; setuid/setgid/sticky are not.
func TestBatchRestore_RestoresArchivedModes(t *testing.T) {
	skipModesOnWindows(t)
	files := []modedFile{
		{"/src/secret.key", []byte("cargoship-mode-secret"), 0o600},
		{"/src/run.sh", []byte("cargoship-mode-script"), 0o755},
		{"/src/suid-bin", []byte("cargoship-mode-suid"), 0o4755 | 0o2000 | 0o1000},
	}
	want := map[string]os.FileMode{"secret.key": 0o600, "run.sh": 0o755, "suid-bin": 0o755}

	for _, checksummed := range []bool{false, true} {
		name := "streaming"
		if checksummed {
			name = "verified"
		}
		t.Run(name, func(t *testing.T) {
			client := &mockS3Client{chunks: map[string][]byte{"p/chunk-0.tar.zst": modedTarZst(t, files)}}
			dest := t.TempDir()
			se := NewSelectiveExtractor(modedManifest(files, checksummed), client, 0)
			stats, err := se.BatchRestore(context.Background(), []string{"/src/secret.key", "/src/run.sh", "/src/suid-bin"}, dest)
			require.NoError(t, err)
			require.Equal(t, int64(3), stats.Restored, "reasons: %v", stats.Errors)
			for base, perm := range want {
				mode := fileMode(t, filepath.Join(dest, base))
				assert.Equal(t, perm, mode.Perm(), "%s permission bits", base)
				assert.Zero(t, mode&specialBits, "%s must not be restored setuid/setgid/sticky", base)
			}
			noTempFiles(t, dest)
		})
	}
}

// TestBatchRestore_OverwriteKeepsPrivateMode is the regression half of #974:
// before #907's atomic write, --overwrite truncated the file in place and kept
// its mode; the temp-file-and-rename replaced a 0600 file with a 0644 one.
func TestBatchRestore_OverwriteKeepsPrivateMode(t *testing.T) {
	skipModesOnWindows(t)
	files := []modedFile{{"/src/id_ed25519", []byte("cargoship-mode-private-key"), 0o600}}
	client := &mockS3Client{chunks: map[string][]byte{"p/chunk-0.tar.zst": modedTarZst(t, files)}}
	dest := t.TempDir()
	target := filepath.Join(dest, "id_ed25519")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))

	se := NewSelectiveExtractor(modedManifest(files, true), client, 0).SetExisting(ExistingOverwrite)
	stats, err := se.BatchRestore(context.Background(), []string{"/src/id_ed25519"}, dest)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.Restored, "reasons: %v", stats.Errors)
	assert.Equal(t, os.FileMode(0o600), fileMode(t, target).Perm())
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, files[0].content, got)
}

// TestBatchRestore_DirectObjectModes: a direct-upload object has no tar header
// and the manifest records no mode, so an overwritten file keeps the mode it
// had and a new one gets 0644.
func TestBatchRestore_DirectObjectModes(t *testing.T) {
	skipModesOnWindows(t)
	content := []byte("cargoship-mode-direct-object")
	sum := sha256.Sum256(content)
	m := &Manifest{
		Version: ManifestVersion, Bucket: "b", Prefix: "p", SourcePath: "/src",
		ChecksumAlgorithm: ChecksumAlgorithmSHA256, TotalFiles: 2,
		Files: []FileEntry{
			{Path: "/src/kept.pem", Size: int64(len(content)), S3Key: "files/kept.pem", Checksum: hex.EncodeToString(sum[:])},
			{Path: "/src/new.txt", Size: int64(len(content)), S3Key: "files/new.txt", Checksum: hex.EncodeToString(sum[:])},
		},
	}
	client := &mockS3Client{chunks: map[string][]byte{"p/files/kept.pem": content, "p/files/new.txt": content}}
	dest := t.TempDir()
	kept := filepath.Join(dest, "kept.pem")
	require.NoError(t, os.WriteFile(kept, []byte("old"), 0o600))

	se := NewSelectiveExtractor(m, client, 0).SetExisting(ExistingOverwrite)
	stats, err := se.BatchRestore(context.Background(), []string{"/src/kept.pem", "/src/new.txt"}, dest)
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Restored, "reasons: %v", stats.Errors)
	assert.Equal(t, os.FileMode(0o600), fileMode(t, kept).Perm(), "an overwritten direct object keeps the existing mode")
	assert.Equal(t, os.FileMode(0o644), fileMode(t, filepath.Join(dest, "new.txt")).Perm())
	noTempFiles(t, dest)
}

// TestBatchRestore_FrameRestoreAppliesMode: the random-access frame path reads
// the mode from the member's header, which sits in the same frame.
func TestBatchRestore_FrameRestoreAppliesMode(t *testing.T) {
	skipModesOnWindows(t)
	files := []framedFile{
		{"data/a.txt", bytes.Repeat([]byte("A"), 20000)},
		{"data/tool", bytes.Repeat([]byte("B"), 20000)},
	}
	modes := map[string]int64{"data/a.txt": 0o640, "data/tool": 0o700}
	chunkBytes, frames, offsets := buildStdlibFramedChunkModes(t, files, 16*1024, modes)
	require.Greater(t, len(frames), 1)
	m := framedManifest("chunk-0.tar.zst", files, frames, offsets)
	dl := &rangeDownloader{object: chunkBytes, honorRange: true}

	dest := t.TempDir()
	se := NewSelectiveExtractor(m, dl, 0)
	stats, err := se.BatchRestore(context.Background(), []string{"data/tool", "data/a.txt"}, dest)
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Restored, "reasons: %v", stats.Errors)
	assert.Equal(t, os.FileMode(0o700), fileMode(t, filepath.Join(dest, "data", "tool")).Perm())
	assert.Equal(t, os.FileMode(0o640), fileMode(t, filepath.Join(dest, "data", "a.txt")).Perm())
}

// TestBatchRestore_MaxLengthBasename pins #977: the temporary file name
// embedded the basename plus 35 bytes, so a legal 255-byte name failed with
// ENAMETOOLONG and a file that uploaded fine could not be restored.
func TestBatchRestore_MaxLengthBasename(t *testing.T) {
	base := strings.Repeat("n", 251) + ".bin" // 255 bytes, the NAME_MAX limit
	require.Len(t, base, 255)
	files := []modedFile{{"/src/" + base, []byte("cargoship-long-name"), 0o644}}
	client := &mockS3Client{chunks: map[string][]byte{"p/chunk-0.tar.zst": modedTarZst(t, files)}}
	dest := t.TempDir()

	for _, checksummed := range []bool{false, true} {
		se := NewSelectiveExtractor(modedManifest(files, checksummed), client, 0).SetExisting(ExistingOverwrite)
		stats, err := se.BatchRestore(context.Background(), []string{"/src/" + base}, dest)
		require.NoError(t, err)
		require.Equal(t, int64(1), stats.Restored, "checksummed=%v reasons: %v", checksummed, stats.Errors)
		got, err := os.ReadFile(filepath.Join(dest, base))
		require.NoError(t, err)
		assert.Equal(t, files[0].content, got)
	}
	noTempFiles(t, dest)
}
