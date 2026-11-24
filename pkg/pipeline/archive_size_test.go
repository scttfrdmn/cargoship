package pipeline

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// archiveFiles runs the archiver's per-chunk file writer over files and returns
// the tar it produced, read back entry by entry.
func archiveFiles(t *testing.T, dir string, files []chunking.File) (*Job, map[string][]byte) {
	t.Helper()
	stage, err := NewArchiverStage(&ArchiverConfig{Workers: 1, SourcePath: dir, FileChecksums: true},
		make(chan *Job), make(chan *Job))
	require.NoError(t, err)
	defer func() { _ = stage.Stop() }()

	var buf bytes.Buffer
	cwU := &countingWriter{w: &buf}
	tw := tar.NewWriter(cwU)
	job := &Job{Chunk: chunking.Chunk{Files: files}, Metadata: map[string]string{}}
	var total int64
	require.NoError(t, stage.addFilesWithParallelIO(tw, &framer{tw: tw, cwU: cwU}, job, files, &total))
	require.NoError(t, tw.Close(), "the tar must be complete")

	got := map[string][]byte{}
	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "the tar must read back cleanly")
		data, err := io.ReadAll(tr)
		require.NoError(t, err)
		require.Equal(t, hdr.Size, int64(len(data)))
		got[hdr.Name] = data
	}
	return job, got
}

// TestArchive_FileChangedSinceScanIsRecordedAsArchived pins #910 for both
// archiver paths (fewer than 3 files streams each file; 3+ reads them in
// parallel). A file whose size changed after the scan must be archived whole as
// it is now, and the manifest-bound entry must carry the archived size, not the
// scan's. Before, the streaming path wrote the scan size into the header (a
// truncated tar for a shrunk file, a silent prefix for a grown one), and the
// parallel path kept the scan size in the manifest, so frame restore padded a
// shrunk file with the next file's bytes.
func TestArchive_FileChangedSinceScanIsRecordedAsArchived(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files int
	}{{"streaming path", 2}, {"parallel path", 4}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			scanTime := time.Unix(1_700_000_000, 0)
			var files []chunking.File
			contents := map[string][]byte{}
			for i := 0; i < tc.files; i++ {
				p := filepath.Join(dir, string(rune('a'+i))+".dat")
				data := bytes.Repeat([]byte{byte('a' + i)}, 1000)
				require.NoError(t, os.WriteFile(p, data, 0o600))
				contents[p] = data
				files = append(files, chunking.File{Path: p, Size: 1000, ModTime: scanTime})
			}
			// After the scan: the first file shrinks, the second grows.
			shrunk, grown := files[0].Path, files[1].Path
			contents[shrunk] = []byte("now short")
			contents[grown] = bytes.Repeat([]byte("G"), 1500)
			require.NoError(t, os.WriteFile(shrunk, contents[shrunk], 0o600))
			require.NoError(t, os.WriteFile(grown, contents[grown], 0o600))

			job, got := archiveFiles(t, dir, files)
			for p, want := range contents {
				assert.Equal(t, want, got[p], "%s must be archived whole, as it is now", filepath.Base(p))
			}

			bound := map[string]int64{}
			for _, f := range job.boundFiles() {
				bound[f.Path] = f.Size
			}
			assert.Equal(t, int64(len(contents[shrunk])), bound[shrunk], "the manifest must record the archived size")
			assert.Equal(t, int64(len(contents[grown])), bound[grown])
			if tc.files > 2 {
				assert.Equal(t, int64(1000), bound[files[2].Path], "an unchanged file keeps its size")
			}
		})
	}
}

func TestNoteArchivedSize(t *testing.T) {
	mod := time.Unix(1_700_000_000, 0)
	f := chunking.File{Path: "p", Size: 10, ModTime: mod}
	job := &Job{Chunk: chunking.Chunk{Files: []chunking.File{f}}}

	noteArchivedSize(job, f, 10, mod) // unchanged: nothing recorded
	assert.Equal(t, int64(10), job.boundFiles()[0].Size)
	assert.Empty(t, job.archived)

	later := mod.Add(time.Minute)
	noteArchivedSize(job, f, 12, later)
	b := job.boundFiles()[0]
	assert.Equal(t, int64(12), b.Size)
	assert.True(t, b.ModTime.Equal(later), "the archived mtime is recorded, so the next sync re-checks the file")

	noteArchivedSize(nil, f, 1, mod) // no job: no-op
	part := chunking.File{Path: "q", Size: 10, TotalParts: 2}
	noteArchivedSize(job, part, 3, mod) // split parts are never re-sized
	assert.NotContains(t, job.archived, "q#0")
}
