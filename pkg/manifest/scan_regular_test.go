package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

// ScanLocalFiles must emit only what the pipeline can actually archive (#693).
//
// The pipeline refuses anything that is not a regular file — the scanner skips symlinks
// and openContainedSourceFile calls ensureRegularFile at use time. When ScanLocalFiles
// emitted those entries anyway they were permanently "New" in the delta: counted as work
// to do, never stored, never present in the resulting manifest, and so New again on the
// next cycle. The delta could not converge and every cycle produced an upload.
func TestScanLocalFilesEmitsOnlyRegularFiles(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("content"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "subdir", "nested.txt"), []byte("nested"), 0o600); err != nil {
		t.Fatalf("write nested: %v", err)
	}
	if err := os.Symlink("real.txt", filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink("nowhere", filepath.Join(root, "broken.txt")); err != nil {
		t.Fatalf("broken symlink: %v", err)
	}
	// A FIFO is the other shape filepath.Walk hands over and the archiver cannot store.
	// Guarded by build tag, not runtime: syscall.Mkfifo does not exist on Windows.
	if err := mkfifoIfSupported(filepath.Join(root, "pipe")); err != nil {
		t.Logf("FIFO case not covered here: %v", err)
	}

	files, err := ScanLocalFiles(root)
	if err != nil {
		t.Fatalf("ScanLocalFiles: %v", err)
	}

	got := map[string]bool{}
	for _, f := range files {
		got[f.Path] = true
		if f.IsDir {
			t.Errorf("a directory was emitted: %q", f.Path)
		}
	}

	for _, want := range []string{"real.txt", filepath.Join("subdir", "nested.txt")} {
		if !got[want] {
			t.Errorf("regular file %q missing from the scan", want)
		}
	}
	// These are the entries that made the delta non-convergent.
	for _, unwanted := range []string{"link.txt", "broken.txt", "subdir", "pipe"} {
		if got[unwanted] {
			t.Errorf("%q was emitted, but the pipeline cannot archive it — it would be "+
				"permanently New in the delta", unwanted)
		}
	}
}
