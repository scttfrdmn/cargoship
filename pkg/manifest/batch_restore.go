// Package manifest provides selective file restoration from CargoShip archives
// using hash-based addressing and LRU chunk caching (Issue #189).
package manifest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"container/list"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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
	// Skipped is the number of files not restored because a file already
	// existed at the destination and the policy is ExistingSkip (#915).
	Skipped int64
	// Errors holds the reasons behind Failed, at most maxRecordedErrors of them,
	// so a failed restore says why instead of only how many (#905).
	Errors []string
}

// maxRecordedErrors caps RestoreStats.Errors so a restore of a huge, broken
// archive doesn't hold a reason for every file.
const maxRecordedErrors = 20

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

	// errMu guards RestoreStats.Errors, which concurrent chunk groups append to.
	errMu sync.Mutex

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

	// existing says what to do about a file already at a destination path
	// (#915). The zero value refuses the whole restore.
	existing ExistingFilePolicy

	// bucket overrides the bucket objects are fetched from. Empty means use
	// manifest.Bucket, which is the historical behavior.
	//
	// The manifest records the bucket it was written to, so an archive copied or
	// replicated elsewhere carried a stale name and every fetch went to the
	// ORIGINAL bucket — silently, since the caller had already been asked for a
	// bucket in the S3 URL and had no reason to think it was ignored (#335). Set
	// with SetBucket to the bucket the manifest was actually read from.
	bucket string

	// prefix, when prefixSet, is the prefix the manifest was read from, which
	// object keys are resolved against instead of manifest.Prefix (#1032).
	prefix    string
	prefixSet bool
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

// SetPrefix resolves object keys against prefix — the prefix the manifest was
// read from — instead of the prefix recorded inside it. "" is a real prefix (an
// archive at the bucket root), so every call takes effect. Returns the receiver
// for chaining.
//
// The prefix half of #335: an archive copied to a new prefix carries the
// original's prefix in its manifest, so a restore from the copy fetched the
// ORIGINAL's objects, succeeding against data the user believed they had moved
// or reporting a 404 for data that was right there (#1032).
func (se *SelectiveExtractor) SetPrefix(prefix string) *SelectiveExtractor {
	se.prefix, se.prefixSet = prefix, true
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

// ExistingFilePolicy says what a restore does about a file already at a
// destination path (#915).
type ExistingFilePolicy int

const (
	// ExistingRefuse refuses the whole restore, listing the conflicts, and writes
	// nothing. The default: restoring into a working copy used to replace newer
	// local edits with the backed-up version, silently.
	ExistingRefuse ExistingFilePolicy = iota
	// ExistingOverwrite replaces existing files (atomically, #907).
	ExistingOverwrite
	// ExistingSkip keeps existing files and restores the rest.
	ExistingSkip
)

// SetExisting sets the policy for files already at the destination. Returns
// the receiver for chaining.
func (se *SelectiveExtractor) SetExisting(p ExistingFilePolicy) *SelectiveExtractor {
	se.existing = p
	return se
}

// SetFlatten toggles flat restore layout (destDir/<basename> per file) instead
// of the default dataset-relative layout. Returns the receiver for chaining.
// Handy for targeted restores where the caller just wants the file(s) in one
// directory. Two files with the same basename are refused (#908).
func (se *SelectiveExtractor) SetFlatten(f bool) *SelectiveExtractor {
	se.flatten = f
	return se
}

// checksumMismatch reports whether restore-time verification is active for this
// entry and the given content fails it. It returns false (no mismatch, restore
// the bytes) only when verification is off or the entry has no recorded
// checksum. When the entry HAS a checksum recorded under an algorithm this build
// cannot recompute it fails CLOSED — returns true (CSH-SEC-005): the file is
// unverifiable, and with verification on we refuse it rather than silently
// writing unchecked bytes, so a manifest declaring an unknown algorithm cannot
// disable integrity checking. Callers must NOT pre-gate on the algorithm; route
// every checksummed entry through here so the fail-closed decision is honored.
func (se *SelectiveExtractor) checksumMismatch(entry *FileEntry, content []byte) bool {
	if !se.verify || entry.Checksum == "" {
		return false
	}
	if !canRecomputeChecksum(se.manifest.ChecksumAlgorithm) {
		return true // unverifiable algorithm: fail closed
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

// atomicFile is a restore target being written. The bytes go to a temporary file
// in the destination's directory, which replaces the destination only once it is
// complete and synced (#907).
//
// Restore used to open the destination itself with O_TRUNC and remove it when the
// write failed. Restoring over an existing file from a truncated chunk, onto a full
// disk, or through a crash therefore first emptied the user's file and then
// deleted it: the restore failed AND the local copy was gone. Now a failure removes
// only the temporary file, and the existing file is untouched until a complete
// replacement is ready.
type atomicFile struct {
	root *os.Root
	rel  string // destination, relative to root
	tmp  string // temporary file, relative to root
	f    *os.File
}

// createAtomic starts writing relPath inside root, refusing to write through a
// symlink at the leaf.
//
// os.Root alone already refuses a leaf symlink that points outside the root —
// the escape #341 is about. The extra Lstat rejects a leaf symlink pointing
// *inside* the root too, because a restore should write the file it was asked to
// write rather than whatever a link at that path names. That second case is a
// correctness guard, not a containment one, and it has an unavoidable TOCTOU
// window. Containment does not depend on it: os.Root re-validates every component
// at open time, and the final rename replaces the link itself rather than
// following it.
func createAtomic(root *os.Root, relPath string) (*atomicFile, error) {
	if fi, err := root.Lstat(relPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to restore %s: destination path is a symlink", relPath)
	}
	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return nil, fmt.Errorf("create %s: temporary name: %w", relPath, err)
	}
	dir, base := path.Split(relPath)
	tmp := dir + ".cargoship-restore-" + base + "-" + hex.EncodeToString(suffix[:]) + ".tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", relPath, err)
	}
	return &atomicFile{root: root, rel: relPath, tmp: tmp, f: f}, nil
}

// Write implements io.Writer.
func (a *atomicFile) Write(p []byte) (int, error) { return a.f.Write(p) }

// commit syncs and closes the temporary file, checking both, then renames it over
// the destination. On any failure the temporary file is removed and the
// destination is left as it was.
func (a *atomicFile) commit() error {
	if err := a.f.Sync(); err != nil {
		a.abort()
		return fmt.Errorf("write %s: sync: %w", a.rel, err)
	}
	if err := a.f.Close(); err != nil {
		_ = a.root.Remove(a.tmp)
		return fmt.Errorf("write %s: close: %w", a.rel, err)
	}
	if err := a.root.Rename(a.tmp, a.rel); err != nil {
		_ = a.root.Remove(a.tmp)
		return fmt.Errorf("write %s: %w", a.rel, err)
	}
	return nil
}

// abort discards the temporary file. The destination is never touched.
func (a *atomicFile) abort() {
	_ = a.f.Close()
	_ = a.root.Remove(a.tmp)
}

// writeContained writes data to relPath inside root, atomically and with the
// same symlink refusal as createAtomic.
func writeContained(root *os.Root, relPath string, data []byte) error {
	a, err := createAtomic(root, relPath)
	if err != nil {
		return err
	}
	if _, err := a.Write(data); err != nil {
		a.abort()
		return fmt.Errorf("write %s: %w", relPath, err)
	}
	return a.commit()
}

// relativeEntryPath returns entryPath relative to the manifest's SourcePath (the
// upload root) when it sits under it; otherwise it returns entryPath unchanged
// (the sanitizer in restorePath still makes it destDir-safe). This is what makes
// the default layout dataset-relative rather than rooted at "/".
func (se *SelectiveExtractor) relativeEntryPath(entryPath string) string {
	return RelativeToSource(se.manifest.SourcePath, entryPath)
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
// bucket, under the prefix set by SetPrefix or else the manifest's own. Single
// place so the pre-flight and download paths cannot drift apart again (#334).
func (se *SelectiveExtractor) resolveKey(s3Key string) string {
	if se.prefixSet {
		return ResolveObjectKeyAt(se.manifest.Prefix, se.prefix, se.manifest.Bucket, s3Key)
	}
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
// chunkGroup collects the files that live in one S3 object (one download).
type chunkGroup struct {
	s3Key string
	files []*FileEntry
}

func (se *SelectiveExtractor) BatchRestore(ctx context.Context, targets []string, destDir string) (*RestoreStats, error) {
	stats := &RestoreStats{}
	if len(targets) == 0 {
		return stats, nil
	}

	// Group FileEntry pointers by S3 key (one S3 key == one chunk download).
	chunkMap := make(map[string]*chunkGroup)

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

	// #436: chunk-by-S3-key lookup for the random-access frame index. Keyed by
	// S3Key (not chunk ID) so it's robust to how keys/ids are assigned.
	var chunkByKey map[string]*ChunkEntry
	if !directMode {
		chunkByKey = make(map[string]*ChunkEntry, len(se.manifest.Chunks))
		for i := range se.manifest.Chunks {
			chunkByKey[se.manifest.Chunks[i].S3Key] = &se.manifest.Chunks[i]
		}
	}

	entries := make([]*FileEntry, len(targets))
	resolveErrs := make([]error, len(targets))
	for i, target := range targets {
		entries[i], resolveErrs[i] = se.resolveEntryErr(target)
	}
	// #908: refuse before writing anything if two different files would land on
	// the same output path, so one can't silently replace the other.
	if err := se.checkOutputCollisions(root, destDir, entries); err != nil {
		return nil, err
	}
	// #915: decide about files already at the destination before writing any.
	skip, err := se.checkExisting(root, destDir, entries)
	if err != nil {
		return nil, err
	}

	for i, target := range targets {
		entry := entries[i]
		if skip[i] {
			stats.Skipped++
			continue
		}
		if entry == nil {
			if resolveErrs[i] != nil {
				se.noteFailure(stats, 1, resolveErrs[i])
			} else {
				se.noteFailure(stats, 1, fmt.Errorf("%s: not found in manifest", target))
			}
			continue
		}
		// #436: fetch and decode just this file's frame when the chunk carries a
		// frame index; otherwise fall through to the whole-chunk group below.
		if !directMode && se.tryFrameRestore(ctx, entry, chunkByKey[entry.S3Key], root, destDir, stats) {
			continue
		}
		key := entry.S3Key
		if _, ok := chunkMap[key]; !ok {
			chunkMap[key] = &chunkGroup{s3Key: key}
		}
		chunkMap[key].files = append(chunkMap[key].files, entry)
	}

	// Download + extract each chunk group concurrently (#472). Serially, a
	// direct-upload restore (one S3 object per file → one group per file) ran at
	// ~1.2 MB/s regardless of file count — each GET waited on the previous one's
	// round trip. Groups are independent (distinct S3 keys, distinct output
	// paths), the chunk cache is already mutex-guarded, and *os.Root writes are
	// safe for concurrent use, so a bounded pool is sufficient. Stats are updated
	// atomically.
	groups := make([]*chunkGroup, 0, len(chunkMap))
	for _, grp := range chunkMap {
		groups = append(groups, grp)
	}
	sem := make(chan struct{}, restoreConcurrency)
	var wg sync.WaitGroup
	for _, grp := range groups {
		grp := grp
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			se.restoreGroup(ctx, grp, directMode, root, destDir, stats)
		}()
	}
	wg.Wait()

	return stats, nil
}

// checkExisting applies the existing-file policy (#915). It returns the indexes
// of entries to skip, or, under ExistingRefuse, an error naming the files that
// already exist.
func (se *SelectiveExtractor) checkExisting(root *os.Root, destDir string, entries []*FileEntry) (map[int]bool, error) {
	if se.existing == ExistingOverwrite {
		return nil, nil
	}
	skip := map[int]bool{}
	var conflicts []string
	n := 0
	seen := map[string]bool{}
	for i, e := range entries {
		if e == nil {
			continue
		}
		out, err := se.restorePath(destDir, e.Path)
		if err != nil {
			continue
		}
		rel, err := destRelPath(destDir, out)
		if err != nil {
			continue
		}
		if _, err := root.Lstat(rel); err != nil {
			continue
		}
		if se.existing == ExistingSkip {
			skip[i] = true
			continue
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true
		n++
		if len(conflicts) < maxListedCollisions {
			conflicts = append(conflicts, rel)
		}
	}
	if n > 0 {
		return nil, fmt.Errorf("%w: %d file(s) already exist at the destination and would be replaced "+
			"(%s); re-run with --overwrite to replace them or --skip-existing to keep them", ErrExistingFiles, n, strings.Join(conflicts, ", "))
	}
	return skip, nil
}

// ErrExistingFiles is returned (wrapped) when a restore is refused because
// files already exist at the destination (#915). Nothing has been written, so
// the restore can simply be retried with a policy.
var ErrExistingFiles = errors.New("refusing to restore")

// maxListedCollisions caps how many collisions the refusal names.
const maxListedCollisions = 10

// checkOutputCollisions returns an error naming the collisions if two different
// manifest entries would restore to the same output path (#908).
//
// --flatten reduces every entry to its basename, so src/a/config.txt and
// src/b/config.txt both became config.txt. Nothing checked, and the concurrent
// group pool assumed distinct outputs: restore could report both files restored
// with one on disk, fail on a racing create, or let the second overwrite the
// first. The same happens without --flatten on a case-insensitive filesystem
// (macOS, Windows) for README and readme from a case-sensitive source, so paths
// are compared case-folded when destDir's filesystem ignores case. Parts of one
// split file share a path legitimately and are not collisions.
func (se *SelectiveExtractor) checkOutputCollisions(root *os.Root, destDir string, entries []*FileEntry) error {
	fold := caseInsensitive(root)
	owner := make(map[string]string, len(entries))
	var collisions []string
	n := 0
	for _, e := range entries {
		if e == nil {
			continue
		}
		out, err := se.restorePath(destDir, e.Path)
		if err != nil {
			continue // fails on its own later, with its own reason
		}
		key := out
		if fold {
			key = strings.ToLower(out)
		}
		prev, seen := owner[key]
		switch {
		case !seen:
			owner[key] = e.Path
		case prev != e.Path:
			n++
			if len(collisions) < maxListedCollisions {
				rel, _ := filepath.Rel(destDir, out)
				collisions = append(collisions, fmt.Sprintf("%s and %s both restore to %s", prev, e.Path, rel))
			}
		}
	}
	if n == 0 {
		return nil
	}
	hint := "restore them to separate directories or without --flatten"
	if fold {
		hint += "; the destination filesystem ignores case"
	}
	return fmt.Errorf("refusing to restore: %d output path collision(s), so a file would be silently lost (%s): %s",
		n, hint, strings.Join(collisions, "; "))
}

// caseInsensitive reports whether the filesystem holding root ignores case, by
// creating a probe file and looking it up under a different case. If the probe
// can't be created it assumes the platform default.
func caseInsensitive(root *os.Root) bool {
	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return platformCaseInsensitive()
	}
	name := ".cargoship-case-probe-" + hex.EncodeToString(suffix[:]) + "-Aa"
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return platformCaseInsensitive()
	}
	_ = f.Close()
	defer func() { _ = root.Remove(name) }()
	_, err = root.Stat(strings.ToLower(name))
	return err == nil
}

// platformCaseInsensitive is the usual default: macOS and Windows filesystems
// ignore case, Linux ones don't.
func platformCaseInsensitive() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

// restoreConcurrency bounds how many chunk groups BatchRestore downloads and
// extracts in parallel. Enough to hide per-request latency (the direct-upload
// win) without holding too many decompressed chunks in memory at once.
const restoreConcurrency = 16

// noteFailure counts n files as failed and records why. Every failure path goes
// through here so Failed and its reasons can't drift apart. Safe for concurrent
// use.
func (se *SelectiveExtractor) noteFailure(stats *RestoreStats, n int64, err error) {
	if n <= 0 {
		return
	}
	atomic.AddInt64(&stats.Failed, n)
	if err == nil {
		return
	}
	se.errMu.Lock()
	if len(stats.Errors) < maxRecordedErrors {
		stats.Errors = append(stats.Errors, err.Error())
	}
	se.errMu.Unlock()
}

// restoreGroup downloads one chunk group (unless cached) and writes its files,
// updating stats atomically. Safe to call concurrently across distinct groups.
func (se *SelectiveExtractor) restoreGroup(ctx context.Context, grp *chunkGroup, directMode bool, root *os.Root, destDir string, stats *RestoreStats) {
	data := se.cache.get(grp.s3Key)
	if data == nil {
		fetched, err := se.downloadChunk(ctx, grp.s3Key)
		if err != nil {
			se.noteFailure(stats, int64(len(grp.files)), err)
			return
		}
		data = fetched
		se.cache.put(grp.s3Key, data)
		atomic.AddInt64(&stats.ChunksDownloaded, 1)
	}

	var restored int
	var written int64
	var err error
	if directMode {
		// One S3 object == one file; write the raw bytes.
		restored, written, err = se.writeDirectFiles(data, grp.files, root, destDir)
	} else {
		restored, written, err = se.extractFromChunkData(data, grp.files, root, destDir)
	}
	atomic.AddInt64(&stats.Restored, int64(restored))
	atomic.AddInt64(&stats.Bytes, written)
	if err != nil {
		se.noteFailure(stats, int64(len(grp.files))-int64(restored), err)
	}
}

// resolveEntry finds the manifest entry for a restore target. It tries an exact
// path match first (via the O(1) index), then falls back to matching by relative
// suffix / basename — so `--file greeting.txt` resolves even though manifests
// currently store the file's absolute source path. (Issue #228)
func (se *SelectiveExtractor) resolveEntry(target string) *FileEntry {
	e, _ := se.resolveEntryErr(target)
	return e
}

// resolveEntryErr resolves target like resolveEntry, and says why when it can't:
// an ambiguous suffix or basename is an error naming the candidates (#921). The
// first suffix match used to win, so --file a/b.txt silently restored x/a/b.txt
// when y/a/b.txt also existed.
func (se *SelectiveExtractor) resolveEntryErr(target string) (*FileEntry, error) {
	if entry := se.query.FindFile(target); entry != nil {
		return entry, nil
	}
	// Manifest paths are always slash-separated (the portable format convention),
	// so resolve in slash space regardless of host OS. filepath here would use "\"
	// on Windows and never match the manifest's "/" paths — `restore --file
	// a/b.txt` then failed on Windows (#563). ToSlash the target (a user-supplied
	// --file may use either separator) and the manifest paths (belt-and-suspenders).
	clean := path.Clean(filepath.ToSlash(target))
	// When the target carries a path (not a bare basename), match it exactly or by
	// path-suffix across ALL entries FIRST, so a unique suffix match always wins
	// even when many files share the basename. (The previous single-pass resolver
	// bailed on basename ambiguity before it could reach the target's suffix match,
	// so a fully-qualified path failed whenever ≥2 other files shared its basename
	// earlier in the manifest — 46% of a random ~/src sample, since source trees are
	// full of repeated names like abort.d.ts / __init__.py.)
	if strings.ContainsRune(clean, '/') {
		var suffix []*FileEntry
		for i := range se.manifest.Files {
			p := filepath.ToSlash(se.manifest.Files[i].Path)
			if p == clean {
				return &se.manifest.Files[i], nil
			}
			if strings.HasSuffix(p, "/"+clean) && !containsPath(suffix, se.manifest.Files[i].Path) {
				suffix = append(suffix, &se.manifest.Files[i])
			}
		}
		switch len(suffix) {
		case 1:
			return suffix[0], nil
		case 0:
		default:
			return nil, ambiguousTarget(target, suffix)
		}
	}
	// Bare basename (e.g. `--file greeting.txt`), or a path with no suffix match:
	// resolve by basename, but only when it is unambiguous.
	base := path.Base(clean)
	var matches []*FileEntry
	for i := range se.manifest.Files {
		if path.Base(filepath.ToSlash(se.manifest.Files[i].Path)) == base && !containsPath(matches, se.manifest.Files[i].Path) {
			matches = append(matches, &se.manifest.Files[i])
		}
	}
	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return matches[0], nil
	default:
		return nil, ambiguousTarget(target, matches) // require a more specific target
	}
}

// containsPath reports whether es already holds an entry for p (split-file
// parts share a path and are one file).
func containsPath(es []*FileEntry, p string) bool {
	for _, e := range es {
		if e.Path == p {
			return true
		}
	}
	return false
}

// ambiguousTarget names up to five of the entries a restore target matches.
func ambiguousTarget(target string, es []*FileEntry) error {
	var names []string
	for i, e := range es {
		if i == 5 {
			names = append(names, fmt.Sprintf("and %d more", len(es)-5))
			break
		}
		names = append(names, e.Path)
	}
	return fmt.Errorf("%s: matches %d files (%s); give more of the path", target, len(es), strings.Join(names, ", "))
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
		// #905: a direct object is the file's raw bytes, so its length must match
		// the recorded size. Without this, a file with no recorded checksum and an
		// empty or short object was written short and counted as restored. Part of
		// verification: --no-verify restores the stored bytes as they are.
		if se.verify && int64(len(data)) != entry.Size {
			return restored, totalBytes, fmt.Errorf("%s: stored object is %d bytes but the manifest records %d", entry.Path, len(data), entry.Size)
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

// expectedObjectBytes returns the size the manifest declares for the object at
// s3Key — a chunk's CompressedSize, else a direct-upload file's Size — or 0 when
// the manifest declares no size for it. Used to bound restore reads
// (CSH-SEC-003).
func (se *SelectiveExtractor) expectedObjectBytes(s3Key string) int64 {
	for i := range se.manifest.Chunks {
		if se.manifest.Chunks[i].S3Key == s3Key {
			return se.manifest.Chunks[i].CompressedSize
		}
	}
	for i := range se.manifest.Files {
		if se.manifest.Files[i].S3Key == s3Key {
			return se.manifest.Files[i].Size
		}
	}
	return 0
}

// trustedChunkSize returns the chunk object's recorded CompressedSize when it is
// known to be the true size of the uploaded bytes, else 0 (unknown). It is a
// lower bound for the stored object only when:
//   - the manifest is format 2.1 or later: before #445 (first released with 2.1,
//     in v0.24.0) CompressedSize held the UNCOMPRESSED estimate, so a v0.16-v0.18
//     chunk records 12 MiB for an 1.8 KB object; and
//   - the size was counted from the written bytes, which a whole-object checksum
//     or a frame index proves. Without either, the writer fell back to the
//     uncompressed estimate.
//
// Direct-upload objects are not covered here; writeDirectFiles checks their size
// against FileEntry.Size, under verification so --no-verify keeps its meaning.
func (se *SelectiveExtractor) trustedChunkSize(s3Key string) int64 {
	if !manifestAtLeast(se.manifest.Version, 2, 1) {
		return 0
	}
	for i := range se.manifest.Chunks {
		c := &se.manifest.Chunks[i]
		if c.S3Key != s3Key {
			continue
		}
		if c.Checksum == "" && len(c.Frames) == 0 {
			return 0
		}
		return c.CompressedSize
	}
	return 0
}

// manifestAtLeast reports whether a "major.minor" manifest version is at least
// major.minor. An unparseable version is treated as older (no new guarantees).
func manifestAtLeast(version string, major, minor int) bool {
	var gotMajor, gotMinor int
	if _, err := fmt.Sscanf(version, "%d.%d", &gotMajor, &gotMinor); err != nil {
		return false
	}
	return gotMajor > major || (gotMajor == major && gotMinor >= minor)
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
	// CSH-SEC-003: bound the read to the manifest's declared object size (a
	// finite fallback when undeclared), rejecting an object larger than declared.
	limit := objectReadLimit(se.expectedObjectBytes(s3Key))
	if err := checkContentLength(out.ContentLength, limit, "chunk object "+s3Key); err != nil {
		return nil, err
	}
	data, err := readAllLimited(out.Body, limit, "chunk object "+s3Key)
	if err != nil {
		return nil, fmt.Errorf("read S3 body %q: %w", s3Key, err)
	}
	// #905: the read limit already rejects an object LARGER than declared; also
	// reject a SHORTER one. A truncated or empty object (e.g. an upload retry
	// that re-sent a drained stream, #901) otherwise decodes to fewer entries
	// than the manifest promises.
	if want := se.trustedChunkSize(s3Key); want > 0 && int64(len(data)) < want {
		return nil, fmt.Errorf("object %q is %d bytes but the manifest records %d: truncated or incomplete upload", s3Key, len(data), want)
	}
	return data, nil
}

// ChunkCompression is the exported entry point to chunkCompression, for callers
// outside this package (e.g. the rebalance path) that must decode a chunk by its
// per-chunk key extension rather than the manifest's single top-level type (#452,
// #482). One implementation, so the two can never drift.
func ChunkCompression(s3Key, manifestType string) string {
	return chunkCompression(s3Key, manifestType)
}

// chunkCompression returns the decompression a chunk needs. The chunk's S3 key
// extension is authoritative when present — the per-chunk signal the format spec
// documents (#452) — because the manifest's top-level compression_type is a
// single value even when an upload produced both compressed (.tar.zst) and plain
// (.tar) chunks, so decoding by it wraps a zstd reader around a plain tar and
// restores nothing. When the key carries no recognized archive extension, fall
// back to the manifest's declared type (the historical behavior).
func chunkCompression(s3Key, manifestType string) string {
	switch {
	case strings.HasSuffix(s3Key, ".tar.zst"), strings.HasSuffix(s3Key, ".zst"):
		return "zstd"
	case strings.HasSuffix(s3Key, ".tar.gz"), strings.HasSuffix(s3Key, ".gz"):
		return "gzip"
	case strings.HasSuffix(s3Key, ".tar"):
		return "none"
	default: // no recognized extension → trust the manifest's declared type
		switch manifestType {
		case "gzip", "gz":
			return "gzip"
		case "none":
			return "none"
		default: // "zstd" or "" → zstd, matching the historical reader default
			return "zstd"
		}
	}
}

// chunkKeyOf returns the S3 key shared by a group of files from the same chunk.
func chunkKeyOf(files []*FileEntry) string {
	if len(files) == 0 {
		return ""
	}
	return files[0].S3Key
}

// downloadChunkRange fetches just the byte range [offset, offset+length) of the
// S3 object at s3Key (format-2.1 random access, #436). partial reports whether
// the server honored the Range (HTTP 206 with a Content-Range header); a backend
// that ignores Range returns the whole object with partial=false, which the
// caller compensates for.
func (se *SelectiveExtractor) downloadChunkRange(ctx context.Context, s3Key string, offset, length int64) (data []byte, partial bool, err error) {
	rng := fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)
	out, err := se.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(se.objectBucket()),
		Key:    aws.String(se.resolveKey(s3Key)),
		Range:  aws.String(rng),
	})
	if err != nil {
		return nil, false, fmt.Errorf("S3 GetObject %q range %s: %w", s3Key, rng, err)
	}
	defer func() { _ = out.Body.Close() }()
	// CSH-SEC-003: a backend that ignores the Range returns the whole object, so
	// bound by the chunk's declared size (which covers both the honored range and
	// the whole-object fallback), with a finite fallback when undeclared.
	limit := objectReadLimit(se.expectedObjectBytes(s3Key))
	if err := checkContentLength(out.ContentLength, limit, "chunk object "+s3Key); err != nil {
		return nil, false, err
	}
	body, err := readAllLimited(out.Body, limit, "chunk object "+s3Key)
	if err != nil {
		return nil, false, fmt.Errorf("read S3 body %q range %s: %w", s3Key, rng, err)
	}
	return body, out.ContentRange != nil, nil
}

// findFrame returns the frame whose uncompressed span fully contains
// [offset, offset+size), or nil if no single frame covers it (#436). Framing cuts
// only at file boundaries, so a whole file is always contained in one frame.
func findFrame(frames []FrameEntry, offset, size int64) *FrameEntry {
	end := offset + size
	for i := range frames {
		f := &frames[i]
		if offset >= f.UncompressedOffset && end <= f.UncompressedOffset+f.UncompressedSize {
			return f
		}
	}
	return nil
}

// tryFrameRestore restores a single whole file by fetching and decoding only the
// zstd frame that contains it, instead of downloading the whole chunk (#436). It
// returns handled=true when it took responsibility for the file (recording a
// success or a definite failure in stats), and false when the file is not
// eligible for random access — a non-zstd chunk, a chunk with no frame index, a
// split-file part (which needs reassembly via the whole-chunk path), or a file
// no single frame covers — so the caller falls back to the whole-chunk path.
func (se *SelectiveExtractor) tryFrameRestore(ctx context.Context, entry *FileEntry, chunk *ChunkEntry, root *os.Root, destDir string, stats *RestoreStats) (handled bool) {
	if chunk == nil || len(chunk.Frames) == 0 || chunkCompression(chunk.S3Key, se.manifest.CompressionType) != "zstd" {
		return false
	}
	if entry.TotalParts > 1 {
		return false // split part: whole-chunk path reassembles it
	}
	fr := findFrame(chunk.Frames, entry.ArchiveOffset, entry.Size)
	if fr == nil {
		return false
	}

	comp, partial, err := se.downloadChunkRange(ctx, entry.S3Key, fr.CompressedOffset, fr.CompressedSize)
	if err != nil {
		se.noteFailure(stats, 1, fmt.Errorf("%s: %w", entry.Path, err))
		return true
	}
	stats.ChunksDownloaded++

	// #439: when the frame carries a content checksum, verify the fetched
	// compressed bytes before decoding — the strong per-range witness that
	// catches a changed/hostile endpoint without trusting the S3 ETag. Only when
	// the Range was honored (otherwise comp is the whole object, not the frame).
	if partial && fr.Checksum != "" {
		if !canRecomputeChecksum(se.manifest.ChecksumAlgorithm) {
			// CSH-SEC-005: unverifiable frame witness, fail closed
			se.noteFailure(stats, 1, fmt.Errorf("%s: frame checksum algorithm %q cannot be verified", entry.Path, se.manifest.ChecksumAlgorithm))
			return true
		}
		got := sha256.Sum256(comp)
		if hex.EncodeToString(got[:]) != fr.Checksum {
			se.noteFailure(stats, 1, fmt.Errorf("%s: frame checksum mismatch in %s", entry.Path, entry.S3Key))
			return true
		}
	}

	raw, err := decodeZstdFrame(comp)
	if err != nil {
		se.noteFailure(stats, 1, fmt.Errorf("%s: %w", entry.Path, err))
		return true
	}
	// The decoded bytes start at fr.UncompressedOffset when the Range was honored;
	// if the backend ignored it and returned the whole object, they start at 0.
	decodedStart := int64(0)
	if partial {
		decodedStart = fr.UncompressedOffset
	}
	sliceStart := entry.ArchiveOffset - decodedStart
	if sliceStart < 0 || sliceStart+entry.Size > int64(len(raw)) {
		se.noteFailure(stats, 1, fmt.Errorf("%s: not within its frame in %s (truncated or mismatched chunk)", entry.Path, entry.S3Key))
		return true
	}
	// #910: bound the slice by the tar header too. Archives written before #910
	// recorded the scan-time size for a file that changed before archiving, so the
	// manifest size can disagree with the tar entry; slicing it padded a shrunk
	// file with the next file's bytes. The entry's 512-byte header sits right
	// before ArchiveOffset; when it's in this frame, its size must agree.
	if hdrSize, ok := tarHeaderSizeBefore(raw, sliceStart); ok && hdrSize != entry.Size {
		se.noteFailure(stats, 1, fmt.Errorf("%s: the archive holds %d bytes but the manifest records %d "+
			"(the file changed while it was being uploaded)", entry.Path, hdrSize, entry.Size))
		return true
	}
	content := raw[sliceStart : sliceStart+entry.Size]

	// #270: mirror the whole-chunk path — never write bytes that fail the
	// recorded checksum. No algorithm pre-gate: checksumMismatch fails closed on
	// an unverifiable algorithm (CSH-SEC-005).
	if se.checksumMismatch(entry, content) {
		se.noteFailure(stats, 1, fmt.Errorf("checksum mismatch for %s: stored data does not match manifest", entry.Path))
		return true
	}

	outPath, err := se.restorePath(destDir, entry.Path)
	if err != nil {
		se.noteFailure(stats, 1, fmt.Errorf("%s: %w", entry.Path, err))
		return true
	}
	rel, err := destRelPath(destDir, outPath)
	if err != nil {
		se.noteFailure(stats, 1, fmt.Errorf("%s: %w", entry.Path, err))
		return true
	}
	if err := prepareParents(root, rel); err != nil {
		se.noteFailure(stats, 1, fmt.Errorf("%s: %w", entry.Path, err))
		return true
	}
	if err := writeContained(root, rel, content); err != nil {
		se.noteFailure(stats, 1, fmt.Errorf("%s: %w", entry.Path, err))
		return true
	}
	restoreModTime(root, rel, entry.ModTime)
	stats.Restored++
	stats.Bytes += int64(len(content))
	return true
}

// tarHeaderSizeBefore parses the tar header block that ends at dataStart in raw
// and returns the entry's size. ok is false when there is no valid header there
// (it lies in an earlier frame, or the size is carried in a PAX record, in which
// case the header's own size field is zero).
func tarHeaderSizeBefore(raw []byte, dataStart int64) (int64, bool) {
	const block = 512
	if dataStart < block || dataStart > int64(len(raw)) {
		return 0, false
	}
	hdr, err := tar.NewReader(bytes.NewReader(raw[dataStart-block : dataStart])).Next()
	if err != nil || hdr.Size == 0 {
		return 0, false
	}
	return hdr.Size, true
}

// decodeZstdFrame decompresses a single self-contained zstd frame (#436).
func decodeZstdFrame(compressed []byte) ([]byte, error) {
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("zstd decoder: %w", err)
	}
	defer dec.Close()
	raw, err := dec.DecodeAll(compressed, nil)
	if err != nil {
		return nil, fmt.Errorf("decode zstd frame: %w", err)
	}
	return raw, nil
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

	// #452: decode by the chunk's KEY EXTENSION, not the manifest's top-level
	// compression_type. A mixed-compressibility upload holds both .tar.zst and
	// plain .tar chunks under a single "zstd" compression_type, so decoding by
	// that field wraps a zstd reader around a plain tar and restores nothing.
	switch chunkCompression(chunkKeyOf(files), se.manifest.CompressionType) {
	case "zstd":
		dec, err := zstd.NewReader(r)
		if err != nil {
			return 0, 0, fmt.Errorf("zstd decoder: %w", err)
		}
		defer dec.Close()
		tarReader = tar.NewReader(dec)
	case "gzip":
		gz, err := gzip.NewReader(r)
		if err != nil {
			return 0, 0, fmt.Errorf("gzip reader: %w", err)
		}
		defer func() { _ = gz.Close() }()
		tarReader = tar.NewReader(gz)
	default: // "none" — a plain .tar chunk
		tarReader = tar.NewReader(r)
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

		// Buffer + verify whenever the entry is checksummed and verification is on;
		// checksumMismatch fails closed if the algorithm is unrecomputable
		// (CSH-SEC-005) rather than streaming unchecked bytes to disk.
		verifyThis := se.verify && entry.Checksum != ""

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
			delete(want, hdr.Name)
			continue
		}

		f, err := createAtomic(root, rel)
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
		if err == nil {
			err = f.commit() // #907: the Close error was discarded here
		} else {
			// Discard the partial copy. An absent file is a correct, loud
			// outcome; a short one that looks complete is not. A file that
			// already existed at this path is left untouched (#907).
			f.abort()
		}
		if err != nil {
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
		delete(want, hdr.Name)
	}

	// #905: a clean end of archive is not success if requested entries never
	// appeared. A truncated or mismatched chunk (an empty object, a chunk
	// rebound by resume, a manifest written for a failed upload) used to return
	// nil here, so restore exited 0 with the files silently absent.
	if len(want) > 0 {
		return restored, totalBytes, missingEntriesError(want, chunkKeyOf(files))
	}
	return restored, totalBytes, nil
}

// missingEntriesError reports requested entries that a chunk did not contain,
// naming a few so the failure is actionable.
func missingEntriesError(want map[string]*FileEntry, s3Key string) error {
	names := make([]string, 0, len(want))
	for name := range want {
		names = append(names, name)
	}
	sort.Strings(names)
	const show = 3
	more := ""
	if len(names) > show {
		more = fmt.Sprintf(" and %d more", len(names)-show)
		names = names[:show]
	}
	return fmt.Errorf("%d requested file(s) missing from chunk %s: %s%s",
		len(want), s3Key, strings.Join(names, ", "), more)
}
