// Package manifest provides selective file restoration from CargoShip archives
// using hash-based addressing and LRU chunk caching (Issue #189).
package manifest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"container/list"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klauspost/compress/zstd"
)

// DefaultChunkCacheSize is the default maximum in-memory cache size for
// downloaded S3 chunks (10 GB).
const DefaultChunkCacheSize = int64(10 * 1024 * 1024 * 1024)

// RestoreStats tracks the outcome of a restore operation (Issue #189).
type RestoreStats struct {
	// Restored is the number of files successfully written to disk.
	Restored int64
	// Failed is the number of files that could not be restored.
	Failed int64
	// Bytes is the total number of bytes written to disk.
	Bytes int64
	// ChunksDownloaded is the number of distinct S3 objects fetched.
	ChunksDownloaded int64
}

// chunkCacheEntry is a single node in the ChunkCache doubly-linked list.
type chunkCacheEntry struct {
	key  string
	data []byte
	elem *list.Element // pointer back to the list element for O(1) removal
}

// ChunkCache is a thread-safe LRU cache for downloaded S3 chunk data, bounded
// by total byte size. Storing a chunk that exceeds maxSize is silently skipped.
// (Issue #189)
type ChunkCache struct {
	mu       sync.Mutex
	maxSize  int64
	currSize int64
	items    map[string]*chunkCacheEntry
	order    *list.List // front = most-recently used; back = least-recently used
}

// NewChunkCache creates a ChunkCache bounded by maxSize bytes. A non-positive
// maxSize falls back to DefaultChunkCacheSize.
func NewChunkCache(maxSize int64) *ChunkCache {
	if maxSize <= 0 {
		maxSize = DefaultChunkCacheSize
	}
	return &ChunkCache{
		maxSize: maxSize,
		items:   make(map[string]*chunkCacheEntry),
		order:   list.New(),
	}
}

// get returns the cached data for key and moves the entry to the MRU position.
// Returns nil when key is absent.
func (c *ChunkCache) get(key string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	if !ok {
		return nil
	}
	c.order.MoveToFront(entry.elem)
	return entry.data
}

// put stores data under key. If the entry already exists, it is updated in
// place. Otherwise, LRU entries are evicted until there is room, then the new
// entry is inserted at the MRU position. A chunk larger than maxSize is not
// cached.
func (c *ChunkCache) put(key string, data []byte) {
	size := int64(len(data))
	if size > c.maxSize {
		return // too large to cache
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.items[key]; ok {
		c.currSize -= int64(len(existing.data))
		existing.data = data
		c.currSize += size
		c.order.MoveToFront(existing.elem)
		return
	}
	// Evict LRU entries until there is room.
	for c.currSize+size > c.maxSize {
		back := c.order.Back()
		if back == nil {
			break
		}
		victim := back.Value.(*chunkCacheEntry)
		c.currSize -= int64(len(victim.data))
		c.order.Remove(back)
		delete(c.items, victim.key)
	}
	entry := &chunkCacheEntry{key: key, data: data}
	entry.elem = c.order.PushFront(entry)
	c.items[key] = entry
	c.currSize += size
}

// Len returns the number of entries currently in the cache.
func (c *ChunkCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// Size returns the total byte size of all cached entries.
func (c *ChunkCache) Size() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currSize
}

// SelectiveExtractor provides hash-based and DVC-aware selective file
// restoration from CargoShip archives. It uses an LRU ChunkCache to avoid
// redundant S3 downloads when multiple target files reside in the same chunk.
// (Issue #189)
type SelectiveExtractor struct {
	manifest *Manifest
	query    *ManifestQuery
	s3Client S3Downloader
	cache    *ChunkCache
}

// NewSelectiveExtractor creates a SelectiveExtractor. maxCacheSize sets the
// LRU cache bound in bytes; pass 0 to use DefaultChunkCacheSize (10 GB).
func NewSelectiveExtractor(manifest *Manifest, s3Client S3Downloader, maxCacheSize int64) *SelectiveExtractor {
	return &SelectiveExtractor{
		manifest: manifest,
		query:    NewManifestQuery(manifest),
		s3Client: s3Client,
		cache:    NewChunkCache(maxCacheSize),
	}
}

// ChunkKeysForPaths returns the deduplicated set of S3 chunk keys that contain
// the requested file paths. Unknown paths are silently skipped. Use this to
// obtain the keys for a Glacier pre-flight check before calling BatchRestore.
func (se *SelectiveExtractor) ChunkKeysForPaths(paths []string) []string {
	seen := make(map[string]struct{})
	var keys []string
	for _, p := range paths {
		entry := se.query.FindFile(p)
		if entry == nil {
			continue
		}
		if _, ok := seen[entry.S3Key]; !ok {
			seen[entry.S3Key] = struct{}{}
			keys = append(keys, entry.S3Key)
		}
	}
	return keys
}

// ChunkKeysForDVCStage returns the S3 chunk keys for all files in a DVC stage.
func (se *SelectiveExtractor) ChunkKeysForDVCStage(stage string) []string {
	entries := se.query.FindFilesByDVCStage(stage)
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	return se.ChunkKeysForPaths(paths)
}

// ChunkKeysForCommit returns the S3 chunk keys for all files in a git commit.
func (se *SelectiveExtractor) ChunkKeysForCommit(commit string) []string {
	entries := se.query.FindFilesByCommit(commit)
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	return se.ChunkKeysForPaths(paths)
}

// AllChunkKeys returns the S3 keys for every chunk in the manifest. Use this
// for a full-archive Glacier pre-flight check.
func (se *SelectiveExtractor) AllChunkKeys() []string {
	keys := make([]string, 0, len(se.manifest.Chunks))
	for _, c := range se.manifest.Chunks {
		keys = append(keys, c.S3Key)
	}
	return keys
}

// ExtractFileByHash locates a file by its MD5 ContentHash and extracts it to
// destDir, preserving the original directory structure. It downloads only the
// containing S3 chunk and scans the tar archive for the matching entry.
// (Issue #189)
func (se *SelectiveExtractor) ExtractFileByHash(ctx context.Context, hash, destDir string) (*RestoreStats, error) {
	entry, ok := se.query.FindFileByHash(hash)
	if !ok {
		return nil, fmt.Errorf("no file with content hash %q in manifest", hash)
	}
	return se.BatchRestore(ctx, []string{entry.Path}, destDir)
}

// BatchRestore restores the files identified by targets (file paths as recorded
// in the manifest) to destDir. Files are grouped by their S3 chunk key so that
// each distinct chunk is downloaded at most once. The LRU cache further reduces
// downloads across multiple BatchRestore calls on the same SelectiveExtractor.
// (Issue #189)
func (se *SelectiveExtractor) BatchRestore(ctx context.Context, targets []string, destDir string) (*RestoreStats, error) {
	stats := &RestoreStats{}
	if len(targets) == 0 {
		return stats, nil
	}

	// Group FileEntry pointers by S3 key (one S3 key == one chunk download).
	type chunkGroup struct {
		s3Key string
		files []*FileEntry
	}
	chunkMap := make(map[string]*chunkGroup)

	for _, target := range targets {
		entry := se.resolveEntry(target)
		if entry == nil {
			stats.Failed++
			continue
		}
		key := entry.S3Key
		if _, ok := chunkMap[key]; !ok {
			chunkMap[key] = &chunkGroup{s3Key: key}
		}
		chunkMap[key].files = append(chunkMap[key].files, entry)
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("create destination directory: %w", err)
	}

	// Direct-upload manifests have no chunks: each file was stored as its own S3
	// object (the object at FileEntry.S3Key IS the raw file, not a tar.zst chunk).
	// Restore those by writing the downloaded bytes directly. (Issue #228)
	directMode := len(se.manifest.Chunks) == 0

	for _, grp := range chunkMap {
		data := se.cache.get(grp.s3Key)
		if data == nil {
			var dlErr error
			data, dlErr = se.downloadChunk(ctx, grp.s3Key)
			if dlErr != nil {
				stats.Failed += int64(len(grp.files))
				continue
			}
			se.cache.put(grp.s3Key, data)
			stats.ChunksDownloaded++
		}

		if directMode {
			// One S3 object == one file; write the raw bytes.
			restored, written, err := se.writeDirectFiles(data, grp.files, destDir)
			stats.Restored += int64(restored)
			stats.Bytes += written
			if err != nil {
				stats.Failed += int64(len(grp.files)) - int64(restored)
			}
			continue
		}

		restored, written, err := se.extractFromChunkData(data, grp.files, destDir)
		stats.Restored += int64(restored)
		stats.Bytes += written
		if err != nil {
			stats.Failed += int64(len(grp.files)) - int64(restored)
		}
	}

	return stats, nil
}

// resolveEntry finds the manifest entry for a restore target. It tries an exact
// path match first (via the O(1) index), then falls back to matching by relative
// suffix / basename — so `--file greeting.txt` resolves even though manifests
// currently store the file's absolute source path. (Issue #228)
func (se *SelectiveExtractor) resolveEntry(target string) *FileEntry {
	if entry := se.query.FindFile(target); entry != nil {
		return entry
	}
	clean := filepath.Clean(target)
	base := filepath.Base(clean)
	var suffixMatch *FileEntry
	for i := range se.manifest.Files {
		p := se.manifest.Files[i].Path
		// e.g. target "sub/greeting.txt" matching stored "/abs/root/sub/greeting.txt"
		if p == clean || strings.HasSuffix(p, string(filepath.Separator)+clean) {
			return &se.manifest.Files[i]
		}
		if filepath.Base(p) == base {
			if suffixMatch != nil {
				// Ambiguous basename; require a more specific target.
				return nil
			}
			suffixMatch = &se.manifest.Files[i]
		}
	}
	return suffixMatch
}

// writeDirectFiles writes direct-upload objects (raw file bytes, one object per
// file) to destDir. The output path is the file's basename (manifests may store
// an absolute source path; we never write outside destDir). (Issue #228)
func (se *SelectiveExtractor) writeDirectFiles(data []byte, files []*FileEntry, destDir string) (int, int64, error) {
	var restored int
	var totalBytes int64
	for _, entry := range files {
		outPath := filepath.Join(destDir, filepath.Base(entry.Path))
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return restored, totalBytes, fmt.Errorf("mkdir for %s: %w", outPath, err)
		}
		if err := os.WriteFile(outPath, data, 0644); err != nil {
			return restored, totalBytes, fmt.Errorf("write %s: %w", outPath, err)
		}
		restored++
		totalBytes += int64(len(data))
	}
	return restored, totalBytes, nil
}

// BatchRestoreByDVCStage restores all files tagged with the given DVC pipeline
// stage name. Returns an error when the stage is not present in the manifest.
func (se *SelectiveExtractor) BatchRestoreByDVCStage(ctx context.Context, stage, destDir string) (*RestoreStats, error) {
	entries := se.query.FindFilesByDVCStage(stage)
	if len(entries) == 0 {
		return &RestoreStats{}, fmt.Errorf("no files found for DVC stage %q", stage)
	}
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	return se.BatchRestore(ctx, paths, destDir)
}

// BatchRestoreByCommit restores all files associated with a git commit SHA.
// Returns an error when the commit is not recorded in the manifest.
func (se *SelectiveExtractor) BatchRestoreByCommit(ctx context.Context, commit, destDir string) (*RestoreStats, error) {
	entries := se.query.FindFilesByCommit(commit)
	if len(entries) == 0 {
		return &RestoreStats{}, fmt.Errorf("no files found for git commit %q", commit)
	}
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	return se.BatchRestore(ctx, paths, destDir)
}

// downloadChunk fetches the S3 object at s3Key from the manifest's bucket and
// returns its raw bytes.
func (se *SelectiveExtractor) downloadChunk(ctx context.Context, s3Key string) ([]byte, error) {
	out, err := se.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(se.manifest.Bucket),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		return nil, fmt.Errorf("S3 GetObject %q: %w", s3Key, err)
	}
	defer func() { _ = out.Body.Close() }()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("read S3 body %q: %w", s3Key, err)
	}
	return data, nil
}

// extractFromChunkData decompresses data (a compressed tar archive) and writes
// the requested files to destDir. Returns the count of files written and total
// bytes written.
func (se *SelectiveExtractor) extractFromChunkData(data []byte, files []*FileEntry, destDir string) (int, int64, error) {
	want := make(map[string]*FileEntry, len(files))
	for _, f := range files {
		want[f.Path] = f
	}

	r := bytes.NewReader(data)
	var tarReader *tar.Reader

	switch se.manifest.CompressionType {
	case "zstd":
		dec, err := zstd.NewReader(r)
		if err != nil {
			return 0, 0, fmt.Errorf("zstd decoder: %w", err)
		}
		defer dec.Close()
		tarReader = tar.NewReader(dec)
	case "gzip", "gz":
		gz, err := gzip.NewReader(r)
		if err != nil {
			return 0, 0, fmt.Errorf("gzip reader: %w", err)
		}
		defer func() { _ = gz.Close() }()
		tarReader = tar.NewReader(gz)
	case "none", "":
		tarReader = tar.NewReader(r)
	default:
		return 0, 0, fmt.Errorf("unsupported compression type %q", se.manifest.CompressionType)
	}

	var restored int
	var totalBytes int64

	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return restored, totalBytes, fmt.Errorf("tar read: %w", err)
		}
		entry, ok := want[hdr.Name]
		if !ok {
			continue
		}
		outPath := filepath.Join(destDir, entry.Path)
		if mkErr := os.MkdirAll(filepath.Dir(outPath), 0755); mkErr != nil {
			return restored, totalBytes, fmt.Errorf("mkdir for %s: %w", outPath, mkErr)
		}
		f, err := os.Create(outPath)
		if err != nil {
			return restored, totalBytes, fmt.Errorf("create %s: %w", outPath, err)
		}
		written, err := io.Copy(f, tarReader)
		_ = f.Close()
		if err != nil {
			return restored, totalBytes, fmt.Errorf("write %s: %w", outPath, err)
		}
		restored++
		totalBytes += written
	}

	return restored, totalBytes, nil
}
