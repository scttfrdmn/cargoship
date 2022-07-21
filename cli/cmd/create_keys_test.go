package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateKeys(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(b)
	// Just do a small key so the test runs fast 🤷‍♀️
	cmd.SetArgs([]string{"create", "keys", "--name", "test", "--email", "test@example.com", "--bits", "1024"})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, b.String(), "Create key files")
}
