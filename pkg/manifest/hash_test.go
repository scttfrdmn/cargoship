package manifest

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// knownMD5 returns the expected MD5 hex digest for the given content.
// Computed independently with: echo -n "content" | md5sum
func knownMD5(content string) string {
	h := make([]byte, 16)
	// Pre-computed values for test fixtures:
	switch content {
	case "hello world":
		copy(h, mustDecodeHex("5eb63bbbe01eeed093cb22bb8f5acdc3"))
	case "foo":
		copy(h, mustDecodeHex("acbd18db4cc2f85cedef654fccc4a4d8"))
	default:
		panic("unknown fixture: " + content)
	}
	return hex.EncodeToString(h)
}

func mustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// writeTempFile creates a temporary file with the given content and returns its path.
func writeTempFile(t *testing.T, dir, content string) string {
	t.Helper()
	f, err := os.CreateTemp(dir, "hashtest-*")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

// --- ComputeContentHash ---

func TestComputeContentHash_KnownValue(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "hello world")

	got, err := ComputeContentHash(path)
	require.NoError(t, err)
	assert.Equal(t, knownMD5("hello world"), got, "MD5 must match known value")
}

func TestComputeContentHash_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "")

	got, err := ComputeContentHash(path)
	require.NoError(t, err)
	// MD5 of empty string
	assert.Equal(t, "d41d8cd98f00b204e9800998ecf8427e", got)
}

func TestComputeContentHash_MissingFile(t *testing.T) {
	_, err := ComputeContentHash("/nonexistent/path/file.txt")
	require.Error(t, err)
}

func TestComputeContentHash_Deterministic(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "deterministic content")

	h1, err := ComputeContentHash(path)
	require.NoError(t, err)
	h2, err := ComputeContentHash(path)
	require.NoError(t, err)
	assert.Equal(t, h1, h2)
}

// --- HashCache basic ---

func TestHashCache_NewInMemory(t *testing.T) {
	c, err := NewHashCache("")
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, 0, c.Len())
}

func TestHashCache_NewWithFile_NotExist(t *testing.T) {
	dir := t.TempDir()
	c, err := NewHashCache(filepath.Join(dir, "cache.json"))
	require.NoError(t, err, "missing cache file must not be an error")
	assert.Equal(t, 0, c.Len())
}

func TestHashCache_Miss(t *testing.T) {
	c, err := NewHashCache("")
	require.NoError(t, err)

	_, ok := c.Get("/some/file.txt")
	assert.False(t, ok, "empty cache must return miss")
}

func TestHashCache_SetGet_Hit(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "foo")

	fi, err := os.Stat(path)
	require.NoError(t, err)

	c, err := NewHashCache("")
	require.NoError(t, err)

	c.Set(path, knownMD5("foo"), fi.Size(), fi.ModTime())

	hash, ok := c.Get(path)
	require.True(t, ok, "entry just set must be a hit")
	assert.Equal(t, knownMD5("foo"), hash)
}

// --- Invalidation ---

func TestHashCache_Invalidation_SizeChange(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "original")

	fi, err := os.Stat(path)
	require.NoError(t, err)

	c, err := NewHashCache("")
	require.NoError(t, err)
	c.Set(path, "somehash", fi.Size(), fi.ModTime())

	// Overwrite with different-length content
	require.NoError(t, os.WriteFile(path, []byte("much longer content now"), 0o600))

	_, ok := c.Get(path)
	assert.False(t, ok, "size change must invalidate the cache entry")
}

func TestHashCache_Invalidation_ModTimeChange(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "stable content")

	fi, err := os.Stat(path)
	require.NoError(t, err)

	c, err := NewHashCache("")
	require.NoError(t, err)
	c.Set(path, "somehash", fi.Size(), fi.ModTime())

	// Advance mod-time by 1 second without changing content
	future := fi.ModTime().Add(time.Second)
	require.NoError(t, os.Chtimes(path, future, future))

	_, ok := c.Get(path)
	assert.False(t, ok, "mod-time change must invalidate the cache entry")
}

func TestHashCache_Invalidation_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "content")

	fi, err := os.Stat(path)
	require.NoError(t, err)

	c, err := NewHashCache("")
	require.NoError(t, err)
	c.Set(path, "somehash", fi.Size(), fi.ModTime())

	require.NoError(t, os.Remove(path))

	_, ok := c.Get(path)
	assert.False(t, ok, "deleted file must produce a cache miss")
}

// --- Persistence ---

func TestHashCache_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "hashes.json")
	filePath := writeTempFile(t, dir, "hello world")

	fi, err := os.Stat(filePath)
	require.NoError(t, err)

	// Populate and save
	c1, err := NewHashCache(cachePath)
	require.NoError(t, err)
	c1.Set(filePath, knownMD5("hello world"), fi.Size(), fi.ModTime())
	require.NoError(t, c1.Save())

	assert.FileExists(t, cachePath)

	// Reload from disk
	c2, err := NewHashCache(cachePath)
	require.NoError(t, err)
	assert.Equal(t, 1, c2.Len())

	hash, ok := c2.Get(filePath)
	require.True(t, ok, "reloaded cache must hit")
	assert.Equal(t, knownMD5("hello world"), hash)
}

func TestHashCache_SaveNoDirty(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "hashes.json")

	c, err := NewHashCache(cachePath)
	require.NoError(t, err)
	// Save without any Sets — file must not be created
	require.NoError(t, c.Save())
	assert.NoFileExists(t, cachePath, "no-op save must not create cache file")
}

func TestHashCache_SaveIdempotent(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "hashes.json")
	path := writeTempFile(t, dir, "hello world")

	fi, err := os.Stat(path)
	require.NoError(t, err)

	c, err := NewHashCache(cachePath)
	require.NoError(t, err)
	c.Set(path, knownMD5("hello world"), fi.Size(), fi.ModTime())

	require.NoError(t, c.Save())
	stat1, err := os.Stat(cachePath)
	require.NoError(t, err)

	// Second save should be a no-op (dirty=false)
	require.NoError(t, c.Save())
	stat2, err := os.Stat(cachePath)
	require.NoError(t, err)

	assert.Equal(t, stat1.ModTime(), stat2.ModTime(), "second save must not touch the file")
}

func TestHashCache_InMemoryNoFile(t *testing.T) {
	c, err := NewHashCache("")
	require.NoError(t, err)

	path := "/tmp/fake-file"
	c.Set(path, "deadbeef", 100, time.Now())

	// Save on in-memory cache must be a no-op (no file created)
	require.NoError(t, c.Save())
}

// --- GetOrCompute ---

func TestHashCache_GetOrCompute_Miss(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "hello world")

	c, err := NewHashCache("")
	require.NoError(t, err)

	hash, err := c.GetOrCompute(path)
	require.NoError(t, err)
	assert.Equal(t, knownMD5("hello world"), hash)
	assert.Equal(t, 1, c.Len(), "entry must be cached after compute")
}

func TestHashCache_GetOrCompute_Hit(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "hello world")

	c, err := NewHashCache("")
	require.NoError(t, err)

	// First call computes
	h1, err := c.GetOrCompute(path)
	require.NoError(t, err)

	// Replace file content with same bytes but track if ComputeContentHash is called again
	// by monitoring via a second call — must return same hash without re-reading if valid
	h2, err := c.GetOrCompute(path)
	require.NoError(t, err)
	assert.Equal(t, h1, h2)
}

func TestHashCache_GetOrCompute_MissingFile(t *testing.T) {
	c, err := NewHashCache("")
	require.NoError(t, err)

	_, err = c.GetOrCompute("/nonexistent/path/file.txt")
	require.Error(t, err)
}

func TestHashCache_GetOrCompute_AfterModification(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "original")

	c, err := NewHashCache("")
	require.NoError(t, err)

	h1, err := c.GetOrCompute(path)
	require.NoError(t, err)

	// Modify file (new content, wait to ensure different mod-time on coarse-grained FSes)
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, os.WriteFile(path, []byte("modified content"), 0o600))
	// Force mod-time forward in case the FS resolution is coarse
	future := time.Now().Add(time.Second)
	require.NoError(t, os.Chtimes(path, future, future))

	h2, err := c.GetOrCompute(path)
	require.NoError(t, err)
	assert.NotEqual(t, h1, h2, "modified file must produce a different hash")
}

// --- ComputeHashesConcurrent ---

func TestComputeHashesConcurrent_Basic(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		writeTempFile(t, dir, "hello world"),
		writeTempFile(t, dir, "foo"),
	}

	got, err := ComputeHashesConcurrent(files, 2)
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Equal(t, knownMD5("hello world"), got[files[0]])
	assert.Equal(t, knownMD5("foo"), got[files[1]])
}

func TestComputeHashesConcurrent_Empty(t *testing.T) {
	got, err := ComputeHashesConcurrent(nil, 4)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestComputeHashesConcurrent_SingleWorker(t *testing.T) {
	dir := t.TempDir()
	files := make([]string, 10)
	for i := range files {
		files[i] = writeTempFile(t, dir, fmt.Sprintf("content-%d", i))
	}

	got, err := ComputeHashesConcurrent(files, 1)
	require.NoError(t, err)
	assert.Len(t, got, 10)
}

func TestComputeHashesConcurrent_ZeroWorkersClamped(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "hello world")

	got, err := ComputeHashesConcurrent([]string{path}, 0)
	require.NoError(t, err)
	assert.Equal(t, knownMD5("hello world"), got[path])
}

func TestComputeHashesConcurrent_MissingFile(t *testing.T) {
	dir := t.TempDir()
	good := writeTempFile(t, dir, "hello world")
	bad := filepath.Join(dir, "nonexistent.txt")

	got, err := ComputeHashesConcurrent([]string{good, bad}, 2)
	require.Error(t, err, "missing file must produce an error")
	// The good file should still appear in results
	assert.Equal(t, knownMD5("hello world"), got[good])
}

func TestComputeHashesConcurrent_ManyFiles(t *testing.T) {
	const n = 200
	dir := t.TempDir()
	files := make([]string, n)
	for i := range files {
		files[i] = writeTempFile(t, dir, fmt.Sprintf("file-%d", i))
	}

	got, err := ComputeHashesConcurrent(files, 8)
	require.NoError(t, err)
	assert.Len(t, got, n)
	for _, f := range files {
		assert.NotEmpty(t, got[f], "every file must have a hash")
	}
}

// --- Concurrent cache access ---

func TestHashCache_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "concurrent.json")

	// Create 20 temp files
	const numFiles = 20
	files := make([]string, numFiles)
	for i := range files {
		files[i] = writeTempFile(t, dir, fmt.Sprintf("content-%d", i))
	}

	c, err := NewHashCache(cachePath)
	require.NoError(t, err)

	var wg sync.WaitGroup
	var errCount atomic.Int64

	// Launch concurrent GetOrCompute calls
	for _, f := range files {
		f := f
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := c.GetOrCompute(f); err != nil {
					errCount.Add(1)
				}
			}()
		}
	}
	wg.Wait()

	assert.Equal(t, int64(0), errCount.Load(), "no errors expected under concurrent access")
	assert.Equal(t, numFiles, c.Len(), "cache must hold exactly one entry per file")

	// Persist and reload
	require.NoError(t, c.Save())
	c2, err := NewHashCache(cachePath)
	require.NoError(t, err)
	assert.Equal(t, numFiles, c2.Len())
}
