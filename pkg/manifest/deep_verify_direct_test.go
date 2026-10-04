package manifest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// directManifest models the direct-upload ("fast path") layout: total_chunks=0,
// no chunk entries at all, and each file stored as its OWN S3 object named by its
// FileEntry.S3Key. This is what `sync`/`upload` produce for a corpus of small
// files, and it is the shape VerifyFiles could not resolve (#724).
func directManifest(files []FileEntry) *Manifest {
	return &Manifest{
		Version: ManifestVersion, Bucket: "bkt", Prefix: "pfx",
		CompressionType: "none", ChecksumAlgorithm: ChecksumAlgorithmSHA256,
		Chunks: nil, // the defining property: nothing to resolve a file through
		Files:  files,
	}
}

// The regression: every file reported MISSING on data that is present and
// restorable. Observed on real S3 as "0 OK, 0 corrupted, 26 missing, 0
// unverifiable (of 26 files)" immediately after a restore verified all 26 files
// byte-identical. "Missing" announces data loss, so this is the most dangerous
// wrong answer deep verify can give.
func TestVerifyFiles_DirectMode_PresentFilesAreNotReportedMissing(t *testing.T) {
	a := []byte("contents of standalone object a")
	b := []byte("contents of standalone object b, longer")

	dl := &fakeDownloader{objects: map[string][]byte{
		"pfx/obj-a": a,
		"pfx/obj-b": b,
	}}
	m := directManifest([]FileEntry{
		{Path: "a.txt", S3Key: "obj-a", Size: int64(len(a)), Checksum: sha256hex(a)},
		{Path: "b.txt", S3Key: "obj-b", Size: int64(len(b)), Checksum: sha256hex(b)},
	})

	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, res.Missing, "present objects must never be reported missing: %+v", res)
	assert.Equal(t, 2, res.OK, "a direct-mode file with a recorded checksum verifies per-file")
	assert.True(t, res.Passed(), "%+v", res)
	assert.True(t, res.FullyVerifiedPerFile(),
		"direct-mode files carry their own checksums, so this is genuine per-file verification")
}

// Post-#725 datasets record a per-file checksum, so corruption of a standalone
// object must be caught — and attributed to the file, not to a chunk.
func TestVerifyFiles_DirectMode_DetectsCorruptedObject(t *testing.T) {
	good := []byte("the bytes the manifest describes")
	stored := []byte("the bytes actually in the bucket")

	dl := &fakeDownloader{objects: map[string][]byte{"pfx/obj-a": stored}}
	m := directManifest([]FileEntry{
		{Path: "a.txt", S3Key: "obj-a", Size: int64(len(stored)), Checksum: sha256hex(good)},
	})

	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.False(t, res.Passed())
	assert.Equal(t, 1, res.Mismatched)
	assert.Equal(t, 0, res.Missing, "corrupt is not the same as absent")
	assert.Contains(t, res.Files[0].Detail, "per-file checksum",
		"the report must say why it failed")
}

// A genuinely absent object must still be reported Missing — the fix must not
// make real data loss invisible, which would be the opposite failure.
func TestVerifyFiles_DirectMode_AbsentObjectIsStillMissing(t *testing.T) {
	dl := &fakeDownloader{objects: map[string][]byte{}} // nothing stored
	m := directManifest([]FileEntry{
		{Path: "a.txt", S3Key: "obj-a", Size: 10, Checksum: sha256hex([]byte("whatever"))},
	})

	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Missing)
	assert.False(t, res.Passed())
	assert.Contains(t, res.Files[0].Detail, "could not be fetched")
}

// Pre-#725 direct datasets recorded NO per-file checksum. Nothing vouches for
// those bytes, so the honest answer is Unverifiable — a FAIL, not a pass. Unlike
// the chunked case (#713) there is no chunk digest to fall back on. But the
// report must make clear the object is PRESENT, or it reads as data loss.
func TestVerifyFiles_DirectMode_NoChecksumIsUnverifiableNotMissing(t *testing.T) {
	a := []byte("stored but unvouched-for")

	dl := &fakeDownloader{objects: map[string][]byte{"pfx/obj-a": a}}
	m := directManifest([]FileEntry{
		{Path: "a.txt", S3Key: "obj-a", Size: int64(len(a))}, // no Checksum: the pre-#725 shape
	})

	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Unverifiable)
	assert.Equal(t, 0, res.Missing, "present-but-unverifiable must not be reported as missing")
	assert.False(t, res.Passed(), "nothing recorded proves these bytes, so this is not a pass")
	assert.Contains(t, res.Files[0].Detail, "present",
		"the operator must be told the data IS there")
	assert.Contains(t, res.Files[0].Detail, "#725")
}

// A size contradiction is detectable even with no checksum recorded: the stored
// object disagrees with the manifest. That is real evidence of a problem and must
// not be folded into "unverifiable".
func TestVerifyFiles_DirectMode_SizeMismatchIsReportedWithoutAChecksum(t *testing.T) {
	dl := &fakeDownloader{objects: map[string][]byte{"pfx/obj-a": []byte("only 14 bytes")}}
	m := directManifest([]FileEntry{
		{Path: "a.txt", S3Key: "obj-a", Size: 9999}, // manifest claims far more
	})

	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Mismatched)
	assert.Equal(t, 0, res.Unverifiable)
	assert.Contains(t, res.Files[0].Detail, "manifest records 9999")
}

// An unrecomputable algorithm must not become a pass via this new path either
// (CSH-SEC-005), mirroring the guard on the chunked path.
func TestVerifyFiles_DirectMode_UnknownAlgorithmIsUnverifiable(t *testing.T) {
	a := []byte("contents")
	dl := &fakeDownloader{objects: map[string][]byte{"pfx/obj-a": a}}
	m := directManifest([]FileEntry{
		{Path: "a.txt", S3Key: "obj-a", Size: int64(len(a)), Checksum: sha256hex(a)},
	})
	m.ChecksumAlgorithm = "not-a-real-algorithm"

	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Unverifiable)
	assert.False(t, res.Passed())
}

// A manifest can hold both layouts at once (a mixed upload, or a chain merged
// across versions where one version took the fast path). Each file must be
// resolved by how IT is stored, not by the manifest's overall shape.
func TestVerifyFiles_MixedChunkedAndStandaloneFiles(t *testing.T) {
	packed := []byte("inside the tar")
	loose := []byte("its own object")
	chunk := makeTarZst(t, map[string][]byte{"packed.txt": packed})

	dl := &fakeDownloader{objects: map[string][]byte{
		"pfx/chunk-0": chunk,
		"pfx/obj-l":   loose,
	}}
	m := &Manifest{
		Version: ManifestVersion, Bucket: "bkt", Prefix: "pfx",
		CompressionType: "zstd", ChecksumAlgorithm: ChecksumAlgorithmSHA256,
		Chunks: []ChunkEntry{{ID: 0, S3Key: "chunk-0", Checksum: sha256hex(chunk)}},
		Files: []FileEntry{
			{Path: "packed.txt", ChunkID: 0, S3Key: "chunk-0", Size: int64(len(packed)), Checksum: sha256hex(packed)},
			{Path: "loose.txt", S3Key: "obj-l", Size: int64(len(loose)), Checksum: sha256hex(loose)},
		},
	}

	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.True(t, res.Passed(), "%+v", res)
	assert.Equal(t, 2, res.OK, "both the packed and the standalone file verify per-file")
	assert.Equal(t, 0, res.Missing)
}
