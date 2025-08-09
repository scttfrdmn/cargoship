package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFind(t *testing.T) {
	out := bytes.NewBufferString("")
	cmd := NewFindCmd()
	cmd.SetOut(out)
	cmd.SetArgs([]string{"--inventory-directory", "../../../pkg/testdata/inventories", "bad.tar"})
	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "destination: bad.tar")
}
