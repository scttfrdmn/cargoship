package manifest

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
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

// ---------------------------------------------------------------------------
// ChunkKeysForPaths / ChunkKeysForDVCStage / ChunkKeysForCommit / AllChunkKeys
// ---------------------------------------------------------------------------

func TestChunkKeysForPaths_Basic(t *testing.T) {
	m := build100FileManifest()
	se := NewSelectiveExtractor(m, &mockS3Client{}, 0)

	// Files 0 and 1 are both in chunk-0.
	keys := se.ChunkKeysForPaths([]string{"data/file-000.bin", "data/file-001.bin"})
	assert.Equal(t, []string{"shard-0/chunk-0.tar.zst"}, keys)
}

func TestChunkKeysForPaths_Deduplicates(t *testing.T) {
	m := build100FileManifest()
	se := NewSelectiveExtractor(m, &mockS3Client{}, 0)

	// Both files are in chunk-0 — result must deduplicate to a single key.
	keys := se.ChunkKeysForPaths([]string{"data/file-000.bin", "data/file-000.bin"})
	assert.Len(t, keys, 1)
}

func TestChunkKeysForPaths_UnknownSkipped(t *testing.T) {
	m := build100FileManifest()
	se := NewSelectiveExtractor(m, &mockS3Client{}, 0)

	keys := se.ChunkKeysForPaths([]string{"does/not/exist.bin"})
	assert.Empty(t, keys)
}

func TestChunkKeysForDVCStage(t *testing.T) {
	m := build100FileManifest()
	se := NewSelectiveExtractor(m, &mockS3Client{}, 0)

	// "preprocess" stage has every even-indexed file — 50 files across all 10 chunks.
	keys := se.ChunkKeysForDVCStage("preprocess")
	assert.Len(t, keys, 10)
}

func TestChunkKeysForDVCStage_Unknown(t *testing.T) {
	m := build100FileManifest()
	se := NewSelectiveExtractor(m, &mockS3Client{}, 0)
	keys := se.ChunkKeysForDVCStage("no-such-stage")
	assert.Empty(t, keys)
}

func TestChunkKeysForCommit(t *testing.T) {
	m := build100FileManifest()
	se := NewSelectiveExtractor(m, &mockS3Client{}, 0)

	// Manifest commit "deadbeef" covers all 100 files across all 10 chunks.
	keys := se.ChunkKeysForCommit("deadbeef")
	assert.Len(t, keys, 10)
}

func TestChunkKeysForCommit_Unknown(t *testing.T) {
	m := build100FileManifest()
	se := NewSelectiveExtractor(m, &mockS3Client{}, 0)
	keys := se.ChunkKeysForCommit("unknown-sha")
	assert.Empty(t, keys)
}

func TestAllChunkKeys(t *testing.T) {
	m := build100FileManifest()
	se := NewSelectiveExtractor(m, &mockS3Client{}, 0)

	keys := se.AllChunkKeys()
	assert.Len(t, keys, 10)
	// Spot-check that known keys are present.
	assert.Contains(t, keys, "shard-0/chunk-0.tar.zst")
	assert.Contains(t, keys, "shard-0/chunk-9.tar.zst")
}

// ---------------------------------------------------------------------------
// #334: the chunk-key accessors feed a Glacier pre-flight HeadObject, so they
// must return the key that actually addresses an object.
//
// Every test above runs against build100FileManifest, which sets NO Prefix — so
// the raw and resolved forms are identical and the missing normalization cannot
// show up. Real uploads to `s3://bucket/archives` record S3Key relative to the
// prefix, the pre-flight HeadObject 404s on the raw value, and the restore
// aborts with "glacier pre-flight check failed". These use a prefixed manifest,
// which is what the fixture should have had all along.
// ---------------------------------------------------------------------------

// buildPrefixedManifest mirrors what a real chunked upload to
// s3://test-bucket/archives writes: S3Key values relative to Prefix.
func buildPrefixedManifest() *Manifest {
	m := build100FileManifest()
	m.Prefix = "archives"
	for i := range m.Chunks {
		m.Chunks[i].S3Key = "uploads/batch-test-001/" + m.Chunks[i].S3Key
	}
	for i := range m.Files {
		m.Files[i].S3Key = "uploads/batch-test-001/" + m.Files[i].S3Key
	}
	return m
}

func TestChunkKeysForPaths_ResolvesAgainstPrefix(t *testing.T) {
	se := NewSelectiveExtractor(buildPrefixedManifest(), &mockS3Client{}, 0)

	keys := se.ChunkKeysForPaths([]string{"data/file-000.bin"})

	// Must be the real object key. Without the prefix this is a 404.
	assert.Equal(t, []string{"archives/uploads/batch-test-001/shard-0/chunk-0.tar.zst"}, keys)
}

func TestAllChunkKeys_ResolvesAgainstPrefix(t *testing.T) {
	se := NewSelectiveExtractor(buildPrefixedManifest(), &mockS3Client{}, 0)

	for _, key := range se.AllChunkKeys() {
		assert.True(t, strings.HasPrefix(key, "archives/"),
			"chunk key %q is not prefix-resolved; a pre-flight HeadObject on it would 404", key)
	}
}

// TestChunkKeysForPaths_MatchesDownloadedKeys is the property that matters: the
// keys the pre-flight check verifies must be the keys the restore then fetches.
// Asserting the resolved string alone would still allow the two paths to drift;
// this pins them to each other.
func TestChunkKeysForPaths_MatchesDownloadedKeys(t *testing.T) {
	m := buildPrefixedManifest()
	client := &mockS3Client{chunks: make(map[string][]byte)}
	for _, chunk := range m.Chunks {
		var files []FileEntry
		for _, fe := range m.Files {
			if fe.ChunkID == chunk.ID {
				files = append(files, fe)
			}
		}
		// Key the mock by the RESOLVED key — that is where the object lives.
		client.chunks[ResolveObjectKey(m.Prefix, m.Bucket, chunk.S3Key)] = buildZstdTar(t, files)
	}
	se := NewSelectiveExtractor(m, client, 0)

	targets := []string{"data/file-000.bin", "data/file-050.bin"}
	preflight := se.ChunkKeysForPaths(targets)

	stats, err := se.BatchRestore(context.Background(), targets, t.TempDir())
	require.NoError(t, err)
	require.Zero(t, stats.Failed)

	sort.Strings(preflight)
	fetched := client.requestedKeys()
	sort.Strings(fetched)
	assert.Equal(t, preflight, fetched,
		"pre-flight checked %v but restore fetched %v", preflight, fetched)
}

// TestChunkKeysForPaths_ResolvesBasenameTargets covers the second half of #334:
// the pre-flight used exact-match lookup while BatchRestore falls back to
// basename/suffix matching (#228). A basename target therefore yielded zero
// keys, the Glacier check silently verified nothing, and the restore proceeded
// — so the documented `--file greeting.txt` ergonomic had no Glacier protection
// at all. That vacuous pass is why the e2e round-trip never caught the 404.
func TestChunkKeysForPaths_ResolvesBasenameTargets(t *testing.T) {
	se := NewSelectiveExtractor(buildPrefixedManifest(), &mockS3Client{}, 0)

	keys := se.ChunkKeysForPaths([]string{"file-000.bin"})

	assert.Equal(t, []string{"archives/uploads/batch-test-001/shard-0/chunk-0.tar.zst"}, keys,
		"a basename target must yield the chunk BatchRestore would download, not nothing")
}

// TestBatchRestore_ChunkedFullURLS3Key is the regression for the round-trip
// property test's finding: a chunked manifest whose ChunkEntry.S3Key was stored
// as a full URL (the #273 wart — some upload managers return result.Location as
// a URL) must still restore. downloadChunk normalizes the key via
// ResolveObjectKey; before that fix, BatchRestore failed every file with
// "chunk not found" / a GetObject miss.
func TestBatchRestore_ChunkedFullURLS3Key(t *testing.T) {
	fileContent := []byte("chunked file restored via a full-URL S3 key")
	filePath := "/abs/src/data/report.txt"

	// The object is stored under the real, prefix-relative key...
	realKey := "backups/uploads/u1/shard-0/chunk-0.tar.zst"
	chunk := makeTarZst(t, map[string][]byte{filePath: fileContent})
	client := &mockS3Client{chunks: map[string][]byte{realKey: chunk}}

	// ...but the manifest records S3Key as a full URL (as an SDK Location may).
	m := &Manifest{
		Version:         ManifestVersion,
		Bucket:          "test-bucket",
		Prefix:          "backups",
		CompressionType: "zstd",
		TotalChunks:     1,
		Chunks: []ChunkEntry{{
			ID: 0, ShardID: 0,
			S3Key:     "http://127.0.0.1:9000/test-bucket/backups/uploads/u1/shard-0/chunk-0.tar.zst",
			FileCount: 1, FilePaths: []string{filePath},
		}},
		Files: []FileEntry{{
			Path:  filePath,
			Size:  int64(len(fileContent)),
			S3Key: "http://127.0.0.1:9000/test-bucket/backups/uploads/u1/shard-0/chunk-0.tar.zst",
		}},
	}
	m.TotalFiles = 1

	se := NewSelectiveExtractor(m, client, 0)
	dest := t.TempDir()
	stats, err := se.BatchRestore(context.Background(), []string{"report.txt"}, dest)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Restored, "should restore despite the full-URL S3Key (failed=%d)", stats.Failed)
	assert.Zero(t, stats.Failed)

	// Content round-trips byte-for-byte (chunked mode writes under the source path).
	got, err := os.ReadFile(filepath.Join(dest, filePath))
	require.NoError(t, err)
	assert.Equal(t, fileContent, got)
}

// ---------------------------------------------------------------------------
// #337: a truncated chunk must not yield a short file that looks complete.
// ---------------------------------------------------------------------------

// makeTruncatedTarZst builds a chunk whose tar stream is cut off mid-entry,
// reproducing what v0.14.0/v0.15.0 wrote under #275 (the final zstd frame was
// never flushed, so the last file in each chunk is short). Written by hand
// rather than by mutating a good archive so the truncation is exact and the test
// does not depend on compression internals.
func makeTruncatedTarZst(t *testing.T, name string, content []byte, keep int) []byte {
	t.Helper()
	require.Less(t, keep, len(content), "keep must be a real truncation")

	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	// Header declares the FULL size; only `keep` bytes of body follow. That
	// mismatch is exactly the on-disk shape of the bug.
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(content))}))
	_, err := tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	// Cut the uncompressed tar mid-body, then compress the fragment.
	full := raw.Bytes()
	cut := full[:512+keep] // 512-byte tar header + partial body

	var out bytes.Buffer
	zw, err := zstd.NewWriter(&out)
	require.NoError(t, err)
	_, err = zw.Write(cut)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return out.Bytes()
}

// TestBatchRestore_TruncatedChunk_NoChecksum is the #337 regression.
//
// The manifest records NO per-file checksum — the case for every archive written
// before #270, which is precisely the set that carries the #275 truncation bug.
// That combination sends the restore down the unverified extract path, where
// io.Copy cannot distinguish a truncated entry from EOF and so returned success
// after writing a short file.
func TestBatchRestore_TruncatedChunk_NoChecksum(t *testing.T) {
	filePath := "/abs/src/big.bin"
	content := bytes.Repeat([]byte("cargoship"), 4096) // 36864 bytes
	const kept = 20000

	chunk := makeTruncatedTarZst(t, filePath, content, kept)
	client := &mockS3Client{chunks: map[string][]byte{"p/chunk-0.tar.zst": chunk}}

	m := &Manifest{
		Version:         ManifestVersion,
		Bucket:          "b",
		Prefix:          "p",
		CompressionType: "zstd",
		TotalChunks:     1,
		TotalFiles:      1,
		Chunks: []ChunkEntry{{
			ID: 0, S3Key: "chunk-0.tar.zst",
			FileCount: 1, FilePaths: []string{filePath},
		}},
		// No Checksum and no ChecksumAlgorithm: pre-#270 manifest shape.
		Files: []FileEntry{{Path: filePath, Size: int64(len(content)), S3Key: "chunk-0.tar.zst"}},
	}

	se := NewSelectiveExtractor(m, client, 0)
	dest := t.TempDir()
	stats, err := se.BatchRestore(context.Background(), []string{filePath}, dest)
	require.NoError(t, err, "BatchRestore itself is fault-tolerant; it counts the failure")

	assert.Zero(t, stats.Restored, "a truncated file must not count as restored")
	assert.Equal(t, int64(1), stats.Failed)

	// The decisive assertion: nothing short left behind. A missing file is a
	// correct, loud outcome; a short one that looks complete is not.
	if got, readErr := os.ReadFile(filepath.Join(dest, filePath)); readErr == nil {
		t.Errorf("restore left a truncated file on disk: %d bytes, manifest declares %d",
			len(got), len(content))
	}
}

// TestBatchRestore_TruncatedChunk_WithChecksum confirms the verified path was
// already safe, so the #337 fix closed a gap rather than moving one.
func TestBatchRestore_TruncatedChunk_WithChecksum(t *testing.T) {
	filePath := "/abs/src/big.bin"
	content := bytes.Repeat([]byte("cargoship"), 4096)

	chunk := makeTruncatedTarZst(t, filePath, content, 20000)
	client := &mockS3Client{chunks: map[string][]byte{"p/chunk-0.tar.zst": chunk}}

	m := &Manifest{
		Version:           ManifestVersion,
		Bucket:            "b",
		Prefix:            "p",
		CompressionType:   "zstd",
		ChecksumAlgorithm: ChecksumAlgorithmSHA256,
		TotalChunks:       1,
		TotalFiles:        1,
		Chunks: []ChunkEntry{{
			ID: 0, S3Key: "chunk-0.tar.zst",
			FileCount: 1, FilePaths: []string{filePath},
		}},
		Files: []FileEntry{{Path: filePath, Size: int64(len(content)), S3Key: "chunk-0.tar.zst", Checksum: sha256hex(content)}},
	}

	se := NewSelectiveExtractor(m, client, 0)
	dest := t.TempDir()
	stats, err := se.BatchRestore(context.Background(), []string{filePath}, dest)
	require.NoError(t, err)
	assert.Zero(t, stats.Restored)
	assert.Equal(t, int64(1), stats.Failed)

	_, readErr := os.ReadFile(filepath.Join(dest, filePath))
	assert.Error(t, readErr, "verified path must not write a truncated file either")
}

// --- Failure injection: restore must detect and refuse corrupt data (#270) ---

// TestBatchRestore_Direct_DetectsCorruption verifies restore fails a
// direct-upload file whose stored object no longer matches the recorded
// checksum, rather than silently writing the corrupt bytes.
func TestBatchRestore_Direct_DetectsCorruption(t *testing.T) {
	good := []byte("the original file content")
	m := &Manifest{
		Version: ManifestVersion, Bucket: "b", CompressionType: "none",
		ChecksumAlgorithm: ChecksumAlgorithmSHA256, TotalChunks: 0,
		Files: []FileEntry{{
			Path: "/abs/src/file.txt", Size: int64(len(good)),
			S3Key: "archives/file.txt", Checksum: sha256hex(good),
		}},
	}
	m.TotalFiles = 1
	// Object in S3 is CORRUPTED (does not match the recorded checksum).
	client := &mockS3Client{chunks: map[string][]byte{"archives/file.txt": []byte("TAMPERED content!!")}}

	se := NewSelectiveExtractor(m, client, 0)
	dest := t.TempDir()
	stats, err := se.BatchRestore(context.Background(), []string{"file.txt"}, dest)
	require.NoError(t, err) // BatchRestore aggregates per-file failures, not a hard error
	assert.Equal(t, int64(0), stats.Restored, "corrupt file must not count as restored")
	assert.Equal(t, int64(1), stats.Failed, "corrupt file must be failed")

	// The corrupt bytes must NOT be on disk (structure-preserving path, #282).
	_, statErr := os.Stat(filepath.Join(dest, "abs/src/file.txt"))
	assert.True(t, os.IsNotExist(statErr), "corrupt file must not be written to disk")
}

// TestBatchRestore_Direct_OptOutSkipsVerification confirms SetVerify(false)
// restores even a mismatching object (opt-out escape hatch).
func TestBatchRestore_Direct_OptOutSkipsVerification(t *testing.T) {
	good := []byte("the original file content")
	tampered := []byte("TAMPERED content!!")
	m := &Manifest{
		Version: ManifestVersion, Bucket: "b", CompressionType: "none",
		ChecksumAlgorithm: ChecksumAlgorithmSHA256, TotalChunks: 0,
		Files: []FileEntry{{Path: "/abs/src/file.txt", Size: int64(len(good)), S3Key: "k", Checksum: sha256hex(good)}},
	}
	m.TotalFiles = 1
	client := &mockS3Client{chunks: map[string][]byte{"k": tampered}}

	se := NewSelectiveExtractor(m, client, 0).SetVerify(false)
	dest := t.TempDir()
	stats, err := se.BatchRestore(context.Background(), []string{"file.txt"}, dest)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Restored, "opt-out restores without verifying")
	got, _ := os.ReadFile(filepath.Join(dest, "abs/src/file.txt"))
	assert.Equal(t, tampered, got)
}

// TestBatchRestore_Direct_NoChecksumRestores confirms a pre-checksum manifest
// (no recorded checksum) still restores — verification only guards what it can.
func TestBatchRestore_Direct_NoChecksumRestores(t *testing.T) {
	content := []byte("legacy file, no checksum recorded")
	m := &Manifest{
		Version: ManifestVersion, Bucket: "b", CompressionType: "none", TotalChunks: 0,
		Files: []FileEntry{{Path: "/abs/src/legacy.txt", Size: int64(len(content)), S3Key: "k"}}, // no Checksum
	}
	m.TotalFiles = 1
	client := &mockS3Client{chunks: map[string][]byte{"k": content}}

	se := NewSelectiveExtractor(m, client, 0)
	dest := t.TempDir()
	stats, err := se.BatchRestore(context.Background(), []string{"legacy.txt"}, dest)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Restored)
	assert.Equal(t, int64(0), stats.Failed)
}

// TestBatchRestore_Chunked_DetectsCorruption verifies the chunked path also
// refuses a file whose extracted content doesn't match the recorded checksum.
func TestBatchRestore_Chunked_DetectsCorruption(t *testing.T) {
	good := []byte("chunked original content")
	tampered := []byte("chunked TAMPERED content")
	filePath := "/abs/src/report.txt"

	// The chunk actually contains tampered bytes; the manifest records good's hash.
	chunk := makeTarZst(t, map[string][]byte{filePath: tampered})
	client := &mockS3Client{chunks: map[string][]byte{"backups/uploads/u/shard-0/chunk-0.tar.zst": chunk}}
	m := &Manifest{
		Version: ManifestVersion, Bucket: "b", Prefix: "backups",
		CompressionType: "zstd", ChecksumAlgorithm: ChecksumAlgorithmSHA256, TotalChunks: 1,
		Chunks: []ChunkEntry{{ID: 0, S3Key: "backups/uploads/u/shard-0/chunk-0.tar.zst", FileCount: 1, FilePaths: []string{filePath}}},
		Files:  []FileEntry{{Path: filePath, Size: int64(len(good)), S3Key: "backups/uploads/u/shard-0/chunk-0.tar.zst", Checksum: sha256hex(good)}},
	}
	m.TotalFiles = 1

	se := NewSelectiveExtractor(m, client, 0)
	dest := t.TempDir()
	stats, err := se.BatchRestore(context.Background(), []string{"report.txt"}, dest)
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.Restored, "corrupt chunked file must not restore")
	assert.Equal(t, int64(1), stats.Failed)
	_, statErr := os.Stat(filepath.Join(dest, filePath))
	assert.True(t, os.IsNotExist(statErr), "corrupt chunked file must not be written")
}

// TestBatchRestore_Chunked_GoodContentVerifies confirms the verifying chunked
// path still restores correct content byte-for-byte.
func TestBatchRestore_Chunked_GoodContentVerifies(t *testing.T) {
	good := []byte("chunked content that matches its checksum")
	filePath := "/abs/src/ok.txt"
	chunk := makeTarZst(t, map[string][]byte{filePath: good})
	client := &mockS3Client{chunks: map[string][]byte{"p/uploads/u/shard-0/chunk-0.tar.zst": chunk}}
	m := &Manifest{
		Version: ManifestVersion, Bucket: "b", Prefix: "p",
		CompressionType: "zstd", ChecksumAlgorithm: ChecksumAlgorithmSHA256, TotalChunks: 1,
		Chunks: []ChunkEntry{{ID: 0, S3Key: "p/uploads/u/shard-0/chunk-0.tar.zst", FileCount: 1, FilePaths: []string{filePath}}},
		Files:  []FileEntry{{Path: filePath, Size: int64(len(good)), S3Key: "p/uploads/u/shard-0/chunk-0.tar.zst", Checksum: sha256hex(good)}},
	}
	m.TotalFiles = 1

	se := NewSelectiveExtractor(m, client, 0)
	dest := t.TempDir()
	stats, err := se.BatchRestore(context.Background(), []string{"ok.txt"}, dest)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Restored)
	assert.Equal(t, int64(0), stats.Failed)
	got, err := os.ReadFile(filepath.Join(dest, filePath))
	require.NoError(t, err)
	assert.Equal(t, good, got)
}

// --- #282: restore layout parity + path-traversal safety ---

// TestSafeRestorePath covers the escape-safe, structure-preserving mapping used
// by both restore paths.
func TestSafeRestorePath(t *testing.T) {
	dest := t.TempDir()
	tests := []struct {
		name      string
		entryPath string
		wantRel   string // expected path relative to dest ("" = expect error)
	}{
		{"relative", "data/a.txt", "data/a.txt"},
		{"absolute source path", "/abs/src/data/a.txt", "abs/src/data/a.txt"},
		{"nested", "a/b/c/deep.bin", "a/b/c/deep.bin"},
		{"dot segments stripped", "a/./b/c.txt", "a/b/c.txt"},
		{"parent traversal stripped", "../../etc/passwd", "etc/passwd"},
		{"embedded traversal stripped", "a/../../b/c.txt", "a/b/c.txt"},
		{"all traversal -> error", "../../..", ""},
		{"empty -> error", "", ""},
	}
	// No SourcePath, no flatten: exercises the sanitization/containment layer.
	se := &SelectiveExtractor{manifest: &Manifest{}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := se.restorePath(dest, tt.entryPath)
			if tt.wantRel == "" {
				assert.Error(t, err, "should reject %q", tt.entryPath)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(dest, filepath.FromSlash(tt.wantRel)), got)
			// Never escapes dest.
			assert.True(t, got == dest || filepathHasPrefix(got, dest),
				"%q resolved outside dest: %s", tt.entryPath, got)
		})
	}
}

func filepathHasPrefix(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	// Contained iff the relative path doesn't start with a ".." segment.
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// TestBatchRestore_TraversalIsContained proves a hostile manifest whose
// FileEntry.Path tries to escape the destination cannot write outside destDir.
func TestBatchRestore_TraversalIsContained(t *testing.T) {
	content := []byte("payload")
	// Manifest claims the file lives at a traversal path.
	m := &Manifest{
		Version: ManifestVersion, Bucket: "b", CompressionType: "none", TotalChunks: 0,
		Files: []FileEntry{{Path: "../../../../tmp/evil.txt", Size: int64(len(content)), S3Key: "k"}},
	}
	m.TotalFiles = 1
	client := &mockS3Client{chunks: map[string][]byte{"k": content}}

	se := NewSelectiveExtractor(m, client, 0)
	dest := t.TempDir()
	stats, err := se.BatchRestore(context.Background(), []string{"evil.txt"}, dest)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.Restored)

	// It must land inside dest (as tmp/evil.txt), never at the traversal target.
	got, err := os.ReadFile(filepath.Join(dest, "tmp/evil.txt"))
	require.NoError(t, err, "file should be contained within dest")
	assert.Equal(t, content, got)
}

// TestBatchRestore_LayoutParity confirms direct and chunked modes restore the
// same logical file to the SAME location under destDir (#282).
func TestBatchRestore_LayoutParity(t *testing.T) {
	content := []byte("parity content")
	srcPath := "/abs/src/dir/report.txt"
	wantRel := "abs/src/dir/report.txt"

	// Direct manifest.
	directM := &Manifest{
		Version: ManifestVersion, Bucket: "b", CompressionType: "none", TotalChunks: 0,
		Files: []FileEntry{{Path: srcPath, Size: int64(len(content)), S3Key: "obj"}},
	}
	directM.TotalFiles = 1
	directClient := &mockS3Client{chunks: map[string][]byte{"obj": content}}
	directDest := t.TempDir()
	_, err := NewSelectiveExtractor(directM, directClient, 0).
		BatchRestore(context.Background(), []string{srcPath}, directDest)
	require.NoError(t, err)

	// Chunked manifest with the same file.
	chunk := makeTarZst(t, map[string][]byte{srcPath: content})
	chunkedM := &Manifest{
		Version: ManifestVersion, Bucket: "b", Prefix: "p", CompressionType: "zstd", TotalChunks: 1,
		Chunks: []ChunkEntry{{ID: 0, S3Key: "p/uploads/u/shard-0/chunk-0.tar.zst", FileCount: 1, FilePaths: []string{srcPath}}},
		Files:  []FileEntry{{Path: srcPath, Size: int64(len(content)), S3Key: "p/uploads/u/shard-0/chunk-0.tar.zst"}},
	}
	chunkedM.TotalFiles = 1
	chunkedClient := &mockS3Client{chunks: map[string][]byte{"p/uploads/u/shard-0/chunk-0.tar.zst": chunk}}
	chunkedDest := t.TempDir()
	_, err = NewSelectiveExtractor(chunkedM, chunkedClient, 0).
		BatchRestore(context.Background(), []string{srcPath}, chunkedDest)
	require.NoError(t, err)

	// Both landed at the identical relative location.
	d, err := os.ReadFile(filepath.Join(directDest, wantRel))
	require.NoError(t, err, "direct mode should write to %s", wantRel)
	c, err := os.ReadFile(filepath.Join(chunkedDest, wantRel))
	require.NoError(t, err, "chunked mode should write to the SAME %s", wantRel)
	assert.Equal(t, content, d)
	assert.Equal(t, content, c)
}

// --- #287: dataset-relative default layout + --flatten ---

// TestRestorePath_StripsSourceRoot verifies the default layout is relative to
// the manifest SourcePath (upload root), not rooted at "/".
func TestRestorePath_StripsSourceRoot(t *testing.T) {
	dest := t.TempDir()
	se := &SelectiveExtractor{manifest: &Manifest{SourcePath: "/home/u/project"}}

	tests := []struct {
		entry, want string
	}{
		{"/home/u/project/data/a.txt", "data/a.txt"}, // under root -> relative
		{"/home/u/project/top.txt", "top.txt"},       // directly under root
		{"/home/u/project", "project"},               // the root itself -> basename
		{"/etc/passwd", "etc/passwd"},                // outside root -> sanitized full
		{"/home/u/project2/x", "home/u/project2/x"},  // sibling, not under root (no false prefix)
	}
	for _, tt := range tests {
		got, err := se.restorePath(dest, tt.entry)
		require.NoError(t, err, tt.entry)
		assert.Equal(t, filepath.Join(dest, filepath.FromSlash(tt.want)), got, "entry %s", tt.entry)
	}
}

// TestRestorePath_Flatten verifies flatten mode writes basenames into destDir.
func TestRestorePath_Flatten(t *testing.T) {
	dest := t.TempDir()
	se := (&SelectiveExtractor{manifest: &Manifest{SourcePath: "/home/u/project"}}).SetFlatten(true)

	got, err := se.restorePath(dest, "/home/u/project/data/deep/report.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dest, "report.txt"), got)
}

// TestBatchRestore_SourceRelativeLayout is the end-to-end #287 default: a file
// uploaded from a source root restores relative to that root.
func TestBatchRestore_SourceRelativeLayout(t *testing.T) {
	content := []byte("dataset-relative content")
	m := &Manifest{
		Version: ManifestVersion, Bucket: "b", CompressionType: "none", TotalChunks: 0,
		SourcePath: "/data/project",
		Files:      []FileEntry{{Path: "/data/project/sub/report.txt", Size: int64(len(content)), S3Key: "k"}},
	}
	m.TotalFiles = 1
	client := &mockS3Client{chunks: map[string][]byte{"k": content}}

	dest := t.TempDir()
	stats, err := NewSelectiveExtractor(m, client, 0).
		BatchRestore(context.Background(), []string{"report.txt"}, dest)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.Restored)

	// Lands at dest/sub/report.txt (root stripped), NOT dest/data/project/sub/...
	got, err := os.ReadFile(filepath.Join(dest, "sub/report.txt"))
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// TestBatchRestore_FlattenLayout is the end-to-end --flatten default: files land
// at dest/<basename>.
func TestBatchRestore_FlattenLayout(t *testing.T) {
	content := []byte("flat content")
	m := &Manifest{
		Version: ManifestVersion, Bucket: "b", CompressionType: "none", TotalChunks: 0,
		SourcePath: "/data/project",
		Files:      []FileEntry{{Path: "/data/project/sub/report.txt", Size: int64(len(content)), S3Key: "k"}},
	}
	m.TotalFiles = 1
	client := &mockS3Client{chunks: map[string][]byte{"k": content}}

	dest := t.TempDir()
	stats, err := NewSelectiveExtractor(m, client, 0).SetFlatten(true).
		BatchRestore(context.Background(), []string{"report.txt"}, dest)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.Restored)

	got, err := os.ReadFile(filepath.Join(dest, "report.txt"))
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// --- #311: chunked-mode containment and mtime preservation ---

// TestBatchRestore_ChunkedTraversalIsContained is the chunked-storage twin of
// TestBatchRestore_TraversalIsContained. `download` carried its own tar loop for
// this path that joined the untrusted tar header name straight onto the output
// directory (#311); this pins the shared path's guard so a future caller can't
// reintroduce the escape.
func TestBatchRestore_ChunkedTraversalIsContained(t *testing.T) {
	content := []byte("chunked payload")
	evil := "../../../../tmp/evil-chunked.txt"
	chunk := makeTarZst(t, map[string][]byte{evil: content})

	m := &Manifest{
		Version: ManifestVersion, Bucket: "b", CompressionType: "zstd", TotalChunks: 1,
		Chunks: []ChunkEntry{{ID: 0, S3Key: "c0", FileCount: 1, FilePaths: []string{evil}}},
		Files:  []FileEntry{{Path: evil, Size: int64(len(content)), S3Key: "c0"}},
	}
	m.TotalFiles = 1
	client := &mockS3Client{chunks: map[string][]byte{"c0": chunk}}

	dest := t.TempDir()
	stats, err := NewSelectiveExtractor(m, client, 0).
		BatchRestore(context.Background(), []string{evil}, dest)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.Restored)

	// Sanitized to a contained path, not the traversal target.
	got, err := os.ReadFile(filepath.Join(dest, "tmp", "evil-chunked.txt"))
	require.NoError(t, err, "file should be contained within dest")
	assert.Equal(t, content, got)
	assert.True(t, filepathHasPrefix(filepath.Join(dest, "tmp", "evil-chunked.txt"), dest))
}

// TestBatchRestore_PreservesModTime checks restore reproduces the source tree's
// modification times rather than stamping the time of the restore. An archival
// tool that loses mtimes has lost metadata the manifest recorded. Covers both
// storage layouts (#311).
func TestBatchRestore_PreservesModTime(t *testing.T) {
	content := []byte("timestamped")
	// Truncated to whole seconds: tar and some filesystems don't keep sub-second
	// resolution, so comparing at second granularity is what's portable.
	want := time.Date(2019, 3, 14, 15, 9, 26, 0, time.UTC)

	t.Run("direct", func(t *testing.T) {
		m := &Manifest{
			Version: ManifestVersion, Bucket: "b", CompressionType: "none", TotalChunks: 0,
			Files: []FileEntry{{Path: "a.txt", Size: int64(len(content)), S3Key: "obj", ModTime: want}},
		}
		m.TotalFiles = 1
		client := &mockS3Client{chunks: map[string][]byte{"obj": content}}
		dest := t.TempDir()
		_, err := NewSelectiveExtractor(m, client, 0).
			BatchRestore(context.Background(), []string{"a.txt"}, dest)
		require.NoError(t, err)

		fi, err := os.Stat(filepath.Join(dest, "a.txt"))
		require.NoError(t, err)
		assert.WithinDuration(t, want, fi.ModTime(), time.Second)
	})

	t.Run("chunked", func(t *testing.T) {
		chunk := makeTarZst(t, map[string][]byte{"a.txt": content})
		m := &Manifest{
			Version: ManifestVersion, Bucket: "b", CompressionType: "zstd", TotalChunks: 1,
			Chunks: []ChunkEntry{{ID: 0, S3Key: "c0", FileCount: 1, FilePaths: []string{"a.txt"}}},
			Files:  []FileEntry{{Path: "a.txt", Size: int64(len(content)), S3Key: "c0", ModTime: want}},
		}
		m.TotalFiles = 1
		client := &mockS3Client{chunks: map[string][]byte{"c0": chunk}}
		dest := t.TempDir()
		_, err := NewSelectiveExtractor(m, client, 0).
			BatchRestore(context.Background(), []string{"a.txt"}, dest)
		require.NoError(t, err)

		fi, err := os.Stat(filepath.Join(dest, "a.txt"))
		require.NoError(t, err)
		assert.WithinDuration(t, want, fi.ModTime(), time.Second)
	})
}

// TestBatchRestore_ZeroModTimeLeavesFileAlone confirms a manifest with no
// recorded ModTime doesn't get stamped with the zero time (year 1), which some
// filesystems reject outright.
func TestBatchRestore_ZeroModTimeLeavesFileAlone(t *testing.T) {
	content := []byte("no mtime")
	m := &Manifest{
		Version: ManifestVersion, Bucket: "b", CompressionType: "none", TotalChunks: 0,
		Files: []FileEntry{{Path: "a.txt", Size: int64(len(content)), S3Key: "obj"}},
	}
	m.TotalFiles = 1
	client := &mockS3Client{chunks: map[string][]byte{"obj": content}}
	dest := t.TempDir()
	_, err := NewSelectiveExtractor(m, client, 0).
		BatchRestore(context.Background(), []string{"a.txt"}, dest)
	require.NoError(t, err)

	fi, err := os.Stat(filepath.Join(dest, "a.txt"))
	require.NoError(t, err)
	assert.False(t, fi.ModTime().IsZero(), "should keep the write time, not stamp year 1")
	assert.True(t, fi.ModTime().Year() > 2000, "got %s", fi.ModTime())
}
