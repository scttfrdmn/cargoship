package inventory

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"

	"github.com/stretchr/testify/require"
)

func TestNewOptions(t *testing.T) {
	// Check some overrides
	o := NewOptions(
		WithUser("foo"),
		WithPrefix("pre"),
		WithMaxSuitcaseSize(500),
		WithLimitFileCount(10),
		WithInventoryFormat("json"),
		WithSuitcaseFormat("tar.gz"),
	)
	require.Equal(t, "foo", o.User)
	require.Equal(t, "pre", o.Prefix)
	require.Equal(t, 10, o.LimitFileCount)
	require.Equal(t, int64(500), o.MaxSuitcaseSize)
	require.Equal(t, "json", o.InventoryFormat)
	require.Equal(t, "tar.gz", o.SuitcaseFormat)

	// Check some defaults
	d := NewOptions()
	require.Equal(t, "tar.zst", d.SuitcaseFormat)
	require.Equal(t, "yaml", d.InventoryFormat)
}

func TestNewDirectoryInventory(t *testing.T) {
	got, err := NewDirectoryInventory(NewOptions(WithDirectories([]string{"../testdata/fake-dir"})))

	require.NoError(t, err)
	require.IsType(t, &DirectoryInventory{}, got)

	require.Greater(t, len(got.Files), 0)
}

func BenchmarkNewDirectoryInventory(b *testing.B) {
	datasets := map[string]struct {
		path string
	}{
		"672M-american-gut": {
			path: "American-Gut",
		},
		"3.3G-Synthetic-cell-images": {
			path: "BBBC005_v1_images",
		},
	}
	bdd := os.Getenv("BENCHMARK_DATA_DIR")
	if bdd == "" {
		bdd = "../../benchmark_data/"
	}

	zerolog.SetGlobalLevel(zerolog.FatalLevel)
	for desc, dataset := range datasets {
		location := path.Join(bdd, dataset.path)
		if _, err := os.Stat(location); err == nil {
			for _, format := range []string{"yaml", "json"} {
				format := format
				b.Run(fmt.Sprintf("suitcase_new_inventory_%v_%v", format, desc), func(b *testing.B) {
					got, err := NewDirectoryInventory(NewOptions(
						WithDirectories([]string{location}),
						WithInventoryFormat(format),
					))
					require.NoError(b, err)
					require.NotNil(b, got)
				})
			}
		}
	}
}

func TestIndexInventory(t *testing.T) {
	i := &DirectoryInventory{
		Files: []*File{
			{Path: "small-file-1", Size: 1},
			{Path: "small-file-2", Size: 2},
			{Path: "big-file-1", Size: 3},
		},
		Options: &Options{},
	}
	err := i.IndexWithSize(3)
	require.NoError(t, err)
	require.Equal(t, 2, i.TotalIndexes)
}

func TestExpandInventoryWithNames(t *testing.T) {
	i := &DirectoryInventory{
		Options: NewOptions(
			WithPrefix("foo"),
			WithUser("bar"),
			WithSuitcaseFormat("tar"),
		),
		Files: []*File{
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
	err := i.IndexWithSize(3)
	require.NoError(t, err)
	require.Equal(t, 2, i.TotalIndexes)

	i.expandSuitcaseNames()
	require.NoError(t, err)
	require.Equal(t, i.SuitcaseNames(), []string{"foo-bar-01-of-02.tar", "foo-bar-02-of-02.tar", "foo-bar-02-of-02.tar"})
	require.Equal(t, i.UniqueSuitcaseNames(), []string{"foo-bar-01-of-02.tar", "foo-bar-02-of-02.tar"})
}

func TestIndexInventoryTooBig(t *testing.T) {
	i := &DirectoryInventory{
		Files: []*File{
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
	err := i.IndexWithSize(3)
	require.EqualError(t, err, "index contains at least one file that is too large")
	require.Equal(t, 0, i.TotalIndexes)
}

func TestNewDirectoryInventoryMissingTopDirs(t *testing.T) {
	_, err := NewDirectoryInventory(NewOptions(
		WithDirectories([]string{}),
	))
	require.Error(t, err)
}

func TestGetMetadataGlob(t *testing.T) {
	got, err := GetMetadataWithGlob("../testdata/fake-dir/suitcase-meta*")
	require.NoError(t, err)
	require.IsType(t, map[string]string{}, got)
	for title, content := range got {
		switch {
		case strings.HasSuffix(title, "/suitcase-meta.txt"):
			require.Equal(t, "Text metadata\n", content)
		case strings.HasSuffix(title, "/suitcase-meta.md"):
			require.Equal(t, "# Markdown Metadata\n", content)
		default:
			require.Fail(t, "unexpected title: %s", title)
		}
	}
}

func TestGetMetadataFiles(t *testing.T) {
	got, err := GetMetadataWithFiles([]string{"../testdata/fake-dir/suitcase-meta.txt", "../testdata/fake-dir/suitcase-meta.md"})
	require.NoError(t, err)
	require.IsType(t, map[string]string{}, got)
	for title, content := range got {
		switch {
		case strings.HasSuffix(title, "/suitcase-meta.txt"):
			require.Equal(t, "Text metadata\n", content)
		case strings.HasSuffix(title, "/suitcase-meta.md"):
			require.Equal(t, "# Markdown Metadata\n", content)
		default:
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
	i, err := NewDirectoryInventory(NewOptions(
		WithDirectories([]string{"../testdata/fake-dir"}),
		WithIgnoreGlobs([]string{"*.out"}),
	))
	require.NoError(t, err)
	for _, f := range i.Files {
		require.NotContains(t, f.Name, ".out")
	}
}

func TestNewSuitcaseWithFollowSymlinks(t *testing.T) {
	i, err := NewDirectoryInventory(NewOptions(
		WithDirectories([]string{"../testdata/fake-dir"}),
		WithFollowSymlinks(),
	))
	require.NoError(t, err)
	paths := []string{}
	for _, f := range i.Files {
		paths = append(paths, f.Path)
	}
	// Get absolute path of the expected file
	abPath, err := filepath.Abs("../testdata/fake-dir/external-symlink/this-is-an-external-data-file.txt")
	require.NoError(t, err)
	require.Contains(t, paths, abPath)
}

func TestNewSuitcaseWithNoFollowSymlinks(t *testing.T) {
	i, err := NewDirectoryInventory(NewOptions(
		WithDirectories([]string{"../testdata/fake-dir"}),
	))
	require.NoError(t, err)
	paths := []string{}
	for _, f := range i.Files {
		paths = append(paths, f.Path)
	}
	require.NotContains(t, paths, "../testdata/fake-dir/external-symlink/this-is-an-external-data-file.txt")
}

func TestNewDirectoryInventoryOptionsWithViper(t *testing.T) {
	v := viper.New()
	cmd := &cobra.Command{}
	BindCobra(cmd)
	cmd.Execute()
	_, err := NewDirectoryInventoryWithViper(v, cmd, []string{"../testdata/fake-dir"})
	require.NoError(t, err)
}

func TestWriteOutDirectoryInventoryAndFileAndInventoyerWithViper(t *testing.T) {
	f := t.TempDir()
	v := viper.New()
	c := &cobra.Command{}
	opts := &config.SuitCaseOpts{
		Destination: f,
	}
	ctx := context.WithValue(context.Background(), SuitcaseOptionsKey, opts)
	c.SetContext(ctx)
	cmd := newInventoryCmd()
	cmd.Execute()
	i, gf, err := WriteInventoryAndFileWithViper(v, cmd, []string{"../testdata/fake-dir"}, "testing")
	require.NoError(t, err)
	require.FileExists(t, gf.Name())
	require.NotNil(t, i)
}

func TestWalkDirLimit(t *testing.T) {
	i := DirectoryInventory{}
	err := walkDir("../testdata/limit-dir", NewOptions(
		WithLimitFileCount(10),
	), &i)
	require.Equal(t, 10, len(i.Files))
	require.EqualError(t, err, "halt")
}

func TestWalkDirExpandArchives(t *testing.T) {
	i := DirectoryInventory{}
	err := walkDir("../testdata/archives", NewOptions(
		WithArchiveTOC(),
	), &i)
	require.NoError(t, err)
	require.Contains(
		t,
		i.Files,
		&File{
			Path:        "../testdata/archives/archive.tar.gz",
			Destination: "/archive.tar.gz",
			Name:        "archive.tar.gz",
			Size:        193,
			ArchiveTOC: []string{
				"archives/file1.txt",
				"archives/sub/file2.txt",
				"archives/thing.png",
			},
			SuitcaseIndex: 0,
			SuitcaseName:  "",
		},
	)
}

func TestCreateOrReadInventory(t *testing.T) {
	cmd := newInventoryCmd()
	cmd.Execute()
	got, err := CreateOrReadInventory("", cmd, []string{"../testdata/limit-dir"}, "dev")
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestWithViper(t *testing.T) {
	v := viper.New()
	v.Set("internal-metadata-glob", "bar")
	v.Set("prefix", "pre")
	v.Set("external-metadata-file", []string{"data.txt"})
	v.Set("ignore-glob", []string{"*.swp"})
	v.Set("encrypt-inner", true)
	v.Set("follow-symlinks", true)
	v.Set("suitcase-format", "tar.gz")
	v.Set("max-suitcase-size", "2.5Gi")

	got := NewOptions(
		WithDirectories([]string{"../testdata/limit-dir"}),
		WithViper(v),
	)
	require.Equal(t, "bar", got.InternalMetadataGlob)
	require.Equal(t, []string{"data.txt"}, got.ExternalMetadataFiles)
	require.Equal(t, []string{"*.swp"}, got.IgnoreGlobs)
	require.Equal(t, "pre", got.Prefix)
	require.True(t, got.EncryptInner)
	require.True(t, got.FollowSymlinks)
	require.Equal(t, "tar.gz", got.SuitcaseFormat)
	require.Equal(t, int64(2684354560), got.MaxSuitcaseSize)
}

func TestGenericSetUser(t *testing.T) {
	// Test with cobra command
	cmd := &cobra.Command{}
	BindCobra(cmd)
	cmd.SetArgs([]string{"--user", "cobra-user"})
	err := cmd.Execute()
	require.NoError(t, err)
	o := NewOptions()
	setUser(*cmd, o)
	require.Equal(t, "cobra-user", o.User)

	// Test with viper
	o = NewOptions()
	v := viper.New()
	v.Set("user", "viper-user")
	setUser(*v, o)
	require.Equal(t, "viper-user", o.User)
}

func TestNewInventoryWithFilename(t *testing.T) {
	testD := t.TempDir()
	invf := filepath.Join(testD, "inventory.yaml")
	fh, err := os.Create(invf)
	require.NoError(t, err)
	err = fh.Close()
	require.NoError(t, err)
	i, err := NewInventoryWithFilename(invf)
	require.NoError(t, err)
	require.NotNil(t, i)
	// require.Equal(t, 5, len(i.Files))
}

func TestInventorySearch(t *testing.T) {
	i := DirectoryInventory{
		Files: []*File{
			{
				Path:         "/foo/bar/baz/thing.txt",
				Destination:  "bar/baz/thing.txt",
				Name:         "thing.txt",
				Size:         1000,
				SuitcaseName: "suitcase-foo-01-of-05.tar.tsz",
			},
			{
				Path:         "/foo/bar/baz/another.txt",
				Destination:  "bar/baz/another.txt",
				Name:         "another.txt",
				Size:         2000,
				SuitcaseName: "suitcase-foo-02-of-05.tar.tsz",
			},
			{
				Path:         "/foo/bar/qux/another.txt",
				Destination:  "bar/qux/another.txt",
				Name:         "another.txt",
				Size:         3000,
				SuitcaseName: "suitcase-foo-01-of-05.tar.tsz",
			},
		},
	}
	got := i.Search("thing")
	require.Equal(t, got.Files, SearchFileMatches{
		{
			Path:         "/foo/bar/baz/thing.txt",
			Destination:  "bar/baz/thing.txt",
			Name:         "thing.txt",
			Size:         1000,
			SuitcaseName: "suitcase-foo-01-of-05.tar.tsz",
		},
	})
	got = i.Search("baz/")
	require.Equal(t, got.Directories, SearchDirMatches{
		{
			Directory:   "bar/baz",
			TotalSize:   3000,
			TotalSizeHR: "3.0 kB",
			Suitcases: []string{
				"suitcase-foo-01-of-05.tar.tsz",
				"suitcase-foo-02-of-05.tar.tsz",
			},
		},
	})
}

func TestUniqueDirs(t *testing.T) {
	test := []string{
		"foo/bar/baz/thing.txt",
		"quux/bar/bing/thing.txt",
		"foo/bar/quux/thing.txt",
	}
	require.Equal(
		t,
		[]string{
			"foo/bar",
			"quux/bar",
		},
		uniqDirs(test, "bar/"),
	)
}

func TestArchiveTOC(t *testing.T) {
	// Good archive
	got, err := archiveTOC("../testdata/archives/archive.tar.gz")
	require.NoError(t, err)
	require.Equal(t, []string{"archives/file1.txt", "archives/sub/file2.txt", "archives/thing.png"}, got)

	// Not an archive
	got, err = archiveTOC("../testdata/archives/thing.png")
	require.Error(t, err)
	require.EqualError(t, err, "could not scan a non archive file")
	require.Nil(t, got)
}
