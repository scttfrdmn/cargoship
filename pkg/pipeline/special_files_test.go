package pipeline

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// withDeadline runs fn and fails the test if it hasn't returned within d. A
// call stuck opening a FIFO can't be cancelled, so it is left blocked (the
// FIFO's cleanup releases it) and the failure is reported here.
func withDeadline(t *testing.T, d time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s hung for %v: it is blocked on the FIFO", what, d)
	}
}

// TestScanner_SkipsNonRegularFiles pins the scanner half of #1010. The pipeline
// scanner emitted FIFOs, sockets and device nodes, which only the delta scan
// filtered (#693). Opening a FIFO blocks until a writer appears, so a chunk with
// one in it hung the upload, and a socket failed it. They are now skipped and
// counted, as the delta skips them.
func TestScanner_SkipsNonRegularFiles(t *testing.T) {
	root := t.TempDir()
	writePrefixedFiles(t, root, "f", 2, 64)
	makeFIFO(t, filepath.Join(root, "pipe"))

	output := make(chan *Job, 10)
	s, err := NewScannerStage(&ScannerConfig{RootPath: root, Workers: 2}, output, nil)
	require.NoError(t, err)
	require.NoError(t, s.Start(context.Background()))
	var got []string
	for job := range output {
		for _, f := range job.Chunk.Files {
			got = append(got, filepath.Base(f.Path))
		}
	}
	require.NoError(t, s.Stop())
	require.NoError(t, s.Error())
	assert.ElementsMatch(t, []string{"f00000.dat", "f00001.dat"}, got, "the FIFO must not be chunked")
	assert.Equal(t, int64(1), s.SkippedNonRegular())
}

// TestPipeline_FIFOInSourceDoesNotHang is the audit's reproduction for #1010:
// three files and a FIFO. The chunk went down the archiver's parallel-I/O path,
// whose os.ReadFile blocked opening the FIFO, and Run never returned.
func TestPipeline_FIFOInSourceDoesNotHang(t *testing.T) {
	root := t.TempDir()
	writePrefixedFiles(t, root, "f", 3, 1024)
	makeFIFO(t, filepath.Join(root, "pipe"))

	p, err := NewPipeline(&PipelineConfig{
		ScannerWorkers: 2, ArchiverWorkers: 2, UploaderWorkers: 2,
		S3Bucket: "test-bucket", SourcePath: root,
	})
	require.NoError(t, err)
	var result *Result
	var runErr error
	withDeadline(t, 15*time.Second, "Run", func() { result, runErr = p.Run(context.Background(), root) })
	require.NoError(t, runErr)
	assert.True(t, result.Success, "errors: %v", result.Errors)
	assert.Equal(t, int64(3), result.TotalFiles)
}

// TestOpenContainedSourceFile_FIFODoesNotBlock covers a FIFO swapped in after
// the scan: the open is refused, rather than blocking until a writer appears.
func TestOpenContainedSourceFile_FIFODoesNotBlock(t *testing.T) {
	for _, sourceDir := range []string{"root", ""} {
		t.Run("sourceDir="+sourceDir, func(t *testing.T) {
			root := t.TempDir()
			fifo := filepath.Join(root, "pipe")
			makeFIFO(t, fifo)
			dir := root
			if sourceDir == "" {
				dir = ""
			}
			var err error
			withDeadline(t, 10*time.Second, "openContainedSourceFile", func() {
				var f *os.File
				if f, err = openContainedSourceFile(dir, fifo); err == nil {
					_ = f.Close()
				}
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not a regular file")
		})
	}
}

// TestArchiver_ParallelPathReadsThroughTheSourceRoot pins the containment half
// of #1010. With three or more files a chunk is read by the parallel-I/O path,
// which used os.Stat and os.ReadFile and so skipped openContainedSourceFile: a
// file swapped to a symlink after the scan was followed, and its target, here a
// secret outside the source tree, was archived (CSH-SEC-001).
func TestArchiver_ParallelPathReadsThroughTheSourceRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privilege on Windows")
	}
	root := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.key")
	require.NoError(t, os.WriteFile(secret, []byte("cargoship-test PRIVATE KEY"), 0o600))
	writePrefixedFiles(t, root, "f", 3, 64)

	var files []chunking.File
	for i := 0; i < 3; i++ {
		path := filepath.Join(root, fmt.Sprintf("f%05d.dat", i))
		info, err := os.Stat(path)
		require.NoError(t, err)
		files = append(files, chunking.File{Path: path, Size: info.Size(), ModTime: info.ModTime()})
	}
	// After the scan, the last file becomes a symlink to the secret.
	require.NoError(t, os.Remove(files[2].Path))
	require.NoError(t, os.Symlink(secret, files[2].Path))

	stage, err := NewArchiverStage(&ArchiverConfig{Workers: 1, SourcePath: root}, make(chan *Job), make(chan *Job))
	require.NoError(t, err)
	defer func() { _ = stage.Stop() }()
	var buf bytes.Buffer
	cwU := &countingWriter{w: &buf}
	tw := tar.NewWriter(cwU)
	job := &Job{Chunk: chunking.Chunk{Files: files}, Metadata: map[string]string{}}
	var total int64
	err = stage.addFilesWithParallelIO(tw, &framer{tw: tw, cwU: cwU}, job, files, &total)
	require.Error(t, err, "the swapped-in symlink must be refused")
	assert.Contains(t, err.Error(), "symlink")
	assert.NotContains(t, buf.String(), "PRIVATE KEY", "the secret must not reach the archive")
}
