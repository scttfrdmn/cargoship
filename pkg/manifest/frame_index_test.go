package manifest

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// framedFile is one file to write into a test chunk.
type framedFile struct {
	name    string
	content []byte
}

// offsetCounter counts bytes written through it — the test's stand-in for the
// archiver's countingWriter, used to derive frame and file offsets with stdlib
// only, proving the 2.1 layout is reconstructible without CargoShip code.
type offsetCounter struct {
	w io.Writer
	n int64
}

func (c *offsetCounter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// buildStdlibFramedChunk writes entries into a multi-frame .tar.zst chunk using
// only stdlib tar + klauspost zstd, cutting a new frame at a file boundary once
// frameSize uncompressed bytes accumulate — exactly what pkg/pipeline's framer
// does (#436). It returns the chunk bytes, the frame index, and each file's data
// offset in the uncompressed tar stream.
func buildStdlibFramedChunk(t *testing.T, entries []framedFile, frameSize int64) ([]byte, []FrameEntry, map[string]int64) {
	t.Helper()
	var out bytes.Buffer
	cwC := &offsetCounter{w: &out}
	enc, err := zstd.NewWriter(cwC)
	require.NoError(t, err)
	cwU := &offsetCounter{w: enc}
	tw := tar.NewWriter(cwU)

	var frames []FrameEntry
	offsets := make(map[string]int64)
	var frameStartU, frameStartC int64

	appendFrame := func() {
		frames = append(frames, FrameEntry{
			CompressedOffset:   frameStartC,
			CompressedSize:     cwC.n - frameStartC,
			UncompressedOffset: frameStartU,
			UncompressedSize:   cwU.n - frameStartU,
		})
	}

	for _, f := range entries {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0644, Size: int64(len(f.content))}))
		offsets[f.name] = cwU.n // data starts right after the header
		_, err := tw.Write(f.content)
		require.NoError(t, err)
		if frameSize > 0 && cwU.n-frameStartU >= frameSize {
			require.NoError(t, tw.Flush())
			require.NoError(t, enc.Close())
			appendFrame()
			frameStartC, frameStartU = cwC.n, cwU.n
			enc.Reset(cwC)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, enc.Close())
	appendFrame() // trailing frame (last file(s) + tar trailer)
	return out.Bytes(), frames, offsets
}

// rangeDownloader serves a single object and (optionally) honors Range like S3:
// a 206 with Content-Range for a byte range, a 200 with the whole object when
// honor is false. It records how many bytes the last call served so a test can
// prove random access fetched only one frame, not the whole chunk.
type rangeDownloader struct {
	object     []byte
	honorRange bool
	lastServed int
	calls      int
}

func (d *rangeDownloader) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	d.calls++
	if in.Range == nil || !d.honorRange {
		d.lastServed = len(d.object)
		return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(d.object))}, nil
	}
	var start, end int64
	if _, err := fmt.Sscanf(*in.Range, "bytes=%d-%d", &start, &end); err != nil {
		return nil, fmt.Errorf("bad range %q: %w", *in.Range, err)
	}
	if start < 0 || end >= int64(len(d.object)) || start > end {
		return nil, fmt.Errorf("range %q out of bounds for %d-byte object", *in.Range, len(d.object))
	}
	slice := d.object[start : end+1]
	d.lastServed = len(slice)
	cr := fmt.Sprintf("bytes %d-%d/%d", start, end, len(d.object))
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(slice)), ContentRange: &cr}, nil
}

// framedManifest builds a 2.1 manifest describing a single framed chunk.
func framedManifest(key string, files []framedFile, frames []FrameEntry, offsets map[string]int64) *Manifest {
	m := &Manifest{
		Version:         ManifestVersion,
		UploadID:        "20260909-frame",
		Bucket:          "test-bucket",
		Prefix:          "backups",
		Region:          "us-east-1",
		CompressionType: "zstd",
		FormatFeatures:  []string{FormatFeatureFrames},
		Chunks: []ChunkEntry{{
			ID: 0, ShardID: 0, S3Key: key, FileCount: len(files),
			CreatedAt: time.Now(), UploadedAt: time.Now(), Frames: frames,
		}},
	}
	for _, f := range files {
		m.Chunks[0].FilePaths = append(m.Chunks[0].FilePaths, f.name)
		m.Files = append(m.Files, FileEntry{
			Path: f.name, Size: int64(len(f.content)), ModTime: time.Now(),
			ChunkID: 0, ShardID: 0, S3Key: key, ArchiveOffset: offsets[f.name],
		})
	}
	return m
}

// TestFrameIndexRandomAccess proves the 2.1 frame index lets a spec-only reader
// fetch and decode just one file's frame — file → frame → ranged slice →
// single-frame decode → offset slice — using only the manifest's new fields.
func TestFrameIndexRandomAccess(t *testing.T) {
	files := []framedFile{
		{"data/a.txt", bytes.Repeat([]byte("A"), 20000)},
		{"data/b.txt", bytes.Repeat([]byte("B"), 20000)},
		{"data/c.txt", bytes.Repeat([]byte("C"), 20000)},
	}
	chunkBytes, frames, offsets := buildStdlibFramedChunk(t, files, 16*1024)
	require.Greater(t, len(frames), 1, "small frame size should produce multiple frames")

	// Spec-only reader: locate file c's frame, decode ONLY that frame, slice.
	want := files[2]
	fr := findFrame(frames, offsets[want.name], int64(len(want.content)))
	require.NotNil(t, fr, "a frame must cover file c")
	require.Less(t, int(fr.CompressedSize), len(chunkBytes), "one frame must be smaller than the whole chunk")

	frameBytes := chunkBytes[fr.CompressedOffset : fr.CompressedOffset+fr.CompressedSize]
	raw, err := decodeZstdFrame(frameBytes)
	require.NoError(t, err)
	sliceStart := offsets[want.name] - fr.UncompressedOffset
	got := raw[sliceStart : sliceStart+int64(len(want.content))]
	assert.Equal(t, want.content, got, "ranged frame decode must yield the exact file bytes")
}

// TestSelectiveExtractorFrameRestore proves BatchRestore takes the random-access
// path when frames are present: it fetches only the file's frame (a byte range),
// not the whole chunk, and writes the exact bytes.
func TestSelectiveExtractorFrameRestore(t *testing.T) {
	files := []framedFile{
		{"data/a.txt", bytes.Repeat([]byte("a"), 20000)},
		{"data/b.txt", bytes.Repeat([]byte("b"), 20000)},
		{"data/c.txt", bytes.Repeat([]byte("c"), 20000)},
	}
	key := "backups/uploads/20260909-frame/shard-0/chunk-0.tar.zst"
	chunkBytes, frames, offsets := buildStdlibFramedChunk(t, files, 16*1024)
	m := framedManifest(key, files, frames, offsets)

	dl := &rangeDownloader{object: chunkBytes, honorRange: true}
	se := NewSelectiveExtractor(m, dl, 0).SetBucket("test-bucket")

	dest := t.TempDir()
	stats, err := se.BatchRestore(context.Background(), []string{"data/c.txt"}, dest)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Restored)
	assert.Equal(t, int64(0), stats.Failed)

	got, err := os.ReadFile(filepath.Join(dest, "data", "c.txt"))
	require.NoError(t, err)
	assert.Equal(t, files[2].content, got, "restored bytes must be byte-identical")

	// The win: only the covering frame was fetched, not the whole chunk.
	cFrame := findFrame(frames, offsets["data/c.txt"], int64(len(files[2].content)))
	require.NotNil(t, cFrame)
	assert.Equal(t, int(cFrame.CompressedSize), dl.lastServed, "should fetch exactly the frame's bytes")
	assert.Less(t, dl.lastServed, len(chunkBytes), "must not download the whole chunk")
}

// TestDeepVerifyFrameIndex proves verify --deep validates the 2.1 frame index:
// a well-formed chunk passes both chunk- and file-level checks, a broken frame
// offset fails the chunk check, and a wrong archive_offset fails the file check.
func TestDeepVerifyFrameIndex(t *testing.T) {
	files := []framedFile{
		{"d/1", bytes.Repeat([]byte("1"), 20000)},
		{"d/2", bytes.Repeat([]byte("2"), 20000)},
		{"d/3", bytes.Repeat([]byte("3"), 20000)},
	}
	key := "backups/uploads/20260909-frame/shard-0/chunk-0.tar.zst"
	chunkBytes, frames, offsets := buildStdlibFramedChunk(t, files, 16*1024)
	require.Greater(t, len(frames), 1)

	m := framedManifest(key, files, frames, offsets)
	m.ChecksumAlgorithm = ChecksumAlgorithmSHA256
	m.Chunks[0].Checksum = sha256hexIndep(chunkBytes)
	for i := range m.Files {
		m.Files[i].Checksum = sha256hexIndep(files[i].content)
	}

	dl := &rangeDownloader{object: chunkBytes, honorRange: true}

	// Well-formed: both levels pass.
	dv := NewDeepVerifier(m, dl).SetBucket("test-bucket")
	chunkRes, err := dv.VerifyChunks(context.Background())
	require.NoError(t, err)
	assert.True(t, chunkRes.Passed(), "well-formed frame index should pass chunk verify: %+v", chunkRes.Chunks)

	fileRes, err := dv.VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.True(t, fileRes.Passed(), "matching archive_offsets should pass file verify")

	// Broken frame tiling: fails the chunk check.
	broken := framedManifest(key, files, frames, offsets)
	broken.ChecksumAlgorithm = ChecksumAlgorithmSHA256
	broken.Chunks[0].Checksum = sha256hexIndep(chunkBytes)
	broken.Chunks[0].Frames[0].CompressedOffset = 7 // no longer starts at 0
	dvBroken := NewDeepVerifier(broken, dl).SetBucket("test-bucket")
	res, err := dvBroken.VerifyChunks(context.Background())
	require.NoError(t, err)
	assert.False(t, res.Passed(), "a frame index that doesn't tile the object must fail")

	// Wrong archive_offset: fails the file check.
	badOff := framedManifest(key, files, frames, offsets)
	badOff.ChecksumAlgorithm = ChecksumAlgorithmSHA256
	badOff.Chunks[0].Checksum = sha256hexIndep(chunkBytes)
	for i := range badOff.Files {
		badOff.Files[i].Checksum = sha256hexIndep(files[i].content)
	}
	badOff.Files[1].ArchiveOffset += 512 // points past the real data start
	dvBadOff := NewDeepVerifier(badOff, dl).SetBucket("test-bucket")
	fres, err := dvBadOff.VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.False(t, fres.Passed(), "a wrong archive_offset must fail file verify")
	assert.Positive(t, fres.Mismatched)
}

// TestSelectiveExtractorFrameRestoreRangeIgnored proves the reader still restores
// correctly against a backend that ignores Range (returns the whole object with
// no Content-Range): it slices at the absolute archive offset instead.
func TestSelectiveExtractorFrameRestoreRangeIgnored(t *testing.T) {
	files := []framedFile{
		{"x/one", bytes.Repeat([]byte("1"), 20000)},
		{"x/two", bytes.Repeat([]byte("2"), 20000)},
		{"x/three", bytes.Repeat([]byte("3"), 20000)},
	}
	key := "backups/uploads/20260909-frame/shard-0/chunk-0.tar.zst"
	chunkBytes, frames, offsets := buildStdlibFramedChunk(t, files, 16*1024)
	m := framedManifest(key, files, frames, offsets)

	dl := &rangeDownloader{object: chunkBytes, honorRange: false} // ignores Range
	se := NewSelectiveExtractor(m, dl, 0).SetBucket("test-bucket")

	dest := t.TempDir()
	stats, err := se.BatchRestore(context.Background(), []string{"x/two"}, dest)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Restored)

	got, err := os.ReadFile(filepath.Join(dest, "x", "two"))
	require.NoError(t, err)
	assert.Equal(t, files[1].content, got, "must restore correctly even when Range is ignored")
}
