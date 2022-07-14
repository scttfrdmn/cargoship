package tar

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/inventory"
)

func TestTarFile(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar"))
	require.NoError(t, err)
	defer f.Close()

	archive := New(f, &config.SuitCaseOpts{
		Format: "tar",
	})
	defer archive.Close() // nolint: errcheck

	require.Error(t, archive.Add(inventory.InventoryFile{
		Path:        "../testdata/never-exist.txt",
		Destination: "never-exist.txt",
	}))
	require.NoError(t, archive.Add(inventory.InventoryFile{
		Path:        "../../testdata/name.txt",
		Destination: "name.txt",
	}))

	require.NoError(t, archive.Close())

	// Ok, now lets look at it
	f, err = os.Open(f.Name())
	require.NoError(t, err)

	var paths []string
	r := tar.NewReader(f)
	for {
		next, err := r.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		if next.Name == "name.txt" {
			d, err := ioutil.ReadAll(r)
			require.NoError(t, err)
			require.Equal(t, "Joe the user\n", string(d))
		}
		paths = append(paths, next.Name)
	}
	require.Equal(t, []string{"name.txt"}, paths)
}
