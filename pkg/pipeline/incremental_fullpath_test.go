package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestIncrementalScanner_FullPathManifest_SkipsUnchanged is the #624 regression for the
// `upload --incremental` path: the previous manifest stores the full FileEntry.Path, but
// ShouldUpload looks up by the source-relative path. An unchanged file (matching size)
// must be skipped, not re-uploaded.
func TestIncrementalScanner_FullPathManifest_SkipsUnchanged(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	prev := &manifest.Manifest{
		SourcePath: dir,
		Files:      []manifest.FileEntry{{Path: filepath.Join(dir, "a.txt"), Size: int64(len(content))}},
	}
	sc, err := NewIncrementalScanner(prev, "")
	if err != nil {
		t.Fatalf("NewIncrementalScanner: %v", err)
	}
	if sc.ShouldUpload(filepath.Join(dir, "a.txt"), "a.txt") {
		t.Fatal("unchanged file (matching size, full-path manifest entry) must be skipped, not re-uploaded")
	}
}
