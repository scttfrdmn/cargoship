package pipeline

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// makeManifest builds a minimal *manifest.Manifest with the given file entries.
func makeManifest(files []manifest.FileEntry) *manifest.Manifest {
	return &manifest.Manifest{
		Version:  "2.0",
		UploadID: "test-upload-id",
		Files:    files,
	}
}

// writeFile creates a file with known content under dir and returns its absolute path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

// md5Of computes the MD5 hex digest of content using the manifest package helper.
func md5Of(t *testing.T, absPath string) string {
	t.Helper()
	h, err := manifest.ComputeContentHash(absPath)
	require.NoError(t, err)
	return h
}

// --- NewIncrementalScanner ---

func TestNewIncrementalScanner_NilManifest(t *testing.T) {
	_, err := NewIncrementalScanner(nil, "")
	require.Error(t, err, "nil manifest must return an error")
}

func TestNewIncrementalScanner_EmptyManifest(t *testing.T) {
	m := makeManifest(nil)
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)
	require.NotNil(t, s)
}

func TestNewIncrementalScanner_InvalidCacheFile(t *testing.T) {
	m := makeManifest(nil)
	// A directory path is not a valid cache file to create/load.
	dir := t.TempDir()
	badPath := filepath.Join(dir, "subdir-that-does-not-exist", "cache.json")
	// NewHashCache only fails on an actual read error of an *existing* file; a
	// non-existent path is silently treated as an empty cache, so no error here.
	_, err := NewIncrementalScanner(m, badPath)
	require.NoError(t, err)
}

// --- ShouldUpload: tier 1 (new file) ---

func TestShouldUpload_NewFile_NotInManifest(t *testing.T) {
	dir := t.TempDir()
	absPath := writeFile(t, dir, "new.txt", "new content")

	m := makeManifest(nil) // empty previous manifest
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	assert.True(t, s.ShouldUpload(absPath, "new.txt"), "file not in manifest must be uploaded")
}

// --- ShouldUpload: tier 2 (size change) ---

func TestShouldUpload_SizeChanged(t *testing.T) {
	dir := t.TempDir()
	absPath := writeFile(t, dir, "data.bin", "short")

	m := makeManifest([]manifest.FileEntry{
		{Path: "data.bin", Size: 9999, ContentHash: "somehash"},
	})
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	assert.True(t, s.ShouldUpload(absPath, "data.bin"), "size mismatch must trigger upload")
}

// --- ShouldUpload: tier 3 (hash comparison) ---

func TestShouldUpload_HashMatch_ShouldSkip(t *testing.T) {
	dir := t.TempDir()
	absPath := writeFile(t, dir, "model.pt", "stable model bytes")
	hash := md5Of(t, absPath)
	size := int64(len("stable model bytes"))

	m := makeManifest([]manifest.FileEntry{
		{Path: "model.pt", Size: size, ContentHash: hash},
	})
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	assert.False(t, s.ShouldUpload(absPath, "model.pt"), "identical hash must skip upload")
}

func TestShouldUpload_HashMismatch_ShouldUpload(t *testing.T) {
	dir := t.TempDir()
	absPath := writeFile(t, dir, "data.csv", "current content")
	size := int64(len("current content"))

	m := makeManifest([]manifest.FileEntry{
		// Same size as current file, but different hash — file was modified in-place.
		{Path: "data.csv", Size: size, ContentHash: "deadbeefdeadbeefdeadbeefdeadbeef"},
	})
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	assert.True(t, s.ShouldUpload(absPath, "data.csv"), "hash mismatch must trigger upload")
}

// --- ShouldUpload: no content hash in manifest ---

func TestShouldUpload_NoHash_SameSizeSkips(t *testing.T) {
	dir := t.TempDir()
	content := "fixed content"
	absPath := writeFile(t, dir, "readme.md", content)

	m := makeManifest([]manifest.FileEntry{
		// No ContentHash — size-only comparison applies.
		{Path: "readme.md", Size: int64(len(content)), ContentHash: ""},
	})
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	assert.False(t, s.ShouldUpload(absPath, "readme.md"),
		"no hash + same size must skip upload")
}

// --- ShouldUpload: file missing on disk ---

func TestShouldUpload_MissingFile_ConservativelyUploads(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "ghost.dat")

	m := makeManifest([]manifest.FileEntry{
		{Path: "ghost.dat", Size: 100, ContentHash: "abc123"},
	})
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	// File does not exist — stat will fail; must conservatively return true.
	assert.True(t, s.ShouldUpload(missing, "ghost.dat"),
		"missing file must be conservatively uploaded")
}

// --- FilterFiles ---

func TestFilterFiles_AllNew(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "aaa")
	writeFile(t, dir, "b.txt", "bbb")

	m := makeManifest(nil) // empty previous manifest
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	files, err := s.FilterFiles(dir)
	require.NoError(t, err)
	assert.Len(t, files, 2, "all new files must be in upload list")
}

func TestFilterFiles_AllUnchanged(t *testing.T) {
	dir := t.TempDir()
	pathA := writeFile(t, dir, "a.txt", "hello")
	pathB := writeFile(t, dir, "b.txt", "world")
	hashA := md5Of(t, pathA)
	hashB := md5Of(t, pathB)

	m := makeManifest([]manifest.FileEntry{
		{Path: "a.txt", Size: int64(len("hello")), ContentHash: hashA},
		{Path: "b.txt", Size: int64(len("world")), ContentHash: hashB},
	})
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	files, err := s.FilterFiles(dir)
	require.NoError(t, err)
	assert.Empty(t, files, "no files should be in upload list when all unchanged")
}

func TestFilterFiles_MixedChanges(t *testing.T) {
	dir := t.TempDir()
	pathA := writeFile(t, dir, "stable.txt", "stable content")
	writeFile(t, dir, "changed.txt", "new content")
	hashA := md5Of(t, pathA)

	m := makeManifest([]manifest.FileEntry{
		{Path: "stable.txt", Size: int64(len("stable content")), ContentHash: hashA},
		// "changed.txt" has different content vs. manifest (different hash)
		{Path: "changed.txt", Size: int64(len("new content")), ContentHash: "oldoldhasholdoldhasholdoldhashxx"},
	})
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	files, err := s.FilterFiles(dir)
	require.NoError(t, err)
	require.Len(t, files, 1, "only changed file must be in upload list")
	assert.Equal(t, "changed.txt", files[0])
}

func TestFilterFiles_SubdirectoryFiles(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	writeFile(t, sub, "deep.txt", "deep content")

	m := makeManifest(nil)
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	files, err := s.FilterFiles(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, filepath.Join("subdir", "deep.txt"), files[0],
		"relative path must include subdirectory")
}

func TestFilterFiles_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	m := makeManifest(nil)
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	files, err := s.FilterFiles(dir)
	require.NoError(t, err)
	assert.Empty(t, files)
}

// --- Stats ---

func TestStats_FilesSkipped(t *testing.T) {
	dir := t.TempDir()
	absPath := writeFile(t, dir, "data.bin", "stable")
	hash := md5Of(t, absPath)

	m := makeManifest([]manifest.FileEntry{
		{Path: "data.bin", Size: int64(len("stable")), ContentHash: hash},
	})
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	s.ShouldUpload(absPath, "data.bin") // should skip
	stats := s.Stats()

	assert.Equal(t, int64(1), stats.FilesScanned)
	assert.Equal(t, int64(1), stats.FilesSkipped)
	assert.Equal(t, int64(0), stats.FilesUploaded)
	assert.Equal(t, int64(len("stable")), stats.BytesSaved)
	assert.Equal(t, int64(0), stats.BytesUploaded)
}

func TestStats_FilesUploaded(t *testing.T) {
	dir := t.TempDir()
	absPath := writeFile(t, dir, "new.bin", "new content here")

	m := makeManifest(nil) // no previous files
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	s.ShouldUpload(absPath, "new.bin")
	stats := s.Stats()

	assert.Equal(t, int64(1), stats.FilesScanned)
	assert.Equal(t, int64(0), stats.FilesSkipped)
	assert.Equal(t, int64(1), stats.FilesUploaded)
}

func TestStats_EstimatedTimeSaved(t *testing.T) {
	dir := t.TempDir()
	// Write 200 MB worth of content to trigger a non-zero time estimate.
	// Instead of actually writing 200 MB, we cheat by manually setting BytesSaved.
	// We achieve this by writing a file and lying about its manifest size.
	absPath := writeFile(t, dir, "big.bin", "x")
	hash := md5Of(t, absPath)

	// File is 1 byte on disk; manifest also says 1 byte + matching hash → skip.
	m := makeManifest([]manifest.FileEntry{
		{Path: "big.bin", Size: 1, ContentHash: hash},
	})
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	s.ShouldUpload(absPath, "big.bin")
	stats := s.Stats()

	// 1 byte saved → 0 seconds (integer division).
	assert.Equal(t, time.Duration(0), stats.EstimatedTimeSaved)
	assert.Equal(t, int64(1), stats.BytesSaved)
}

func TestStats_AccumulatesMixed(t *testing.T) {
	dir := t.TempDir()
	stablePath := writeFile(t, dir, "stable.dat", "aaa")
	stableHash := md5Of(t, stablePath)
	writeFile(t, dir, "new.dat", "bbb")

	m := makeManifest([]manifest.FileEntry{
		{Path: "stable.dat", Size: 3, ContentHash: stableHash},
	})
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	_, err = s.FilterFiles(dir)
	require.NoError(t, err)

	stats := s.Stats()
	assert.Equal(t, int64(2), stats.FilesScanned)
	assert.Equal(t, int64(1), stats.FilesSkipped)
	assert.Equal(t, int64(1), stats.FilesUploaded)
}

// --- SaveCache ---

func TestSaveCache_InMemory(t *testing.T) {
	m := makeManifest(nil)
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	// In-memory cache: Save is a no-op and must not error.
	require.NoError(t, s.SaveCache())
}

func TestSaveCache_PersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	absPath := writeFile(t, dir, "x.bin", "hello")

	// Create scanner with a file-backed cache.
	cacheFile := filepath.Join(dir, "cache.json")

	// Put the file in the manifest with matching size so tier 3 runs and
	// GetOrCompute is called, populating the cache.
	hash := md5Of(t, absPath)
	m := makeManifest([]manifest.FileEntry{
		{Path: "x.bin", Size: int64(len("hello")), ContentHash: hash},
	})
	s, err := NewIncrementalScanner(m, cacheFile)
	require.NoError(t, err)

	// Trigger a hash computation via tier 3.
	s.ShouldUpload(absPath, "x.bin")

	require.NoError(t, s.SaveCache())
	assert.FileExists(t, cacheFile, "cache file must be written after Save")
}

// --- Concurrent safety ---

func TestShouldUpload_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	const n = 30
	type fileRecord struct {
		abs  string
		rel  string
		hash string
	}
	records := make([]fileRecord, n)
	entries := make([]manifest.FileEntry, n)

	for i := range records {
		name := filepath.Join("file", string(rune('a'+i%26))+"_"+string(rune('0'+i/26))+".dat")
		abs := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		content := string(rune('A' + i%26))
		require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
		hash, err := manifest.ComputeContentHash(abs)
		require.NoError(t, err)
		records[i] = fileRecord{abs: abs, rel: name, hash: hash}
		entries[i] = manifest.FileEntry{Path: name, Size: 1, ContentHash: hash}
	}

	m := makeManifest(entries)
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	var wg sync.WaitGroup
	results := make([]bool, n)
	for i, rec := range records {
		i, rec := i, rec
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = s.ShouldUpload(rec.abs, rec.rel)
		}()
	}
	wg.Wait()

	for i, shouldUpload := range results {
		assert.False(t, shouldUpload, "file %d (unchanged) must be skipped", i)
	}

	stats := s.Stats()
	assert.Equal(t, int64(n), stats.FilesScanned)
	assert.Equal(t, int64(n), stats.FilesSkipped)
	assert.Equal(t, int64(0), stats.FilesUploaded)
}

// --- Integration: FilterFiles wires into PipelineConfig.IncludeOnlyFiles ---

func TestFilterFiles_OutputMatchesScannerExpectation(t *testing.T) {
	dir := t.TempDir()

	// Create 5 files; 2 are new, 3 are unchanged.
	type fixture struct {
		name    string
		content string
		isNew   bool
	}
	fixtures := []fixture{
		{"alpha.txt", "alpha", false},
		{"beta.txt", "beta", false},
		{"gamma.txt", "gamma", false},
		{"delta.txt", "delta", true},
		{"epsilon.txt", "epsilon", true},
	}

	var entries []manifest.FileEntry
	for _, fx := range fixtures {
		abs := writeFile(t, dir, fx.name, fx.content)
		if !fx.isNew {
			hash := md5Of(t, abs)
			entries = append(entries, manifest.FileEntry{
				Path:        fx.name,
				Size:        int64(len(fx.content)),
				ContentHash: hash,
			})
		}
	}

	m := makeManifest(entries)
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	toUpload, err := s.FilterFiles(dir)
	require.NoError(t, err)

	assert.Len(t, toUpload, 2, "only new files must appear in upload list")

	uploadSet := make(map[string]bool)
	for _, f := range toUpload {
		uploadSet[f] = true
	}
	assert.True(t, uploadSet["delta.txt"])
	assert.True(t, uploadSet["epsilon.txt"])
	assert.False(t, uploadSet["alpha.txt"])
	assert.False(t, uploadSet["beta.txt"])
	assert.False(t, uploadSet["gamma.txt"])

	stats := s.Stats()
	assert.Equal(t, int64(5), stats.FilesScanned)
	assert.Equal(t, int64(3), stats.FilesSkipped)
	assert.Equal(t, int64(2), stats.FilesUploaded)
}

// --- LoadManifestFromFile (manifest package integration) ---

func TestLoadManifestFromFile_JSON(t *testing.T) {
	dir := t.TempDir()
	m := makeManifest([]manifest.FileEntry{
		{Path: "a.txt", Size: 5},
	})
	data, err := m.ToJSON()
	require.NoError(t, err)

	path := filepath.Join(dir, "manifest.json")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	loaded, err := manifest.LoadManifestFromFile(path)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "test-upload-id", loaded.UploadID)
	assert.Len(t, loaded.Files, 1)
}

func TestLoadManifestFromFile_GZ(t *testing.T) {
	dir := t.TempDir()
	m := makeManifest(nil)
	data, err := m.ToJSONCompressed()
	require.NoError(t, err)

	path := filepath.Join(dir, "manifest.json.gz")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	loaded, err := manifest.LoadManifestFromFile(path)
	require.NoError(t, err)
	require.NotNil(t, loaded)
}

func TestLoadManifestFromFile_NotExist(t *testing.T) {
	_, err := manifest.LoadManifestFromFile("/nonexistent/path/manifest.json")
	require.Error(t, err)
}

// Ensure IncrementalStats.EstimatedTimeSaved is non-zero for large BytesSaved.
func TestStats_EstimatedTimeSavedLargeFile(t *testing.T) {
	dir := t.TempDir()
	content := "x"
	absPath := writeFile(t, dir, "fake.bin", content)
	hash := md5Of(t, absPath)

	m := makeManifest([]manifest.FileEntry{
		// Use a large synthetic size to trigger non-zero time estimate.
		// The actual file is 1 byte but we patch the manifest size to match.
		{Path: "fake.bin", Size: 1, ContentHash: hash},
	})
	s, err := NewIncrementalScanner(m, "")
	require.NoError(t, err)

	// Skip the 1-byte file.
	s.ShouldUpload(absPath, "fake.bin")

	// Manually inject a large BytesSaved to test time calculation.
	// (This tests the formula, not production data.)
	s.mu.Lock()
	s.stats.BytesSaved = 300 * 1024 * 1024 // 300 MB
	s.mu.Unlock()

	stats := s.Stats()
	assert.Equal(t, 3*time.Second, stats.EstimatedTimeSaved,
		"300 MB at 100 MB/s = 3 seconds")
}
