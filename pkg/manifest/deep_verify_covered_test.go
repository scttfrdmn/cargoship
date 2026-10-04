package manifest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every dataset written since #548 looks like this: the chunk carries a
// whole-object checksum, and FileEntry.Checksum is empty for every file. Before
// #713 that made VerifyFiles report all of them Unverifiable and fail, so
// `verify --deep` failed on every healthy backup — observed on a real 410-file
// NAS deployment as "0 OK, 410 unverifiable (of 410 files)".
//
// The chunk digest proves those bytes, so this must PASS, counted as covered
// rather than silently folded into OK.
func TestVerifyFiles_NoPerFileChecksum_CoveredByChunkDigest(t *testing.T) {
	fileA := []byte("contents of file a")
	fileB := []byte("contents of file b, longer")
	chunk := makeTarZst(t, map[string][]byte{"a.txt": fileA, "b.txt": fileB})

	dl := &fakeDownloader{objects: map[string][]byte{"pfx/chunk-0": chunk}}
	m := &Manifest{
		Version: ManifestVersion, Bucket: "bkt", Prefix: "pfx",
		CompressionType: "zstd", ChecksumAlgorithm: ChecksumAlgorithmSHA256,
		Chunks: []ChunkEntry{{ID: 0, S3Key: "chunk-0", Checksum: sha256hex(chunk)}},
		Files: []FileEntry{
			{Path: "a.txt", ChunkID: 0, S3Key: "chunk-0"}, // no Checksum — the #548 shape
			{Path: "b.txt", ChunkID: 0, S3Key: "chunk-0"},
		},
	}

	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.True(t, res.Passed(), "a dataset with no per-file checksums but an intact chunk must pass: %+v", res)
	assert.Equal(t, 2, res.CoveredByChunk, "both files are proven via the chunk digest")
	assert.Equal(t, 0, res.Unverifiable)
	assert.Equal(t, 0, res.OK, "no file carried its own checksum, so none is OK in the per-file sense")

	// The distinction has to stay visible: this is NOT per-file verification.
	assert.False(t, res.FullyVerifiedPerFile(),
		"coverage via a chunk digest must not be reported as per-file verification")
}

// Coverage must not paper over corruption. If the chunk's bytes do not match the
// digest the manifest recorded, nothing vouches for the files inside it.
func TestVerifyFiles_NoPerFileChecksum_CorruptChunkIsNotCovered(t *testing.T) {
	chunk := makeTarZst(t, map[string][]byte{"a.txt": []byte("the stored bytes")})

	dl := &fakeDownloader{objects: map[string][]byte{"pfx/chunk-0": chunk}}
	m := &Manifest{
		Version: ManifestVersion, Bucket: "bkt", Prefix: "pfx",
		CompressionType: "zstd", ChecksumAlgorithm: ChecksumAlgorithmSHA256,
		// A digest that is not this object's.
		Chunks: []ChunkEntry{{ID: 0, S3Key: "chunk-0", Checksum: sha256hex([]byte("something else entirely"))}},
		Files:  []FileEntry{{Path: "a.txt", ChunkID: 0, S3Key: "chunk-0"}},
	}

	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.False(t, res.Passed(), "a chunk whose bytes contradict its recorded digest must not cover its files")
	assert.Equal(t, 0, res.CoveredByChunk)
	assert.Equal(t, 1, res.Unverifiable)
}

// With neither a per-file checksum nor any chunk digest, nothing proves the
// bytes. This is the case Unverifiable is actually for, and it must still fail —
// otherwise #713's fix would turn a real gap into a false pass.
func TestVerifyFiles_NoChecksumsAnywhere_StaysUnverifiable(t *testing.T) {
	chunk := makeTarZst(t, map[string][]byte{"a.txt": []byte("unvouched-for bytes")})

	dl := &fakeDownloader{objects: map[string][]byte{"pfx/chunk-0": chunk}}
	m := &Manifest{
		Version: ManifestVersion, Bucket: "bkt", Prefix: "pfx",
		CompressionType: "zstd", ChecksumAlgorithm: ChecksumAlgorithmSHA256,
		Chunks: []ChunkEntry{{ID: 0, S3Key: "chunk-0"}}, // no Checksum, no Frames
		Files:  []FileEntry{{Path: "a.txt", ChunkID: 0, S3Key: "chunk-0"}},
	}

	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.False(t, res.Passed())
	assert.Equal(t, 1, res.Unverifiable)
	assert.Equal(t, 0, res.CoveredByChunk)
}

// CSH-SEC-005 on the covered path: an algorithm we cannot recompute must not be
// able to launder itself into a pass via chunk coverage. Without this, setting
// checksum_algorithm to something unrecognised would make every file "covered".
func TestVerifyFiles_UnknownAlgorithm_IsNotCovered(t *testing.T) {
	chunk := makeTarZst(t, map[string][]byte{"a.txt": []byte("contents")})

	dl := &fakeDownloader{objects: map[string][]byte{"pfx/chunk-0": chunk}}
	m := &Manifest{
		Version: ManifestVersion, Bucket: "bkt", Prefix: "pfx",
		CompressionType: "zstd", ChecksumAlgorithm: "crc32-but-claimed",
		Chunks: []ChunkEntry{{ID: 0, S3Key: "chunk-0", Checksum: sha256hex(chunk)}},
		Files:  []FileEntry{{Path: "a.txt", ChunkID: 0, S3Key: "chunk-0"}},
	}

	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.False(t, res.Passed(), "an unrecomputable algorithm must not yield chunk coverage")
	assert.Equal(t, 1, res.Unverifiable)
	assert.Equal(t, 0, res.CoveredByChunk)
}

// A file WITH its own checksum keeps being verified per-file even when the chunk
// would have covered it, so the stronger claim is not downgraded.
func TestVerifyFiles_PerFileChecksumStillTakesPrecedence(t *testing.T) {
	fileA := []byte("checked per file")
	fileB := []byte("covered by the chunk only")
	chunk := makeTarZst(t, map[string][]byte{"a.txt": fileA, "b.txt": fileB})

	dl := &fakeDownloader{objects: map[string][]byte{"pfx/chunk-0": chunk}}
	m := &Manifest{
		Version: ManifestVersion, Bucket: "bkt", Prefix: "pfx",
		CompressionType: "zstd", ChecksumAlgorithm: ChecksumAlgorithmSHA256,
		Chunks: []ChunkEntry{{ID: 0, S3Key: "chunk-0", Checksum: sha256hex(chunk)}},
		Files: []FileEntry{
			{Path: "a.txt", ChunkID: 0, S3Key: "chunk-0", Checksum: sha256hex(fileA)},
			{Path: "b.txt", ChunkID: 0, S3Key: "chunk-0"},
		},
	}

	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.True(t, res.Passed(), "%+v", res)
	assert.Equal(t, 1, res.OK, "the file with its own checksum is verified per-file")
	assert.Equal(t, 1, res.CoveredByChunk)
	assert.False(t, res.FullyVerifiedPerFile(), "one file still relies on chunk coverage")
}

// A per-file checksum that MISMATCHES must fail even though the chunk digest
// matches. Reaching the covered path first would mask real corruption of a file
// the manifest does make a per-file claim about.
func TestVerifyFiles_PerFileMismatchNotMaskedByChunkCoverage(t *testing.T) {
	stored := []byte("what is actually stored")
	chunk := makeTarZst(t, map[string][]byte{"a.txt": stored, "b.txt": []byte("other")})

	dl := &fakeDownloader{objects: map[string][]byte{"pfx/chunk-0": chunk}}
	m := &Manifest{
		Version: ManifestVersion, Bucket: "bkt", Prefix: "pfx",
		CompressionType: "zstd", ChecksumAlgorithm: ChecksumAlgorithmSHA256,
		// The chunk digest is correct for the object as stored...
		Chunks: []ChunkEntry{{ID: 0, S3Key: "chunk-0", Checksum: sha256hex(chunk)}},
		Files: []FileEntry{
			// ...but the manifest claims a different hash for a.txt.
			{Path: "a.txt", ChunkID: 0, S3Key: "chunk-0", Checksum: sha256hex([]byte("a different file"))},
			{Path: "b.txt", ChunkID: 0, S3Key: "chunk-0"},
		},
	}

	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.False(t, res.Passed(), "a per-file checksum mismatch must fail regardless of chunk coverage")
	assert.Equal(t, 1, res.Mismatched)
}

// chunkBytesMatchManifest is the gate the covered status hangs on, so the
// frame-index path gets a direct test: a framed chunk records no whole-object
// checksum (#522), and coverage must require that EVERY frame carries one.
func TestChunkBytesMatchManifest_FramedChunk(t *testing.T) {
	// Two single-frame zstd streams concatenated: each is a valid frame, so the
	// object tiles as two frames. makeTarZst emits one frame, so reuse it twice.
	f1 := makeTarZst(t, map[string][]byte{"a.txt": []byte("frame one payload")})
	f2 := makeTarZst(t, map[string][]byte{"b.txt": []byte("frame two payload")})
	object := append(append([]byte{}, f1...), f2...)

	// UncompressedSize must be positive: validateFrameTiling rejects a
	// non-positive size, so a frame index missing it is not a valid index.
	frames := []FrameEntry{
		{CompressedOffset: 0, CompressedSize: int64(len(f1)),
			UncompressedSize: 2048, Checksum: sha256hex(f1)},
		{CompressedOffset: int64(len(f1)), CompressedSize: int64(len(f2)),
			UncompressedSize: 2048, Checksum: sha256hex(f2)},
	}

	base := &Manifest{ChecksumAlgorithm: ChecksumAlgorithmSHA256}

	t.Run("all frames checksummed and intact is covered", func(t *testing.T) {
		dv := NewDeepVerifier(base, &fakeDownloader{})
		assert.True(t, dv.chunkBytesMatchManifest(&ChunkEntry{Frames: frames}, object))
	})

	t.Run("a frame with no checksum leaves its bytes unproven", func(t *testing.T) {
		partial := []FrameEntry{frames[0], {CompressedOffset: frames[1].CompressedOffset,
			CompressedSize: frames[1].CompressedSize, UncompressedSize: frames[1].UncompressedSize}}
		dv := NewDeepVerifier(base, &fakeDownloader{})
		assert.False(t, dv.chunkBytesMatchManifest(&ChunkEntry{Frames: partial}, object),
			"a checksum-less frame means part of the object is unverified, so files in it are not covered")
	})

	t.Run("tampered frame bytes are not covered", func(t *testing.T) {
		bad := append([]byte{}, object...)
		bad[len(bad)-1] ^= 0xFF
		dv := NewDeepVerifier(base, &fakeDownloader{})
		assert.False(t, dv.chunkBytesMatchManifest(&ChunkEntry{Frames: frames}, bad))
	})
}
