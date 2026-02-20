package archivefs

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// buildTestManifest returns a manifest with the following structure:
//
//	data/
//	  raw/
//	    input.csv
//	  train/
//	    features.parquet  (stage: train)
//	    labels.csv        (stage: train)
//	models/
//	  model.pkl           (stage: train)
//	README.md
func buildTestManifest() *manifest.Manifest {
	now := time.Now()
	files := []manifest.FileEntry{
		{Path: "data/raw/input.csv", Size: 1024, ModTime: now, S3Key: "shard-0/chunk-0.tar.zst"},
		{Path: "data/train/features.parquet", Size: 13000000, ModTime: now, S3Key: "shard-0/chunk-1.tar.zst",
			ContentHash: "d8e8fca2dc0f896fd7cb4cb0031ba249",
			DVCMetadata: &manifest.DVCMetadata{Stage: "train"}},
		{Path: "data/train/labels.csv", Size: 2100000, ModTime: now, S3Key: "shard-0/chunk-1.tar.zst",
			DVCMetadata: &manifest.DVCMetadata{Stage: "train"}},
		{Path: "models/model.pkl", Size: 500000, ModTime: now, S3Key: "shard-0/chunk-2.tar.zst",
			DVCMetadata: &manifest.DVCMetadata{Stage: "train"}},
		{Path: "README.md", Size: 2048, ModTime: now, S3Key: "shard-0/chunk-0.tar.zst"},
	}
	return &manifest.Manifest{
		Version:     manifest.ManifestVersion,
		UploadID:    "test-001",
		Bucket:      "test-bucket",
		Files:       files,
		TotalFiles:  int64(len(files)),
		GitMetadata: &manifest.GitMetadata{Commit: "deadbeef"},
	}
}

func TestNew_RootChildren(t *testing.T) {
	m := buildTestManifest()
	vfs := New(m)

	root := vfs.List("")
	require.NotNil(t, root)

	names := make([]string, len(root))
	for i, e := range root {
		names[i] = e.Name
		if e.IsDir {
			names[i] += "/"
		}
	}
	assert.Equal(t, []string{"data/", "models/", "README.md"}, names)
}

func TestNew_NestedChildren(t *testing.T) {
	vfs := New(buildTestManifest())

	data := vfs.List("data")
	require.Len(t, data, 2)
	assert.Equal(t, "raw", data[0].Name)
	assert.True(t, data[0].IsDir)
	assert.Equal(t, "train", data[1].Name)
	assert.True(t, data[1].IsDir)

	train := vfs.List("data/train")
	require.Len(t, train, 2)
	assert.Equal(t, "features.parquet", train[0].Name)
	assert.False(t, train[0].IsDir)
	assert.Equal(t, "labels.csv", train[1].Name)
}

func TestNew_NonexistentDir(t *testing.T) {
	vfs := New(buildTestManifest())
	assert.Nil(t, vfs.List("does/not/exist"))
}

func TestIsDir(t *testing.T) {
	vfs := New(buildTestManifest())
	assert.True(t, vfs.IsDir(""))
	assert.True(t, vfs.IsDir("data"))
	assert.True(t, vfs.IsDir("data/train"))
	assert.False(t, vfs.IsDir("data/train/labels.csv"))
	assert.False(t, vfs.IsDir("nonexistent"))
}

func TestStat_File(t *testing.T) {
	vfs := New(buildTestManifest())
	fe := vfs.Stat("data/train/features.parquet")
	require.NotNil(t, fe)
	assert.Equal(t, "d8e8fca2dc0f896fd7cb4cb0031ba249", fe.ContentHash)
}

func TestStat_Directory(t *testing.T) {
	vfs := New(buildTestManifest())
	// Directories are not in the manifest; Stat returns nil for them.
	assert.Nil(t, vfs.Stat("data/train"))
}

func TestStat_Missing(t *testing.T) {
	vfs := New(buildTestManifest())
	assert.Nil(t, vfs.Stat("does/not/exist.txt"))
}

func TestResolve_RelativeFromRoot(t *testing.T) {
	vfs := New(buildTestManifest())
	assert.Equal(t, "data/train", vfs.Resolve("", "data/train"))
}

func TestResolve_RelativeFromSubdir(t *testing.T) {
	vfs := New(buildTestManifest())
	assert.Equal(t, "data/train", vfs.Resolve("data", "train"))
}

func TestResolve_DotDot(t *testing.T) {
	vfs := New(buildTestManifest())
	assert.Equal(t, "data", vfs.Resolve("data/train", ".."))
	assert.Equal(t, "", vfs.Resolve("data", ".."))
}

func TestResolve_Absolute(t *testing.T) {
	vfs := New(buildTestManifest())
	assert.Equal(t, "models", vfs.Resolve("data/train", "/models"))
	assert.Equal(t, "", vfs.Resolve("data/train", "/"))
}

func TestResolve_Dot(t *testing.T) {
	vfs := New(buildTestManifest())
	assert.Equal(t, "data/train", vfs.Resolve("data/train", "."))
	assert.Equal(t, "", vfs.Resolve("", "."))
}

func TestFindGlob_FullPath(t *testing.T) {
	vfs := New(buildTestManifest())
	results := vfs.FindGlob("data/train/*.csv")
	require.Len(t, results, 1)
	assert.Equal(t, "data/train/labels.csv", results[0].Path)
}

func TestFindGlob_Basename(t *testing.T) {
	vfs := New(buildTestManifest())
	results := vfs.FindGlob("*.csv")
	assert.Len(t, results, 2) // input.csv + labels.csv
}

func TestFindGlob_NoMatch(t *testing.T) {
	vfs := New(buildTestManifest())
	assert.Empty(t, vfs.FindGlob("*.xyz"))
}

func TestStages(t *testing.T) {
	vfs := New(buildTestManifest())
	stages := vfs.Stages()
	assert.Equal(t, map[string]int{"train": 3}, stages)
}

func TestFilesForStage(t *testing.T) {
	vfs := New(buildTestManifest())
	files := vfs.FilesForStage("train")
	assert.Len(t, files, 3)
}

func TestFilesForStage_Unknown(t *testing.T) {
	vfs := New(buildTestManifest())
	assert.Empty(t, vfs.FilesForStage("no-such-stage"))
}

func TestNew_RootFile(t *testing.T) {
	// README.md lives directly at the root — verify it appears correctly.
	vfs := New(buildTestManifest())
	root := vfs.List("")
	var found bool
	for _, e := range root {
		if e.Name == "README.md" && !e.IsDir {
			found = true
			break
		}
	}
	assert.True(t, found, "README.md must appear as a file in the root listing")
}

func TestNew_LargeManifest(t *testing.T) {
	// Verify VirtualFS handles a large flat manifest without panicking.
	m := &manifest.Manifest{}
	for i := range 1000 {
		m.Files = append(m.Files, manifest.FileEntry{
			Path:    fmt.Sprintf("flat/file-%04d.bin", i),
			S3Key:   fmt.Sprintf("shard-0/chunk-%d.tar.zst", i/100),
			ModTime: time.Now(),
		})
	}
	m.TotalFiles = int64(len(m.Files))
	vfs := New(m)
	assert.Len(t, vfs.List("flat"), 1000)
}
