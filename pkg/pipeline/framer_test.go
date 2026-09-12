package pipeline

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// newTestFramer wires a framer over a real zstd encoder the same way the archiver
// goroutine does: tw → cwU → encoder → cwC → buf.
func newTestFramer(t *testing.T, buf *bytes.Buffer, frameSize int64) *framer {
	t.Helper()
	cwC := &countingWriter{w: buf, h: sha256.New()}
	enc, err := zstd.NewWriter(cwC)
	require.NoError(t, err)
	cwU := &countingWriter{w: enc}
	tw := tar.NewWriter(cwU)
	return &framer{tw: tw, encoder: enc, cwU: cwU, cwC: cwC, frameSize: frameSize}
}

// TestFramerCutsFramesAndRecordsOffsets exercises the real framer: writing files
// through it with a small frame size cuts multiple frames, records each file's
// data offset, and produces a valid concatenated-frame zstd stream whose per-
// frame slices decode to the exact file bytes (#436).
func TestFramerCutsFramesAndRecordsOffsets(t *testing.T) {
	files := []struct {
		path    string
		content []byte
	}{
		{"a.txt", bytes.Repeat([]byte("a"), 40000)},
		{"b.txt", bytes.Repeat([]byte("b"), 40000)},
		{"c.txt", bytes.Repeat([]byte("c"), 40000)},
	}

	var buf bytes.Buffer
	fr := newTestFramer(t, &buf, 32*1024) // small: forces cuts between files
	job := &Job{}

	for _, f := range files {
		require.NoError(t, fr.tw.WriteHeader(&tar.Header{Name: f.path, Mode: 0644, Size: int64(len(f.content))}))
		fr.recordOffset(job, chunking.File{Path: f.path})
		_, err := fr.tw.Write(f.content)
		require.NoError(t, err)
		require.NoError(t, fr.maybeCut())
	}
	require.NoError(t, fr.tw.Close())
	require.NoError(t, fr.encoder.Close())
	fr.finalize()

	require.Greater(t, len(fr.frames), 1, "small frame size should cut multiple frames")

	// Offsets were recorded for every file.
	offsets := job.FileArchiveOffsets()
	require.Len(t, offsets, len(files))

	// Frames tile the compressed stream contiguously and cover the whole object,
	// and each carries a correct per-frame content checksum (#439).
	var next int64
	obj := buf.Bytes()
	for i, fe := range fr.frames {
		assert.Equal(t, next, fe.CompressedOffset, "frame %d must start where the previous ended", i)
		want := sha256.Sum256(obj[fe.CompressedOffset : fe.CompressedOffset+fe.CompressedSize])
		assert.Equal(t, hex.EncodeToString(want[:]), fe.Checksum, "frame %d checksum must match its compressed bytes", i)
		next += fe.CompressedSize
	}
	assert.Equal(t, int64(buf.Len()), next, "frames must cover the whole object")

	// Each file's covering frame decodes to its exact bytes via offset slicing.
	dec, err := zstd.NewReader(nil)
	require.NoError(t, err)
	defer dec.Close()
	for _, f := range files {
		off := offsets[f.path]
		var covering *manifest.FrameEntry
		for i := range fr.frames {
			fe := &fr.frames[i]
			if off >= fe.UncompressedOffset && off+int64(len(f.content)) <= fe.UncompressedOffset+fe.UncompressedSize {
				covering = fe
				break
			}
		}
		require.NotNil(t, covering, "a frame must cover %s", f.path)
		raw, err := dec.DecodeAll(buf.Bytes()[covering.CompressedOffset:covering.CompressedOffset+covering.CompressedSize], nil)
		require.NoError(t, err)
		start := off - covering.UncompressedOffset
		assert.Equal(t, f.content, raw[start:start+int64(len(f.content))], "sliced frame bytes must match %s", f.path)
	}
}

// TestFramerSubFramesLargeFile is the #502 guard: a single file larger than
// frameSize must be cut into many independently-decodable frames (not one
// unseekable frame), the stream must round-trip byte-identically, and a byte
// range spanning a frame boundary must be reconstructable from only its covering
// frames — the random-access promise for large files.
func TestFramerSubFramesLargeFile(t *testing.T) {
	const frameSize = int64(64 * 1024) // 64 KiB frames
	var buf bytes.Buffer
	fr := newTestFramer(t, &buf, frameSize)
	job := &Job{}
	s := &ArchiverStage{}

	// One compressible file far larger than frameSize.
	content := bytes.Repeat([]byte("cargoship-502-"), 80000) // ~1.12 MB
	require.NoError(t, fr.tw.WriteHeader(&tar.Header{Name: "big.dat", Mode: 0o644, Size: int64(len(content))}))
	fr.recordOffset(job, chunking.File{Path: "big.dat"})
	require.NoError(t, s.copyFileFramed(fr.tw, fr, bytes.NewReader(content)))
	require.NoError(t, fr.maybeCut())
	require.NoError(t, fr.tw.Close())
	require.NoError(t, fr.encoder.Close())
	fr.finalize()

	// The large file spans many frames (pre-#502 it was a single frame).
	require.Greater(t, len(fr.frames), 5, "a large file must be cut into many sub-frames (#502)")

	obj := buf.Bytes()
	var next int64
	for i, fe := range fr.frames {
		assert.Equal(t, next, fe.CompressedOffset, "frame %d must start where the previous ended", i)
		// No frame's uncompressed span materially exceeds frameSize (one block slack).
		assert.LessOrEqual(t, fe.UncompressedSize, frameSize+frameSize, "frame %d span ~frameSize", i)
		want := sha256.Sum256(obj[fe.CompressedOffset : fe.CompressedOffset+fe.CompressedSize])
		assert.Equal(t, hex.EncodeToString(want[:]), fe.Checksum, "frame %d checksum", i)
		next += fe.CompressedSize
	}
	assert.Equal(t, int64(buf.Len()), next, "frames must cover the whole object")

	// Whole-stream round-trip is byte-identical.
	dec, err := zstd.NewReader(nil)
	require.NoError(t, err)
	defer dec.Close()
	raw, err := dec.DecodeAll(obj, nil)
	require.NoError(t, err)
	tr := tar.NewReader(bytes.NewReader(raw))
	hdr, err := tr.Next()
	require.NoError(t, err)
	require.Equal(t, "big.dat", hdr.Name)
	got, err := io.ReadAll(tr)
	require.NoError(t, err)
	require.Equal(t, content, got, "round-trip must be byte-identical")

	// Random access: reconstruct a range that spans multiple frames from ONLY its
	// covering frames, the way a frame-index reader (lith) would.
	off := job.FileArchiveOffsets()["big.dat"]
	start, n := frameSize, frameSize+5000 // deliberately spans >1 frame boundary
	require.LessOrEqual(t, start+n, int64(len(content)))
	absStart := off + start

	var covering []*manifest.FrameEntry
	for i := range fr.frames {
		fe := &fr.frames[i]
		if fe.UncompressedOffset+fe.UncompressedSize > absStart && fe.UncompressedOffset < absStart+n {
			covering = append(covering, fe)
		}
	}
	require.Greater(t, len(covering), 1, "a range of frameSize+ must span multiple frames")
	var region []byte
	for _, fe := range covering {
		d, derr := dec.DecodeAll(obj[fe.CompressedOffset:fe.CompressedOffset+fe.CompressedSize], nil)
		require.NoError(t, derr)
		region = append(region, d...)
	}
	rel := absStart - covering[0].UncompressedOffset
	assert.Equal(t, content[start:start+n], region[rel:rel+n],
		"a byte range must be recoverable from only its covering frames")
}

// TestFramerInactiveIsNoOp confirms an inactive framer (frameSize 0) never CUTS
// frames — the single-frame, pre-2.1 layout — but STILL records each file's
// archive offset as long as it has an uncompressed counter (#492: plain/unframed
// chunks need offsets too). A nil framer records nothing and is safe to call.
func TestFramerInactiveIsNoOp(t *testing.T) {
	var buf bytes.Buffer
	fr := newTestFramer(t, &buf, 0) // framing disabled (has cwU, no frame cutting)
	job := &Job{}

	require.NoError(t, fr.tw.WriteHeader(&tar.Header{Name: "x", Mode: 0644, Size: 3}))
	fr.recordOffset(job, chunking.File{Path: "x"})
	_, err := fr.tw.Write([]byte("xyz"))
	require.NoError(t, err)
	require.NoError(t, fr.maybeCut())
	require.NoError(t, fr.tw.Close())
	require.NoError(t, fr.encoder.Close())
	fr.finalize()

	assert.False(t, fr.active())
	assert.Empty(t, fr.frames, "inactive framer must not cut frames")
	// #492: the offset is recorded (512 = right after the tar header) even though
	// no frames were cut, so a plain-chunk reader can range-GET the file.
	assert.Equal(t, map[string]int64{"x": 512}, job.FileArchiveOffsets(),
		"an inactive framer with a counter still records archive offsets (#492)")

	// A nil framer records nothing and is safe to call (defensive).
	njob := &Job{}
	var nilFr *framer
	assert.False(t, nilFr.active())
	nilFr.recordOffset(njob, chunking.File{Path: "y"})
	require.NoError(t, nilFr.maybeCut())
	nilFr.finalize()
	assert.Nil(t, njob.FileArchiveOffsets(), "a nil framer records no offsets")
}
