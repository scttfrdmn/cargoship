package suitcase

import (
	"bytes"
	"io"
	"os"
	"path"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/scttfrdmn/cargoship/pkg/config"
	"github.com/scttfrdmn/cargoship/pkg/gpg"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
	"gopkg.in/yaml.v3"
)

func TestSuitcaseWithInaccessibleFiles(t *testing.T) {
	dir := t.TempDir()
	f1 := path.Join(dir, "good.txt")
	f2 := path.Join(dir, "bad.txt")
	require.NoError(t, os.WriteFile(f1, []byte("good"), 0o644))
	require.NoError(t, os.WriteFile(f2, []byte("bad"), 0o644))
	i, err := inventory.NewDirectoryInventory(inventory.NewOptions(inventory.WithDirectories([]string{dir})))
	require.NoError(t, err)
	require.NotNil(t, i)
	require.NoError(t, i.ValidateAccess())

	// Now pretend one of the files becomes inaccessible
	/*
		require.NoError(t, os.Chmod(f2, 0o000))
		s, err := New(io.Discard, &config.SuitCaseOpts{
			Format: "tar.gz",
		})
		require.NoError(t, err)
		_, err = FillWithInventoryIndex(s, i, 0, nil)
		require.EqualError(t, err, "foo")
	*/
}

func TestNewSuitcase(t *testing.T) {
	folder := t.TempDir()
	empty, err := os.Create(folder + "/empty.txt")
	require.NoError(t, err)
	require.NoError(t, empty.Close())
	require.NoError(t, os.Mkdir(folder+"/folder-inside", 0o755))

	for _, format := range []string{"tar", "tar.gz", "tar.zst", "tar.bz2"} {
		t.Run(format, func(t *testing.T) {
			archive, err := New(io.Discard, &config.SuitCaseOpts{
				Format: format,
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, archive.Close())
			})
			_, err = archive.Add(inventory.File{
				Path:        empty.Name(),
				Destination: "empty.txt",
			})
			require.NoError(t, err)
			_, err = archive.Add(inventory.File{
				Path:        empty.Name() + "_nope",
				Destination: "dont.txt",
			})
			require.Error(t, err)
		})
	}

	t.Run("7z", func(t *testing.T) {
		_, err := New(io.Discard, &config.SuitCaseOpts{
			Format: "7z",
		})
		require.EqualError(t, err, "invalid archive format: 7z")
	})
}

func TestNewGPGSuitcase(t *testing.T) {
	folder := t.TempDir()
	empty, err := os.Create(folder + "/empty.txt")
	require.NoError(t, err)
	require.NoError(t, empty.Close())
	require.NoError(t, os.Mkdir(folder+"/folder-inside", 0o755))

	pubKey, err := gpg.ReadEntity("../testdata/fakey-public.key")
	require.NoError(t, err)

	for _, format := range []string{"tar.gpg", "tar.gz.gpg"} {
		t.Run(format, func(t *testing.T) {
			archive, err := New(io.Discard, &config.SuitCaseOpts{
				Format:    format,
				EncryptTo: &openpgp.EntityList{pubKey},
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, archive.Close())
			})
			_, err = archive.Add(inventory.File{
				Path:        empty.Name(),
				Destination: "empty.txt",
			})
			require.NoError(t, err)
			_, err = archive.Add(inventory.File{
				Path:        empty.Name() + "_nope",
				Destination: "dont.txt",
			})
			require.Error(t, err)
		})
	}
}

/*
func TestFillWithInventoryIndex(t *testing.T) {
	s, err := New(io.Discard, &config.SuitCaseOpts{
		Format: "tar",
	})
	require.NoError(t, err)
	i, err := inventory.NewDirectoryInventory(inventory.NewOptions(
		inventory.WithDirectories([]string{"../testdata/fake-dir"}),
	))
	require.NoError(t, err)
	_, err = Fill(s, i, 0, nil)
	require.NoError(t, err)
}
*/

func TestFillWithInventoryIndexMissingDir(t *testing.T) {
	_, err := inventory.NewDirectoryInventory(inventory.NewOptions(
		inventory.WithDirectories([]string{"../testdata/never-exist"}),
	))
	require.EqualError(t, err, "not a directory")
}

/*
func TestFillFileWithInventoryIndex(t *testing.T) {
	d := t.TempDir()
	so := &config.SuitCaseOpts{
		Format:      "tar",
		Destination: d,
	}
	i, err := inventory.NewDirectoryInventory(inventory.NewOptions(
		inventory.WithDirectories([]string{"../testdata/fake-dir"}),
	))
	require.NoError(t, err)
	sf, err := WriteSuitcaseFile(so, i, 1, nil)
	// err = FillWithInventoryIndex(s, i, 0, nil)
	require.NoError(t, err)
	require.FileExists(t, sf)
}

func TestFillFileWithInventoryIndexHashInner(t *testing.T) {
	d := t.TempDir()
	so := &config.SuitCaseOpts{
		Format:      "tar",
		Destination: d,
		HashInner:   true,
	}
	i, err := inventory.NewDirectoryInventory(inventory.NewOptions(
		inventory.WithDirectories([]string{"../testdata/fake-dir"}),
	))
	require.NoError(t, err)
	sf, err := WriteSuitcaseFile(so, i, 1, nil)
	sfs := fmt.Sprintf("%v.md5", sf)
	require.NoError(t, err)
	require.FileExists(t, sfs)

	c, err := os.ReadFile(sfs)
	require.NoError(t, err)
	// Make sure our known hash file is up in here
	require.Contains(t, string(c), "ef3d6ae3230876bc9d15b3df72b89797ce8be0dd872315b78c0be72a4600d466")
}

func BenchmarkNewSuitcase(b *testing.B) {
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
	var err error
	bdd, err = filepath.Abs(bdd)
	require.NoError(b, err)

	for desc, dataset := range datasets {
		location := path.Join(bdd, dataset.path)
		if _, err := os.Stat(location); err == nil {
			desc := desc
			b.Run(fmt.Sprintf("suitcase_inventory_%v", desc), func(b *testing.B) {
				inventory, err := inventory.NewDirectoryInventory(inventory.NewOptions(
					inventory.WithDirectories([]string{location}),
				))
				require.NoError(b, err)
				require.NotNil(b, inventory)
				for _, af := range []string{"tar", "tar.gz", "tar.zst", "tar.bz2"} {
					af := af
					b.Run(fmt.Sprintf("suitcase_create_%v_%v", desc, af), func(b *testing.B) {
						out := b.TempDir()
						sn, err := WriteSuitcaseFile(&config.SuitCaseOpts{
							Format:      af,
							Destination: out,
						}, inventory, 1, nil)
						require.NoError(b, err)
						require.NotEmpty(b, sn)
					})
				}
			})
		}
	}
}
*/

func TestHexToBin(t *testing.T) {
	require.Equal(
		t,
		"kOKOhBhQjtnw9B7gDxYcCw==",
		mustHexToBin("90e28e8418508ed9f0f41ee00f161c0b"),
	)
}

func TestFormatComplete(t *testing.T) {
	got, _ := FormatCompletion(&cobra.Command{}, []string{}, "zst")
	require.Contains(t, got, "tar.zst")
	require.NotContains(t, got, "tar.gz")
}

func TestWriteHashfileBin(t *testing.T) {
	buf := bytes.Buffer{}
	err := WriteHashFileBin([]config.HashSet{
		{
			Filename: "foo",
			Hash:     "b25f62d0856d4c81831cf701b92e3e74",
		},
	}, &buf)
	require.NoError(t, err)
	require.Equal(t, "sl9i0IVtTIGDHPcBuS4+dA==\tfoo\n", buf.String())
}

func TestWriteHashfile(t *testing.T) {
	buf := bytes.Buffer{}
	err := WriteHashFile([]config.HashSet{
		{
			Filename: "foo",
			Hash:     "b25f62d0856d4c81831cf701b92e3e74",
		},
	}, &buf)
	require.NoError(t, err)
	require.Equal(t, "b25f62d0856d4c81831cf701b92e3e74\tfoo\n", buf.String())
}

func TestWriteHashfileBinFail(t *testing.T) {
	buf := bytes.Buffer{}
	err := WriteHashFileBin([]config.HashSet{
		{
			Filename: "foo",
			Hash:     "not-a-hash",
		},
	}, &buf)
	require.Error(t, err)
	require.Equal(t, "", buf.String())
}

func TestFormatStrings(t *testing.T) {
	require.Equal(
		t,
		"tar",
		formatMap["tar"].String(),
	)
}

func TestValidateSuitcase(t *testing.T) {
	var i inventory.Inventory
	b, err := os.ReadFile("../testdata/validations/inventory.yaml")
	require.NoError(t, err)
	require.NotNil(t, i)
	err = yaml.Unmarshal(b, &i)
	require.NoError(t, err)
	require.True(t, validateSuitcase("../testdata/validations/complete/suitcase-joebob-01-of-01.tar.zst", i, 1))
}

func TestValidateSuitcaseInvalid(t *testing.T) {
	var i inventory.Inventory
	b, err := os.ReadFile("../testdata/validations/inventory.yaml")
	require.NoError(t, err)
	require.NotNil(t, i)
	err = yaml.Unmarshal(b, &i)
	require.NoError(t, err)
	require.False(t, validateSuitcase("../testdata/validations/incomplete/suitcase-joebob-01-of-01.tar.zst", i, 1))
}

func TestInProcessName(t *testing.T) {
	require.Equal(
		t,
		"/tmp/.__creating-foo.txt",
		inProcessName("/tmp/foo.txt"),
	)
}

// Test Format methods (0% coverage functions)
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
		{"valid tar", "tar", TarFormat, false},
		{"valid tar.gz", "tar.gz", TarGzFormat, false},
		{"valid tar.zst", "tar.zst", TarZstFormat, false},
		{"valid tar.gpg", "tar.gpg", TarGpgFormat, false},
		{"valid tar.gz.gpg", "tar.gz.gpg", TarGzGpgFormat, false},
		{"valid tar.zst.gpg", "tar.zst.gpg", TarZstGpgFormat, false},
		{"valid empty", "", NullFormat, false},
		{"invalid value", "invalid", NullFormat, true},
		{"case sensitive", "TAR", NullFormat, true},
		{"partial match", "tar.g", NullFormat, true},
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
		{"TarFormat", TarFormat, `"tar"`},
		{"TarGzFormat", TarGzFormat, `"tar.gz"`},
		{"TarZstFormat", TarZstFormat, `"tar.zst"`},
		{"TarGpgFormat", TarGpgFormat, `"tar.gpg"`},
		{"TarGzGpgFormat", TarGzGpgFormat, `"tar.gz.gpg"`},
		{"TarZstGpgFormat", TarZstGpgFormat, `"tar.zst.gpg"`},
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

func TestFormatStringPanic(t *testing.T) {
	// Test with invalid format value
	require.Panics(t, func() {
		var invalidFormat Format = 999
		_ = invalidFormat.String()
	})
}
