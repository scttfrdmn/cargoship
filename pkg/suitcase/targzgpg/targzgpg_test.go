package targzgpg

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/pgzip"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/scttfrdmn/cargoship/pkg/config"
	"github.com/scttfrdmn/cargoship/pkg/gpg"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
	"github.com/stretchr/testify/require"
)

func TestTarGPGFileCorrupt(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)
	archive, err := New(f, &config.SuitCaseOpts{
		EncryptTo: &openpgp.EntityList{pubKey},
	})
	require.NoError(t, err)
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

	// Verify tar reader cannot read encrypted content
	r := tar.NewReader(f)
	_, err = r.Next()
	if err != io.EOF {
		require.EqualError(t, err, "archive/tar: invalid tar header")
	}
}

func TestTarGPGFileWithTar(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	archive, err := New(f, &config.SuitCaseOpts{
		EncryptTo: &openpgp.EntityList{pubKey},
	})

	require.NoError(t, err)
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
	_, err = r.Next()
	if err != io.EOF {
		require.EqualError(t, err, "archive/tar: invalid tar header")
	}
}

func TestTarGZGPGFile(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.gpg"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	archive, err := New(f, &config.SuitCaseOpts{
		Format:    "tar.gz.gpg",
		EncryptTo: &openpgp.EntityList{pubKey},
	})

	require.NoError(t, err)
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

	g, err := pgzip.NewReader(md.UnverifiedBody)
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
	f, err := os.Create(filepath.Join(tmp, "test.tar.gz.gpg"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	opts := &config.SuitCaseOpts{
		Format:    "tar.gz.gpg",
		EncryptTo: &openpgp.EntityList{pubKey},
	}

	archive, err := New(f, opts)

	require.NoError(t, err)
	defer archive.Close() // nolint: errcheck

	// Test Config method
	config := archive.Config()
	require.NotNil(t, config)
	require.Equal(t, opts, config)
	require.Equal(t, "tar.gz.gpg", config.Format)
}

// Test New function error path
func TestNewPanic(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.gz.gpg"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	// Test error when EncryptTo is nil
	_, err = New(f, &config.SuitCaseOpts{
		Format:    "tar.gz.gpg",
		EncryptTo: nil, // This should cause an error
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "EncryptTo is required")
}

// Test Close method error handling
func TestCloseErrors(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.gz.gpg"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	archive, err := New(f, &config.SuitCaseOpts{
		Format:    "tar.gz.gpg",
		EncryptTo: &openpgp.EntityList{pubKey},
	})

	require.NoError(t, err)

	// Add a file to make sure all writers are properly initialized
	_, err = archive.Add(inventory.File{
		Path:        "../../testdata/name.txt",
		Destination: "name.txt",
	})
	require.NoError(t, err)

	// Test successful close
	err = archive.Close()
	require.NoError(t, err)

	// Test that closing again doesn't cause issues
	// (the underlying writers should handle this gracefully)
	_ = archive.Close()
	// This may or may not error depending on the underlying writers
	// but we test it to ensure coverage
}

// Test New function with different parameters
func TestNewWithDifferentOpts(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.gz.gpg"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	// Test with minimal options
	archive, err := New(f, &config.SuitCaseOpts{
		EncryptTo: &openpgp.EntityList{pubKey},
	})
	require.NoError(t, err)
	defer archive.Close() // nolint: errcheck

	// Verify that the archive was created correctly
	require.NotNil(t, archive.tw)
	require.NotNil(t, archive.gw)
	require.NotNil(t, archive.cw)
	require.NotNil(t, archive.opts)
	require.Equal(t, &openpgp.EntityList{pubKey}, archive.opts.EncryptTo)
}

func TestGetHashes(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.gz.gpg"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	archive, err := New(f, &config.SuitCaseOpts{
		Format:    "tar.gz.gpg",
		EncryptTo: &openpgp.EntityList{pubKey},
	})

	require.NoError(t, err)
	defer archive.Close() // nolint: errcheck

	// Test GetHashes method (should return empty slice initially)
	hashes := archive.GetHashes()
	// The slice can be nil or empty initially
	require.True(t, len(hashes) == 0, "Hashes should be empty initially")
}

func TestAddEncrypt(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.gz.gpg"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pubKey, err := gpg.ReadEntity("../../testdata/fakey-public.key")
	require.NoError(t, err)

	archive, err := New(f, &config.SuitCaseOpts{
		Format:    "tar.gz.gpg",
		EncryptTo: &openpgp.EntityList{pubKey},
	})

	require.NoError(t, err)
	defer archive.Close() // nolint: errcheck

	// Test AddEncrypt method (should return error for already encrypted archives)
	err = archive.AddEncrypt(inventory.File{
		Path:        "../../testdata/name.txt",
		Destination: "name.txt",
	})
	require.Error(t, err)
	require.EqualError(t, err, "file encryption not supported on already encrypted archives")
}
