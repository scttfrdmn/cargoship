// Package manifest provides selective file restoration from CargoShip archives
// using hash-based addressing and LRU chunk caching (Issue #189).
package manifest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

	// verify enables restore-time integrity checking: each restored file's
	// content is hashed and compared to the manifest's recorded checksum, and a
	// mismatch fails that file rather than writing corrupt bytes (#270). On by
	// default; disable with SetVerify(false). Files with no recorded checksum
	// (or a non-sha256 algorithm) are restored without a check.
	verify bool

	// flatten writes each restored file at destDir/<basename> instead of the
	// default dataset-relative layout (#287). Useful for targeted single-file
	// restores. Set with SetFlatten(true).
	flatten bool

	// bucket overrides the bucket objects are fetched from. Empty means use
	// manifest.Bucket, which is the historical behavior.
	//
	// The manifest records the bucket it was written to, so an archive copied or
	// replicated elsewhere carried a stale name and every fetch went to the
	// ORIGINAL bucket — silently, since the caller had already been asked for a
	// bucket in the S3 URL and had no reason to think it was ignored (#335). Set
	// with SetBucket to the bucket the manifest was actually read from.
	bucket string
}

// NewSelectiveExtractor creates a SelectiveExtractor. maxCacheSize sets the
// LRU cache bound in bytes; pass 0 to use DefaultChunkCacheSize (10 GB).
// Restore-time checksum verification is enabled by default.
func NewSelectiveExtractor(manifest *Manifest, s3Client S3Downloader, maxCacheSize int64) *SelectiveExtractor {
	return &SelectiveExtractor{
		manifest: manifest,
		query:    NewManifestQuery(manifest),
		s3Client: s3Client,
		cache:    NewChunkCache(maxCacheSize),
		verify:   true,
	}
}

// SetVerify toggles restore-time checksum verification. It returns the receiver
// for chaining. Disabling it restores the pre-#270 behavior (write whatever S3
// returns); use only when throughput matters more than catching corruption.
func (se *SelectiveExtractor) SetVerify(v bool) *SelectiveExtractor {
	se.verify = v
	return se
}

// SetBucket overrides the bucket objects are fetched from, for callers that know
// where the archive actually lives — normally the bucket the manifest was just
// read from. Passing "" keeps the manifest's own recorded bucket.
//
// Without this, a copied, replicated, or renamed archive fetches from the bucket
// baked in at upload time: the restore either 404s or, worse, succeeds against a
// stale original that the user believed they had moved away from (#335).
// Returns the receiver for chaining.
func (se *SelectiveExtractor) SetBucket(bucket string) *SelectiveExtractor {
	se.bucket = bucket
	return se
}

// objectBucket returns the bucket to fetch from: the explicit override when set,
// otherwise the manifest's recorded bucket.
func (se *SelectiveExtractor) objectBucket() string {
	if se.bucket != "" {
		return se.bucket
	}
	return se.manifest.Bucket
}

// SetFlatten toggles flat restore layout (destDir/<basename> per file) instead
// of the default dataset-relative layout. Returns the receiver for chaining.
// Handy for targeted restores where the caller just wants the file(s) in one
// directory; not recommended for full restores that contain same-named files in
// different directories (later wins).
func (se *SelectiveExtractor) SetFlatten(f bool) *SelectiveExtractor {
	se.flatten = f
	return se
}

// checksumMismatch reports whether restore-time verification is active for this
// entry and the given content fails it. It returns false (no mismatch) when
// verification is off, the entry has no recorded checksum, or the manifest's
// algorithm isn't the sha256 we can recompute.
func (se *SelectiveExtractor) checksumMismatch(entry *FileEntry, content []byte) bool {
	if !se.verify || entry.Checksum == "" {
		return false
	}
	if se.manifest.ChecksumAlgorithm != "" && se.manifest.ChecksumAlgorithm != ChecksumAlgorithmSHA256 {
		return false // unknown algorithm; can't recompute, don't false-fail
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]) != entry.Checksum
}

// restorePath maps a manifest FileEntry.Path to a destination path under
// destDir. By default it lays the file out relative to the manifest's
// SourcePath (the upload root), so `/home/u/project/data/a.txt` uploaded from
// root `/home/u/project` restores to `destDir/data/a.txt` — the intuitive,
// dataset-relative layout for full and targeted restores alike (#287). When
// se.flatten is set, the file lands at destDir/<basename>. In all cases the
// result is guaranteed to stay inside destDir (#282): manifests store absolute
// source paths and a crafted manifest could contain `..`, so leading
// slashes/volume and `.`/`..` segments are stripped and containment verified.
func (se *SelectiveExtractor) restorePath(destDir, entryPath string) (string, error) {
	rel := se.relativeEntryPath(entryPath)

	if se.flatten {
		rel = filepath.Base(filepath.FromSlash(rel))
	}

	// Sanitize: slash-normalize, drop volume/leading slash and `.`/`..` segs.
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, filepath.VolumeName(entryPath))
	rel = strings.TrimLeft(rel, "/")
	var clean []string
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		clean = append(clean, seg)
	}
	if len(clean) == 0 {
		return "", fmt.Errorf("refusing to restore %q: no safe path components", entryPath)
	}

	out := filepath.Join(append([]string{destDir}, clean...)...)

	// Belt-and-suspenders: confirm the result is within destDir.
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return "", err
	}
	absOut, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	if absOut != absDest && !strings.HasPrefix(absOut, absDest+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing to restore %q: path escapes destination", entryPath)
	}
	return out, nil
}

// restoreModTime stamps a restored file with the modification time the manifest
// recorded at upload. Archival restores should reproduce the source tree, not
// the time of the restore. A failure here is deliberately non-fatal: the file
// content is already correct on disk, and refusing the restore over a timestamp
// would be worse than an imprecise timestamp. (#311)
func restoreModTime(root *os.Root, relPath string, modTime time.Time) {
	if modTime.IsZero() {
		return
	}
	_ = root.Chtimes(relPath, modTime, modTime)
}

// destRoot opens destDir as an os.Root, the containment boundary every restore
// write goes through (#341).
//
// restorePath's own check is LEXICAL — it strips volume names, leading slashes
// and `.`/`..` segments and confirms the joined result is textually under
// destDir. That closes the crafted-manifest traversal of #282, but neither
// filepath.Abs nor filepath.Clean resolves symlinks, so a lexically-contained
// path can still escape the *filesystem* when a component inside destDir is a
// pre-existing symlink pointing elsewhere (CWE-59). With
// `destDir/cache -> /etc`, an ordinary entry `cache/passwd` passes containment
// and the OS follows the link. The hostile input is the destination's shape, not
// the manifest — a world-writable staging dir, a shared scratch mount, or an
// unpacked tarball that shipped its own symlinks is enough.
//
// os.Root resolves every component with openat(2) relative to a held directory
// descriptor and refuses any component that leaves the root. That also closes
// the TOCTOU window a check-then-write approach leaves open: a `Lstat` walk can
// verify a parent is a real directory and have it replaced with a symlink before
// the write lands, whereas os.Root re-validates at each open. It is the standard
// library's answer to exactly this class, and it behaves consistently across all
// the platforms goreleaser builds (linux, darwin, windows).
func destRoot(destDir string) (*os.Root, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("create destination directory: %w", err)
	}
	root, err := os.OpenRoot(destDir)
	if err != nil {
		return nil, fmt.Errorf("open destination directory: %w", err)
	}
	return root, nil
}

// destRelPath converts the absolute output path restorePath produced back into a
// slash-separated path relative to destDir, for use with os.Root's methods
// (which take root-relative names). restorePath has already guaranteed lexical
// containment, so a failure here means a caller passed mismatched paths.
func destRelPath(destDir, outPath string) (string, error) {
	rel, err := filepath.Rel(destDir, outPath)
	if err != nil {
		return "", fmt.Errorf("relativize %q against %q: %w", outPath, destDir, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing to restore %q: path escapes destination", outPath)
	}
	return filepath.ToSlash(rel), nil
}

// prepareParents creates the parent directories of relPath inside root, walking
// one component at a time and refusing any that already exists as a symlink. An
// ordinary pre-existing directory is reused as usual.
//
// root.MkdirAll would not be enough. os.Root refuses a symlink that leaves the
// root — the escape this issue is about — but a symlink pointing to another path
// *inside* destDir is still followed, so `dest/cache -> real` would silently
// divert `cache/config.txt` into `real/`. That is a correctness failure rather
// than an escape: a restore must reproduce the paths the manifest recorded.
func prepareParents(root *os.Root, relPath string) error {
	dir := path.Dir(relPath)
	if dir == "." || dir == "/" || dir == "" {
		return nil
	}
	var cur string
	for _, comp := range strings.Split(dir, "/") {
		if comp == "" || comp == "." {
			continue
		}
		if cur == "" {
			cur = comp
		} else {
			cur += "/" + comp
		}

		fi, err := root.Lstat(cur)
		if err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing to restore under %s: path component is a symlink", cur)
			}
			if !fi.IsDir() {
				return fmt.Errorf("refusing to restore under %s: path component is not a directory", cur)
			}
			continue
		}
		if err := root.Mkdir(cur, 0755); err != nil && !os.IsExist(err) {
			return fmt.Errorf("mkdir %s: %w", cur, err)
		}
	}
	return nil
}

// createContained opens relPath for writing inside root, refusing to write
// through a symlink at the leaf.
//
// os.Root alone already refuses a leaf symlink that points outside the root —
// that is the escape this issue is about. The extra Lstat rejects a leaf symlink
// pointing *inside* the root too, because a restore should write the file it was
// asked to write rather than through whatever link happens to sit at that path.
// That second case is a correctness guard, not a containment one, and it carries
// an unavoidable TOCTOU window (the link could be introduced between the Lstat
// and the open). The containment guarantee does not depend on it: os.Root
// re-validates every component at open time, so even if the race is won the
// write still cannot leave destDir. O_NOFOLLOW would close the window but is not
// portable — it is absent on Windows, which goreleaser builds.
//
// O_TRUNC preserves the existing overwrite behavior for real files.
func createContained(root *os.Root, relPath string) (*os.File, error) {
	if fi, err := root.Lstat(relPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to restore %s: destination path is a symlink", relPath)
	}
	f, err := root.OpenFile(relPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", relPath, err)
	}
	return f, nil
}

// writeContained writes data to relPath inside root with the same symlink
// refusal as createContained.
func writeContained(root *os.Root, relPath string, data []byte) error {
	f, err := createContained(root, relPath)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", relPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write %s: %w", relPath, err)
	}
	return nil
}

// relativeEntryPath returns entryPath relative to the manifest's SourcePath (the
// upload root) when it sits under it; otherwise it returns entryPath unchanged
// (the sanitizer in restorePath still makes it destDir-safe). This is what makes
// the default layout dataset-relative rather than rooted at "/".
func (se *SelectiveExtractor) relativeEntryPath(entryPath string) string {
	root := se.manifest.SourcePath
	if root == "" {
		return entryPath
	}
	// Compare in slash form; require a path-segment boundary so "/a/bc" isn't
	// treated as under root "/a/b".
	e := filepath.ToSlash(entryPath)
	r := strings.TrimRight(filepath.ToSlash(root), "/")
	if e == r {
		return filepath.Base(e)
	}
	if strings.HasPrefix(e, r+"/") {
		return strings.TrimPrefix(e, r+"/")
	}
	return entryPath
}

// ChunkKeysForPaths returns the deduplicated set of S3 chunk keys that contain
// the requested file paths. Unknown paths are silently skipped. Use this to
// obtain the keys for a Glacier pre-flight check before calling BatchRestore.
//
// Keys are returned in RESOLVED form — the real object key within the bucket,
// as downloadChunk would fetch it. Manifests record S3Key relative to the
// manifest Prefix, so the raw value addresses no object in a prefixed archive
// and a pre-flight HeadObject on it 404s (#334).
//
// Targets are resolved with the same matching BatchRestore uses, so the keys
// returned describe exactly what a subsequent restore will download. Resolving
// them differently is how the pre-flight check came to silently pass on
// basename targets while the restore itself succeeded on a different set (#334).
func (se *SelectiveExtractor) ChunkKeysForPaths(paths []string) []string {
	seen := make(map[string]struct{})
	var keys []string
	for _, p := range paths {
		entry := se.resolveEntry(p)
		if entry == nil {
			continue
		}
		key := se.resolveKey(entry.S3Key)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			keys = append(keys, key)
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

// AllChunkKeys returns the resolved S3 object keys for every chunk in the
// manifest. Use this for a full-archive Glacier pre-flight check. As with
// ChunkKeysForPaths, keys are resolved rather than raw (#334).
func (se *SelectiveExtractor) AllChunkKeys() []string {
	keys := make([]string, 0, len(se.manifest.Chunks))
	for _, c := range se.manifest.Chunks {
		keys = append(keys, se.resolveKey(c.S3Key))
	}
	return keys
}

// resolveKey maps a manifest-recorded S3Key to the object key within the
// manifest's bucket. Single place so the pre-flight and download paths cannot
// drift apart again (#334).
func (se *SelectiveExtractor) resolveKey(s3Key string) string {
	return ResolveObjectKey(se.manifest.Prefix, se.manifest.Bucket, s3Key)
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

	// #341: every write below goes through this root, so a symlinked component
	// inside destDir is refused rather than followed.
	root, err := destRoot(destDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

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
			restored, written, err := se.writeDirectFiles(data, grp.files, root, destDir)
			stats.Restored += int64(restored)
			stats.Bytes += written
			if err != nil {
				stats.Failed += int64(len(grp.files)) - int64(restored)
			}
			continue
		}

		restored, written, err := se.extractFromChunkData(data, grp.files, root, destDir)
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
func (se *SelectiveExtractor) writeDirectFiles(data []byte, files []*FileEntry, root *os.Root, destDir string) (int, int64, error) {
	var restored int
	var totalBytes int64
	for _, entry := range files {
		// #270: never write corrupt bytes. If the downloaded object doesn't
		// match the recorded checksum, skip it and let the caller count it as
		// failed rather than restoring a bad file.
		if se.checksumMismatch(entry, data) {
			return restored, totalBytes, fmt.Errorf("checksum mismatch for %s: stored object does not match manifest", entry.Path)
		}
		// #282: preserve directory structure under destDir, escape-safe.
		outPath, err := se.restorePath(destDir, entry.Path)
		if err != nil {
			return restored, totalBytes, err
		}
		// #341: resolve through the root so a symlinked parent or leaf inside
		// destDir is refused instead of followed.
		rel, err := destRelPath(destDir, outPath)
		if err != nil {
			return restored, totalBytes, err
		}
		if err := prepareParents(root, rel); err != nil {
			return restored, totalBytes, err
		}
		if err := writeContained(root, rel, data); err != nil {
			return restored, totalBytes, err
		}
		restoreModTime(root, rel, entry.ModTime)
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
	// Normalize the stored key: some upload paths record ChunkEntry/FileEntry
	// S3Key as a full URL or a bucket-qualified path (#273), which can't be used
	// as an object key verbatim. ResolveObjectKey maps all shapes to the real
	// object key relative to the bucket.
	out, err := se.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(se.objectBucket()),
		Key:    aws.String(se.resolveKey(s3Key)),
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
func (se *SelectiveExtractor) extractFromChunkData(data []byte, files []*FileEntry, root *os.Root, destDir string) (int, int64, error) {
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
		// #282: same escape-safe, structure-preserving layout as the direct path.
		outPath, err := se.restorePath(destDir, entry.Path)
		if err != nil {
			return restored, totalBytes, err
		}
		// #341: same root-relative, symlink-refusing write as the direct path.
		rel, err := destRelPath(destDir, outPath)
		if err != nil {
			return restored, totalBytes, err
		}
		if mkErr := prepareParents(root, rel); mkErr != nil {
			return restored, totalBytes, mkErr
		}

		verifyThis := se.verify && entry.Checksum != "" &&
			(se.manifest.ChecksumAlgorithm == "" || se.manifest.ChecksumAlgorithm == ChecksumAlgorithmSHA256)

		if verifyThis {
			// #270: hash the extracted content and only write it if it matches
			// the manifest, so restore never lands corrupt bytes on disk. The
			// entry is bounded by its declared size (hdr.Size), and CopyN both
			// caps reads and detects a truncated entry.
			buf := make([]byte, 0, hdr.Size)
			w := bytes.NewBuffer(buf)
			if _, err := io.CopyN(w, tarReader, hdr.Size); err != nil {
				return restored, totalBytes, fmt.Errorf("read %s: %w", entry.Path, err)
			}
			content := w.Bytes()
			if se.checksumMismatch(entry, content) {
				return restored, totalBytes, fmt.Errorf("checksum mismatch for %s: stored data does not match manifest", entry.Path)
			}
			if err := writeContained(root, rel, content); err != nil {
				return restored, totalBytes, err
			}
			restoreModTime(root, rel, entry.ModTime)
			restored++
			totalBytes += int64(len(content))
			continue
		}

		f, err := createContained(root, rel)
		if err != nil {
			return restored, totalBytes, err
		}
		// #337: CopyN, not Copy. A tar entry cut short mid-stream is
		// indistinguishable from EOF to io.Copy, which returns nil having
		// written a short file — so a truncated archive restored as a
		// silently-incomplete file with a success exit code. CopyN is bounded by
		// the declared size and returns io.EOF when the entry ends early.
		//
		// This is the path taken when no per-file checksum was recorded, which
		// is precisely the case for archives written before #270 — and v0.14.0 /
		// v0.15.0, which carry the #275 truncation bug, are exactly those. The
		// archives most likely to be truncated took the one path that could not
		// notice.
		written, err := io.CopyN(f, tarReader, hdr.Size) // nosemgrep: go.lang.security.decompression_bomb.potential-dos-via-decompression-bomb -- bounded by the manifest's declared size
		_ = f.Close()
		if err != nil {
			// Remove the partial file. An absent file is a correct, loud
			// outcome; a short one that looks complete is not.
			_ = root.Remove(rel)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return restored, totalBytes, fmt.Errorf(
					"truncated archive: %s declares %d bytes but the stored chunk ended after %d",
					entry.Path, hdr.Size, written)
			}
			return restored, totalBytes, fmt.Errorf("write %s: %w", outPath, err)
		}
		restoreModTime(root, rel, entry.ModTime)
		restored++
		totalBytes += written
	}

	return restored, totalBytes, nil
}
