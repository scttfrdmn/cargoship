package manifest

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dedupFixture is a chunk holding an original and an unrelated file of the same
// size, and a manifest that also lists a duplicate of the original the way
// --enable-dedup records one: no tar member of its own, the original's S3Key,
// and the content hash, but none of the original's tar name or archive offset.
// The original carries no per-file checksum (the #548 framed-chunk norm), so
// nothing but the content hash links the duplicate to it.
type dedupFixture struct {
	m      *Manifest
	object []byte
	shared []byte
}

const (
	dedupOrig  = "a/orig.bin"
	dedupOther = "c/other.bin"
	dedupDup   = "b/dup.bin"
)

func newDedupFixture(t *testing.T, framed bool) *dedupFixture {
	t.Helper()
	shared := bytes.Repeat([]byte("cargoship-dedup-shared-"), 1000)
	other := bytes.Repeat([]byte("cargoship-dedup-differ-"), 1000)
	files := []framedFile{{dedupOrig, shared}, {dedupOther, other}}

	var object []byte
	var frames []FrameEntry
	offsets := map[string]int64{}
	if framed {
		object, frames, offsets = buildStdlibFramedChunk(t, files, 16*1024)
		require.Greater(t, len(frames), 1)
	} else {
		object = plainTarZst(t, files)
	}
	m := framedManifest("chunk-0.tar.zst", files, frames, offsets)
	m.ChecksumAlgorithm = ChecksumAlgorithmSHA256
	if !framed {
		// A whole-object digest vouches for the checksum-less files (#713).
		objSum := sha256.Sum256(object)
		m.Chunks[0].Checksum = hex.EncodeToString(objSum[:])
	}
	sum := sha256.Sum256(shared)
	hash := hex.EncodeToString(sum[:])
	m.Files = append(m.Files, FileEntry{
		Path: dedupDup, Size: int64(len(shared)), S3Key: "chunk-0.tar.zst",
		IsDuplicate: true, DuplicateOfHash: hash, Checksum: hash, OriginalS3Key: "chunk-0.tar.zst",
	})
	m.TotalFiles = int64(len(m.Files))
	m.TotalChunks = 1
	return &dedupFixture{m: m, object: object, shared: shared}
}

// plainTarZst writes files into a single-frame .tar.zst with no frame index.
func plainTarZst(t *testing.T, files []framedFile) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for _, f := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.content))}))
		_, err := tw.Write(f.content)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	var out bytes.Buffer
	zw, err := zstd.NewWriter(&out)
	require.NoError(t, err)
	_, err = zw.Write(raw.Bytes())
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return out.Bytes()
}

// TestBatchRestore_ChunkedDuplicateRestoresOriginalBytes pins #970. A duplicate
// in a chunked upload has no tar member, so the whole-chunk path never matched
// it and reported it missing, and the frame path sliced at its unset (zero)
// archive offset — under --no-verify, writing the tar header and the wrong
// bytes as a "restored" file.
func TestBatchRestore_ChunkedDuplicateRestoresOriginalBytes(t *testing.T) {
	for _, framed := range []bool{false, true} {
		for _, verify := range []bool{true, false} {
			for _, targets := range [][]string{{dedupDup}, {dedupOrig, dedupDup}} {
				name := fmt.Sprintf("framed=%v/verify=%v/targets=%d", framed, verify, len(targets))
				t.Run(name, func(t *testing.T) {
					fx := newDedupFixture(t, framed)
					dl := &rangeDownloader{object: fx.object, honorRange: true}
					dest := t.TempDir()
					se := NewSelectiveExtractor(fx.m, dl, 0).SetVerify(verify)
					stats, err := se.BatchRestore(context.Background(), targets, dest)
					require.NoError(t, err)
					assert.Zero(t, stats.Failed, "reasons: %v", stats.Errors)
					assert.Equal(t, int64(len(targets)), stats.Restored, "reasons: %v", stats.Errors)
					for _, p := range targets {
						got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(p)))
						require.NoError(t, err, "%s should be restored", p)
						assert.True(t, bytes.Equal(fx.shared, got), "%s must hold the original's bytes", p)
					}
					noTempFiles(t, filepath.Join(dest, "b"))
				})
			}
		}
	}
}

// TestBatchRestore_ChunkedDuplicateWithoutItsContentFails: when no member of
// the chunk holds the duplicate's content, the duplicate fails loudly rather
// than being restored from the wrong bytes.
func TestBatchRestore_ChunkedDuplicateWithoutItsContentFails(t *testing.T) {
	for _, verify := range []bool{true, false} {
		fx := newDedupFixture(t, false)
		dup := &fx.m.Files[len(fx.m.Files)-1]
		dup.DuplicateOfHash = hex.EncodeToString(make([]byte, 32))
		dup.Checksum = dup.DuplicateOfHash
		dl := &rangeDownloader{object: fx.object, honorRange: true}
		dest := t.TempDir()
		stats, err := NewSelectiveExtractor(fx.m, dl, 0).SetVerify(verify).BatchRestore(context.Background(), []string{dedupDup}, dest)
		require.NoError(t, err)
		assert.Equal(t, int64(1), stats.Failed, "verify=%v", verify)
		assert.Zero(t, stats.Restored)
		_, statErr := os.Stat(filepath.Join(dest, "b", "dup.bin"))
		assert.True(t, os.IsNotExist(statErr), "nothing may be written for an unresolvable duplicate")
	}
}

// TestDeepVerify_CoversChunkedDuplicates: verify --deep skipped every
// duplicate, so a duplicate that could not be restored still verified. It is
// now checked through the chunk member holding its content.
func TestDeepVerify_CoversChunkedDuplicates(t *testing.T) {
	for _, framed := range []bool{false, true} {
		t.Run(fmt.Sprintf("framed=%v", framed), func(t *testing.T) {
			fx := newDedupFixture(t, framed)
			dv := NewDeepVerifier(fx.m, &rangeDownloader{object: fx.object})
			res, err := dv.VerifyFiles(context.Background())
			require.NoError(t, err)
			assert.Equal(t, 3, res.TotalFiles, "the duplicate is a file of the dataset")
			dup := findFileResult(res, dedupDup)
			require.NotNil(t, dup, "the duplicate must be reported, not skipped")
			assert.Equal(t, ChunkVerifyOK, dup.Status, dup.Detail)
			assert.True(t, res.Passed())

			// Point the duplicate at content no member holds: it must fail.
			fx.m.Files[len(fx.m.Files)-1].DuplicateOfHash = hex.EncodeToString(make([]byte, 32))
			res, err = NewDeepVerifier(fx.m, &rangeDownloader{object: fx.object}).VerifyFiles(context.Background())
			require.NoError(t, err)
			dup = findFileResult(res, dedupDup)
			require.NotNil(t, dup)
			assert.Equal(t, ChunkVerifyMismatch, dup.Status)
			assert.False(t, res.Passed())
		})
	}
}

func findFileResult(res *FilesVerifyResult, path string) *FileVerifyResult {
	for i := range res.Files {
		if res.Files[i].Path == path {
			return &res.Files[i]
		}
	}
	return nil
}
