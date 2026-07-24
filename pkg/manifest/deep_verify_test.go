package manifest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDownloader serves object bytes from an in-memory map and can simulate
// missing objects. It implements manifest.S3Downloader.
type fakeDownloader struct {
	objects map[string][]byte
}

func (f *fakeDownloader) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	key := *in.Key
	data, ok := f.objects[key]
	if !ok {
		return nil, fmt.Errorf("NoSuchKey: %s", key)
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// manifestWith builds a minimal manifest with the given chunks and algorithm.
func manifestWith(algo string, chunks []ChunkEntry) *Manifest {
	return &Manifest{
		Version:           ManifestVersion,
		Bucket:            "test-bucket",
		ChecksumAlgorithm: algo,
		Chunks:            chunks,
	}
}

// TestDeepVerify_AllOK verifies a clean pass when every object matches.
func TestDeepVerify_AllOK(t *testing.T) {
	c0 := []byte("chunk-zero-contents")
	c1 := []byte("chunk-one-different-contents")
	dl := &fakeDownloader{objects: map[string][]byte{
		"uploads/u/shard-0/chunk-0.tar.zst": c0,
		"uploads/u/shard-0/chunk-1.tar.zst": c1,
	}}
	m := manifestWith(ChecksumAlgorithmSHA256, []ChunkEntry{
		{ID: 0, S3Key: "uploads/u/shard-0/chunk-0.tar.zst", Checksum: sha256hex(c0)},
		{ID: 1, S3Key: "uploads/u/shard-0/chunk-1.tar.zst", Checksum: sha256hex(c1)},
	})

	res, err := NewDeepVerifier(m, dl).VerifyChunks(context.Background())
	require.NoError(t, err)
	assert.True(t, res.Passed())
	assert.Equal(t, 2, res.OK)
	assert.Equal(t, 0, res.Mismatched)
	assert.Equal(t, int64(len(c0)), res.Chunks[0].SizeGot)
}

// TestDeepVerify_DetectsCorruption is the trust-defining test: a stored object
// whose bytes were altered must be flagged as a mismatch and fail the verify.
func TestDeepVerify_DetectsCorruption(t *testing.T) {
	original := []byte("the original archive bytes")
	corrupted := []byte("the CORRUPTED archive bytes")
	dl := &fakeDownloader{objects: map[string][]byte{
		"uploads/u/shard-0/chunk-0.tar.zst": corrupted, // stored bytes differ
	}}
	m := manifestWith(ChecksumAlgorithmSHA256, []ChunkEntry{
		{ID: 0, S3Key: "uploads/u/shard-0/chunk-0.tar.zst", Checksum: sha256hex(original)},
	})

	res, err := NewDeepVerifier(m, dl).VerifyChunks(context.Background())
	require.NoError(t, err)
	assert.False(t, res.Passed(), "corruption must fail verification")
	assert.Equal(t, 1, res.Mismatched)
	assert.Equal(t, ChunkVerifyMismatch, res.Chunks[0].Status)
	assert.Equal(t, sha256hex(original), res.Chunks[0].Expected)
	assert.Equal(t, sha256hex(corrupted), res.Chunks[0].Actual)
}

// TestDeepVerify_MissingObject flags an object that isn't in S3.
func TestDeepVerify_MissingObject(t *testing.T) {
	dl := &fakeDownloader{objects: map[string][]byte{}} // nothing stored
	m := manifestWith(ChecksumAlgorithmSHA256, []ChunkEntry{
		{ID: 0, S3Key: "uploads/u/shard-0/chunk-0.tar.zst", Checksum: sha256hex([]byte("x"))},
	})

	res, err := NewDeepVerifier(m, dl).VerifyChunks(context.Background())
	require.NoError(t, err)
	assert.False(t, res.Passed())
	assert.Equal(t, 1, res.Missing)
	assert.Equal(t, ChunkVerifyMissing, res.Chunks[0].Status)
	assert.Contains(t, res.Chunks[0].Err, "NoSuchKey")
}

// TestDeepVerify_UnverifiableNoChecksum fails when a chunk has no recorded
// checksum even though the algorithm is set.
func TestDeepVerify_UnverifiableNoChecksum(t *testing.T) {
	c0 := []byte("data")
	dl := &fakeDownloader{objects: map[string][]byte{"k0": c0}}
	m := manifestWith(ChecksumAlgorithmSHA256, []ChunkEntry{
		{ID: 0, S3Key: "k0", Checksum: ""}, // no checksum recorded
	})

	res, err := NewDeepVerifier(m, dl).VerifyChunks(context.Background())
	require.NoError(t, err)
	assert.False(t, res.Passed(), "unverifiable must fail in deep mode")
	assert.Equal(t, 1, res.Unverifiable)
	assert.Equal(t, ChunkVerifyUnverifiable, res.Chunks[0].Status)
}

// TestDeepVerify_PreCaptureManifest fails cleanly when the manifest predates
// checksum capture (no algorithm) — every chunk is unverifiable, not a false pass.
func TestDeepVerify_PreCaptureManifest(t *testing.T) {
	dl := &fakeDownloader{objects: map[string][]byte{"k0": []byte("x")}}
	m := manifestWith("", []ChunkEntry{
		{ID: 0, S3Key: "k0", Checksum: "deadbeef"}, // checksum present but no algorithm
	})

	res, err := NewDeepVerifier(m, dl).VerifyChunks(context.Background())
	require.NoError(t, err)
	assert.False(t, res.Passed())
	assert.Equal(t, 1, res.Unverifiable)
}

// TestDeepVerify_MixedContinuesPastFailure verifies the walk reports every chunk
// rather than stopping at the first failure.
func TestDeepVerify_MixedContinuesPastFailure(t *testing.T) {
	good := []byte("good")
	dl := &fakeDownloader{objects: map[string][]byte{
		"k0": good,
		"k1": []byte("corrupted"),
		// k2 missing
	}}
	m := manifestWith(ChecksumAlgorithmSHA256, []ChunkEntry{
		{ID: 0, S3Key: "k0", Checksum: sha256hex(good)},
		{ID: 1, S3Key: "k1", Checksum: sha256hex([]byte("expected"))},
		{ID: 2, S3Key: "k2", Checksum: sha256hex([]byte("z"))},
	})

	res, err := NewDeepVerifier(m, dl).VerifyChunks(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, res.TotalChunks)
	assert.Equal(t, 1, res.OK)
	assert.Equal(t, 1, res.Mismatched)
	assert.Equal(t, 1, res.Missing)
	assert.Len(t, res.Chunks, 3, "reports all chunks")
}

// TestResolveObjectKey covers the three S3Key shapes deep verify must handle.
func TestResolveObjectKey(t *testing.T) {
	tests := []struct {
		name           string
		prefix, bucket string
		s3Key          string
		want           string
	}{
		{"relative key with prefix", "myprefix", "b", "uploads/u/shard-0/chunk-0.tar.zst", "myprefix/uploads/u/shard-0/chunk-0.tar.zst"},
		{"relative key no prefix", "", "b", "uploads/u/shard-0/chunk-0.tar.zst", "uploads/u/shard-0/chunk-0.tar.zst"},
		{"already prefixed", "myprefix", "b", "myprefix/uploads/u/chunk-0", "myprefix/uploads/u/chunk-0"},
		{"bucket-qualified", "myprefix", "b", "b/myprefix/uploads/u/chunk-0", "myprefix/uploads/u/chunk-0"},
		{"full URL", "myprefix", "cargoship-test", "http://127.0.0.1:9000/cargoship-test/myprefix/uploads/u/chunk-0", "myprefix/uploads/u/chunk-0"},
		{"https URL no prefix", "", "b", "https://s3.amazonaws.com/b/uploads/u/chunk-0", "uploads/u/chunk-0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveObjectKey(tt.prefix, tt.bucket, tt.s3Key))
		})
	}
}

// TestDeepVerify_ResolvesURLKeys verifies the verifier fetches the right object
// when S3Key was stored as a full URL (as some upload managers return).
func TestDeepVerify_ResolvesURLKeys(t *testing.T) {
	c0 := []byte("archive-contents")
	dl := &fakeDownloader{objects: map[string][]byte{
		"pfx/uploads/u/shard-0/chunk-0.tar.zst": c0, // stored under the resolved key
	}}
	m := &Manifest{
		Version: ManifestVersion, Bucket: "bkt", Prefix: "pfx",
		ChecksumAlgorithm: ChecksumAlgorithmSHA256,
		Chunks: []ChunkEntry{{
			ID:       0,
			S3Key:    "http://host:9000/bkt/pfx/uploads/u/shard-0/chunk-0.tar.zst",
			Checksum: sha256hex(c0),
		}},
	}
	res, err := NewDeepVerifier(m, dl).VerifyChunks(context.Background())
	require.NoError(t, err)
	assert.True(t, res.Passed(), "should resolve the URL key and verify OK")
}

// TestDeepVerify_EmptyManifestNotPass ensures a manifest with zero chunks is not
// reported as a pass (nothing was actually verified).
func TestDeepVerify_EmptyManifestNotPass(t *testing.T) {
	m := manifestWith(ChecksumAlgorithmSHA256, nil)
	res, err := NewDeepVerifier(m, &fakeDownloader{}).VerifyChunks(context.Background())
	require.NoError(t, err)
	assert.False(t, res.Passed())
	assert.Equal(t, 0, res.TotalChunks)
}
