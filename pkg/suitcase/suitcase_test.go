package suitcase

import (
	"fmt"
	"io"
	"os"
	"path"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/gpg"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
)

func TestNewSuitcase(t *testing.T) {
	folder := t.TempDir()
	empty, err := os.Create(folder + "/empty.txt")
	require.NoError(t, err)
	require.NoError(t, empty.Close())
	require.NoError(t, os.Mkdir(folder+"/folder-inside", 0o755))

	for _, format := range []string{"tar", "tar.gz", "tar.zst"} {
		format := format
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
		format := format
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

func TestFillWithInventoryIndex(t *testing.T) {
	s, err := New(io.Discard, &config.SuitCaseOpts{
		Format: "tar",
	})
	require.NoError(t, err)
	i, err := inventory.NewDirectoryInventory(inventory.NewOptions(
		inventory.WithDirectories([]string{"../testdata/fake-dir"}),
	))
	require.NoError(t, err)
	_, err = FillWithInventoryIndex(s, i, 0, nil)
	require.NoError(t, err)
}

func TestFillWithInventoryIndexMissingDir(t *testing.T) {
	_, err := inventory.NewDirectoryInventory(inventory.NewOptions(
		inventory.WithDirectories([]string{"../testdata/never-exist"}),
	))
	require.EqualError(t, err, "not a directory")
}

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
	sfs := fmt.Sprintf("%v.sha1", sf)
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

	zerolog.SetGlobalLevel(zerolog.FatalLevel)
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
