package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTree(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(b)
	cmd.SetArgs([]string{
		"tree", "../../../pkg/testdata/inventories/example-inventory.yaml",
	})
	require.NoError(t, cmd.Execute())
	require.Equal(t, "/\n└── Users\n    └── drews\n        └── Desktop\n            └── example-suitcase\n                └── bad.tar\n", b.String())
}
