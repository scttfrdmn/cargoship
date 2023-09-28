package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitFsRemote(t *testing.T) {
	fs, remote := splitFsRemote("foo:/bar")
	require.Equal(t, "foo:", fs)
	require.Equal(t, "/bar", remote)

	fs, remote = splitFsRemote("foo")
	require.Equal(t, "foo:", fs)
	require.Equal(t, "", remote)
}
