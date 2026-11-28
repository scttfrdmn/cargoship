//go:build integration

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// uploadIDs returns the sorted upload IDs under prefix/uploads/.
func uploadIDs(keys []string, prefix string) []string {
	seen := map[string]bool{}
	for _, k := range keys {
		rest, ok := strings.CutPrefix(k, prefix+"/uploads/")
		if !ok {
			continue
		}
		if id, _, ok := strings.Cut(rest, "/"); ok {
			seen[id] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// TestUpload_IncrementalUploadsOnlyChanges pins #1002 through the upload
// command: an unchanged tree uploads nothing (an empty change set used to mean
// "upload everything", a full upload labelled incremental), a same-size edit is
// uploaded (the size-only rule skipped it), and the delta joins its
// predecessor's dataset instead of starting its own v1.
func TestUpload_IncrementalUploadsOnlyChanges(t *testing.T) {
	e := newUploadEmulator(t)
	ctx := context.Background()
	src := writeSourceTree(t, map[string]string{
		"config.ini":     "enabled=true",
		"data/table.csv": "cargoship-test a,b,c\n1,2,3\n",
	})
	const prefix = "incr"

	_, err := e.upload(t, src, prefix)
	require.NoError(t, err)
	ids := uploadIDs(e.keys(t, prefix), prefix)
	require.Len(t, ids, 1)
	first := ids[0]
	prev, err := manifest.DownloadFromS3(ctx, e.s3, e.bucket, prefix, first)
	require.NoError(t, err)
	prevJSON, err := prev.ToJSON()
	require.NoError(t, err)
	prevFile := filepath.Join(t.TempDir(), "prev.json")
	require.NoError(t, os.WriteFile(prevFile, prevJSON, 0o600))

	// Nothing changed: nothing is uploaded.
	_, err = e.upload(t, src, prefix, "--incremental", "--prev-manifest", prevFile)
	require.NoError(t, err)
	assert.Equal(t, []string{first}, uploadIDs(e.keys(t, prefix), prefix),
		"an unchanged tree must not produce a new upload")

	// A same-size in-place edit is a change.
	p := filepath.Join(src, "config.ini")
	require.NoError(t, os.WriteFile(p, []byte("enabled=fals"), 0o600))
	later := time.Now().Add(time.Minute)
	require.NoError(t, os.Chtimes(p, later, later))
	_, err = e.upload(t, src, prefix, "--incremental", "--prev-manifest", prevFile)
	require.NoError(t, err)
	ids = uploadIDs(e.keys(t, prefix), prefix)
	require.Len(t, ids, 2, "the edit must be uploaded")
	var delta string
	for _, id := range ids {
		if id != first {
			delta = id
		}
	}
	m, err := manifest.DownloadFromS3(ctx, e.s3, e.bucket, prefix, delta)
	require.NoError(t, err)
	require.Len(t, m.Files, 1)
	assert.Equal(t, "config.ini", filepath.Base(m.Files[0].Path))
	assert.Equal(t, manifest.SyncTypeIncremental, m.SyncType)
	assert.Equal(t, first, m.PreviousManifestID)
	assert.Equal(t, prev.DatasetID, m.DatasetID, "the delta must join its predecessor's dataset")
	assert.Equal(t, 2, m.VersionOrdinal)
}

// TestUpload_IncrementalRequiresPrevManifest pins the scope-4 finding on #1002:
// --incremental without --prev-manifest silently ran a full upload labelled
// incremental. It is a usage error now.
func TestUpload_IncrementalRequiresPrevManifest(t *testing.T) {
	e := newUploadEmulator(t)
	src := writeSourceTree(t, map[string]string{"a.txt": "cargoship-test a\n"})
	_, err := e.upload(t, src, "incr-noprev", "--incremental")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUsage)
	assert.Empty(t, e.keys(t, "incr-noprev"), "nothing may be uploaded")
}
