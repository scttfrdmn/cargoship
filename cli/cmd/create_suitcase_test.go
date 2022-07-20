package cmd

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/cli/cmdhelpers"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/inventory"
)

func TestNewDirectoryInventoryOptionsWithCmd(t *testing.T) {
	testD := t.TempDir()
	cmd := NewCreateSuitcaseCmd()
	cmd.SetArgs([]string{testD})
	err := cmd.Execute()
	require.NoError(t, err)
	// f := cmd.PersistentFlags()
	// log.Warn().Msgf("%+v", f)
	_, err = cmdhelpers.NewDirectoryInventoryOptionsWithCmd(cmd, nil)

	require.NoError(t, err)
}

func TestNewSuitcaseWithDir(t *testing.T) {
	testD := t.TempDir()
	cmd := NewCreateSuitcaseCmd()
	cmd.SetArgs([]string{testD})
	err := cmd.Execute()
	require.NoError(t, err)
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

	cmd := NewCreateSuitcaseCmd()
	cmd.SetArgs([]string{"--inventory-file", outF.Name()})
	err = cmd.Execute()
	require.NoError(t, err)
}

func TestNewSuitcaseWithInventoryAndDir(t *testing.T) {
	fakeDir := t.TempDir()
	cmd := NewCreateSuitcaseCmd()
	cmd.SetArgs([]string{"--inventory-file", "doesnt-matter", fakeDir})
	err := cmd.Execute()
	require.Error(t, err)
	require.EqualError(t, err, "Error: You can't specify an inventory file and target dir arguments at the same time")
}
