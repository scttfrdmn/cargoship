package pipeline

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

// TestFramerInactiveIsNoOp confirms a framer with frameSize 0 (or a nil framer)
// records nothing and never cuts — the single-frame, pre-2.1 layout.
func TestFramerInactiveIsNoOp(t *testing.T) {
	var buf bytes.Buffer
	fr := newTestFramer(t, &buf, 0) // framing disabled
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
	assert.Empty(t, fr.frames, "inactive framer must not record frames")
	assert.Nil(t, job.FileArchiveOffsets(), "inactive framer must not record offsets")

	// A nil framer is safe to call (plain .tar path passes nil).
	var nilFr *framer
	assert.False(t, nilFr.active())
	nilFr.recordOffset(job, chunking.File{Path: "y"})
	require.NoError(t, nilFr.maybeCut())
	nilFr.finalize()
}
