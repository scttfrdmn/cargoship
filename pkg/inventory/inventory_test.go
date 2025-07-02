package inventory

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/stretchr/testify/require"
)

func TestNewOptions(t *testing.T) {
	// Check some overrides
	o := NewOptions(
		WithUser("foo"),
		WithPrefix("pre"),
		WithMaxSuitcaseSize(500),
		WithLimitFileCount(10),
		WithInventoryFormat("yaml"),
		WithSuitcaseFormat("tar.gz"),
	)
	require.Equal(t, "foo", o.User)
	require.Equal(t, "pre", o.Prefix)
	require.Equal(t, 10, o.LimitFileCount)
	require.Equal(t, int64(500), o.MaxSuitcaseSize)
	require.Equal(t, "yaml", o.InventoryFormat)
	require.Equal(t, "tar.gz", o.SuitcaseFormat)

	// Check some defaults
	d := NewOptions()
	require.Equal(t, "tar.zst", d.SuitcaseFormat)
	require.Equal(t, "yaml", d.InventoryFormat)
}

func TestNewDirectoryInventory(t *testing.T) {
	got, err := NewDirectoryInventory(NewOptions(WithDirectories([]string{"../testdata/fake-dir"})))

	require.NoError(t, err)
	require.IsType(t, &Inventory{}, got)

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

	for desc, dataset := range datasets {
		location := path.Join(bdd, dataset.path)
		if _, err := os.Stat(location); err == nil {
			for _, format := range []string{"yaml", "json"} {
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
	i := &Inventory{
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
	i := &Inventory{
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
	i := &Inventory{
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

func TestWalkDirLimit(t *testing.T) {
	i := Inventory{}
	err := walkDir("../testdata/limit-dir", NewOptions(
		WithLimitFileCount(10),
	), &i)
	require.Equal(t, 10, len(i.Files))
	require.EqualError(t, err, "halt")
}

func TestWonkyTOC(t *testing.T) {
	got, err := ArchiveTOC("../testdata/archives/archive.tar.gz")
	require.NoError(t, err)
	require.Equal(t, []string{"archives/file1.txt", "archives/sub/file2.txt", "archives/thing.png"}, got)

	// Test one of these wonky tars
	got, err = ArchiveTOC("../testdata/archives/self-tarred.tar")
	require.NoError(t, err)
	require.Equal(t, []string{"._.", "._1mb", "._2mb", "1mb", "2mb"}, got)
}

func TestWalkDirExpandArchives(t *testing.T) {
	i := Inventory{}
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

func TestWalkDirExpandArchivesDeep(t *testing.T) {
	i := Inventory{}
	err := walkDir("../testdata/archives", NewOptions(
		WithArchiveTOCDeep(),
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

/*
func TestCreateOrReadInventory(t *testing.T) {
	cmd := NewInventoryCmd()
	cmd.Execute()
	got, err := CreateOrReadInventory("", cmd, []string{"../testdata/limit-dir"}, "dev")
	require.NoError(t, err)
	require.NotNil(t, got)
}
*/

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
	i := Inventory{
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
	got, err := ArchiveTOC("../testdata/archives/archive.tar.gz")
	require.NoError(t, err)
	require.Equal(t, []string{"archives/file1.txt", "archives/sub/file2.txt", "archives/thing.png"}, got)

	// Not an archive
	got, err = ArchiveTOC("../testdata/archives/thing.png")
	require.Error(t, err)
	require.EqualError(t, err, "could not scan a non archive file")
	require.Nil(t, got)
}

func TestCollectionWithDirs(t *testing.T) {
	got, err := CollectionWithDirs([]string{"../testdata/inventories/"})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Contains(t, *got, "../testdata/inventories/inventory1.yaml")
	require.Contains(t, *got, "../testdata/inventories/sub/inventory2.yaml")
}

func TestAnalysis(t *testing.T) {
	i := Inventory{
		Files: []*File{
			{Size: 1},
			{Size: 5},
			{Size: 2},
			{Size: 1},
		},
	}
	require.Equal(
		t,
		Analysis{
			LargestFileSize:   5,
			LargestFileSizeHR: "5 B",
			FileCount:         4,
			AverageFileSize:   2,
			AverageFileSizeHR: "2 B",
			TotalFileSize:     9,
			TotalFileSizeHR:   "9 B",
		},
		i.Analyze(),
	)
}

func TestTOCAble(t *testing.T) {
	tests := map[string]struct {
		fn     string
		expect bool
	}{
		"simple-zip": {
			fn:     "foo.zip",
			expect: true,
		},
		"no-toc": {
			fn:     "thing.txt",
			expect: false,
		},
	}
	for desc, tt := range tests {
		require.Equal(t, tt.expect, isTOCAble(tt.fn), desc)
	}
}

func TestHashCompletion(t *testing.T) {
	got, _ := HashCompletion(&cobra.Command{}, []string{}, "md")
	require.Contains(t, got[0], "md5")
}

func TestFormatCompletion(t *testing.T) {
	got, _ := FormatCompletion(&cobra.Command{}, []string{}, "ya")
	require.Contains(t, got[0], "yaml")
}

func TestInaccessableFilesInInventory(t *testing.T) {
	dir := t.TempDir()
	tfile := path.Join(dir, "bad-mode.txt")
	require.NoError(t, os.WriteFile(tfile, []byte("hello"), 0o00))
	got, err := NewDirectoryInventory(NewOptions(WithDirectories([]string{dir})))
	require.NoError(t, err)
	require.NotNil(t, got)
	require.EqualError(t, got.ValidateAccess(), fmt.Sprintf("the following files are not readable: %v", tfile))
}

// Test HashAlgorithm methods
func TestHashAlgorithmString(t *testing.T) {
	tests := []struct {
		name     string
		hash     HashAlgorithm
		expected string
	}{
		{"MD5Hash", MD5Hash, "md5"},
		{"SHA1Hash", SHA1Hash, "sha1"},
		{"SHA256Hash", SHA256Hash, "sha256"},
		{"SHA512Hash", SHA512Hash, "sha512"},
		{"NullHash", NullHash, ""},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.hash.String()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestHashAlgorithmStringPanic(t *testing.T) {
	// Test with invalid hash algorithm value
	require.Panics(t, func() {
		var invalidHash HashAlgorithm = 999
		_ = invalidHash.String()
	})
}

func TestHashAlgorithmType(t *testing.T) {
	var hash HashAlgorithm
	require.Equal(t, "HashAlgorithm", hash.Type())
}

func TestHashAlgorithmSet(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		expectedVal HashAlgorithm
		expectError bool
	}{
		{"valid md5", "md5", MD5Hash, false},
		{"valid sha1", "sha1", SHA1Hash, false},
		{"valid sha256", "sha256", SHA256Hash, false},
		{"valid sha512", "sha512", SHA512Hash, false},
		{"valid empty", "", NullHash, false},
		{"invalid value", "invalid", NullHash, true},
		{"case sensitive", "MD5", NullHash, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hash HashAlgorithm
			err := hash.Set(tt.value)
			
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "HashAlgorithm should be one of")
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedVal, hash)
			}
		})
	}
}

func TestHashAlgorithmMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		hash     HashAlgorithm
		expected string
	}{
		{"MD5Hash", MD5Hash, `"md5"`},
		{"SHA1Hash", SHA1Hash, `"sha1"`},
		{"SHA256Hash", SHA256Hash, `"sha256"`},
		{"SHA512Hash", SHA512Hash, `"sha512"`},
		{"NullHash", NullHash, `""`},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.hash.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, tt.expected, string(result))
		})
	}
}

// Test Format methods
func TestFormatString(t *testing.T) {
	tests := []struct {
		name     string
		format   Format
		expected string
	}{
		{"YAMLFormat", YAMLFormat, "yaml"},
		{"JSONFormat", JSONFormat, "json"},
		{"NullFormat", NullFormat, ""},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.format.String()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatStringPanic(t *testing.T) {
	// Test with invalid format value
	require.Panics(t, func() {
		var invalidFormat Format = 999
		_ = invalidFormat.String()
	})
}

func TestFormatType(t *testing.T) {
	var format Format
	require.Equal(t, "Format", format.Type())
}

func TestFormatSet(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		expectedVal Format
		expectError bool
	}{
		{"valid yaml", "yaml", YAMLFormat, false},
		{"valid json", "json", JSONFormat, false},
		{"valid empty", "", NullFormat, false},
		{"invalid value", "invalid", NullFormat, true},
		{"case sensitive", "YAML", NullFormat, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var format Format
			err := format.Set(tt.value)
			
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "ProductionLevel should be one of")
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedVal, format)
			}
		})
	}
}

func TestFormatMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		format   Format
		expected string
	}{
		{"YAMLFormat", YAMLFormat, `"yaml"`},
		{"JSONFormat", JSONFormat, `"json"`},
		{"NullFormat", NullFormat, `""`},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.format.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, tt.expected, string(result))
		})
	}
}

// Test Inventory JSON methods
func TestInventoryJSONString(t *testing.T) {
	inventory := &Inventory{
		Files: []*File{
			{Path: "test.txt", Size: 100},
		},
		Options: NewOptions(),
	}
	
	jsonStr, err := inventory.JSONString()
	require.NoError(t, err)
	require.Contains(t, jsonStr, "test.txt")
	require.Contains(t, jsonStr, "files")
	require.Contains(t, jsonStr, "options")
}

func TestInventoryMustJSONString(t *testing.T) {
	inventory := &Inventory{
		Files: []*File{
			{Path: "test.txt", Size: 100},
		},
		Options: NewOptions(),
	}
	
	// Should not panic with valid inventory
	require.NotPanics(t, func() {
		jsonStr := inventory.MustJSONString()
		require.Contains(t, jsonStr, "test.txt")
	})
}

// Test utility functions
func TestReverseMapString(t *testing.T) {
	original := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	
	reversed := reverseMap(original)
	
	require.Equal(t, "key1", reversed["value1"])
	require.Equal(t, "key2", reversed["value2"])
	require.Equal(t, "key3", reversed["value3"])
	require.Len(t, reversed, 3)
}

func TestReverseMapHashAlgorithm(t *testing.T) {
	reversed := reverseMap(hashMap)
	
	require.Equal(t, "md5", reversed[MD5Hash])
	require.Equal(t, "sha1", reversed[SHA1Hash])
	require.Equal(t, "sha256", reversed[SHA256Hash])
	require.Equal(t, "sha512", reversed[SHA512Hash])
	require.Equal(t, "", reversed[NullHash])
}

func TestReverseMapFormat(t *testing.T) {
	reversed := reverseMap(formatMap)
	
	require.Equal(t, "yaml", reversed[YAMLFormat])
	require.Equal(t, "json", reversed[JSONFormat])
	require.Equal(t, "", reversed[NullFormat])
}

// Test dclose function
func TestDclose(t *testing.T) {
	// Test with a valid closer
	file, err := os.CreateTemp("", "test_dclose_*.txt")
	require.NoError(t, err)
	defer func() { _ = os.Remove(file.Name()) }()
	
	// Should not panic
	require.NotPanics(t, func() {
		dclose(file)
	})
	
	// File should be closed
	_, err = file.Write([]byte("test"))
	require.Error(t, err) // Should error because file is closed
}

func TestDcloseWithAlreadyClosedFile(t *testing.T) {
	file, err := os.CreateTemp("", "test_dclose_*.txt")
	require.NoError(t, err)
	defer func() { _ = os.Remove(file.Name()) }()
	
	// Close file first
	err = file.Close()
	require.NoError(t, err)
	
	// dclose should handle already closed file gracefully
	require.NotPanics(t, func() {
		dclose(file)
	})
}

// Test SummaryLog function
func TestInventorySummaryLog(t *testing.T) {
	inventory := &Inventory{
		Files: []*File{
			{Path: "file1.txt", Size: 100},
			{Path: "file2.txt", Size: 200},
			{Path: "file3.txt", Size: 300},
		},
		Options: NewOptions(),
		TotalIndexes: 2,
	}
	
	// Should not panic when calling SummaryLog
	require.NotPanics(t, func() {
		inventory.SummaryLog()
	})
}

func TestInventorySummaryLogEmpty(t *testing.T) {
	inventory := &Inventory{
		Files:   []*File{},
		Options: NewOptions(),
	}
	
	// Should not panic with empty inventory
	require.NotPanics(t, func() {
		inventory.SummaryLog()
	})
}

// Test additional 0% coverage functions
func TestWithHashAlgorithms(t *testing.T) {
	tests := []struct {
		name string
		hash HashAlgorithm
	}{
		{"MD5Hash", MD5Hash},
		{"SHA1Hash", SHA1Hash},
		{"SHA256Hash", SHA256Hash},
		{"SHA512Hash", SHA512Hash},
		{"NullHash", NullHash},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := NewOptions(WithHashAlgorithms(tt.hash))
			require.Equal(t, tt.hash, options.HashAlgorithm)
		})
	}
}

func TestWithWizardForm(t *testing.T) {
	form := WizardForm{
		Source:           "/tmp",
		TravelAgentToken: "test-token",
		MaxSize:          "1GB",
	}
	
	options := NewOptions(WithWizardForm(form))
	require.Equal(t, []string{"/tmp/"}, options.Directories)
}

func TestWithWizardFormRelative(t *testing.T) {
	form := WizardForm{
		Source: ".",
	}
	
	options := NewOptions(WithWizardForm(form))
	// Should convert relative path to absolute
	require.Len(t, options.Directories, 1)
	require.NotEqual(t, ".", options.Directories[0])
	require.Contains(t, options.Directories[0], "cargoship/pkg/inventory")
}

func TestWithCobra(t *testing.T) {
	// Create a test cobra command with basic flags
	cmd := &cobra.Command{}
	BindCobra(cmd)
	
	// Set command line arguments (avoid the problematic array flags for now)
	cmd.SetArgs([]string{
		"--user", "test-user",
		"--prefix", "test-prefix", 
		"--max-suitcase-size", "500MB",
		"--follow-symlinks",
		"--hash-inner",
		"--encrypt-inner",
		"--archive-toc",
		"--archive-toc-deep",
		"--limit-file-count", "100",
		"--internal-metadata-glob", "*.meta",
	})
	
	// Execute to parse flags
	err := cmd.Execute()
	require.NoError(t, err)
	
	// Test WithCobra with arguments
	args := []string{"/tmp"}
	options := NewOptions(WithCobra(cmd, args))
	
	// Verify basic options are set correctly
	require.Equal(t, "test-user", options.User)
	require.Equal(t, "test-prefix", options.Prefix)
	require.Equal(t, []string{"/tmp/"}, options.Directories)
	require.Equal(t, int64(500000000), options.MaxSuitcaseSize) // 500MB
	require.True(t, options.FollowSymlinks)
	require.True(t, options.HashInner)
	require.True(t, options.EncryptInner)
	require.True(t, options.IncludeArchiveTOC)
	require.True(t, options.IncludeArchiveTOCDeep)
	require.Equal(t, 100, options.LimitFileCount)
	require.Equal(t, "*.meta", options.InternalMetadataGlob)
}

func TestWithCobraNoArgs(t *testing.T) {
	// Test WithCobra without arguments
	cmd := &cobra.Command{}
	BindCobra(cmd)
	
	cmd.SetArgs([]string{
		"--user", "test-user",
		"--prefix", "test-prefix",
	})
	
	err := cmd.Execute()
	require.NoError(t, err)
	
	// Test WithCobra without args (empty slice)
	options := NewOptions(WithCobra(cmd, []string{}))
	
	require.Equal(t, "test-user", options.User)
	require.Equal(t, "test-prefix", options.Prefix)
	require.Empty(t, options.Directories) // Should remain empty when no args provided
}

func TestNewInventoryCmd(t *testing.T) {
	cmd := NewInventoryCmd()
	
	// Verify command is created and bound properly
	require.NotNil(t, cmd)
	require.NotNil(t, cmd.PersistentFlags())
	
	// Check that key flags are bound by BindCobra
	flag := cmd.PersistentFlags().Lookup("concurrency")
	require.NotNil(t, flag)
	require.Equal(t, "10", flag.DefValue)
	
	flag = cmd.PersistentFlags().Lookup("inventory-file")
	require.NotNil(t, flag)
	require.Equal(t, "", flag.DefValue)
	
	flag = cmd.PersistentFlags().Lookup("max-suitcase-size")
	require.NotNil(t, flag)
	require.Equal(t, "500GiB", flag.DefValue)
	
	flag = cmd.PersistentFlags().Lookup("internal-metadata-glob")
	require.NotNil(t, flag)
	require.Equal(t, "suitcase-meta*", flag.DefValue)
	
	// Check array flags
	flag = cmd.PersistentFlags().Lookup("external-metadata-file")
	require.NotNil(t, flag)
	
	flag = cmd.PersistentFlags().Lookup("ignore-glob")
	require.NotNil(t, flag)
}
