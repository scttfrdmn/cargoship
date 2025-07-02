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
