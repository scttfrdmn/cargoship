package tar

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/scttfrdmn/cargoship-cli/pkg/config"
	"github.com/scttfrdmn/cargoship-cli/pkg/inventory"
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

	var paths []string
	r := tar.NewReader(f)
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
		paths = append(paths, next.Name)
	}
	require.Equal(t, []string{"name.txt"}, paths)
}

func TestTarFileAddHash(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar"))
	require.NoError(t, err)
	defer f.Close()

	archive := New(f, &config.SuitCaseOpts{
		Format:    "tar",
		HashInner: true,
	})
	defer archive.Close() // nolint: errcheck

	hs, err := archive.Add(inventory.File{
		Path:        "../../testdata/name.txt",
		Destination: "name.txt",
	})
	require.NoError(t, err)

	require.True(t, strings.HasSuffix(hs.Filename, "name.txt"))
	require.Equal(t, "68e6c64a20407c35ebc20d905c941e03c63b3bfe3c853a708a93ec5a95532fbd", hs.Hash)

	require.NoError(t, archive.Close())
}
