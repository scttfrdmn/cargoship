package manifest

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestManifest constructs a Manifest with predictable FileEntry values.
func buildTestManifest() *Manifest {
	return &Manifest{
		Version:  ManifestVersion,
		UploadID: "test-query-001",
		GitMetadata: &GitMetadata{
			Commit: "abc1234567890",
			Branch: "main",
		},
		Files: []FileEntry{
			{
				Path:        "data/train.csv",
				Size:        1024,
				ModTime:     time.Now(),
				ContentHash: "d8e8fca2dc0f896fd7cb4cb0031ba249",
				DVCMetadata: &DVCMetadata{Stage: "preprocess", Pipeline: "dvc.yaml"},
			},
			{
				Path:        "data/test.csv",
				Size:        512,
				ModTime:     time.Now(),
				ContentHash: "b026324c6904b2a9cb4b88d6d61c81d1",
				DVCMetadata: &DVCMetadata{Stage: "preprocess", Pipeline: "dvc.yaml"},
			},
			{
				Path:        "models/model.pkl",
				Size:        2048,
				ModTime:     time.Now(),
				ContentHash: "26ab0db90d72e28ad0ba1e22ee510510",
				DVCMetadata: &DVCMetadata{Stage: "train", Pipeline: "dvc.yaml"},
			},
			{
				// No ContentHash, no DVCMetadata — should not appear in hash or stage indices
				Path:    "README.md",
				Size:    256,
				ModTime: time.Now(),
			},
		},
	}
}

// ---------------------------------------------------------------------------
// FindFileByHash
// ---------------------------------------------------------------------------

func TestFindFileByHash_Hit(t *testing.T) {
	mq := NewManifestQuery(buildTestManifest())

	f, ok := mq.FindFileByHash("d8e8fca2dc0f896fd7cb4cb0031ba249")
	require.True(t, ok)
	assert.Equal(t, "data/train.csv", f.Path)
}

func TestFindFileByHash_Miss(t *testing.T) {
	mq := NewManifestQuery(buildTestManifest())

	f, ok := mq.FindFileByHash("0000000000000000000000000000dead")
	assert.False(t, ok)
	assert.Nil(t, f)
}

func TestFindFileByHash_EmptyString(t *testing.T) {
	mq := NewManifestQuery(buildTestManifest())
	f, ok := mq.FindFileByHash("")
	assert.False(t, ok)
	assert.Nil(t, f)
}

func TestFindFileByHash_FileWithoutHash(t *testing.T) {
	// README.md has no ContentHash — querying any hash should not return it
	mq := NewManifestQuery(buildTestManifest())

	// Verify README.md exists via path lookup
	readme := mq.FindFile("README.md")
	require.NotNil(t, readme)
	assert.Empty(t, readme.ContentHash)

	// Hash index must not contain empty-string key
	f, ok := mq.FindFileByHash("")
	assert.False(t, ok)
	assert.Nil(t, f)
}

// ---------------------------------------------------------------------------
// FindFilesByCommit
// ---------------------------------------------------------------------------

func TestFindFilesByCommit_MatchingCommit(t *testing.T) {
	mq := NewManifestQuery(buildTestManifest())

	files := mq.FindFilesByCommit("abc1234567890")
	assert.Len(t, files, 4) // all 4 files in the manifest
}

func TestFindFilesByCommit_WrongCommit(t *testing.T) {
	mq := NewManifestQuery(buildTestManifest())

	files := mq.FindFilesByCommit("deadbeef")
	assert.Nil(t, files)
}

func TestFindFilesByCommit_EmptyString(t *testing.T) {
	mq := NewManifestQuery(buildTestManifest())
	assert.Nil(t, mq.FindFilesByCommit(""))
}

func TestFindFilesByCommit_NoGitMetadata(t *testing.T) {
	m := buildTestManifest()
	m.GitMetadata = nil
	mq := NewManifestQuery(m)

	// No commit index was built; any query returns nil
	assert.Nil(t, mq.FindFilesByCommit("abc1234567890"))
}

// ---------------------------------------------------------------------------
// FindFilesByDVCStage
// ---------------------------------------------------------------------------

func TestFindFilesByDVCStage_Hit(t *testing.T) {
	mq := NewManifestQuery(buildTestManifest())

	preprocessFiles := mq.FindFilesByDVCStage("preprocess")
	require.Len(t, preprocessFiles, 2)

	paths := []string{preprocessFiles[0].Path, preprocessFiles[1].Path}
	assert.ElementsMatch(t, []string{"data/train.csv", "data/test.csv"}, paths)
}

func TestFindFilesByDVCStage_SingleResult(t *testing.T) {
	mq := NewManifestQuery(buildTestManifest())

	trainFiles := mq.FindFilesByDVCStage("train")
	require.Len(t, trainFiles, 1)
	assert.Equal(t, "models/model.pkl", trainFiles[0].Path)
}

func TestFindFilesByDVCStage_Miss(t *testing.T) {
	mq := NewManifestQuery(buildTestManifest())
	assert.Nil(t, mq.FindFilesByDVCStage("evaluate"))
}

func TestFindFilesByDVCStage_EmptyString(t *testing.T) {
	mq := NewManifestQuery(buildTestManifest())
	assert.Nil(t, mq.FindFilesByDVCStage(""))
}

func TestFindFilesByDVCStage_FileWithoutDVCMetadata(t *testing.T) {
	// README.md has no DVCMetadata — it must not appear in any stage
	mq := NewManifestQuery(buildTestManifest())

	// All 4 files exist via path
	assert.NotNil(t, mq.FindFile("README.md"))

	// But README.md must not bleed into any stage result
	var found bool
	for _, stage := range []string{"preprocess", "train"} {
		for _, f := range mq.FindFilesByDVCStage(stage) {
			if f.Path == "README.md" {
				found = true
			}
		}
	}
	assert.False(t, found, "README.md must not appear in any DVC stage index")
}

// ---------------------------------------------------------------------------
// RebuildIndex
// ---------------------------------------------------------------------------

func TestRebuildIndex_ReflectsUpdatedFiles(t *testing.T) {
	m := buildTestManifest()
	mq := NewManifestQuery(m)

	// Hash not present yet
	_, ok := mq.FindFileByHash("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
	assert.False(t, ok)

	// Mutate the manifest (simulating pipeline append)
	m.Files = append(m.Files, FileEntry{
		Path:        "new/file.bin",
		ContentHash: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		DVCMetadata: &DVCMetadata{Stage: "evaluate"},
	})
	mq.RebuildIndex()

	f, ok := mq.FindFileByHash("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
	require.True(t, ok)
	assert.Equal(t, "new/file.bin", f.Path)

	evalFiles := mq.FindFilesByDVCStage("evaluate")
	require.Len(t, evalFiles, 1)
	assert.Equal(t, "new/file.bin", evalFiles[0].Path)
}

// ---------------------------------------------------------------------------
// Benchmarks: 1M entry index built in < 100ms (realistic distribution)
// ---------------------------------------------------------------------------

// BenchmarkNewManifestQuery_1MFiles tests index build time for a realistic
// distribution where ~5% of files are DVC-annotated.  This should be < 100ms.
func BenchmarkNewManifestQuery_1MFiles(b *testing.B) {
	files := make([]FileEntry, 1_000_000)
	for i := range files {
		fe := FileEntry{
			Path: fmt.Sprintf("data/file-%07d.bin", i),
		}
		// ~5% of files have a ContentHash (DVC-tracked outputs)
		if i%20 == 0 {
			fe.ContentHash = fmt.Sprintf("%032x", i)
		}
		// ~1% of files have full DVC stage metadata
		if i%100 == 0 {
			fe.DVCMetadata = &DVCMetadata{Stage: fmt.Sprintf("stage-%d", i%10)}
		}
		files[i] = fe
	}
	m := &Manifest{
		Version:     ManifestVersion,
		GitMetadata: &GitMetadata{Commit: "benchcommit"},
		Files:       files,
	}

	b.ResetTimer()
	for range b.N {
		_ = NewManifestQuery(m)
	}
}

// BenchmarkNewManifestQuery_1MFiles_FullDVC benchmarks the worst case where
// every file carries ContentHash and DVCMetadata.
func BenchmarkNewManifestQuery_1MFiles_FullDVC(b *testing.B) {
	files := make([]FileEntry, 1_000_000)
	for i := range files {
		files[i] = FileEntry{
			Path:        fmt.Sprintf("data/file-%07d.bin", i),
			ContentHash: fmt.Sprintf("%032x", i),
			DVCMetadata: &DVCMetadata{Stage: fmt.Sprintf("stage-%d", i%10)},
		}
	}
	m := &Manifest{
		Version:     ManifestVersion,
		GitMetadata: &GitMetadata{Commit: "benchcommit"},
		Files:       files,
	}

	b.ResetTimer()
	for range b.N {
		_ = NewManifestQuery(m)
	}
}
