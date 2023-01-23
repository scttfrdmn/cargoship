package inventory

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"

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

func TestExpandInventoryWithNames(t *testing.T) {
	i := &DirectoryInventory{
		Options: &DirectoryInventoryOptions{
			Prefix: "foo",
			User:   "bar",
		},
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

	err = ExpandSuitcaseNames(i)
	require.NoError(t, err)
	require.Equal(t, ExtractSuitcaseNames(i), []string{"foo-bar-01-of-02.tar", "foo-bar-02-of-02.tar", "foo-bar-02-of-02.tar"})
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

func TestGetMetadataGlob(t *testing.T) {
	got, err := GetMetadataWithGlob("../testdata/fake-dir/suitcase-meta*")
	require.NoError(t, err)
	require.IsType(t, map[string]string{}, got)
	for title, content := range got {
		if strings.HasSuffix(title, "/suitcase-meta.txt") {
			require.Equal(t, "Text metadata\n", content)
		} else if strings.HasSuffix(title, "/suitcase-meta.md") {
			require.Equal(t, "# Markdown Metadata\n", content)
		} else {
			require.Fail(t, "unexpected title: %s", title)
		}
	}
}

func TestGetMetadataFiles(t *testing.T) {
	got, err := GetMetadataWithFiles([]string{"../testdata/fake-dir/suitcase-meta.txt", "../testdata/fake-dir/suitcase-meta.md"})
	require.NoError(t, err)
	require.IsType(t, map[string]string{}, got)
	for title, content := range got {
		if strings.HasSuffix(title, "/suitcase-meta.txt") {
			require.Equal(t, "Text metadata\n", content)
		} else if strings.HasSuffix(title, "/suitcase-meta.md") {
			require.Equal(t, "# Markdown Metadata\n", content)
		} else {
			require.Fail(t, "unexpected title: %s", title)
		}
	}
}

func TestNewInventoryerWithFilename(t *testing.T) {
	tests := []struct {
		filename     string
		expectedType string
	}{
		{
			filename:     "thing.yaml",
			expectedType: "*inventory.VAMLer",
		},
		{
			filename:     "thing.yml",
			expectedType: "*inventory.VAMLer",
		},
		{
			filename:     "thing.json",
			expectedType: "*inventory.EJSONer",
		},
	}
	for _, tt := range tests {
		got, err := NewInventoryerWithFilename(tt.filename)
		require.NoError(t, err)
		// log.Fatal().Msgf("%+v", reflect.TypeOf(*got))
		require.Equal(t, tt.expectedType, fmt.Sprintf("%+v", reflect.TypeOf(got)))
	}
}

func TestNewInventoryerWithBadFilename(t *testing.T) {
	tests := []struct {
		filename string
	}{
		{
			filename: "thing.thing",
		},
		{
			filename: "thing",
		},
		{
			filename: "thing.jsn",
		},
	}
	for _, tt := range tests {
		_, err := NewInventoryerWithFilename(tt.filename)
		require.Error(t, err)
	}
}

func TestNewSuitcaseWithIgnoreGlobs(t *testing.T) {
	i, err := NewDirectoryInventory(&DirectoryInventoryOptions{
		TopLevelDirectories: []string{"../testdata/fake-dir"},
		IgnoreGlobs:         []string{"*.out"},
	})
	require.NoError(t, err)
	for _, f := range i.Files {
		require.NotContains(t, f.Name, ".out")
	}
}

func TestNewSuitcaseWithFollowSymlinks(t *testing.T) {
	i, err := NewDirectoryInventory(&DirectoryInventoryOptions{
		TopLevelDirectories: []string{"../testdata/fake-dir"},
		FollowSymlinks:      true,
	})
	require.NoError(t, err)
	paths := []string{}
	for _, f := range i.Files {
		paths = append(paths, f.Path)
	}
	require.Contains(t, paths, "../testdata/fake-dir/external-symlink/this-is-an-external-data-file.txt")
}

func TestNewSuitcaseWithNoFollowSymlinks(t *testing.T) {
	i, err := NewDirectoryInventory(&DirectoryInventoryOptions{
		TopLevelDirectories: []string{"../testdata/fake-dir"},
		FollowSymlinks:      false,
	})
	require.NoError(t, err)
	paths := []string{}
	for _, f := range i.Files {
		paths = append(paths, f.Path)
	}
	require.NotContains(t, paths, "../testdata/fake-dir/external-symlink/this-is-an-external-data-file.txt")
}

func TestNewDirectoryInventoryOptionsWithViper(t *testing.T) {
	v := viper.New()
	_, err := NewDirectoryInventoryWithViper(v, []string{"../testdata/fake-dir"})
	require.NoError(t, err)
}

func TestWriteOutDirectoryInventoryAndFileAndInventoyerWithViper(t *testing.T) {
	f := t.TempDir()
	v := viper.New()
	i, gf, err := WriteOutDirectoryInventoryAndFileAndInventoyerWithViper(v, []string{"../testdata/fake-dir"}, f, "testing")
	require.NoError(t, err)
	require.FileExists(t, gf.Name())
	require.NotNil(t, i)
}
