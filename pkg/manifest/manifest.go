package manifest

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/scttfrdmn/cargoship/pkg/encryption"
)

const (
	// ManifestVersion is the current manifest format version. 2.2 adds dataset
	// versioning (Manifest.DatasetID, VersionOrdinal; #521). 2.1 added the
	// optional random-access frame index (ChunkEntry.Frames, FileEntry.
	// ArchiveOffset, Manifest.FormatFeatures; #436). The schema is additive: the
	// reader is version-tolerant and still parses older (2.0/2.1) manifests,
	// ignoring fields it does not recognize.
	ManifestVersion = "2.2"

	// ChecksumAlgorithmSHA256 is the hash algorithm recorded in
	// Manifest.ChecksumAlgorithm for chunk (and per-file) checksums. It matches
	// the SHA-256 (hex) used by the deduplication index. (#271)
	ChecksumAlgorithmSHA256 = "sha256"

	// FormatFeatureFrames is the Manifest.FormatFeatures marker set when at least
	// one chunk carries a random-access frame index (#436).
	FormatFeatureFrames = "frames"

	// FormatFeatureVersioning is the Manifest.FormatFeatures marker set when the
	// manifest carries dataset-versioning identity (DatasetID/VersionOrdinal; #521).
	FormatFeatureVersioning = "versioning"

	// FormatFeatureWriters is the Manifest.FormatFeatures marker set when the upload
	// is writer-isolated (Manifest.WriterID set; objects under writers/<id>/; #520).
	FormatFeatureWriters = "writers"

	// ManifestFileName is the standard manifest filename
	ManifestFileName = "manifest.json"

	// ManifestFileNameGZ is the compressed manifest filename
	ManifestFileNameGZ = "manifest.json.gz"
)

// canRecomputeChecksum reports whether algo names a hash this build can
// actually recompute to verify content: sha256, or "" which legacy manifests
// (pre-#271) used to mean sha256 by default. Any other value is a hash we do not
// implement — a future format we cannot yet read, or a tampered manifest.
//
// Callers on the restore/verify paths use this to fail CLOSED (CSH-SEC-005): a
// recorded checksum we cannot recompute is treated as unverifiable, never as a
// silent pass, so a manifest declaring an unknown algorithm can no longer switch
// off integrity checking while still presenting legitimate-looking checksums.
func canRecomputeChecksum(algo string) bool {
	return algo == "" || algo == ChecksumAlgorithmSHA256
}

// Builder helps construct a manifest incrementally during upload
// All methods are thread-safe for concurrent use (Issue #88)
type Builder struct {
	mu       sync.RWMutex
	manifest *Manifest

	// Hostname for upload source tracking
	hostname string

	// #447: keys of files already recorded, so a resumed run (which preloads the
	// prior manifest's Files) does not re-append them when the scanner rediscovers
	// the same corpus and doubles TotalFiles. Keyed by fileEntryKey.
	seen map[string]bool
}

// fileEntryKey is a FileEntry's unique identity for dedup — its path, plus the
// part index for a split file (so each part of a split file is distinct).
func fileEntryKey(e FileEntry) string {
	return e.Path + "#" + strconv.Itoa(e.PartIndex)
}

// NewBuilder creates a new manifest builder
func NewBuilder(uploadID, sourcePath, bucket, prefix, region string) (*Builder, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	return &Builder{
		manifest: &Manifest{
			Version:           ManifestVersion,
			UploadID:          uploadID,
			CreatedAt:         time.Now(),
			SourcePath:        sourcePath,
			Hostname:          hostname,
			Bucket:            bucket,
			Prefix:            prefix,
			Region:            region,
			ChecksumAlgorithm: ChecksumAlgorithmSHA256,
			Files:             make([]FileEntry, 0),
			Chunks:            make([]ChunkEntry, 0),
			Shards:            make([]ShardEntry, 0),
		},
		hostname: hostname,
		seen:     make(map[string]bool),
	}, nil
}

// NewBuilderFromExisting creates a builder from an existing manifest for resume (Issue #157)
func NewBuilderFromExisting(existing *Manifest) (*Builder, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Clone the existing manifest to avoid modifying the original
	cloned := &Manifest{
		Version:           existing.Version,
		UploadID:          existing.UploadID,
		CreatedAt:         existing.CreatedAt,
		CompletedAt:       existing.CompletedAt,
		SourcePath:        existing.SourcePath,
		Hostname:          hostname, // Use current hostname for resumed portion
		Bucket:            existing.Bucket,
		Prefix:            existing.Prefix,
		Region:            existing.Region,
		TotalFiles:        existing.TotalFiles,
		TotalBytes:        existing.TotalBytes,
		TotalChunks:       existing.TotalChunks,
		ShardCount:        existing.ShardCount,
		CompressionType:   existing.CompressionType,
		CompressionLevel:  existing.CompressionLevel,
		CompressionRatio:  existing.CompressionRatio,
		ChecksumAlgorithm: existing.ChecksumAlgorithm,
		FormatFeatures:    append([]string(nil), existing.FormatFeatures...),
		Files:             storedFiles(existing.Files),
		Chunks:            append([]ChunkEntry(nil), existing.Chunks...),
		Shards:            append([]ShardEntry(nil), existing.Shards...),
		VersionInfo:       existing.VersionInfo,
		GitMetadata:       existing.GitMetadata,
		DVCCompatibility:  existing.DVCCompatibility,
		DVCPipeline:       existing.DVCPipeline,
		// #913: these were dropped, so a resumed upload lost its encryption
		// settings (and wrote a plaintext manifest for an --encrypt-manifest
		// upload) and could detach from its dataset chain.
		PreviousManifestID: existing.PreviousManifestID,
		SyncType:           existing.SyncType,
		DatasetID:          existing.DatasetID,
		VersionOrdinal:     existing.VersionOrdinal,
		WriterID:           existing.WriterID,
		DeletedPaths:       append([]string(nil), existing.DeletedPaths...),
		Encryption:         cloneEncryption(existing.Encryption),
		Deduplication:      existing.Deduplication,
	}

	// #447: seed the dedup set from the preloaded files so a resumed run that
	// re-scans the same corpus doesn't append them again (which doubled TotalFiles).
	seen := make(map[string]bool, len(cloned.Files))
	cloned.TotalFiles, cloned.TotalBytes = 0, 0
	for _, f := range cloned.Files {
		seen[fileEntryKey(f)] = true
		cloned.TotalFiles++
		cloned.TotalBytes += f.Size
	}

	return &Builder{
		manifest: cloned,
		hostname: hostname,
		seen:     seen,
	}, nil
}

// cloneEncryption copies e so a resumed builder can't alias the source manifest.
func cloneEncryption(e *EncryptionMetadata) *EncryptionMetadata {
	if e == nil {
		return nil
	}
	c := *e
	return &c
}

// storedFiles returns the entries that reached S3 (non-empty S3Key). A partial
// manifest also lists files whose chunk had not been uploaded yet, bound to that
// run's chunk IDs. Those IDs mean nothing to the resumed run, whose scan assigns new
// ones (#904), so the entries are dropped and the re-scan records them afresh.
func storedFiles(files []FileEntry) []FileEntry {
	out := make([]FileEntry, 0, len(files))
	for _, f := range files {
		if f.S3Key != "" {
			out = append(out, f)
		}
	}
	return out
}

// ChunkFile identifies one file, or one part of a split file, packed into a chunk,
// with the size and modification time the scan recorded for it.
type ChunkFile struct {
	Path      string
	PartIndex int
	Size      int64
	ModTime   time.Time
}

// BindChunkFiles points the entries for files at the chunk that now holds them,
// matching by path and part rather than by chunk ID, and records the new chunk ID,
// shard, S3 key, size and modification time (thread-safe).
//
// Binding by chunk ID was wrong on resume (#904). The scan skips a file already
// preloaded from the partial manifest, so its entry keeps the PREVIOUS run's chunk
// ID. Chunk IDs are not stable between runs, so that old ID could belong to an
// unrelated new chunk, and binding by ID pointed entries at objects that do not
// hold them. Binding by path is right in both cases, and on a fresh run it is the
// same as binding by ID.
func (b *Builder) BindChunkFiles(chunkID, shardID int, s3Key string, files []ChunkFile) {
	if len(files) == 0 {
		return
	}
	want := make(map[string]ChunkFile, len(files))
	for _, f := range files {
		want[f.Path+"#"+strconv.Itoa(f.PartIndex)] = f
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.manifest.Files {
		e := &b.manifest.Files[i]
		f, ok := want[fileEntryKey(*e)]
		if !ok {
			continue
		}
		b.manifest.TotalBytes += f.Size - e.Size
		e.ChunkID = chunkID
		e.ShardID = shardID
		e.S3Key = s3Key
		e.Size = f.Size
		e.ModTime = f.ModTime
		// A rebound entry's checksum and tar offset described the old chunk's copy.
		// The caller sets this chunk's values next; clear them so a stale value
		// can't survive when checksums or framing are off.
		e.Checksum = ""
		e.ArchiveOffset = 0
	}
}

// AddFile adds a file to the manifest (thread-safe)
func (b *Builder) AddFile(entry FileEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := fileEntryKey(entry)
	if b.seen[key] { // #447: already recorded (e.g. preloaded on resume) — don't duplicate
		return
	}
	b.seen[key] = true
	b.manifest.Files = append(b.manifest.Files, entry)
	b.manifest.TotalFiles++
	b.manifest.TotalBytes += entry.Size
}

// AddFileBatch adds multiple files to the manifest in a single lock (Issue #34 Phase 1.4)
// More efficient than calling AddFile repeatedly when adding many files at once
func (b *Builder) AddFileBatch(entries []FileEntry) {
	if len(entries) == 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// #447: skip entries already recorded (resume preloads the prior Files), so a
	// re-scan of the same corpus can't double TotalFiles.
	for _, entry := range entries {
		key := fileEntryKey(entry)
		if b.seen[key] {
			continue
		}
		b.seen[key] = true
		b.manifest.Files = append(b.manifest.Files, entry)
		b.manifest.TotalFiles++
		b.manifest.TotalBytes += entry.Size
	}
}

// AddChunk adds a chunk to the manifest (thread-safe)
func (b *Builder) AddChunk(entry ChunkEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.Chunks = append(b.manifest.Chunks, entry)
	b.manifest.TotalChunks++
}

// SetCompression sets compression information (thread-safe)
func (b *Builder) SetCompression(compressionType string, level int, ratio float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.CompressionType = compressionType
	b.manifest.CompressionLevel = level
	b.manifest.CompressionRatio = ratio
}

// SetShardCount sets the number of shards (thread-safe)
func (b *Builder) SetShardCount(count int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.ShardCount = count
	// Initialize shard entries
	b.manifest.Shards = make([]ShardEntry, count)
	for i := 0; i < count; i++ {
		b.manifest.Shards[i] = ShardEntry{
			ID:        i,
			Prefix:    fmt.Sprintf("%s/uploads/%s/shard-%d", b.manifest.Prefix, b.manifest.UploadID, i),
			ChunkKeys: make([]string, 0),
		}
	}
}

// SetSyncInfo sets sync-related fields for incremental sync (Issue #148, thread-safe)
func (b *Builder) SetSyncInfo(syncType string, previousUploadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.SyncType = syncType
	b.manifest.PreviousManifestID = previousUploadID
}

// SetDeletedPaths records the paths removed in this incremental version relative
// to its predecessor (Issue #555). Called only when delete tracking is enabled.
func (b *Builder) SetDeletedPaths(paths []string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.DeletedPaths = paths
}

// SetDatasetInfo records the dataset-versioning identity (Issue #521): the
// DatasetID this upload belongs to (the version chain's stable name — the chain
// root's UploadID, or a user slug) and its 1-based VersionOrdinal. It also marks
// the manifest with the "versioning" format feature. A datasetID of "" leaves
// the manifest un-versioned (legacy behavior). Thread-safe.
func (b *Builder) SetDatasetInfo(datasetID string, ordinal int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if datasetID == "" {
		return
	}
	b.manifest.DatasetID = datasetID
	b.manifest.VersionOrdinal = ordinal
	for _, f := range b.manifest.FormatFeatures {
		if f == FormatFeatureVersioning {
			return
		}
	}
	b.manifest.FormatFeatures = append(b.manifest.FormatFeatures, FormatFeatureVersioning)
}

// SetWriterID records the writer/agent identity that produced this upload
// (Issue #520) and marks the manifest with the "writers" format feature. An empty
// writerID leaves the manifest single-writer (legacy behavior). The value is
// advisory attribution; the writers/<id>/ key segment (folded into the S3 prefix,
// see pipeline.WriterPrefix) is authoritative for location. Thread-safe.
func (b *Builder) SetWriterID(writerID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if writerID == "" {
		return
	}
	b.manifest.WriterID = writerID
	for _, f := range b.manifest.FormatFeatures {
		if f == FormatFeatureWriters {
			return
		}
	}
	b.manifest.FormatFeatures = append(b.manifest.FormatFeatures, FormatFeatureWriters)
}

// SetEncryption sets encryption metadata (Issue #163, thread-safe)
func (b *Builder) SetEncryption(kmsKeyID string, manifestEncrypted bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if kmsKeyID == "" && !manifestEncrypted {
		b.manifest.Encryption = nil
		return
	}

	b.manifest.Encryption = &EncryptionMetadata{
		Enabled:           kmsKeyID != "",
		DataKMSKeyID:      kmsKeyID,
		ManifestEncrypted: manifestEncrypted,
	}
}

// SetDeduplication sets deduplication metadata (Issue #108, thread-safe)
func (b *Builder) SetDeduplication(dedup *ManifestDeduplication) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.Deduplication = dedup
}

// SetGitMetadata stores Git repository state in the manifest (Issue #185, thread-safe).
// Pass nil to clear a previously set value.
func (b *Builder) SetGitMetadata(meta *GitMetadata) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.GitMetadata = meta
}

// SetDVCPipeline stores DVC pipeline provenance in the manifest (Issue #185, thread-safe).
// Pass nil to clear a previously set value.
func (b *Builder) SetDVCPipeline(pipeline *DVCPipeline) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.DVCPipeline = pipeline
}

// UpdateShardStats updates statistics for a shard (thread-safe)
func (b *Builder) UpdateShardStats(shardID int, chunkKey string, fileCount int64, uncompressed, compressed int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if shardID < 0 || shardID >= len(b.manifest.Shards) {
		return
	}

	shard := &b.manifest.Shards[shardID]
	shard.ChunkCount++
	shard.FileCount += fileCount
	shard.UncompressedSize += uncompressed
	shard.CompressedSize += compressed
	shard.ChunkKeys = append(shard.ChunkKeys, chunkKey)
}

// UpdateFileS3Keys updates all files in a chunk with the chunk's S3 key and shard ID (thread-safe)
//
// Deprecated: it binds by chunk ID, which misbinds a resumed upload's preloaded
// entries (#904). Use BindChunkFiles.
func (b *Builder) UpdateFileS3Keys(chunkID int, shardID int, s3Key string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i := range b.manifest.Files {
		if b.manifest.Files[i].ChunkID == chunkID {
			b.manifest.Files[i].ShardID = shardID
			b.manifest.Files[i].S3Key = s3Key
		}
	}
}

// SetFileChecksums records per-file content hashes into the matching FileEntry
// records for the given chunk (#271). Keys are FileEntry.Path, or "path#part"
// for split-file parts; a whole-file key sets Checksum on every entry with that
// path in the chunk, while a "path#part" key sets it on the matching part.
// Files not present in the map are left unchanged (e.g. checksums disabled).
func (b *Builder) SetFileChecksums(chunkID int, checksums map[string]string) {
	if len(checksums) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	for i := range b.manifest.Files {
		f := &b.manifest.Files[i]
		if f.ChunkID != chunkID {
			continue
		}
		key := f.Path
		if f.TotalParts > 1 {
			key = fmt.Sprintf("%s#%d", f.Path, f.PartIndex)
		}
		if sum, ok := checksums[key]; ok {
			f.Checksum = sum
		}
	}
}

// SetFileArchiveOffsets records each file's data offset within the chunk's
// uncompressed tar stream into the matching FileEntry records for the given
// chunk (format 2.1, #436). Keys follow the same convention as SetFileChecksums:
// FileEntry.Path, or "path#part" for split-file parts. Files not present in the
// map are left unchanged (e.g. framing disabled, --frame-size 0).
func (b *Builder) SetFileArchiveOffsets(chunkID int, offsets map[string]int64) {
	if len(offsets) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	for i := range b.manifest.Files {
		f := &b.manifest.Files[i]
		if f.ChunkID != chunkID {
			continue
		}
		key := f.Path
		if f.TotalParts > 1 {
			key = fmt.Sprintf("%s#%d", f.Path, f.PartIndex)
		}
		if off, ok := offsets[key]; ok {
			f.ArchiveOffset = off
		}
	}
}

// AddFormatFeature records an optional format-2.1 capability marker (e.g.
// FormatFeatureFrames) on the manifest, de-duplicating (#436). Thread-safe.
func (b *Builder) AddFormatFeature(feature string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, f := range b.manifest.FormatFeatures {
		if f == feature {
			return
		}
	}
	b.manifest.FormatFeatures = append(b.manifest.FormatFeatures, feature)
}

// UpdateFileS3KeyByPath updates the S3Key, ShardID, ChunkID and (when non-empty)
// Checksum for the single FileEntry whose Path matches path. Used by direct
// upload mode where each file has its own S3 key (unlike chunk-based mode where
// all files in a chunk share the same key). checksum is the SHA-256 of the file
// bytes as streamed to S3, so direct mode carries the same per-file integrity as
// packed mode (CSH-SEC-002); pass "" when checksums are disabled.
func (b *Builder) UpdateFileS3KeyByPath(path string, shardID int, s3Key, checksum string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.manifest.Files {
		if b.manifest.Files[i].Path == path {
			b.manifest.Files[i].S3Key = s3Key
			b.manifest.Files[i].ShardID = shardID
			b.manifest.Files[i].ChunkID = 0 // direct mode: no chunking
			if checksum != "" {
				b.manifest.Files[i].Checksum = checksum
			}
			return
		}
	}
}

// PatchDuplicateLocations fills in the real chunk/object location for every
// duplicate FileEntry (#481). A duplicate is recorded at SCAN time with the
// placeholder location the dedup index held then — an empty S3Key and ChunkID
// -1 — because the file's real location isn't known until its content's first
// occurrence has been uploaded. Left unpatched, a duplicate has S3Key "" and is
// unrestorable (restore keys on S3Key; an empty key even aborts the whole
// restore via the glacier pre-flight). Call after uploads complete, before
// Finalize.
//
// The index maps a content hash to the first occurrence's PATH (set at scan
// time, so it's available in both direct and chunked modes); the original's own
// FileEntry has the real S3Key by now (patched by UpdateFileS3Keys /
// UpdateFileS3KeyByPath). So we resolve each duplicate through the original
// entry, which works regardless of upload mode.
func (b *Builder) PatchDuplicateLocations(index *FileDeduplicationIndex) {
	if index == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	origByPath := make(map[string]*FileEntry)
	for i := range b.manifest.Files {
		if f := &b.manifest.Files[i]; !f.IsDuplicate {
			origByPath[f.Path] = f
		}
	}
	for i := range b.manifest.Files {
		f := &b.manifest.Files[i]
		if !f.IsDuplicate {
			continue
		}
		loc := index.FindFile(f.DuplicateOfHash)
		if loc == nil {
			continue
		}
		orig := origByPath[loc.Path]
		if orig == nil || orig.S3Key == "" {
			continue // original not found / not yet located; leave as-is
		}
		f.S3Key = orig.S3Key
		f.ChunkID = orig.ChunkID
		f.ShardID = orig.ShardID
		f.OriginalS3Key = orig.S3Key
		f.OriginalChunkID = orig.ChunkID
		f.OriginalShardID = orig.ShardID
	}
}

// Finalize completes the manifest and returns it (thread-safe)
func (b *Builder) Finalize() *Manifest {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.CompletedAt = time.Now()
	return b.manifest
}

// Build returns the current manifest (without finalizing, thread-safe)
func (b *Builder) Build() *Manifest {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.manifest
}

// ToJSON serializes the manifest to JSON.
//
// The files, chunks, and shards collections are always emitted as JSON arrays,
// never null: the published schema declares them required with type "array", and
// encoding/json renders a nil slice as null. A manifest with an empty section
// (e.g. an upload that produced no chunks) would otherwise be written in a shape
// the format's own schema rejects. Found by FuzzValidateAgainstSchema.
func (m *Manifest) ToJSON() ([]byte, error) {
	out := *m
	if out.Files == nil {
		out.Files = []FileEntry{}
	}
	if out.Chunks == nil {
		out.Chunks = []ChunkEntry{}
	}
	if out.Shards == nil {
		out.Shards = []ShardEntry{}
	}
	return json.MarshalIndent(&out, "", "  ")
}

// ToJSONCompressed serializes the manifest to gzip-compressed JSON
func (m *Manifest) ToJSONCompressed() ([]byte, error) {
	jsonData, err := m.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)

	if _, err := gzipWriter.Write(jsonData); err != nil {
		return nil, fmt.Errorf("failed to compress JSON: %w", err)
	}

	if err := gzipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// LoadManifestFromFile reads a manifest from the local filesystem.
// Files ending in ".gz" are treated as gzip-compressed JSON; all other files
// are read as plain JSON.
func LoadManifestFromFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	if len(path) > 3 && path[len(path)-3:] == ".gz" {
		return FromJSONCompressed(data)
	}
	return FromJSON(data)
}

// FromJSON deserializes a manifest from JSON
func FromJSON(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	// CSH-SEC-003: reject a manifest whose entry counts exceed finite safety
	// limits before any downstream code allocates per-entry.
	if err := validateManifestCounts(len(m.Files), len(m.Chunks)); err != nil {
		return nil, err
	}
	if err := checkMajorVersion(m.Version); err != nil {
		return nil, err
	}
	return &m, nil
}

// supportedMajorVersion is the manifest format major version this build reads.
const supportedMajorVersion = 2

// checkMajorVersion refuses a manifest from a newer major format version
// (#921). Per the reader contract, a major bump is a breaking change, and
// reading one as 2.x would silently misread it. A version with no parseable
// major (absent in early manifests) is accepted as before.
func checkMajorVersion(v string) error {
	digits := strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(digits, '.'); i >= 0 {
		digits = digits[:i]
	}
	major, err := strconv.Atoi(digits)
	if err != nil {
		return nil
	}
	if major > supportedMajorVersion {
		return fmt.Errorf("manifest format version %s is newer than this cargoship reads (%d.x); upgrade cargoship to read it",
			v, supportedMajorVersion)
	}
	return nil
}

// FromJSONCompressed deserializes a manifest from gzip-compressed JSON
func FromJSONCompressed(data []byte) (*Manifest, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() {
		if closeErr := gzipReader.Close(); closeErr != nil {
			// Log but don't fail on close error during read
			_ = closeErr
		}
	}()

	jsonData, err := readAllLimited(gzipReader, MaxManifestJSONBytes, "decompressed manifest")
	if err != nil {
		return nil, fmt.Errorf("failed to decompress JSON: %w", err)
	}

	return FromJSON(jsonData)
}

// parseManifestJSON deserializes manifest bytes that may or may not be
// gzip-compressed, dispatching on the gzip magic number (0x1f 0x8b). The
// encrypted-manifest write path encrypts the COMPRESSED manifest when
// compress=true, so the plaintext recovered after KMS decryption is itself
// gzip — feeding it straight to FromJSON produced "invalid character '\x1f'"
// and made every encrypted manifest unreadable (#474). Auto-detecting here keeps
// the reader correct regardless of whether the inner payload was compressed.
func parseManifestJSON(data []byte) (*Manifest, error) {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		return FromJSONCompressed(data)
	}
	return FromJSON(data)
}

// UploadToS3 uploads the manifest to S3 in plaintext. It refuses a manifest
// marked ManifestEncrypted (--encrypt-manifest): that one must go through
// UploadToS3WithEncryption, or not be written at all (#1001).
func (m *Manifest) UploadToS3(ctx context.Context, s3Client *s3.Client, compress bool) error {
	if m.Encryption != nil && m.Encryption.ManifestEncrypted {
		return fmt.Errorf("refusing to write manifest %s in plaintext: it is marked encrypted (--encrypt-manifest)", m.UploadID)
	}

	var data []byte
	var key string
	var contentType string
	var contentEncoding string
	var err error

	if compress {
		data, err = m.ToJSONCompressed()
		key = fmt.Sprintf("%s/uploads/%s/%s", m.Prefix, m.UploadID, ManifestFileNameGZ)
		contentType = "application/json"
		contentEncoding = "gzip"
	} else {
		data, err = m.ToJSON()
		key = fmt.Sprintf("%s/uploads/%s/%s", m.Prefix, m.UploadID, ManifestFileName)
		contentType = "application/json"
	}

	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}

	// Upload to S3
	input := &s3.PutObjectInput{
		Bucket:      aws.String(m.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}

	if contentEncoding != "" {
		input.ContentEncoding = aws.String(contentEncoding)
	}
	m.applyManifestSSE(input)

	_, err = s3Client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload manifest to S3: %w", err)
	}

	return nil
}

// applyManifestSSE stores a manifest object with SSE-KMS under the upload's
// data key (--kms-key-id) when it has one (#1016). The manifest holds every
// path, size and checksum; without this it sat under the bucket default (often
// SSE-S3) while the data used the customer key. An upload without a data key
// keeps the bucket default.
func (m *Manifest) applyManifestSSE(input *s3.PutObjectInput) {
	if m.Encryption == nil || m.Encryption.DataKMSKeyID == "" {
		return
	}
	input.ServerSideEncryption = types.ServerSideEncryptionAwsKms
	input.SSEKMSKeyId = aws.String(m.Encryption.DataKMSKeyID)
}

// UploadToS3WithEncryption uploads the manifest to S3 with optional KMS encryption (Issue #163)
func (m *Manifest) UploadToS3WithEncryption(ctx context.Context, s3Client *s3.Client, kmsClient encryption.KMSClient, compress bool) error {
	// Check if manifest encryption is enabled
	if m.Encryption == nil || !m.Encryption.ManifestEncrypted {
		// No encryption - use regular upload
		return m.UploadToS3(ctx, s3Client, compress)
	}
	// #1001: an encrypted manifest with no key used to fall through to the
	// plaintext upload. Encrypt or fail.
	if m.Encryption.ManifestKMSKeyID == "" {
		return fmt.Errorf("refusing to write manifest %s in plaintext: it is marked encrypted but has no manifest KMS key", m.UploadID)
	}
	if kmsClient == nil {
		return fmt.Errorf("refusing to write manifest %s in plaintext: it is marked encrypted but no KMS client was given", m.UploadID)
	}

	// Serialize manifest to JSON
	var manifestJSON []byte
	var err error
	if compress {
		manifestJSON, err = m.ToJSONCompressed()
	} else {
		manifestJSON, err = m.ToJSON()
	}
	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}

	// Encrypt the manifest using KMS envelope encryption, bound to this upload's
	// identity so it can't be substituted with another upload's manifest
	// (CSH-SEC-004).
	encryptor := encryption.NewKMSEncryptor(kmsClient, m.Encryption.ManifestKMSKeyID)
	encrypted, err := encryptor.EncryptManifest(ctx, manifestJSON, encryption.ManifestIdentity{UploadID: m.UploadID})
	if err != nil {
		return fmt.Errorf("failed to encrypt manifest: %w", err)
	}

	// Store encryption metadata in manifest
	m.Encryption.Algorithm = encrypted.Algorithm
	m.Encryption.EncryptedDEK = encrypted.EncryptedDEK

	// Serialize the encrypted manifest wrapper
	encryptedJSON, err := json.MarshalIndent(encrypted, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal encrypted manifest: %w", err)
	}

	// Determine S3 key for encrypted manifest
	key := fmt.Sprintf("%s/uploads/%s/manifest.encrypted.json", m.Prefix, m.UploadID)
	if compress {
		key = fmt.Sprintf("%s/uploads/%s/manifest.encrypted.json.gz", m.Prefix, m.UploadID)

		// Compress the encrypted JSON
		var buf bytes.Buffer
		gzipWriter := gzip.NewWriter(&buf)
		if _, err := gzipWriter.Write(encryptedJSON); err != nil {
			return fmt.Errorf("failed to compress encrypted manifest: %w", err)
		}
		if err := gzipWriter.Close(); err != nil {
			return fmt.Errorf("failed to close gzip writer: %w", err)
		}
		encryptedJSON = buf.Bytes()
	}

	// Upload encrypted manifest to S3
	input := &s3.PutObjectInput{
		Bucket:      aws.String(m.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(encryptedJSON),
		ContentType: aws.String("application/json"),
	}

	if compress {
		input.ContentEncoding = aws.String("gzip")
	}
	m.applyManifestSSE(input)

	_, err = s3Client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload encrypted manifest to S3: %w", err)
	}

	return nil
}

// DownloadFromS3 downloads a manifest from S3
//
// #919: a compressed manifest that exists but can't be read (corrupt, a 403,
// throttling) is an error. It used to fall through to the uncompressed name and
// report NoSuchKey, so a damaged manifest looked absent, and the delete chain
// guard skipped it as if the version didn't exist.
func DownloadFromS3(ctx context.Context, s3Client *s3.Client, bucket, prefix, uploadID string) (*Manifest, error) {
	// Try compressed version first
	key := fmt.Sprintf("%s/uploads/%s/%s", prefix, uploadID, ManifestFileNameGZ)
	data, err := downloadS3Object(ctx, s3Client, bucket, key)
	if err == nil {
		manifest, err := FromJSONCompressed(data)
		if err != nil {
			return nil, fmt.Errorf("manifest s3://%s/%s exists but could not be parsed: %w", bucket, key, err)
		}
		return manifest, nil
	}
	if !isNotFound(err) {
		return nil, fmt.Errorf("download manifest s3://%s/%s: %w", bucket, key, err)
	}

	// Fall back to uncompressed version
	key = fmt.Sprintf("%s/uploads/%s/%s", prefix, uploadID, ManifestFileName)
	data, err = downloadS3Object(ctx, s3Client, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("failed to download manifest from S3: %w", err)
	}

	return FromJSON(data)
}

// DownloadFromS3WithDecryption downloads a manifest from S3 with optional KMS decryption (Issue #163)
//
// #912: when an encrypted manifest exists but can't be read (a KMS error, a
// key policy, a corrupt object), that error is returned. It used to be
// swallowed, and the fallback to the plaintext name reported NoSuchKey, so the
// user saw "manifest missing" for an archive that is intact, which invites
// deleting it. The plaintext manifest is tried only when no encrypted one exists.
func DownloadFromS3WithDecryption(ctx context.Context, s3Client *s3.Client, kmsClient encryption.KMSClient, bucket, prefix, uploadID string) (*Manifest, error) {
	// CSH-SEC-004: the identity a bound manifest must decrypt under is derived
	// from the upload we are fetching, not from the ciphertext — so a manifest
	// swapped in from another upload fails to decrypt.
	expected := encryption.ManifestIdentity{UploadID: uploadID}

	for _, name := range []string{"manifest.encrypted.json.gz", "manifest.encrypted.json"} {
		key := fmt.Sprintf("%s/uploads/%s/%s", prefix, uploadID, name)
		data, err := downloadS3Object(ctx, s3Client, bucket, key)
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("download encrypted manifest s3://%s/%s: %w", bucket, key, err)
		}
		m, err := openEncryptedManifest(ctx, kmsClient, data, strings.HasSuffix(name, ".gz"), expected)
		if err != nil {
			return nil, fmt.Errorf("encrypted manifest s3://%s/%s exists but could not be read "+
				"(the archive is not missing; check the KMS key and your access to it): %w", bucket, key, err)
		}
		return m, nil
	}

	// No encrypted manifest: fall back to the regular (non-encrypted) one.
	return DownloadFromS3(ctx, s3Client, bucket, prefix, uploadID)
}

// openEncryptedManifest decodes and decrypts an encrypted manifest object.
func openEncryptedManifest(ctx context.Context, kmsClient encryption.KMSClient, data []byte, gzipped bool, expected encryption.ManifestIdentity) (*Manifest, error) {
	if kmsClient == nil {
		return nil, fmt.Errorf("a KMS client is required to decrypt it")
	}
	if gzipped {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("decompress: %w", err)
		}
		defer func() { _ = zr.Close() }()
		if data, err = readAllLimited(zr, MaxManifestJSONBytes, "decompressed encrypted manifest"); err != nil {
			return nil, err
		}
	}
	var sealed encryption.EncryptedManifest
	if err := json.Unmarshal(data, &sealed); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	plain, err := encryption.DecryptManifestBytes(ctx, kmsClient, &sealed, expected)
	if err != nil {
		return nil, err
	}
	return parseManifestJSON(plain)
}

// DownloadPartialManifestFromS3 downloads a partial manifest from S3 (Issue #157: Resume capability)
func DownloadPartialManifestFromS3(ctx context.Context, s3Client *s3.Client, bucket, prefix, uploadID string) (*Manifest, error) {
	// Try partial manifest (compressed only)
	key := fmt.Sprintf("%s/uploads/%s/manifest.partial.json.gz", prefix, uploadID)
	data, err := downloadS3Object(ctx, s3Client, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("failed to download partial manifest from S3: %w", err)
	}

	return FromJSONCompressed(data)
}

// downloadS3Object downloads an object from S3
func downloadS3Object(ctx context.Context, s3Client *s3.Client, bucket, key string) ([]byte, error) {
	result, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := result.Body.Close(); closeErr != nil {
			// Log but don't fail on close error
			_ = closeErr
		}
	}()

	// CSH-SEC-003: bound the manifest object read; validate S3's reported size
	// up front, then cap the read itself in case ContentLength is unset or lies.
	if err := checkContentLength(result.ContentLength, MaxManifestObjectBytes, "manifest object "+key); err != nil {
		return nil, err
	}
	return readAllLimited(result.Body, MaxManifestObjectBytes, "manifest object "+key)
}

// ParseS3URL parses an S3 URL into bucket and prefix
// Supports formats: s3://bucket/prefix, s3://bucket
func ParseS3URL(s3URL string) (bucket, prefix string, err error) {
	if len(s3URL) < 5 || s3URL[:5] != "s3://" {
		return "", "", fmt.Errorf("invalid S3 URL format: must start with s3://")
	}

	path := s3URL[5:] // Remove "s3://"

	// Find first slash to separate bucket from prefix
	slashIdx := -1
	for i, c := range path {
		if c == '/' {
			slashIdx = i
			break
		}
	}

	if slashIdx == -1 {
		// No prefix, just bucket
		return path, "", nil
	}

	bucket = path[:slashIdx]
	prefix = path[slashIdx+1:]

	return bucket, prefix, nil
}
