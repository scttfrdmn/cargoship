package cmd

import (
	"io"
	"os"
	"path"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/cli/cmdhelpers"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/inventory"
)

/*
func TestNewDirectoryInventoryOptionsWithCmd(t *testing.T) {
	testD := t.TempDir()
	// cmd := NewCreateSuitcaseCmd()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", testD})
	err := cmd.Execute()
	require.NoError(t, err)
	// This is really gross 🤮. What's a better way to get the subcommand yoinked out of root?
	for _, ccmd := range cmd.Commands() {
		if ccmd.Name() == "create" {
			for _, cccmd := range ccmd.Commands() {
				if cccmd.Name() == "suitcase" {
					_, err = cmdhelpers.NewDirectoryInventoryOptionsWithCmd(cccmd, nil)
					assert.NoError(t, err)
				}
			}
		}
	}
}
*/

func TestNewDirectoryInventoryOptionsWithViper(t *testing.T) {
	v := viper.New()
	_, err := cmdhelpers.NewDirectoryInventoryOptionsWithViper(v, nil)
	assert.NoError(t, err)
}

func TestNewSuitcaseWithDir(t *testing.T) {
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", testD})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestNewSuitcaseWithViper(t *testing.T) {
	// testD := t.TempDir()
	testD := os.TempDir()
	defer os.RemoveAll(testD)
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "-o", testD, "../../pkg/testdata/viper-enabled-target"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.FileExists(t, path.Join(testD, "snakey-thing-joebob-01-of-01.tar.gz"))
}

// Ensure that if we set a value on the CLI that it gets preference over whatever is in the user overrides
func TestNewSuitcaseWithViperFlag(t *testing.T) {
	testD := os.TempDir()
	defer os.RemoveAll(testD)
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "-o", testD, "--user", "darcy", "../../pkg/testdata/viper-enabled-target"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.FileExists(t, path.Join(testD, "snakey-thing-darcy-01-of-01.tar.gz"))
}

func TestNewSuitcaseWithInventory(t *testing.T) {
	outDir := t.TempDir()
	i, err := inventory.NewDirectoryInventory(&inventory.DirectoryInventoryOptions{
		TopLevelDirectories: []string{"../../pkg/testdata/fake-dir"},
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
	cmd.SetArgs([]string{"create", "suitcase", "--inventory-file", outF.Name()})
	err = cmd.Execute()
	require.NoError(t, err)
}

func TestNewSuitcaseWithInventoryAndDir(t *testing.T) {
	fakeDir := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "--inventory-file", "doesnt-matter", fakeDir})
	err := cmd.Execute()
	require.Error(t, err)
	require.EqualError(t, err, "Error: You can't specify an inventory file and target dir arguments at the same time")
}
