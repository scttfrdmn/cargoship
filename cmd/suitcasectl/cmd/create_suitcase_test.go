package cmd

import (
	"bytes"
	"io"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
)

func TestNewSuitcaseWithDir(t *testing.T) {
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", testD})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestNewSuitcaseWithViper(t *testing.T) {
	// testD := t.TempDir()
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "-o", testD, "../../../pkg/testdata/viper-enabled-target"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.FileExists(t, path.Join(testD, "snakey-thing-joebob-01-of-01.tar.gz"))
}

// Ensure that if we set a value on the CLI that it gets preference over whatever is in the user overrides
func TestNewSuitcaseWithViperFlag(t *testing.T) {
	// testD, err := os.MkdirTemp("", "")
	// require.NoError(t, err)
	// defer os.RemoveAll(testD)
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "--destination", testD, "--user", "darcy", "../../../pkg/testdata/viper-enabled-target"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.FileExists(t, path.Join(testD, "snakey-thing-darcy-01-of-01.tar.gz"))
}

func TestNewSuitcaseWithInventory(t *testing.T) {
	// outDir, err := os.MkdirTemp("", "")
	// require.NoError(t, err)
	// defer os.RemoveAll(outDir)
	outDir := t.TempDir()
	i, err := inventory.NewDirectoryInventory(&inventory.DirectoryInventoryOptions{
		TopLevelDirectories: []string{"../../../pkg/testdata/fake-dir"},
		SuitcaseFormat:      "tar",
		InventoryFormat:     "yaml",
	})
	require.NoError(t, err)
	outF, err := os.Create(path.Join(outDir, "inventory.yaml"))
	require.NoError(t, err)
	ir, err := inventory.NewInventoryerWithFilename(outF.Name())
	require.NoError(t, err)

	err = ir.Write(outF, i)
	require.NoError(t, err)

	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "--destination", outDir, "--inventory-file", outF.Name()})
	err = cmd.Execute()
	require.NoError(t, err)
}

func TestNewSuitcaseWithInventoryAndDir(t *testing.T) {
	fakeDir := t.TempDir()
	fakeTemp := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "--destination", fakeTemp, "--inventory-file", "doesnt-matter", fakeDir})
	err := cmd.Execute()
	require.Error(t, err, "Did NOT get an error when executing command")
	require.EqualError(t, err, "error: You can't specify an inventory file and target dir arguments at the same time", "Got an unexpected error")
}

func TestInventoryFormatComplete(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(io.Discard)
	cmd.SetOut(b)
	cmd.SetArgs([]string{"__complete", "create", "suitcase", "--inventory-format", ""})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Equal(t, "json\nyaml\n:4\n", b.String())
}

func TestSuitcaseFormatComplete(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(io.Discard)
	cmd.SetOut(b)
	cmd.SetArgs([]string{"__complete", "create", "suitcase", "--suitcase-format", ""})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Equal(t, "tar\ntar.gpg\ntar.gz\ntar.gz.gpg\n:4\n", b.String())
}
