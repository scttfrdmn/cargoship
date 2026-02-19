package manifest

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ChunkCache
// ---------------------------------------------------------------------------

func TestChunkCache_GetPut(t *testing.T) {
	c := NewChunkCache(1024)
	assert.Nil(t, c.get("missing"))

	c.put("k1", []byte("hello"))
	assert.Equal(t, []byte("hello"), c.get("k1"))
}

func TestChunkCache_UpdateExisting(t *testing.T) {
	c := NewChunkCache(1024)
	c.put("k", []byte("old"))
	c.put("k", []byte("new"))
	assert.Equal(t, []byte("new"), c.get("k"))
	assert.Equal(t, 1, c.Len())
}

func TestChunkCache_LRUEviction(t *testing.T) {
	// Cache holds at most 10 bytes.
	c := NewChunkCache(10)

	c.put("a", bytes.Repeat([]byte("x"), 5)) // 5 bytes — fits
	c.put("b", bytes.Repeat([]byte("y"), 5)) // 5 bytes — total == limit

	// Access "a" to make "b" the LRU.
	_ = c.get("a")

	// Adding "c" (5 bytes) must evict "b" (LRU), not "a" (MRU).
	c.put("c", bytes.Repeat([]byte("z"), 5))

	assert.NotNil(t, c.get("a"), "a (MRU) must survive eviction")
	assert.Nil(t, c.get("b"), "b (LRU) must be evicted")
	assert.NotNil(t, c.get("c"), "c (newly added) must be present")
}

func TestChunkCache_OversizedItemSkipped(t *testing.T) {
	c := NewChunkCache(10)
	c.put("big", bytes.Repeat([]byte("x"), 11)) // 11 > 10
	assert.Nil(t, c.get("big"))
	assert.Equal(t, 0, c.Len())
}

func TestChunkCache_SizeTracking(t *testing.T) {
	c := NewChunkCache(1024)
	c.put("a", []byte("12345"))
	assert.Equal(t, int64(5), c.Size())
	c.put("b", []byte("67890"))
	assert.Equal(t, int64(10), c.Size())
}

// ---------------------------------------------------------------------------
// Helpers for SelectiveExtractor tests
// ---------------------------------------------------------------------------

// build100FileManifest creates a manifest with 100 files spread across 10
// chunks (10 files per chunk, 1 shard). Each file gets a ContentHash.
func build100FileManifest() *Manifest {
	m := &Manifest{
		Version:         ManifestVersion,
		UploadID:        "batch-test-001",
		Bucket:          "test-bucket",
		CompressionType: "zstd",
		GitMetadata:     &GitMetadata{Commit: "deadbeef"},
	}

	for chunkID := 0; chunkID < 10; chunkID++ {
		s3Key := fmt.Sprintf("shard-0/chunk-%d.tar.zst", chunkID)
		chunk := ChunkEntry{
			ID:      chunkID,
			ShardID: 0,
			S3Key:   s3Key,
		}
		for fileIdx := 0; fileIdx < 10; fileIdx++ {
			n := chunkID*10 + fileIdx
			fe := FileEntry{
				Path:        fmt.Sprintf("data/file-%03d.bin", n),
				Size:        128,
				ModTime:     time.Now(),
				ChunkID:     chunkID,
				ShardID:     0,
				S3Key:       s3Key,
				ContentHash: fmt.Sprintf("%032x", n),
			}
			// Tag every other file with a DVC stage.
			if n%2 == 0 {
				fe.DVCMetadata = &DVCMetadata{Stage: "preprocess"}
			}
			m.Files = append(m.Files, fe)
			chunk.FilePaths = append(chunk.FilePaths, fe.Path)
		}
		m.Chunks = append(m.Chunks, chunk)
	}

	m.TotalFiles = int64(len(m.Files))
	return m
}

// buildMockChunksForManifest creates zstd-compressed tar archives for every
// chunk in m, using the mockS3Client type from extract_test.go.
func buildMockChunksForManifest(t *testing.T, m *Manifest) *mockS3Client {
	t.Helper()
	client := &mockS3Client{chunks: make(map[string][]byte)}
	for _, chunk := range m.Chunks {
		var files []FileEntry
		for _, fe := range m.Files {
			if fe.ChunkID == chunk.ID {
				files = append(files, fe)
			}
		}
		client.chunks[chunk.S3Key] = buildZstdTar(t, files)
	}
	return client
}

// buildZstdTar builds a zstd-compressed tar archive containing stub file data.
func buildZstdTar(t *testing.T, files []FileEntry) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	// Use createMockChunks' logic in a simpler form (single chunk).
	m := &Manifest{
		CompressionType: "zstd",
		Chunks: []ChunkEntry{{
			ID:    files[0].ChunkID,
			S3Key: files[0].S3Key,
		}},
		Files: files,
	}
	chunks := createMockChunks(t, m)
	for _, data := range chunks {
		_, _ = buf.Write(data)
	}
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// NewSelectiveExtractor
// ---------------------------------------------------------------------------

func TestNewSelectiveExtractor(t *testing.T) {
	m := build100FileManifest()
	client := buildMockChunksForManifest(t, m)
	se := NewSelectiveExtractor(m, client, 0)
	require.NotNil(t, se)
	assert.NotNil(t, se.cache)
	assert.NotNil(t, se.query)
}

// ---------------------------------------------------------------------------
// ExtractFileByHash
// ---------------------------------------------------------------------------

func TestExtractFileByHash_Hit(t *testing.T) {
	m := build100FileManifest()
	client := buildMockChunksForManifest(t, m)
	se := NewSelectiveExtractor(m, client, 0)

	destDir := t.TempDir()

	// File 7 lives in chunk 0.
	hash := fmt.Sprintf("%032x", 7)
	stats, err := se.ExtractFileByHash(context.Background(), hash, destDir)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Restored)
	assert.Equal(t, int64(0), stats.Failed)
	assert.Equal(t, int64(1), stats.ChunksDownloaded)

	// Verify file exists on disk.
	_, statErr := os.Stat(filepath.Join(destDir, "data/file-007.bin"))
	require.NoError(t, statErr)
}

func TestExtractFileByHash_Miss(t *testing.T) {
	m := build100FileManifest()
	se := NewSelectiveExtractor(m, &mockS3Client{}, 0)

	_, err := se.ExtractFileByHash(context.Background(), "nonexistenthash00000000000000000", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistenthash")
}

// ---------------------------------------------------------------------------
// BatchRestore — chunk grouping and download minimisation
// ---------------------------------------------------------------------------

// TestBatchRestore_MinimizesDownloads is the integration scenario from Issue #189:
// 100 files in 10 chunks; restore 5 files from 2 distinct chunks → 2 downloads.
func TestBatchRestore_MinimizesDownloads(t *testing.T) {
	m := build100FileManifest()
	client := buildMockChunksForManifest(t, m)
	se := NewSelectiveExtractor(m, client, 0)

	destDir := t.TempDir()

	// Files 0-9 → chunk 0; files 10-19 → chunk 1.
	// Pick 3 from chunk 0 and 2 from chunk 1.
	targets := []string{
		"data/file-000.bin",
		"data/file-003.bin",
		"data/file-005.bin",
		"data/file-010.bin",
		"data/file-012.bin",
	}

	stats, err := se.BatchRestore(context.Background(), targets, destDir)
	require.NoError(t, err)
	assert.Equal(t, int64(5), stats.Restored)
	assert.Equal(t, int64(0), stats.Failed)
	assert.Equal(t, int64(2), stats.ChunksDownloaded)

	for _, path := range targets {
		_, statErr := os.Stat(filepath.Join(destDir, path))
		require.NoError(t, statErr, "file %s should be extracted", path)
	}
}

func TestBatchRestore_CacheEliminatesRedundantDownloads(t *testing.T) {
	m := build100FileManifest()
	client := buildMockChunksForManifest(t, m)
	se := NewSelectiveExtractor(m, client, 0)

	destDir := t.TempDir()

	// First call restores file from chunk 0.
	stats1, err := se.BatchRestore(context.Background(), []string{"data/file-000.bin"}, destDir)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats1.ChunksDownloaded)

	// Second call restores another file from the same chunk.
	// Cache must absorb the download — ChunksDownloaded must be 0.
	destDir2 := t.TempDir()
	stats2, err := se.BatchRestore(context.Background(), []string{"data/file-001.bin"}, destDir2)
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats2.ChunksDownloaded, "chunk should come from cache")
	assert.Equal(t, int64(1), stats2.Restored)
}

func TestBatchRestore_UnknownTarget_IncrementsFailed(t *testing.T) {
	m := build100FileManifest()
	se := NewSelectiveExtractor(m, &mockS3Client{chunks: map[string][]byte{}}, 0)

	stats, err := se.BatchRestore(context.Background(), []string{"does/not/exist.bin"}, t.TempDir())
	require.NoError(t, err) // no error returned for unknown paths
	assert.Equal(t, int64(0), stats.Restored)
	assert.Equal(t, int64(1), stats.Failed)
	assert.Equal(t, int64(0), stats.ChunksDownloaded)
}

func TestBatchRestore_EmptyTargets(t *testing.T) {
	m := build100FileManifest()
	se := NewSelectiveExtractor(m, &mockS3Client{}, 0)

	stats, err := se.BatchRestore(context.Background(), nil, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.Restored)
	assert.Equal(t, int64(0), stats.ChunksDownloaded)
}

// ---------------------------------------------------------------------------
// BatchRestoreByDVCStage
// ---------------------------------------------------------------------------

func TestBatchRestoreByDVCStage_Hit(t *testing.T) {
	m := build100FileManifest()
	client := buildMockChunksForManifest(t, m)
	se := NewSelectiveExtractor(m, client, 0)

	destDir := t.TempDir()
	stats, err := se.BatchRestoreByDVCStage(context.Background(), "preprocess", destDir)
	require.NoError(t, err)
	// 50 even-indexed files are tagged with "preprocess"
	assert.Equal(t, int64(50), stats.Restored)
	assert.Equal(t, int64(10), stats.ChunksDownloaded) // all 10 chunks needed
}

func TestBatchRestoreByDVCStage_Miss(t *testing.T) {
	m := build100FileManifest()
	se := NewSelectiveExtractor(m, &mockS3Client{}, 0)
	_, err := se.BatchRestoreByDVCStage(context.Background(), "nonexistent", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

// ---------------------------------------------------------------------------
// BatchRestoreByCommit
// ---------------------------------------------------------------------------

func TestBatchRestoreByCommit_Hit(t *testing.T) {
	m := build100FileManifest()
	client := buildMockChunksForManifest(t, m)
	se := NewSelectiveExtractor(m, client, 0)

	destDir := t.TempDir()
	stats, err := se.BatchRestoreByCommit(context.Background(), "deadbeef", destDir)
	require.NoError(t, err)
	assert.Equal(t, int64(100), stats.Restored) // all files in manifest
}

func TestBatchRestoreByCommit_Miss(t *testing.T) {
	m := build100FileManifest()
	se := NewSelectiveExtractor(m, &mockS3Client{}, 0)
	_, err := se.BatchRestoreByCommit(context.Background(), "unknown", t.TempDir())
	require.Error(t, err)
}
