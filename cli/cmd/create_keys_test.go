package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateKeys(t *testing.T) {
	cmd := NewRootCmd()
	// Just do a small key so the test runs fast 🤷‍♀️
	cmd.SetArgs([]string{"create", "keys", "--name", "test", "--email", "test@example.com", "--bits", "1024"})
	err := cmd.Execute()
	require.NoError(t, err)
}
