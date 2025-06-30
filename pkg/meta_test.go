package porter

import (
	"bytes"
	"path"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestNewCLIMetaWithCobra(t *testing.T) {
	cmd := &cobra.Command{
		Version: "1.2.3",
	}
	_ = cmd.Execute() // Test helper

	got := NewCLIMetaWithCobra(cmd, []string{})
	require.NotNil(t, got)

	// Make sure at least 1 known string shows up
	var buf bytes.Buffer
	got.Print(&buf)
	require.Contains(t, buf.String(), "hostname:")
	require.Contains(t, buf.String(), `version: 1.2.3`)

	// Make sure the close works
	tdir := t.TempDir()
	complete := got.MustComplete(tdir)
	require.Equal(t, path.Join(tdir, "cargoship-invocation-meta.yaml"), complete)
	require.FileExists(t, complete)
}

func TestNewCLIMeta(t *testing.T) {
	started := time.Date(2009, 11, 17, 20, 34, 58, 651387237, time.UTC)

	got := NewCLIMeta(
		WithStart(&started),
		WithMetaVersion("1.2.3"),
	)
	require.NotNil(t, got)
	require.Equal(t, &started, got.StartedAt)
	require.Equal(t, "1.2.3", got.Version)
}
