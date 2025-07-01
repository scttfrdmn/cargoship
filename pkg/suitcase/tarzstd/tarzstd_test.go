package tarzstd

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/stretchr/testify/require"
	"github.com/scttfrdmn/cargoship/pkg/config"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
	"github.com/scttfrdmn/cargoship/pkg/gpg"
)

func TestTarZstFile(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.zst"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

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

	gr, err := zstd.NewReader(f)
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
			d, err := io.ReadAll(r)
			require.NoError(t, err)
			require.Equal(t, "Joe the user\n", string(d))
		}
	}

	// require.NotNil(t, archive.GetHashes())
}

func TestConfig(t *testing.T) {
	// Test that Config() returns the correct configuration
	opts := &config.SuitCaseOpts{
		Format:       "tar.zst",
		HashInner:    true,
		EncryptInner: false,
	}
	
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.zst"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	
	archive := New(f, opts)
	defer func() { _ = archive.Close() }()
	
	// Test that Config() returns the same options we passed in
	config := archive.Config()
	require.Equal(t, opts, config)
	require.Equal(t, "tar.zst", config.Format)
	require.True(t, config.HashInner)
	require.False(t, config.EncryptInner)
}

func TestGetHashes(t *testing.T) {
	// Test GetHashes functionality
	opts := &config.SuitCaseOpts{
		Format:    "tar.zst",
		HashInner: false,
	}
	
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.zst"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	
	archive := New(f, opts)
	defer func() { _ = archive.Close() }()
	
	// Initially should be empty
	hashes := archive.GetHashes()
	require.Len(t, hashes, 0)
}

func TestAddEncrypt(t *testing.T) {
	// Create a test GPG key for encryption
	keyOpts := &gpg.KeyOpts{
		Name:    "test",
		Email:   "test@example.com",
		KeyType: "rsa",
		Bits:    1024, // Use smaller key for faster testing
	}
	
	keyPair, err := gpg.NewKeyPair(keyOpts)
	require.NoError(t, err)
	
	// Read the public key to create entity list
	keyFiles, err := gpg.NewKeyFilesWithPair(keyPair, "")
	require.NoError(t, err)
	defer func() {
		for _, kf := range keyFiles {
			_ = os.Remove(kf)
		}
	}()
	
	// Find public key file
	var pubKeyFile string
	for _, kf := range keyFiles {
		if strings.Contains(kf, "public.key") {
			pubKeyFile = kf
			break
		}
	}
	require.NotEmpty(t, pubKeyFile)
	
	// Read entity from public key
	entity, err := gpg.ReadEntity(pubKeyFile)
	require.NoError(t, err)
	
	encryptTo := &openpgp.EntityList{entity}
	
	// Create test archive with encryption
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.zst"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	
	archive := New(f, &config.SuitCaseOpts{
		Format:       "tar.zst",
		EncryptInner: true,
		EncryptTo:    encryptTo,
	})
	defer func() { _ = archive.Close() }()
	
	// Test AddEncrypt with valid file
	err = archive.AddEncrypt(inventory.File{
		Path:        "../../testdata/name.txt",
		Destination: "name.txt",
	})
	require.NoError(t, err)
	
	// Test AddEncrypt with non-existent file
	err = archive.AddEncrypt(inventory.File{
		Path:        "../testdata/never-exist.txt",
		Destination: "never-exist.txt",
	})
	require.Error(t, err)
}

func TestAddEncrypt_InvalidEncryption(t *testing.T) {
	// Test AddEncrypt without encryption entities (should fail)
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.zst"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	
	archive := New(f, &config.SuitCaseOpts{
		Format:       "tar.zst",
		EncryptInner: true,
		EncryptTo:    &openpgp.EntityList{}, // Empty entity list
	})
	defer func() { _ = archive.Close() }()
	
	err = archive.AddEncrypt(inventory.File{
		Path:        "../../testdata/name.txt",
		Destination: "name.txt",
	})
	require.Error(t, err) // Should fail due to empty entity list
}

func TestNew(t *testing.T) {
	// Test New function creates suitcase with correct configuration
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.zst"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	
	opts := &config.SuitCaseOpts{
		Format:    "tar.zst",
		HashInner: true,
	}
	
	suitcase := New(f, opts)
	require.NotNil(t, suitcase.tw)
	require.NotNil(t, suitcase.gw)
	require.Equal(t, opts, suitcase.Config())
	defer func() { _ = suitcase.Close() }()
}

func TestClose(t *testing.T) {
	// Test Close functionality
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.zst"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	
	opts := &config.SuitCaseOpts{
		Format: "tar.zst",
	}
	
	archive := New(f, opts)
	
	// Add some content
	_, err = archive.Add(inventory.File{
		Path:        "../../testdata/name.txt",
		Destination: "name.txt",
	})
	require.NoError(t, err)
	
	// Close should work without error
	err = archive.Close()
	require.NoError(t, err)
}

func TestTarZstdWithHashing(t *testing.T) {
	// Test with hashing enabled
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "test.tar.zst"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	archive := New(f, &config.SuitCaseOpts{
		Format:    "tar.zst",
		HashInner: true,
	})
	defer func() { _ = archive.Close() }()

	hs, err := archive.Add(inventory.File{
		Path:        "../../testdata/name.txt",
		Destination: "name.txt",
	})
	require.NoError(t, err)
	require.NotNil(t, hs)
	require.NotEmpty(t, hs.Hash)

	require.NoError(t, archive.Close())
}
