package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// goldenPath returns the path to a golden file under testdata/dvc_files/.
func goldenPath(name string) string {
	return filepath.Join("testdata", "dvc_files", name)
}

// readGolden reads a golden file and returns its contents.
func readGolden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(goldenPath(name))
	require.NoError(t, err, "read golden file %s", name)
	return string(data)
}

// makeTestManifest returns a minimal *Manifest suitable for export tests.
func makeTestManifest(files []FileEntry) *Manifest {
	return &Manifest{
		Version:  ManifestVersion,
		UploadID: "test-upload",
		Bucket:   "test-bucket",
		Files:    files,
	}
}

// --- GenerateDVCFiles ---

func TestGenerateDVCFiles_SingleFile(t *testing.T) {
	dir := t.TempDir()
	m := makeTestManifest([]FileEntry{
		{
			Path:        "data.csv",
			Size:        3,
			ContentHash: "acbd18db4cc2f85cedef654fccc4a4d8",
			S3Key:       "uploads/test-upload/shard-0/chunk-0.tar.zst",
		},
	})

	n, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := os.ReadFile(filepath.Join(dir, "data.csv.dvc"))
	require.NoError(t, err)
	assert.Equal(t, readGolden(t, "simple.dvc"), string(got))
}

func TestGenerateDVCFiles_SkipsEntryWithoutHash(t *testing.T) {
	dir := t.TempDir()
	m := makeTestManifest([]FileEntry{
		{Path: "README.md", Size: 100, ContentHash: ""},
	})

	n, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "entry without MD5 must be skipped")
	assert.NoFileExists(t, filepath.Join(dir, "README.md.dvc"))
}

func TestGenerateDVCFiles_OnlyHashedFilesWritten(t *testing.T) {
	dir := t.TempDir()
	m := makeTestManifest([]FileEntry{
		{Path: "a.txt", Size: 5, ContentHash: "5eb63bbbe01eeed093cb22bb8f5acdc3", S3Key: "k1"},
		{Path: "b.txt", Size: 0, ContentHash: ""},
		{Path: "c.txt", Size: 3, ContentHash: "acbd18db4cc2f85cedef654fccc4a4d8", S3Key: "k2"},
	})

	n, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.FileExists(t, filepath.Join(dir, "a.txt.dvc"))
	assert.NoFileExists(t, filepath.Join(dir, "b.txt.dvc"))
	assert.FileExists(t, filepath.Join(dir, "c.txt.dvc"))
}

func TestGenerateDVCFiles_SubdirectoryPreserved(t *testing.T) {
	dir := t.TempDir()
	m := makeTestManifest([]FileEntry{
		{
			Path:        filepath.Join("models", "weights.pt"),
			Size:        8,
			ContentHash: "acbd18db4cc2f85cedef654fccc4a4d8",
			S3Key:       "some/key",
		},
	})

	n, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	dvcFile := filepath.Join(dir, "models", "weights.pt.dvc")
	assert.FileExists(t, dvcFile, ".dvc file must mirror source tree structure")

	data, err := os.ReadFile(dvcFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "path: weights.pt", "path must be basename only")
}

func TestGenerateDVCFiles_DeepNesting(t *testing.T) {
	dir := t.TempDir()
	m := makeTestManifest([]FileEntry{
		{
			Path:        filepath.Join("a", "b", "c", "deep.csv"),
			Size:        1,
			ContentHash: "acbd18db4cc2f85cedef654fccc4a4d8",
			S3Key:       "k",
		},
	})

	_, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "a", "b", "c", "deep.csv.dvc"))
}

func TestGenerateDVCFiles_EmptyManifest(t *testing.T) {
	dir := t.TempDir()
	m := makeTestManifest(nil)

	n, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestGenerateDVCFiles_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	files := []FileEntry{
		{Path: "data/train.csv", Size: 100, ContentHash: "aaa", S3Key: "k1"},
		{Path: "data/test.csv", Size: 50, ContentHash: "bbb", S3Key: "k2"},
		{Path: "models/net.pt", Size: 200, ContentHash: "ccc", S3Key: "k3"},
	}
	m := makeTestManifest(files)

	n, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.FileExists(t, filepath.Join(dir, "data", "train.csv.dvc"))
	assert.FileExists(t, filepath.Join(dir, "data", "test.csv.dvc"))
	assert.FileExists(t, filepath.Join(dir, "models", "net.pt.dvc"))
}

// --- YAML content validation ---

func TestGenerateDVCFiles_YAMLContainsRequiredFields(t *testing.T) {
	dir := t.TempDir()
	m := makeTestManifest([]FileEntry{
		{
			Path:        "experiment.csv",
			Size:        42,
			ContentHash: "deadbeefdeadbeefdeadbeefdeadbeef",
			S3Key:       "uploads/abc/shard-1/chunk-2.tar.zst",
		},
	})

	_, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "experiment.csv.dvc"))
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "outs:")
	assert.Contains(t, content, "path: experiment.csv")
	assert.Contains(t, content, "md5: deadbeefdeadbeefdeadbeefdeadbeef")
	assert.Contains(t, content, "size: 42")
	assert.Contains(t, content, "cloud_bucket: test-bucket")
	assert.Contains(t, content, "cloud_key: uploads/abc/shard-1/chunk-2.tar.zst")
	assert.Contains(t, content, "upload_id: test-upload")
}

func TestGenerateDVCFiles_PathInDVCIsBasenameOnly(t *testing.T) {
	dir := t.TempDir()
	m := makeTestManifest([]FileEntry{
		{Path: "data/subdir/my_file.csv", Size: 1, ContentHash: "abc123", S3Key: "k"},
	})

	_, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "data", "subdir", "my_file.csv.dvc"))
	require.NoError(t, err)

	// Must contain just the basename, not the full relative path.
	assert.Contains(t, string(data), "path: my_file.csv")
	assert.NotContains(t, string(data), "data/subdir")
}

// --- DVCCompatibility metadata ---

func TestGenerateDVCFiles_SetsCompatibilityFlag(t *testing.T) {
	dir := t.TempDir()
	m := makeTestManifest([]FileEntry{
		{Path: "f.txt", Size: 1, ContentHash: "abc"},
	})
	assert.Nil(t, m.DVCCompatibility)

	_, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)

	require.NotNil(t, m.DVCCompatibility)
	assert.True(t, m.DVCCompatibility.DVCFilesGenerated)
}

func TestGenerateDVCFiles_StoresCacheDir(t *testing.T) {
	dir := t.TempDir()
	m := makeTestManifest([]FileEntry{
		{Path: "f.txt", Size: 1, ContentHash: "abc"},
	})

	opts := &DVCGenerateOptions{CacheDir: ".dvc/cache"}
	_, err := m.GenerateDVCFiles(dir, opts)
	require.NoError(t, err)

	require.NotNil(t, m.DVCCompatibility)
	assert.Equal(t, ".dvc/cache", m.DVCCompatibility.CacheDir)
}

func TestGenerateDVCFiles_CompatibilitySetEvenWithZeroWritten(t *testing.T) {
	dir := t.TempDir()
	// All entries lack ContentHash — nothing written, but flag must still be set.
	m := makeTestManifest([]FileEntry{
		{Path: "x.bin", Size: 5, ContentHash: ""},
	})

	n, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	require.NotNil(t, m.DVCCompatibility)
	assert.True(t, m.DVCCompatibility.DVCFilesGenerated)
}

// --- Idempotency ---

func TestGenerateDVCFiles_Idempotent(t *testing.T) {
	dir := t.TempDir()
	m := makeTestManifest([]FileEntry{
		{Path: "data.txt", Size: 5, ContentHash: "abc", S3Key: "k"},
	})

	n1, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)

	n2, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)

	assert.Equal(t, n1, n2)

	got, err := os.ReadFile(filepath.Join(dir, "data.txt.dvc"))
	require.NoError(t, err)
	assert.Contains(t, string(got), "md5: abc")
}

// --- YAML round-trip ---

func TestDVCFile_YAMLRoundTrip(t *testing.T) {
	original := DVCFile{
		Outs: []DVCOutput{
			{
				Path: "model.pt",
				MD5:  "deadbeef01234567deadbeef01234567",
				Size: 1024,
				Meta: &DVCMeta{
					CloudBucket: "my-bucket",
					CloudKey:    "uploads/abc/shard-0/chunk-1.tar.zst",
					UploadID:    "abc",
				},
			},
		},
	}

	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	var loaded DVCFile
	require.NoError(t, yaml.Unmarshal(data, &loaded))

	require.Len(t, loaded.Outs, 1)
	out := loaded.Outs[0]
	assert.Equal(t, "model.pt", out.Path)
	assert.Equal(t, "deadbeef01234567deadbeef01234567", out.MD5)
	assert.Equal(t, int64(1024), out.Size)
	require.NotNil(t, out.Meta)
	assert.Equal(t, "my-bucket", out.Meta.CloudBucket)
	assert.Equal(t, "uploads/abc/shard-0/chunk-1.tar.zst", out.Meta.CloudKey)
	assert.Equal(t, "abc", out.Meta.UploadID)
}

// --- No meta when no cloud info ---

func TestGenerateDVCFiles_NoMetaWhenNoCloudInfo(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		Version:  ManifestVersion,
		UploadID: "", // no upload ID
		Bucket:   "", // no bucket
		Files: []FileEntry{
			{Path: "local.csv", Size: 5, ContentHash: "abc", S3Key: ""},
		},
	}

	_, err := m.GenerateDVCFiles(dir, nil)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "local.csv.dvc"))
	require.NoError(t, err)
	content := string(data)

	// meta block should be absent when all cloud fields are empty
	assert.NotContains(t, content, "meta:")
	assert.Contains(t, content, "md5: abc")
}
