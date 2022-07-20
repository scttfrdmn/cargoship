package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/cli/cmdhelpers"
)

func TestNewDirectoryInventoryOptionsWithCmd(t *testing.T) {
	testD := t.TempDir()
	cmd := NewCreateSuitcaseCmd()
	PopulateCreateSuitcaseFlags(cmd)
	cmd.SetArgs([]string{testD})
	cmd.Execute()
	// f := cmd.PersistentFlags()
	// log.Warn().Msgf("%+v", f)
	_, err := cmdhelpers.NewDirectoryInventoryOptionsWithCmd(cmd, nil)

	require.NoError(t, err)
}
