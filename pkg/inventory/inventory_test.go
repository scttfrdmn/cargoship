package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewDirectoryInventory(t *testing.T) {
	got, err := NewDirectoryInventory(&DirectoryInventoryOptions{
		TopLevelDirectories: []string{"../testdata/fake-dir"},
	})

	require.NoError(t, err)
	require.IsType(t, &DirectoryInventory{}, got)

	require.Greater(t, len(got.Files), 0)
}

func TestIndexInventory(t *testing.T) {
	i := &DirectoryInventory{
		Files: []*InventoryFile{
			{
				Path: "small-file-1",
				Size: 1,
			},
			{
				Path: "small-file-2",
				Size: 2,
			},
			{
				Path: "big-file-1",
				Size: 3,
			},
		},
	}
	err := IndexInventory(i, 3)
	require.NoError(t, err)
	require.Equal(t, 2, i.TotalIndexes)
}

func TestIndexInventoryTooBig(t *testing.T) {
	i := &DirectoryInventory{
		Files: []*InventoryFile{
			{
				Path: "small-file-1",
				Size: 1,
			},
			{
				Path: "small-file-2",
				Size: 4,
			},
			{
				Path: "big-file-1",
				Size: 3,
			},
		},
	}
	err := IndexInventory(i, 3)
	require.EqualError(t, err, "index containes at least one file that is too large")
	require.Equal(t, 0, i.TotalIndexes)
}

func TestNewDirectoryInventoryMissingTopDirs(t *testing.T) {
	_, err := NewDirectoryInventory(&DirectoryInventoryOptions{
		TopLevelDirectories: []string{},
	})
	require.Error(t, err)
}
