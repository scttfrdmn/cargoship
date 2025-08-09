package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManCmd(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := newManCmd()
	cmd.SetOut(b)
	require.NoError(t, cmd.Execute())
	require.Contains(t, b.String(), "Generates GoReleaser's command line manpages")
}
