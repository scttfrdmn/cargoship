package tarbz2

import (
	"archive/tar"
	"compress/bzip2"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
)

func TestTarGzFile(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.bz2"))
	require.NoError(t, err)
	defer f.Close()

	archive := New(f, &config.SuitCaseOpts{
		Format: "tar.zst",
	})
	defer archive.Close() // nolint: errcheck

	_, err = archive.Add(inventory.File{
		Path:        "../testdata/never-exist.txt",
		Destination: "never-exist.txt",
	})
	require.Error(t, err)
	_, err = archive.Add(inventory.File{
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
	gr := bzip2.NewReader(f)

	// Make sure a normal tar reader can't actually open this
	r := tar.NewReader(gr)
	for {
		next, err := r.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		if next.Name == "name.txt" {
			d, err := io.ReadAll(r)
			require.NoError(t, err)
			require.Equal(t, "Joe the user\n", string(d))
		}
	}
}
