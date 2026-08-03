package manifest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klauspost/compress/zstd"
)

// ChunkVerifyStatus is the outcome of deep-verifying a single chunk object.
type ChunkVerifyStatus string

const (
	// ChunkVerifyOK means the recomputed hash matched the manifest.
	ChunkVerifyOK ChunkVerifyStatus = "ok"
	// ChunkVerifyMismatch means the stored bytes hashed to a different value —
	// the object was corrupted or replaced.
	ChunkVerifyMismatch ChunkVerifyStatus = "mismatch"
	// ChunkVerifyMissing means the object could not be fetched from S3.
	ChunkVerifyMissing ChunkVerifyStatus = "missing"
	// ChunkVerifyUnverifiable means the manifest recorded no checksum for this
	// chunk, so its integrity cannot be confirmed.
	ChunkVerifyUnverifiable ChunkVerifyStatus = "unverifiable"
)

// ChunkVerifyResult is the per-chunk outcome of a deep verification.
type ChunkVerifyResult struct {
	ChunkID  int               `json:"chunk_id"`
	S3Key    string            `json:"s3_key"`
	Status   ChunkVerifyStatus `json:"status"`
	Expected string            `json:"expected,omitempty"`
	Actual   string            `json:"actual,omitempty"`
	SizeGot  int64             `json:"size_got,omitempty"`
	Err      string            `json:"error,omitempty"`
}

// DeepVerifyResult aggregates the outcome of a data-level verification.
type DeepVerifyResult struct {
	Algorithm    string              `json:"algorithm"`
	TotalChunks  int                 `json:"total_chunks"`
	OK           int                 `json:"ok"`
	Mismatched   int                 `json:"mismatched"`
	Missing      int                 `json:"missing"`
	Unverifiable int                 `json:"unverifiable"`
	Chunks       []ChunkVerifyResult `json:"chunks"`
}

// Passed reports whether the deep verification is a clean pass: every chunk
// verified OK, with no mismatches, missing objects, or unverifiable chunks.
// Unverifiable counts as a failure in deep mode — the whole point is to confirm
// data, and a manifest without checksums can't.
func (r *DeepVerifyResult) Passed() bool {
	return r.Mismatched == 0 && r.Missing == 0 && r.Unverifiable == 0 && r.TotalChunks > 0
}

// DeepVerifier performs data-level integrity verification by re-downloading the
// stored chunk objects and recomputing their checksums against the manifest
// (#271). It is the mechanism behind CargoShip's integrity guarantee: it proves
// the bytes in S3 still match what was recorded at upload time, not merely that
// the manifest is internally consistent.
type DeepVerifier struct {
	manifest *Manifest
	s3Client S3Downloader

	// bucket overrides the bucket objects are fetched from; empty means use
	// manifest.Bucket. See SetBucket (#335).
	bucket string
}

// NewDeepVerifier creates a deep verifier for a manifest.
func NewDeepVerifier(m *Manifest, s3Client S3Downloader) *DeepVerifier {
	return &DeepVerifier{manifest: m, s3Client: s3Client}
}

// SetBucket overrides the bucket chunk objects are fetched from — normally the
// bucket the manifest was just read from. Passing "" keeps the manifest's own
// recorded bucket. Returns the receiver for chaining.
//
// This matters more for verification than for restore: a deep verify against a
// copied archive that silently read the ORIGINAL bucket would report the copy as
// intact while never having touched a byte of it, which is the one thing an
// integrity check must not do (#335).
func (dv *DeepVerifier) SetBucket(bucket string) *DeepVerifier {
	dv.bucket = bucket
	return dv
}

// objectBucket returns the bucket to fetch from: the explicit override when set,
// otherwise the manifest's recorded bucket.
func (dv *DeepVerifier) objectBucket() string {
	if dv.bucket != "" {
		return dv.bucket
	}
	return dv.manifest.Bucket
}

// VerifyChunks re-downloads every chunk object, recomputes its SHA-256, and
// compares it to ChunkEntry.Checksum. It streams each object through the hasher
// (no full-object buffering) so memory stays bounded regardless of chunk size.
// A chunk whose object is missing or whose hash differs is recorded; the walk
// continues so the report covers every chunk rather than stopping at the first
// failure.
func (dv *DeepVerifier) VerifyChunks(ctx context.Context) (*DeepVerifyResult, error) {
	// A manifest with no recorded ChecksumAlgorithm predates checksum capture;
	// every chunk will report unverifiable (handled per-chunk in verifyChunk)
	// rather than silently passing.

	// Verify in a stable chunk-ID order for deterministic reports.
	chunks := make([]ChunkEntry, len(dv.manifest.Chunks))
	copy(chunks, dv.manifest.Chunks)
	sort.Slice(chunks, func(i, j int) bool { return chunks[i].ID < chunks[j].ID })

	result := &DeepVerifyResult{Algorithm: dv.manifest.ChecksumAlgorithm, TotalChunks: len(chunks)}

	for i := range chunks {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		cr := dv.verifyChunk(ctx, &chunks[i])
		switch cr.Status {
		case ChunkVerifyOK:
			result.OK++
		case ChunkVerifyMismatch:
			result.Mismatched++
		case ChunkVerifyMissing:
			result.Missing++
		case ChunkVerifyUnverifiable:
			result.Unverifiable++
		}
		result.Chunks = append(result.Chunks, cr)
	}

	return result, nil
}

// ResolveObjectKey normalizes a ChunkEntry.S3Key into the S3 object key to
// fetch. S3Key can take one of three shapes depending on the upload path:
//   - a full URL (some SDK upload managers return result.Location as a URL that
//     the uploader stores verbatim, e.g. "http://host/bucket/prefix/.../chunk");
//   - a bucket-qualified path ("bucket/prefix/.../chunk");
//   - a key relative to the manifest Prefix (".../chunk"), the uploader default.
//
// All three normalize to the object key within the bucket, prepending prefix
// only when the key doesn't already contain it. Exported so callers that need
// the real object location (e.g. tests, tooling) resolve it identically.
func ResolveObjectKey(prefix, bucket, s3Key string) string {
	// Drop leading slashes. S3 treats "prefix//key" and "prefix/key" as distinct
	// objects, so a key stored with a leading "/" would resolve to an object that
	// doesn't exist. Done first so the scheme check below sees the scheme in
	// leading position. Found by FuzzResolveObjectKey.
	key := strings.TrimLeft(s3Key, "/")

	// Strip a scheme://host/ prefix if S3Key was stored as a full URL. Only a
	// "://" in true scheme position counts: S3 permits ':' in object keys, so
	// matching "://" anywhere would mangle a legitimate key (a file named
	// "weird://name.txt" once resolved to just the bare prefix). Looped, with a
	// re-trim each pass, so a doubly-wrapped URL can't leave a scheme behind.
	// Found by FuzzResolveObjectKey.
	for {
		i := strings.Index(key, "://")
		if i <= 0 || !isURLScheme(key[:i]) {
			break
		}
		rest := key[i+3:]
		slash := strings.Index(rest, "/")
		if slash < 0 {
			key = ""
			break
		}
		key = strings.TrimLeft(rest[slash+1:], "/") // drop host, keep path
	}

	// The prefix is trimmed of surrounding slashes: a manifest written from a
	// user-typed "s3://bucket/archives/" carries a trailing slash, and joining
	// naively would yield "archives//key" — a different object in S3. Found by
	// FuzzResolveObjectKey.
	prefix = strings.Trim(prefix, "/")

	// An already prefix-scoped key is the resolved form; return it untouched.
	// Testing this BEFORE the bucket strip is what makes resolution idempotent:
	// otherwise a resolved key whose leading segment happens to equal the bucket
	// name would have that segment stripped on a second pass, silently dropping a
	// path component. Found by FuzzResolveObjectKey.
	if prefix != "" && (key == prefix || strings.HasPrefix(key, prefix+"/")) {
		return key
	}

	// Strip a leading "bucket/" if present, then re-trim: stripping the bucket
	// can expose another leading slash ("bucket//key" -> "/key").
	if bucket != "" {
		key = strings.TrimLeft(strings.TrimPrefix(key, bucket+"/"), "/")
	}

	// Prepend the manifest Prefix unless it's already there (or empty).
	if prefix != "" && key != prefix && !strings.HasPrefix(key, prefix+"/") {
		key = prefix + "/" + key
	}
	return key
}

// isURLScheme reports whether s is a plausible URI scheme per RFC 3986:
// ALPHA *( ALPHA / DIGIT / "+" / "-" / "." ). Anything else preceding a "://"
// is part of an object key, not a scheme.
func isURLScheme(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			// always allowed
		case i == 0:
			return false // must start with a letter
		case c >= '0' && c <= '9', c == '+', c == '-', c == '.':
			// allowed after the first character
		default:
			return false
		}
	}
	return true
}

// objectKey resolves a chunk key against this manifest's prefix/bucket.
func (dv *DeepVerifier) objectKey(chunkKey string) string {
	return ResolveObjectKey(dv.manifest.Prefix, dv.manifest.Bucket, chunkKey)
}

// verifyChunk fetches and hashes a single chunk object.
func (dv *DeepVerifier) verifyChunk(ctx context.Context, chunk *ChunkEntry) ChunkVerifyResult {
	cr := ChunkVerifyResult{ChunkID: chunk.ID, S3Key: chunk.S3Key, Expected: chunk.Checksum}

	// No recorded checksum (or no algorithm) => can't verify.
	if chunk.Checksum == "" || dv.manifest.ChecksumAlgorithm == "" {
		cr.Status = ChunkVerifyUnverifiable
		return cr
	}

	output, err := dv.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(dv.objectBucket()),
		Key:    aws.String(dv.objectKey(chunk.S3Key)),
	})
	if err != nil {
		cr.Status = ChunkVerifyMissing
		cr.Err = err.Error()
		return cr
	}
	defer func() { _ = output.Body.Close() }()

	hasher := sha256.New()
	n, err := io.Copy(hasher, output.Body)
	if err != nil {
		cr.Status = ChunkVerifyMissing
		cr.Err = fmt.Sprintf("read object: %v", err)
		return cr
	}
	cr.SizeGot = n
	cr.Actual = hex.EncodeToString(hasher.Sum(nil))

	if cr.Actual == chunk.Checksum {
		cr.Status = ChunkVerifyOK
	} else {
		cr.Status = ChunkVerifyMismatch
	}
	return cr
}

// fileKey identifies a file (or split-file part) for checksum lookup.
type fileKey struct {
	path string
	part int
}

// FileVerifyResult is the per-file outcome of file-level deep verification.
type FileVerifyResult struct {
	Path     string            `json:"path"`
	ChunkID  int               `json:"chunk_id"`
	Status   ChunkVerifyStatus `json:"status"` // reuses ok/mismatch/missing/unverifiable
	Expected string            `json:"expected,omitempty"`
	Actual   string            `json:"actual,omitempty"`
}

// FileVerifyResult aggregate.
type FilesVerifyResult struct {
	Algorithm    string             `json:"algorithm"`
	TotalFiles   int                `json:"total_files"`
	OK           int                `json:"ok"`
	Mismatched   int                `json:"mismatched"`
	Missing      int                `json:"missing"`
	Unverifiable int                `json:"unverifiable"`
	Files        []FileVerifyResult `json:"files"`
}

// Passed reports a clean file-level pass: every file hashed and matched.
func (r *FilesVerifyResult) Passed() bool {
	return r.Mismatched == 0 && r.Missing == 0 && r.Unverifiable == 0 && r.TotalFiles > 0
}

// tarNameToFileKey maps a tar entry name back to the key used in the manifest's
// FileEntry checksum lookup. Split-file parts are stored under "path.partN" in
// the tar (see archiver), which correspond to FileEntry{Path, PartIndex}. Whole
// files use the plain path.
func tarNameToFileKey(tarName string) (path string, partIndex int, isPart bool) {
	if i := strings.LastIndex(tarName, ".part"); i >= 0 {
		var p int
		if _, err := fmt.Sscanf(tarName[i+len(".part"):], "%d", &p); err == nil {
			return tarName[:i], p, true
		}
	}
	return tarName, 0, false
}

// VerifyFiles re-downloads each chunk, extracts every file, recomputes its
// content SHA-256, and compares to FileEntry.Checksum (#271). This is the
// end-to-end source->restore integrity check: it proves each restored file's
// bytes match what was recorded at upload, not merely that the chunk object is
// intact. Files whose FileEntry carries no checksum are reported unverifiable.
func (dv *DeepVerifier) VerifyFiles(ctx context.Context) (*FilesVerifyResult, error) {
	result := &FilesVerifyResult{Algorithm: dv.manifest.ChecksumAlgorithm}

	// Index expected checksums by (path, partIndex).
	expected := make(map[fileKey]string)
	for _, f := range dv.manifest.Files {
		if f.IsDuplicate {
			continue // deduplicated files aren't stored in a chunk of their own
		}
		expected[fileKey{f.Path, f.PartIndex}] = f.Checksum
		result.TotalFiles++
	}

	// Group files by chunk so we download each chunk object once.
	byChunk := make(map[int]bool)
	for _, f := range dv.manifest.Files {
		if !f.IsDuplicate {
			byChunk[f.ChunkID] = true
		}
	}
	chunkIDs := make([]int, 0, len(byChunk))
	for id := range byChunk {
		chunkIDs = append(chunkIDs, id)
	}
	sort.Ints(chunkIDs)

	seen := make(map[fileKey]bool)

	for _, chunkID := range chunkIDs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		chunk := dv.chunkByID(chunkID)
		if chunk == nil {
			continue
		}
		fileResults, err := dv.verifyChunkFiles(ctx, chunk, expected, seen)
		if err != nil {
			// Chunk object unreadable: every file in it is missing.
			for _, f := range dv.manifest.Files {
				if f.ChunkID == chunkID && !f.IsDuplicate {
					result.Files = append(result.Files, FileVerifyResult{
						Path: f.Path, ChunkID: chunkID, Status: ChunkVerifyMissing,
					})
					result.Missing++
				}
			}
			continue
		}
		for _, fr := range fileResults {
			switch fr.Status {
			case ChunkVerifyOK:
				result.OK++
			case ChunkVerifyMismatch:
				result.Mismatched++
			case ChunkVerifyUnverifiable:
				result.Unverifiable++
			}
			result.Files = append(result.Files, fr)
		}
	}

	// Any expected file we never saw in its chunk's tar is missing.
	for _, f := range dv.manifest.Files {
		if f.IsDuplicate {
			continue
		}
		k := fileKey{f.Path, f.PartIndex}
		if !seen[k] {
			result.Files = append(result.Files, FileVerifyResult{
				Path: f.Path, ChunkID: f.ChunkID, Status: ChunkVerifyMissing,
			})
			result.Missing++
		}
	}

	return result, nil
}

// chunkByID returns the chunk with the given ID, or nil.
func (dv *DeepVerifier) chunkByID(id int) *ChunkEntry {
	for i := range dv.manifest.Chunks {
		if dv.manifest.Chunks[i].ID == id {
			return &dv.manifest.Chunks[i]
		}
	}
	return nil
}

// verifyChunkFiles downloads one chunk, walks its tar, and hashes each file
// entry, comparing to the expected checksums. It marks seen keys so the caller
// can detect files that never appeared.
func (dv *DeepVerifier) verifyChunkFiles(ctx context.Context, chunk *ChunkEntry, expected map[fileKey]string, seen map[fileKey]bool) ([]FileVerifyResult, error) {
	output, err := dv.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(dv.objectBucket()),
		Key:    aws.String(dv.objectKey(chunk.S3Key)),
	})
	if err != nil {
		return nil, fmt.Errorf("get chunk object: %w", err)
	}
	// Buffer the compressed object before decompressing. The tar/zstd readers
	// need the complete stream; reading directly from the S3 body can surface a
	// premature "unexpected EOF" on some endpoints. Chunk objects are bounded
	// (target sizes are tens of MB), so this stays memory-safe.
	objectBytes, err := io.ReadAll(output.Body)
	_ = output.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read chunk object: %w", err)
	}
	body := bytes.NewReader(objectBytes)

	var decompressed io.Reader
	switch dv.manifest.CompressionType {
	case "zstd", "":
		dec, err := zstd.NewReader(body)
		if err != nil {
			return nil, fmt.Errorf("zstd reader: %w", err)
		}
		defer dec.Close()
		decompressed = dec
	case "gzip", "gz":
		gz, err := gzip.NewReader(body)
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		defer func() { _ = gz.Close() }()
		decompressed = gz
	case "none":
		decompressed = body
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", dv.manifest.CompressionType)
	}

	var results []FileVerifyResult
	tr := tar.NewReader(decompressed)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar read: %w", err)
		}
		if hdr.Name == ".padding" {
			continue // synthetic padding entry, not a real file
		}
		path, part, _ := tarNameToFileKey(hdr.Name)
		k := fileKey{path, part}
		exp, isExpected := expected[k]
		if !isExpected {
			continue // not a manifest file we track (shouldn't happen)
		}
		seen[k] = true

		// Hash exactly the file's declared size via CopyN. The bound guards
		// against a decompression bomb (a malicious manifest/chunk can't stream
		// unbounded data through us) and surfaces a truncated entry as a
		// mismatch. tar.Reader already caps reads at the entry boundary.
		hasher := sha256.New()
		if _, err := io.CopyN(hasher, tr, hdr.Size); err != nil {
			// Short entry (fewer bytes than declared) — record a mismatch and
			// keep going so the report still covers the remaining files.
			results = append(results, FileVerifyResult{
				Path: path, ChunkID: chunk.ID, Expected: exp, Status: ChunkVerifyMismatch,
			})
			continue
		}
		actual := hex.EncodeToString(hasher.Sum(nil))

		fr := FileVerifyResult{Path: path, ChunkID: chunk.ID, Expected: exp, Actual: actual}
		switch {
		case exp == "" || dv.manifest.ChecksumAlgorithm == "":
			fr.Status = ChunkVerifyUnverifiable
		case actual == exp:
			fr.Status = ChunkVerifyOK
		default:
			fr.Status = ChunkVerifyMismatch
		}
		results = append(results, fr)
	}
	return results, nil
}
