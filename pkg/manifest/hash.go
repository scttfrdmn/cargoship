package manifest

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// ComputeContentHash returns the MD5 hex digest of the file at filePath.
//
// MD5 is used rather than SHA-256 for DVC compatibility: DVC records md5 sums
// in its .dvc files and CargoShip must produce matching values (Issue #172).
func ComputeContentHash(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", filePath, err)
	}
	defer func() { _ = f.Close() }()

	h := md5.New() //nolint:gosec // MD5 required for DVC protocol compatibility, not used for security
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", filePath, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// computeContentHashWithStat hashes filePath and returns the hash together with
// the file's size and modification time in a single open+stat+read pass.
func computeContentHashWithStat(filePath string) (hash string, size int64, modTime time.Time, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", 0, time.Time{}, fmt.Errorf("open %s: %w", filePath, err)
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		return "", 0, time.Time{}, fmt.Errorf("stat %s: %w", filePath, err)
	}

	h := md5.New() //nolint:gosec // MD5 required for DVC protocol compatibility, not used for security
	if _, err := io.Copy(h, f); err != nil {
		return "", 0, time.Time{}, fmt.Errorf("hash %s: %w", filePath, err)
	}
	return hex.EncodeToString(h.Sum(nil)), fi.Size(), fi.ModTime(), nil
}

// hashCacheEntry is one record persisted in the JSON cache file.
// ModTimeNs stores the file's modification time as UnixNano to avoid
// precision loss when round-tripping through JSON time.Time serialization.
type hashCacheEntry struct {
	Hash      string `json:"hash"`
	Size      int64  `json:"size"`
	ModTimeNs int64  `json:"mod_time_ns"` // time.Time.UnixNano()
}

// HashCache is a persistent, invalidation-aware cache of MD5 content hashes.
//
// An entry is considered valid as long as the file's size and modification
// time match the values recorded at the time the hash was computed.  Any
// change to either field causes the entry to be treated as a cache miss so
// that the hash is recomputed on the next call to Get or GetOrCompute.
//
// The cache is stored as a JSON file on disk so that hashes computed in one
// process run are available to the next.  Writes to the file are atomic
// (write-to-temp then rename).  All exported methods are safe for concurrent
// use by multiple goroutines.
type HashCache struct {
	mu      sync.RWMutex
	entries map[string]hashCacheEntry // key: file path
	path    string                    // backing JSON file; empty = in-memory only
	dirty   bool                      // true when entries differ from on-disk state
}

// NewHashCache creates a HashCache backed by the JSON file at path.
// If the file already exists its contents are loaded into memory.
// If the file does not exist the cache starts empty and will be created on the
// first call to Save.
// Pass an empty path for an in-memory-only cache (Save becomes a no-op).
func NewHashCache(path string) (*HashCache, error) {
	c := &HashCache{
		entries: make(map[string]hashCacheEntry),
		path:    path,
	}
	if path == "" {
		return c, nil
	}
	if err := c.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load hash cache %s: %w", path, err)
	}
	return c, nil
}

// load reads and parses the JSON cache file.
func (c *HashCache) load() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}
	var entries map[string]hashCacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	c.entries = entries
	return nil
}

// Save writes the in-memory cache to the JSON backing file atomically.
// If the cache has no backing file path, Save is a no-op.
// Save is a no-op when no entries have changed since the last successful save.
func (c *HashCache) Save() error {
	if c.path == "" {
		return nil
	}

	// Snapshot entries and clear dirty flag under read lock so writers are not
	// blocked for the duration of the file write.
	c.mu.RLock()
	if !c.dirty {
		c.mu.RUnlock()
		return nil
	}
	data, err := json.MarshalIndent(c.entries, "", "  ")
	c.mu.RUnlock()

	if err != nil {
		return fmt.Errorf("marshal hash cache: %w", err)
	}

	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write hash cache: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename hash cache: %w", err)
	}

	c.mu.Lock()
	c.dirty = false
	c.mu.Unlock()
	return nil
}

// Get returns the cached MD5 hash for filePath if the entry is still valid.
//
// Validity is checked by stat-ing the file and comparing its current size and
// modification time against the recorded values.  Returns ("", false) on a
// cache miss, an invalid (stale) entry, or a stat error.
func (c *HashCache) Get(filePath string) (hash string, ok bool) {
	c.mu.RLock()
	entry, found := c.entries[filePath]
	c.mu.RUnlock()

	if !found {
		return "", false
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		return "", false
	}
	if fi.Size() != entry.Size || fi.ModTime().UnixNano() != entry.ModTimeNs {
		return "", false
	}
	return entry.Hash, true
}

// Set records (or updates) the cached hash for filePath stamped with size and
// modTime.  The dirty flag is set so that the next Save will persist the change.
func (c *HashCache) Set(filePath, hash string, size int64, modTime time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[filePath] = hashCacheEntry{
		Hash:      hash,
		Size:      size,
		ModTimeNs: modTime.UnixNano(),
	}
	c.dirty = true
}

// GetOrCompute returns the cached hash for filePath, computing and caching it
// if the entry is absent or stale.  The cache is updated in memory; call Save
// to persist to disk.
func (c *HashCache) GetOrCompute(filePath string) (string, error) {
	if hash, ok := c.Get(filePath); ok {
		return hash, nil
	}
	hash, size, modTime, err := computeContentHashWithStat(filePath)
	if err != nil {
		return "", err
	}
	c.Set(filePath, hash, size, modTime)
	return hash, nil
}

// Len returns the number of entries currently held in memory.
func (c *HashCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// ComputeHashesConcurrent computes MD5 hashes for all files in files using a
// bounded worker pool of size workers (clamped to 1 if ≤ 0).
//
// Returns a map[filePath]md5hex for every file that was hashed successfully,
// together with the first error encountered (files that failed are omitted from
// the map).  All workers are always drained before the function returns.
func ComputeHashesConcurrent(files []string, workers int) (map[string]string, error) {
	if workers <= 0 {
		workers = 1
	}
	if len(files) == 0 {
		return make(map[string]string), nil
	}

	type result struct {
		path string
		hash string
		err  error
	}

	jobs := make(chan string, len(files))
	results := make(chan result, len(files))

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				h, err := ComputeContentHash(path)
				results <- result{path: path, hash: h, err: err}
			}
		}()
	}

	for _, f := range files {
		jobs <- f
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	out := make(map[string]string, len(files))
	var firstErr error
	for r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		out[r.path] = r.hash
	}
	return out, firstErr
}
