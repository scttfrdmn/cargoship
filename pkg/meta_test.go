package porter

import (
	"bytes"
	"path"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestNewCLIMeta(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Execute()

	got := NewCLIMeta(cmd, []string{})
	require.NotNil(t, got)

	// Make sure at least 1 known string shows up
	var buf bytes.Buffer
	got.Print(&buf)
	require.Contains(t, buf.String(), "hostname:")

	// Make sure the close works
	tdir := t.TempDir()
	complete, err := got.Complete(tdir)
	require.NoError(t, err)
	require.Equal(t, path.Join(tdir, "suitcasectl-invocation-meta.yaml"), complete)
	require.FileExists(t, complete)
}
