package gpg

import (
	"os"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestReadEntity(t *testing.T) {
	got, err := ReadEntity("../testdata/fakey-public.key")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.IsType(t, &openpgp.Entity{}, got)
}

func TestEncryptToWithCmd(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringArray("public-key", []string{"../testdata/fakey-public.key"}, "")
	cmd.Flags().Bool("exclude-systems-pubkeys", false, "")
	_, err := EncryptToWithCmd(cmd)
	require.NoError(t, err)
}

func TestEncrypt(t *testing.T) {
	d := []byte("hello world")
	encryptionKey, err := ReadEntity("../testdata/fakey-public.key")
	require.NoError(t, err)
	// Non Armored test
	got, err := Encrypt(d, &openpgp.EntityList{encryptionKey}, false)
	require.NoError(t, err)
	require.NotNil(t, got)
	// Armor the encrypted content
	got, err = Encrypt(d, &openpgp.EntityList{encryptionKey}, true)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestFileInfo(t *testing.T) {
	// Create a mock os.FileInfo for testing
	data := []byte("test data for file")

	// Create a temporary file to get real FileInfo
	tempFile, err := os.CreateTemp("", "test-file.txt")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tempFile.Name()) }()
	defer func() { _ = tempFile.Close() }()

	// Write some data and get FileInfo
	_, err = tempFile.Write(data)
	require.NoError(t, err)

	origFileInfo, err := tempFile.Stat()
	require.NoError(t, err)

	// Test FileInfo implementation
	fileInfo, err := NewFileInfo(data, origFileInfo)
	require.NoError(t, err)

	require.Equal(t, origFileInfo.Name()+".gpg", fileInfo.Name())
	require.Equal(t, int64(len(data)), fileInfo.Size())
	require.Equal(t, origFileInfo.Mode(), fileInfo.Mode())
	require.False(t, fileInfo.IsDir())
	require.Nil(t, fileInfo.Sys())

	// ModTime should return the original file's time
	modTime := fileInfo.ModTime()
	require.Equal(t, origFileInfo.ModTime(), modTime)
}

func TestFileInfo_ZeroSize(t *testing.T) {
	// Test with zero size data
	data := []byte{}

	// Create a temporary file to get real FileInfo
	tempFile, err := os.CreateTemp("", "empty.txt")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tempFile.Name()) }()
	defer func() { _ = tempFile.Close() }()

	origFileInfo, err := tempFile.Stat()
	require.NoError(t, err)

	fileInfo, err := NewFileInfo(data, origFileInfo)
	require.NoError(t, err)

	require.Equal(t, origFileInfo.Name()+".gpg", fileInfo.Name())
	require.Equal(t, int64(0), fileInfo.Size())
}

func TestFileInfo_LargeFile(t *testing.T) {
	// Test with large data (simulate large file)
	largeDataSize := 1024 * 1024 // 1MB for testing
	largeData := make([]byte, largeDataSize)

	// Create a temporary file to get real FileInfo
	tempFile, err := os.CreateTemp("", "large-file.bin")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tempFile.Name()) }()
	defer func() { _ = tempFile.Close() }()

	origFileInfo, err := tempFile.Stat()
	require.NoError(t, err)

	fileInfo, err := NewFileInfo(largeData, origFileInfo)
	require.NoError(t, err)

	require.Equal(t, origFileInfo.Name()+".gpg", fileInfo.Name())
	require.Equal(t, int64(largeDataSize), fileInfo.Size())
}

// Additional tests to push coverage over 80%

func TestReadEntity_FileNotFound(t *testing.T) {
	_, err := ReadEntity("nonexistent-file.key")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such file or directory")
}

func TestEncryptToWithCmd_ExcludeSystems(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringArray("public-key", []string{"../testdata/fakey-public.key"}, "")
	cmd.Flags().Bool("exclude-systems-pubkeys", true, "")

	el, err := EncryptToWithCmd(cmd)
	require.NoError(t, err)
	require.Len(t, *el, 1) // Only the specified key, no system keys
}

func TestCollectGPGPubKeys_NoKeysFound(t *testing.T) {
	// Create a temporary directory with no .gpg files
	tempDir := t.TempDir()

	_, err := CollectGPGPubKeys(tempDir)
	require.Error(t, err)
	require.Equal(t, "no gpg keys found", err.Error())
}

// Test CollectGPGPubKeys with invalid GPG files
func TestCollectGPGPubKeys_InvalidGPGFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create a file with .gpg extension but invalid content
	invalidGPGFile := tempDir + "/invalid.gpg"
	err := os.WriteFile(invalidGPGFile, []byte("invalid gpg content"), 0644)
	require.NoError(t, err)

	// Should return error as no valid keys found
	_, err = CollectGPGPubKeys(tempDir)
	require.Error(t, err)
	require.Equal(t, "no gpg keys found", err.Error())
}

// Test CollectGPGPubKeys with valid GPG files
func TestCollectGPGPubKeys_ValidKeys(t *testing.T) {
	tempDir := t.TempDir()

	// Copy the test GPG file to temp directory
	testKey := "../testdata/fakey-public.key"
	testKeyContent, err := os.ReadFile(testKey)
	require.NoError(t, err)

	gpgFile := tempDir + "/test.gpg"
	err = os.WriteFile(gpgFile, testKeyContent, 0644)
	require.NoError(t, err)

	// Should find the valid key
	els, err := CollectGPGPubKeys(tempDir)
	require.NoError(t, err)
	require.NotNil(t, els)
	require.Len(t, *els, 1)
}

// Test EncryptToWithCmd with no public keys specified
func TestEncryptToWithCmd_NoKeys(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringArray("public-key", []string{}, "")
	cmd.Flags().Bool("exclude-systems-pubkeys", true, "")

	// Should not fail when no keys are specified
	el, err := EncryptToWithCmd(cmd)
	require.NoError(t, err)
	require.NotNil(t, el)
	require.Len(t, *el, 0)
}

// Test EncryptToWithCmd with invalid public key file
func TestEncryptToWithCmd_InvalidKeyFile(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringArray("public-key", []string{"nonexistent.key"}, "")
	cmd.Flags().Bool("exclude-systems-pubkeys", true, "")

	_, err := EncryptToWithCmd(cmd)
	require.Error(t, err)
}

// Test Encrypt function error paths
func TestEncrypt_ErrorPaths(t *testing.T) {
	// Test with empty entity list
	d := []byte("test data")
	emptyEntityList := &openpgp.EntityList{}

	_, err := Encrypt(d, emptyEntityList, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad Cipher")

	// Test armored version with empty entity list
	_, err = Encrypt(d, emptyEntityList, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad Cipher")
}

// Test ReadEntity with invalid file content
func TestReadEntity_InvalidContent(t *testing.T) {
	// Create a temporary file with invalid GPG content
	tempFile, err := os.CreateTemp("", "invalid.key")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tempFile.Name()) }()
	defer func() { _ = tempFile.Close() }()

	// Write invalid content
	_, err = tempFile.WriteString("invalid gpg key content")
	require.NoError(t, err)
	_ = tempFile.Close()

	_, err = ReadEntity(tempFile.Name())
	require.Error(t, err)
}

// Test EncryptToWithCmd with missing flags
func TestEncryptToWithCmd_MissingFlags(t *testing.T) {
	cmd := &cobra.Command{}
	// Don't set any flags to test error paths

	_, err := EncryptToWithCmd(cmd)
	require.Error(t, err) // Should fail due to missing flags
}
