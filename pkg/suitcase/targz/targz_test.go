package targz

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/inventory"
)

func TestTarGzFile(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.gz"))
	require.NoError(t, err)
	defer f.Close()

	archive := New(f, &config.SuitCaseOpts{
		Format: "tar.gz",
	})
	defer archive.Close() // nolint: errcheck

	_, err = archive.Add(inventory.InventoryFile{
		Path:        "../testdata/never-exist.txt",
		Destination: "never-exist.txt",
	})
	require.Error(t, err)
	_, err = archive.Add(inventory.InventoryFile{
		Path:        "../../testdata/name.txt",
		Destination: "name.txt",
	})
	require.NoError(t, err)

	require.NoError(t, archive.Close())

	// Ok, now lets look at it
	f, err = os.Open(f.Name())
	require.NoError(t, err)

	/*
		md, err := openpgp.ReadMessage(f, entityList, nil, nil)
		require.NoError(t, err)
	*/
	gr, err := gzip.NewReader(f)
	require.NoError(t, err)

	// Make sure a normal tar reader can't actually open this
	r := tar.NewReader(gr)
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

	}
}
