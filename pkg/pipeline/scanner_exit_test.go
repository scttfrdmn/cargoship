package pipeline

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// writePrefixedFiles writes n files of size bytes under dir. Each starts with a
// fixed text prefix, so random bytes can never look like a magic number.
func writePrefixedFiles(t *testing.T, dir, name string, n, size int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o750))
	for i := 0; i < n; i++ {
		buf := make([]byte, size)
		_, err := rand.Read(buf)
		require.NoError(t, err)
		copy(buf, fmt.Sprintf("cargoship-test %s %05d\n", name, i))
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s%05d.dat", name, i)), buf, 0o600))
	}
}

// lockedDir creates dir with one file in it and then removes every permission
// from it, so the walk can't read it. The test is skipped where that can't work.
func lockedDir(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("mode 000 does not make a directory unreadable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read a mode-000 directory")
	}
	writePrefixedFiles(t, dir, "hidden", 1, 64)
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) }) //nolint:gosec // restore so TempDir cleanup can remove it
}

// scanOnce runs a bare scanner over root and returns its error. The output is
// drained from drainAfter on; until then the batch workers block on their sends.
func scanOnce(t *testing.T, root string, drainAfter time.Duration) error {
	t.Helper()
	output := make(chan *Job)
	s, err := NewScannerStage(&ScannerConfig{RootPath: root, Workers: 2}, output, nil)
	require.NoError(t, err)
	require.NoError(t, s.Start(context.Background()))

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		if drainAfter > 0 {
			time.Sleep(drainAfter)
		}
		for range output { //nolint:revive // draining
		}
	}()
	select {
	case <-drained:
	case <-time.After(30 * time.Second):
		t.Fatal("the scanner never closed its output")
	}
	require.NoError(t, s.Stop())
	return s.Error()
}

// TestScanner_WalkErrorIsNeverLost pins #1005. The walker sends its error and then
// closes both channels, and run() used to select randomly between the pending
// error and the closed file channel. When it took the closed channel it finished
// as if the walk had succeeded, so the upload exited 0 without the files after the
// failure point.
//
// One file followed by an unreadable directory, on a single P, makes the race
// near-certain on every iteration: the walker hands the file to the waiting
// scanner and runs on to the error and both closes before the scanner is
// scheduled, so the scanner's next select sees both channels ready.
func TestScanner_WalkErrorIsNeverLost(t *testing.T) {
	root := t.TempDir()
	writePrefixedFiles(t, root, "a", 1, 64)
	lockedDir(t, filepath.Join(root, "b-locked"))

	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	const iterations = 300
	lost := 0
	for i := 0; i < iterations; i++ {
		if scanOnce(t, root, 0) == nil {
			lost++
		}
	}
	require.Zero(t, lost, "the walk error was lost in %d of %d scans", lost, iterations)
}

// TestScanner_WalkErrorWithBatchesInFlight pins #1006. On a walk error run()
// returned without waiting for the batch workers, and its deferred close of the
// output raced their sends: "panic: send on closed channel", which crashed the
// upload. Here a full batch is submitted, and its worker is still blocked on its
// first send when the walk reaches the unreadable directory, so before the fix
// the close always panicked.
func TestScanner_WalkErrorWithBatchesInFlight(t *testing.T) {
	root := t.TempDir()
	writePrefixedFiles(t, filepath.Join(root, "a"), "f", 1000, 64) // exactly one batch
	lockedDir(t, filepath.Join(root, "b-locked"))

	for i := 0; i < 3; i++ {
		require.Error(t, scanOnce(t, root, 50*time.Millisecond), "iteration %d: the walk error must be reported", i)
	}
}

// TestScanner_WalkErrorStress runs the audit's reproduction for #1005 and #1006
// many times: a full batch and then some, followed by an unreadable directory,
// with the output drained as it is produced.
func TestScanner_WalkErrorStress(t *testing.T) {
	root := t.TempDir()
	writePrefixedFiles(t, filepath.Join(root, "a"), "f", 1100, 16)
	lockedDir(t, filepath.Join(root, "b-locked"))

	const iterations = 300
	lost := 0
	for i := 0; i < iterations; i++ {
		if scanOnce(t, root, 0) == nil {
			lost++
		}
	}
	require.Zero(t, lost, "the walk error was lost in %d of %d scans", lost, iterations)
}
