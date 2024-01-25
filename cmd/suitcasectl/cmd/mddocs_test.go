package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMDDocs(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(b)
	dir := t.TempDir()
	cmd.SetArgs([]string{"mddocs", dir})
	require.NoError(t, cmd.Execute())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Greater(t, len(entries), 0)
}
