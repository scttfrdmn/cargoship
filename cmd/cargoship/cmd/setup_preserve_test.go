package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const userConfig = `# my CargoShip settings - do not lose
aws:
  region: eu-west-1
  profile: research   # the lab account
storage:
  default_bucket: lab-archive
  default_storage_class: GLACIER
  sse_encryption: false
upload:
  max_concurrency: 24
  chunk_size: 64MB
  compression_type: none
metrics:
  enabled: false
logging:
  level: warn
cost_control:
  budgets:
    - project: genome
      cost_usd: 1000
`

// stubSetupAWS keeps setup's credential and bucket checks off the network.
func stubSetupAWS(t *testing.T) {
	t.Helper()
	cred, bucket := setupVerifyCredentials, setupVerifyBucket
	setupVerifyCredentials = func(string, string) error { return nil }
	setupVerifyBucket = func(string, string, string) error { return nil }
	t.Cleanup(func() { setupVerifyCredentials, setupVerifyBucket = cred, bucket })
}

func writeUserConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".cargoship.yaml")
	require.NoError(t, os.WriteFile(path, []byte(userConfig), 0o600))
	return path
}

func readYAML(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, yaml.Unmarshal(data, &m))
	return m
}

func section(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	s, ok := m[key].(map[string]any)
	require.True(t, ok, "section %s missing: %v", key, m)
	return s
}

// TestSetup_NonInteractiveRefusesExistingConfig pins #909: `setup
// --non-interactive </dev/null` exited 0 having replaced the user's config with
// defaults. Without --force it must refuse and leave the file byte-identical.
func TestSetup_NonInteractiveRefusesExistingConfig(t *testing.T) {
	stubSetupAWS(t)
	path := writeUserConfig(t)

	err := runSetupWith(strings.NewReader(""), true, false, path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--force")
	got, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, userConfig, string(got))
}

// TestSetup_EndOfInputDoesNotConfirm: interactively, end of input used to count
// as "yes" to everything, overwriting the file.
func TestSetup_EndOfInputDoesNotConfirm(t *testing.T) {
	stubSetupAWS(t)
	path := writeUserConfig(t)

	err := runSetupWith(strings.NewReader(""), false, false, path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unchanged")
	got, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, userConfig, string(got))
}

// TestSetup_ForceKeepsEverySettingItDidNotChange: accepting every default on an
// existing file must keep its values, the sections setup doesn't own
// (cost_control) and its comments, and leave a .bak of the original.
func TestSetup_ForceKeepsEverySettingItDidNotChange(t *testing.T) {
	stubSetupAWS(t)
	path := writeUserConfig(t)

	require.NoError(t, runSetupWith(strings.NewReader(""), true, true, path))

	m := readYAML(t, path)
	assert.Equal(t, "eu-west-1", section(t, m, "aws")["region"])
	assert.Equal(t, "research", section(t, m, "aws")["profile"])
	assert.Equal(t, "lab-archive", section(t, m, "storage")["default_bucket"])
	assert.Equal(t, "GLACIER", section(t, m, "storage")["default_storage_class"])
	assert.Equal(t, false, section(t, m, "storage")["sse_encryption"])
	assert.Equal(t, 24, section(t, m, "upload")["max_concurrency"])
	assert.Equal(t, "64MB", section(t, m, "upload")["chunk_size"])
	assert.Equal(t, "none", section(t, m, "upload")["compression_type"])
	assert.Equal(t, false, section(t, m, "metrics")["enabled"])
	assert.Equal(t, "warn", section(t, m, "logging")["level"])
	require.Contains(t, m, "cost_control", "a section setup doesn't own must survive")

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "# my CargoShip settings - do not lose")
	assert.Contains(t, string(raw), "# the lab account")

	bak, err := os.ReadFile(path + ".bak")
	require.NoError(t, err)
	assert.Equal(t, userConfig, string(bak))
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm(), "the file's mode is kept")
	}
}

// TestSetup_InteractiveChangesOnlyWhatIsAnswered: a typed "yes", then a new
// region and empty answers elsewhere, changes the region and nothing else.
func TestSetup_InteractiveChangesOnlyWhatIsAnswered(t *testing.T) {
	stubSetupAWS(t)
	path := writeUserConfig(t)

	require.NoError(t, runSetupWith(strings.NewReader("yes\nus-west-2\n"), false, false, path))

	m := readYAML(t, path)
	assert.Equal(t, "us-west-2", section(t, m, "aws")["region"])
	assert.Equal(t, "research", section(t, m, "aws")["profile"])
	assert.Equal(t, "GLACIER", section(t, m, "storage")["default_storage_class"])
	assert.Equal(t, "warn", section(t, m, "logging")["level"])
	require.Contains(t, m, "cost_control")
}

// TestSetup_FreshConfigUsesWizardDefaults: with no file, the wizard's own
// defaults apply as before and the new file is private.
func TestSetup_FreshConfigUsesWizardDefaults(t *testing.T) {
	stubSetupAWS(t)
	path := filepath.Join(t.TempDir(), "sub", ".cargoship.yaml")

	require.NoError(t, runSetupWith(strings.NewReader(""), true, false, path))

	m := readYAML(t, path)
	assert.Equal(t, "INTELLIGENT_TIERING", section(t, m, "storage")["default_storage_class"])
	assert.Equal(t, "zstd", section(t, m, "upload")["compression_type"])
	assert.Equal(t, "info", section(t, m, "logging")["level"])
	_, err := os.Stat(path + ".bak")
	assert.True(t, os.IsNotExist(err), "no backup for a new file")
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
	}
}

func TestSetup_UnreadableConfigIsNotOverwritten(t *testing.T) {
	stubSetupAWS(t)
	path := filepath.Join(t.TempDir(), ".cargoship.yaml")
	require.NoError(t, os.WriteFile(path, []byte("aws: [not: a mapping"), 0o600))

	err := runSetupWith(strings.NewReader(""), true, true, path)
	require.Error(t, err)
	got, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, "aws: [not: a mapping", string(got))
}
