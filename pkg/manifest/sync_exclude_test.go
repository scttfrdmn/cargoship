package manifest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
}

// #716: the delta must not report files the uploader will refuse to store. An
// excluded file is never stored, so it is never in the previous manifest, so it is
// New again next cycle — HasChanges() stays true forever, the no-changes path is
// never reached, and every cycle writes a new empty manifest version. Observed on a
// real deployment as 12,622 excluded files producing one empty dataset version
// every 6 hours.
func TestScanLocalFilesExcluding_OmitsExcluded(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"keep.txt":                "a",
		"sub/keep2.txt":           "b",
		"#recycle/old.txt":        "c",
		"#recycle/deep/older.txt": "d",
		".aws/credentials":        "secret",
		".ssh/id_rsa":             "secret",
		"photos/@eaDir/thumb.jpg": "e",
		"notes/.DS_Store":         "f",
	})

	got, err := ScanLocalFilesExcluding(root, []string{"#recycle", ".aws", ".ssh", "@eaDir", ".DS_Store"})
	require.NoError(t, err)

	var paths []string
	for _, f := range got {
		paths = append(paths, filepath.ToSlash(f.Path))
	}
	assert.ElementsMatch(t, []string{"keep.txt", "sub/keep2.txt"}, paths,
		"only non-excluded regular files may be reported")
}

// The property that actually matters: with exclusions applied, a second cycle over
// an unchanged tree reports NO changes. Before the fix this returned the excluded
// files as New every time.
func TestComputeDelta_ConvergesWithExclusions(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"keep.txt":         "a",
		"#recycle/old.txt": "c",
		".aws/credentials": "secret",
	})
	excludes := []string{"#recycle", ".aws"}

	// Cycle 1: full, uploads only the kept file.
	local, err := ScanLocalFilesExcluding(root, excludes)
	require.NoError(t, err)
	d1, err := ComputeDelta(local, nil, &SyncOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(d1.New), "only keep.txt is new: %+v", d1.New)

	// The manifest the uploader would then write: exactly what it stored.
	stored := &Manifest{SourcePath: root, Files: []FileEntry{
		{Path: filepath.Join(root, "keep.txt"), Size: 1, ModTime: local[0].ModTime},
	}}

	// Cycle 2: nothing changed on disk.
	local2, err := ScanLocalFilesExcluding(root, excludes)
	require.NoError(t, err)
	d2, err := ComputeDelta(local2, stored, &SyncOptions{})
	require.NoError(t, err)

	assert.False(t, d2.HasChanges(),
		"an unchanged tree must produce no changes, or every cycle writes an empty manifest version (#716); got New=%+v Modified=%+v",
		d2.New, d2.Modified)
	assert.Equal(t, 0, len(d2.New))
}

// Without exclusions the delta cannot converge — this is the pre-fix behaviour,
// pinned so the asymmetry is visible if the wiring is ever removed again.
func TestComputeDelta_WithoutExclusionsCannotConverge(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"keep.txt":         "a",
		"#recycle/old.txt": "c",
	})

	local, err := ScanLocalFiles(root) // no exclusions: sees everything
	require.NoError(t, err)
	stored := &Manifest{SourcePath: root, Files: []FileEntry{
		{Path: filepath.Join(root, "keep.txt"), Size: 1, ModTime: fileModTime(t, local, "keep.txt")},
	}}

	d, err := ComputeDelta(local, stored, &SyncOptions{})
	require.NoError(t, err)
	assert.True(t, d.HasChanges(),
		"documents WHY the wiring is needed: an unexcluded scan keeps reporting #recycle/old.txt as New")
	assert.Equal(t, 1, len(d.New))
	assert.Equal(t, "#recycle/old.txt", filepath.ToSlash(d.New[0].Path))
}

// SyncOptions.IgnorePatterns was declared and never read until #716. Callers that
// build their own file list (the library examples do) need it to mean something.
func TestComputeDelta_HonoursIgnorePatterns(t *testing.T) {
	local := []FileInfo{
		{Path: "keep.txt", Size: 1},
		{Path: "#recycle/old.txt", Size: 1},
		{Path: ".aws/credentials", Size: 1},
	}
	d, err := ComputeDelta(local, nil, &SyncOptions{IgnorePatterns: []string{"#recycle", ".aws"}})
	require.NoError(t, err)
	require.Equal(t, 1, len(d.New), "ignored paths must not appear in the delta: %+v", d.New)
	assert.Equal(t, "keep.txt", d.New[0].Path)
}

// An excluded directory must be PRUNED, not walked and filtered: on the deployment
// that exposed this, the excluded subtree was 20 GB / 12,237 files scanned every
// cycle only to be discarded. Verified by making descent observably impossible.
func TestScanLocalFilesExcluding_PrunesExcludedDirectories(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"keep.txt": "a", "skipme/inner/f.txt": "b"})

	// Make the excluded directory unreadable. filepath.Walk would surface an error for
	// its children if it descended; pruning means it never looks. Guarded first: where
	// an unreadable path cannot be created the assertion still PASSES but proves
	// nothing, since the exclusion alone produces the same result.
	requireUnreadablePathsArePossible(t)
	inner := filepath.Join(root, "skipme")
	require.NoError(t, os.Chmod(inner, 0o000))
	t.Cleanup(func() { _ = os.Chmod(inner, 0o755) })

	got, err := ScanLocalFilesExcluding(root, []string{"skipme"})
	require.NoError(t, err)
	var paths []string
	for _, f := range got {
		paths = append(paths, filepath.ToSlash(f.Path))
	}
	assert.Equal(t, []string{"keep.txt"}, paths)
}

// fileModTime returns the scanned mod time for a relative path, so a synthetic
// "previous manifest" matches on size AND mtime (the two things hasChanged compares).
func fileModTime(t *testing.T, files []FileInfo, rel string) time.Time {
	t.Helper()
	for _, f := range files {
		if filepath.ToSlash(f.Path) == rel {
			return f.ModTime
		}
	}
	t.Fatalf("%s not found in scan", rel)
	return time.Time{}
}
