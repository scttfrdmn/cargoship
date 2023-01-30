package targpg

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/gpg"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
)

func TestTarGPGFileCorrupt(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar"))
	require.NoError(t, err)
	defer f.Close()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)
	archive := New(f, &config.SuitCaseOpts{
		EncryptTo: &openpgp.EntityList{pubKey},
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

	// var paths []string
	r := tar.NewReader(f)
	for {
		_, err := r.Next()
		if err == io.EOF {
			break
		}
		require.EqualError(t, err, "archive/tar: invalid tar header")

		break // nolint need to do something better here
	}
	// require.Equal(t, []string{"name.txt"}, paths)
}

func TestTarGPGFileWithTar(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar"))
	require.NoError(t, err)
	defer f.Close()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	archive := New(f, &config.SuitCaseOpts{
		EncryptTo: &openpgp.EntityList{pubKey},
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
	defer f.Close()

	// Make sure a normal tar reader can't actually open this
	r := tar.NewReader(f)
	for {
		_, err := r.Next()
		if err == io.EOF {
			break
		}
		require.EqualError(t, err, "archive/tar: invalid tar header")

		break // nolint need to do something better here
	}
}

func TestTarGPGFile(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.gpg"))
	require.NoError(t, err)
	defer f.Close()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	archive := New(f, &config.SuitCaseOpts{
		Format:    "tar.gpg",
		EncryptTo: &openpgp.EntityList{pubKey},
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

	privk, err := os.Open("../../testdata/fakey-private.key")
	require.NoError(t, err)
	defer privk.Close()

	entityList, err := openpgp.ReadArmoredKeyRing(privk)
	require.NoError(t, err)

	md, err := openpgp.ReadMessage(f, entityList, nil, nil)
	require.NoError(t, err)

	// Make sure a normal tar reader can't actually open this
	r := tar.NewReader(md.UnverifiedBody)
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
