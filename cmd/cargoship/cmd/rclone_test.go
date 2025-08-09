package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRclone(t *testing.T) {
	tdir := t.TempDir()
	cmd := NewRcloneCmd()
	cmd.SetArgs([]string{"../../../pkg/testdata/archives/", tdir})
	require.NoError(
		t, cmd.Execute(),
	)
}
