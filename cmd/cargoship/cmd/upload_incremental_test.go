package cmd

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// writePrevManifest saves m as a local manifest file for --prev-manifest.
func writePrevManifest(t *testing.T, m *manifest.Manifest) string {
	t.Helper()
	data, err := m.ToJSON()
	require.NoError(t, err)
	p := filepath.Join(t.TempDir(), "prev.json")
	require.NoError(t, os.WriteFile(p, data, 0o600))
	return p
}

// noFetch is a chain fetcher for predecessors that never need one.
func noFetch(context.Context, string) (*manifest.Manifest, error) {
	return nil, os.ErrNotExist
}

// TestPlanIncremental pins #1002's plan: only new and changed files (a same-size
// edit counts), and the version joins the predecessor's dataset.
func TestPlanIncremental(t *testing.T) {
	src := t.TempDir()
	same := filepath.Join(src, "same.txt")
	edited := filepath.Join(src, "edited.txt")
	require.NoError(t, os.WriteFile(same, []byte("cargoship-test same"), 0o600))
	require.NoError(t, os.WriteFile(edited, []byte("enabled=fals"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(src, "new.txt"), []byte("cargoship-test new"), 0o600))
	sameInfo, err := os.Stat(same)
	require.NoError(t, err)
	editedInfo, err := os.Stat(edited)
	require.NoError(t, err)

	prev := &manifest.Manifest{UploadID: "u-prev", SourcePath: src, DatasetID: "ds-root", VersionOrdinal: 3,
		Files: []manifest.FileEntry{
			{Path: same, Size: sameInfo.Size(), ModTime: sameInfo.ModTime()},
			{Path: edited, Size: editedInfo.Size(), ModTime: editedInfo.ModTime().Add(-time.Hour)},
		}}
	plan, err := planIncremental(context.Background(), writePrevManifest(t, prev), src, noFetch)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"edited.txt", "new.txt"}, plan.files)
	assert.Equal(t, "u-prev", plan.prevUploadID)
	assert.Equal(t, "ds-root", plan.datasetID)
	assert.Equal(t, 4, plan.versionOrdinal)

	// A predecessor with no dataset identity is its own root (#521).
	prev.DatasetID, prev.VersionOrdinal = "", 0
	plan, err = planIncremental(context.Background(), writePrevManifest(t, prev), src, noFetch)
	require.NoError(t, err)
	assert.Equal(t, "u-prev", plan.datasetID)
	assert.Equal(t, 2, plan.versionOrdinal)
}

// TestPlanIncremental_ScanErrorFails pins #1002: a scan that fails part-way
// used to proceed with the partial change list, silently dropping every change
// after the error.
func TestPlanIncremental_ScanErrorFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	src := t.TempDir()
	locked := filepath.Join(src, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(locked, "f.txt"), []byte("cargoship-test"), 0o600))
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })

	_, err := planIncremental(context.Background(), writePrevManifest(t, &manifest.Manifest{UploadID: "u", SourcePath: src}), src, noFetch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing was uploaded")

	_, err = planIncremental(context.Background(), filepath.Join(t.TempDir(), "missing.json"), src, noFetch)
	require.Error(t, err, "a missing previous manifest is an error")
}

// TestUploadCmd_IncrementalFlagPairing pins the usage errors: --incremental
// needs --prev-manifest, and --prev-manifest alone was silently ignored.
func TestUploadCmd_IncrementalFlagPairing(t *testing.T) {
	for _, args := range [][]string{
		{t.TempDir(), "s3://b/p", "--incremental"},
		{t.TempDir(), "s3://b/p", "--prev-manifest", "prev.json"},
	} {
		c := NewUploadCmd()
		c.SetArgs(args)
		c.SilenceUsage, c.SilenceErrors = true, true
		err := c.Execute()
		require.Error(t, err, "%v", args)
		assert.ErrorIs(t, err, ErrUsage, "%v", args)
	}
}
