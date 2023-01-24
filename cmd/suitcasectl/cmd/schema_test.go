package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestSchemaRunE(t *testing.T) {
	err := schemaRunE(&cobra.Command{}, []string{})
	require.NoError(t, err)
}
