package tarzstgpg

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
	"github.com/scttfrdmn/cargoship/pkg/config"
	"github.com/scttfrdmn/cargoship/pkg/gpg"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
)

func TestTarGPGFileCorrupt(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)
	archive := New(f, &config.SuitCaseOpts{
		EncryptTo: &openpgp.EntityList{pubKey},
	})
	defer archive.Close() // nolint: errcheck

	/*
		_, err = archive.Add(inventory.InventoryFile{
			Path:        "../../testdata/name.txt",
			Destination: "name.txt",
		})
		require.Error(t, err)
	*/
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

		break // nolint removing this is bad...
	}
	// require.Equal(t, []string{"name.txt"}, paths)
}

func TestTarGPGFileWithTar(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

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
	defer func() { _ = f.Close() }()

	// Make sure a normal tar reader can't actually open this
	r := tar.NewReader(f)
	for {
		_, err := r.Next()
		if err == io.EOF {
			break
		}
		require.EqualError(t, err, "archive/tar: invalid tar header")

		break // nolint removing this is bad...
	}
}

func TestTarGZGPGFile(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.gpg"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	archive := New(f, &config.SuitCaseOpts{
		Format:    "tar.gz.gpg",
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
	defer func() { _ = privk.Close() }()

	entityList, err := openpgp.ReadArmoredKeyRing(privk)
	require.NoError(t, err)

	md, err := openpgp.ReadMessage(f, entityList, nil, nil)
	require.NoError(t, err)

	g, err := zstd.NewReader(md.UnverifiedBody)
	require.NoError(t, err)
	// Make sure a normal tar reader can't actually open this
	r := tar.NewReader(g)
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

// Test 0% coverage functions
func TestConfig(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.zst.gpg"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	opts := &config.SuitCaseOpts{
		Format:    "tar.zst.gpg",
		EncryptTo: &openpgp.EntityList{pubKey},
	}

	archive := New(f, opts)
	defer archive.Close() // nolint: errcheck

	// Test Config method
	config := archive.Config()
	require.NotNil(t, config)
	require.Equal(t, opts, config)
	require.Equal(t, "tar.zst.gpg", config.Format)
}

func TestGetHashes(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.zst.gpg"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	archive := New(f, &config.SuitCaseOpts{
		Format:    "tar.zst.gpg",
		EncryptTo: &openpgp.EntityList{pubKey},
	})
	defer archive.Close() // nolint: errcheck

	// Test GetHashes method (should return empty slice initially)
	hashes := archive.GetHashes()
	// The slice can be nil or empty initially
	require.True(t, len(hashes) == 0, "Hashes should be empty initially")
}

func TestAddEncrypt(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.zst.gpg"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	archive := New(f, &config.SuitCaseOpts{
		Format:    "tar.zst.gpg",
		EncryptTo: &openpgp.EntityList{pubKey},
	})
	defer archive.Close() // nolint: errcheck

	// Test AddEncrypt method (should return error for already encrypted archives)
	err = archive.AddEncrypt(inventory.File{
		Path:        "../../testdata/name.txt",
		Destination: "name.txt",
	})
	require.Error(t, err)
	require.EqualError(t, err, "file encryption not supported on already encrypted archives")
}

// Test panic condition when EncryptTo is nil (covers missing New function coverage)
func TestNewWithNilEncryptTo(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			require.Contains(t, fmt.Sprint(r), "NEED ENCRYPT TO")
		} else {
			t.Error("Expected panic when EncryptTo is nil")
		}
	}()

	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	// This should panic
	_ = New(f, &config.SuitCaseOpts{
		EncryptTo: nil, // This will trigger panic
	})
}

// Test error paths in Close function by closing underlying resources
func TestCloseWithErrors(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar"))
	require.NoError(t, err)

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	archive := New(f, &config.SuitCaseOpts{
		EncryptTo: &openpgp.EntityList{pubKey},
	})

	// Add a file first to make the archive valid
	_, err = archive.Add(inventory.File{
		Path:        "../../testdata/name.txt",
		Destination: "name.txt",
	})
	require.NoError(t, err)

	// Force close the underlying file to potentially trigger errors
	_ = f.Close()

	// This may or may not error depending on the implementation,
	// but it exercises the error paths in Close()
	_ = archive.Close()
}
