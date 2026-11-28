package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// scanFiles runs a bare scanner over root and returns the relative paths it
// chunked, and its error. onFirstJob, if set, runs once the first chunk arrives.
func scanFiles(t *testing.T, cfg *ScannerConfig, onFirstJob func()) ([]string, error) {
	t.Helper()
	output := make(chan *Job)
	s, err := NewScannerStage(cfg, output, nil)
	require.NoError(t, err)
	require.NoError(t, s.Start(context.Background()))

	var got []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		first := true
		for job := range output {
			if first && onFirstJob != nil {
				onFirstJob()
			}
			first = false
			for _, f := range job.Chunk.Files {
				rel, err := filepath.Rel(cfg.RootPath, f.Path)
				if err == nil {
					got = append(got, filepath.ToSlash(rel))
				}
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("the scanner never closed its output")
	}
	require.NoError(t, s.Stop())
	sort.Strings(got)
	return got, s.Error()
}

// TestScanner_ExcludedUnreadableDirIsSkipped pins the first half of #1007. The
// pipeline walk returned any walk error before it looked at the exclusions, so an
// excluded directory it couldn't read (a NAS #recycle, say) failed the upload,
// although the delta scan prunes it and #705's error recommends exactly that
// exclusion as the remedy.
func TestScanner_ExcludedUnreadableDirIsSkipped(t *testing.T) {
	root := t.TempDir()
	writePrefixedFiles(t, filepath.Join(root, "docs"), "f", 2, 64)
	lockedDir(t, filepath.Join(root, "#recycle"))
	patterns := []string{"#recycle"}

	delta, err := manifest.ScanLocalFilesExcluding(root, patterns)
	require.NoError(t, err, "the delta scan prunes the excluded directory")

	got, err := scanFiles(t, &ScannerConfig{RootPath: root, Workers: 2, ExcludePatterns: patterns}, nil)
	require.NoError(t, err, "the pipeline walk must prune it too")
	assert.Equal(t, []string{"docs/f00000.dat", "docs/f00001.dat"}, got)
	assert.Len(t, delta, len(got), "the delta and the upload must see the same files")
}

// TestScanner_UnreadableDirStaysLoud keeps the other side of the policy: a
// directory that is not excluded and can't be read still fails the scan.
func TestScanner_UnreadableDirStaysLoud(t *testing.T) {
	root := t.TempDir()
	writePrefixedFiles(t, filepath.Join(root, "docs"), "f", 2, 64)
	lockedDir(t, filepath.Join(root, "private"))

	_, err := scanFiles(t, &ScannerConfig{RootPath: root, Workers: 2, ExcludePatterns: []string{"#recycle"}}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private")
}

// TestScanner_FilesDeletedMidWalkAreSkipped pins the second half of #1007: a file
// deleted between the directory read and its lstat aborted the whole upload,
// although the delta scan skips it as routine churn. The test deletes files from
// the end of a large directory once the first chunk arrives, while the walk is
// still lstat-ing the entries it read at the start.
func TestScanner_FilesDeletedMidWalkAreSkipped(t *testing.T) {
	root := t.TempDir()
	const n = 6000
	for i := 0; i < n; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(root, fmt.Sprintf("f%05d", i)), nil, 0o600))
	}
	deleted := 0
	got, err := scanFiles(t, &ScannerConfig{RootPath: root, Workers: 2}, func() {
		for i := n - 1; i >= n-500; i-- {
			if os.Remove(filepath.Join(root, fmt.Sprintf("f%05d", i))) == nil {
				deleted++
			}
		}
	})
	require.NoError(t, err, "a file deleted mid-walk must not fail the scan")
	require.Positive(t, deleted)
	assert.GreaterOrEqual(t, len(got), n-500, "every file that was not deleted is scanned")
	assert.LessOrEqual(t, len(got), n)
}
