package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteConfigSections_MergesAndKeeps pins #909's writer: only the named
// sections change, merged key by key, so unknown keys, other sections and
// comments survive, and the original is kept as .bak.
func TestWriteConfigSections_MergesAndKeeps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	orig := `# top comment
aws:
  region: eu-west-1 # keep this note
  custom_endpoint_note: lab   # not a Config field
cost_control:
  budgets: [genome]
`
	require.NoError(t, os.WriteFile(path, []byte(orig), 0o640))

	cfg := DefaultConfig()
	cfg.AWS.Region = "us-west-2"
	cfg.Logging.Level = "debug"
	backup, err := WriteConfigSections(path, cfg, []string{"aws", "logging"})
	require.NoError(t, err)
	assert.Equal(t, path+".bak", backup)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	s := string(got)
	assert.Contains(t, s, "region: us-west-2")
	assert.Contains(t, s, "# keep this note", "a replaced value keeps its comment")
	assert.Contains(t, s, "custom_endpoint_note: lab", "unknown keys inside a section survive")
	assert.Contains(t, s, "# top comment")
	assert.Contains(t, s, "cost_control:")
	assert.Contains(t, s, "level: debug", "a missing section is added")
	assert.NotContains(t, s, "storage:", "sections not named are not written")

	bak, err := os.ReadFile(backup)
	require.NoError(t, err)
	assert.Equal(t, orig, string(bak))
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), fi.Mode().Perm())
	}

	cfg2, err := ReadConfigFile(path)
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", cfg2.AWS.Region)
	assert.Equal(t, "debug", cfg2.Logging.Level)
}

func TestWriteConfigSections_RefusesWhatItCannotMerge(t *testing.T) {
	for name, content := range map[string]string{
		"invalid YAML":    "aws: [broken",
		"not a mapping":   "- a\n- b\n",
		"scalar document": "just text\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "c.yaml")
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
			_, err := WriteConfigSections(path, DefaultConfig(), []string{"aws"})
			require.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), "unchanged"), err.Error())
			got, rerr := os.ReadFile(path)
			require.NoError(t, rerr)
			assert.Equal(t, content, string(got))
			_, serr := os.Stat(path + ".bak")
			assert.True(t, os.IsNotExist(serr))
		})
	}
}

func TestWriteConfigSections_EmptyFileAndNewFile(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.yaml")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	_, err := WriteConfigSections(empty, DefaultConfig(), []string{"aws"})
	require.NoError(t, err)
	got, err := os.ReadFile(empty)
	require.NoError(t, err)
	assert.Contains(t, string(got), "aws:")

	fresh := filepath.Join(dir, "nested", "new.yaml")
	backup, err := WriteConfigSections(fresh, DefaultConfig(), []string{"aws"})
	require.NoError(t, err)
	assert.Empty(t, backup)
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(fresh)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
	}
}

func TestReadConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	require.NoError(t, os.WriteFile(path, []byte("aws:\n  region: ap-south-1\n"), 0o600))
	t.Setenv("CARGOSHIP_AWS_REGION", "from-env")
	cfg, err := ReadConfigFile(path)
	require.NoError(t, err)
	assert.Equal(t, "ap-south-1", cfg.AWS.Region, "the file, not the environment")
	assert.Equal(t, DefaultConfig().Upload.ChunkSize, cfg.Upload.ChunkSize, "defaults fill the rest")

	_, err = ReadConfigFile(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
}

func TestWriteConfigSections_FilesystemErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission and rename semantics differ on Windows")
	}
	dir := t.TempDir()

	// The path is a directory: reading it fails, nothing is written.
	asDir := filepath.Join(dir, "is-a-dir")
	require.NoError(t, os.Mkdir(asDir, 0o755))
	_, err := WriteConfigSections(asDir, DefaultConfig(), []string{"aws"})
	require.Error(t, err)

	// The parent is a file: the directory can't be created.
	parentFile := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(parentFile, nil, 0o600))
	_, err = WriteConfigSections(filepath.Join(parentFile, "c.yaml"), DefaultConfig(), []string{"aws"})
	require.Error(t, err)

	// An unwritable directory: the temporary file can't be created, and the
	// existing file is left as it was.
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	ro := filepath.Join(dir, "ro")
	require.NoError(t, os.Mkdir(ro, 0o755))
	existing := filepath.Join(ro, "c.yaml")
	require.NoError(t, os.WriteFile(existing, []byte("aws:\n  region: x\n"), 0o600))
	require.NoError(t, os.Chmod(ro, 0o500))
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	_, err = WriteConfigSections(existing, DefaultConfig(), []string{"aws"})
	require.Error(t, err)
	got, rerr := os.ReadFile(existing)
	require.NoError(t, rerr)
	assert.Equal(t, "aws:\n  region: x\n", string(got))
}

func TestWriteFileAtomic_RenameOntoDirectoryFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("rename semantics differ on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	require.NoError(t, os.Mkdir(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "keep"), nil, 0o600))
	require.Error(t, writeFileAtomic(target, []byte("x"), 0o600))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the temporary file is removed on failure")
}

func TestReadConfigFile_InvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	require.NoError(t, os.WriteFile(path, []byte("aws: [broken"), 0o600))
	_, err := ReadConfigFile(path)
	require.Error(t, err)
}
