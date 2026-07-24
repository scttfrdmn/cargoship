package manifest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
}

// NewDeepVerifier creates a deep verifier for a manifest.
func NewDeepVerifier(m *Manifest, s3Client S3Downloader) *DeepVerifier {
	return &DeepVerifier{manifest: m, s3Client: s3Client}
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
	key := s3Key

	// Strip a scheme://host/ prefix if S3Key was stored as a full URL.
	if i := strings.Index(key, "://"); i >= 0 {
		rest := key[i+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			key = rest[slash+1:] // drop host, keep path
		} else {
			key = ""
		}
	}

	// Strip a leading "bucket/" if present.
	if bucket != "" {
		key = strings.TrimPrefix(key, bucket+"/")
	}

	// Prepend the manifest Prefix unless it's already there (or empty).
	if prefix != "" && !strings.HasPrefix(key, prefix+"/") {
		key = prefix + "/" + key
	}
	return key
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
		Bucket: aws.String(dv.manifest.Bucket),
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
