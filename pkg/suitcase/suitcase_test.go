package suitcase

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/inventory"
)

func TestSuitcase(t *testing.T) {
	folder := t.TempDir()
	empty, err := os.Create(folder + "/empty.txt")
	require.NoError(t, err)
	require.NoError(t, empty.Close())
	require.NoError(t, os.Mkdir(folder+"/folder-inside", 0o755))

	for _, format := range []string{"tar", "tar.gz"} {
		format := format
		t.Run(format, func(t *testing.T) {
			archive, err := New(io.Discard, &config.SuitCaseOpts{
				Format: format,
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, archive.Close())
			})
			require.NoError(t, archive.Add(inventory.InventoryFile{
				Path:        empty.Name(),
				Destination: "empty.txt",
			}))
			require.Error(t, archive.Add(inventory.InventoryFile{
				Path:        empty.Name() + "_nope",
				Destination: "dont.txt",
			}))
		})
	}

	t.Run("7z", func(t *testing.T) {
		_, err := New(io.Discard, &config.SuitCaseOpts{
			Format: "7z",
		})
		require.EqualError(t, err, "invalid archive format: 7z")
	})
}
