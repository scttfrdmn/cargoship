package porter

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
)

func TestPorterCreateHashes(t *testing.T) {
	p := New(
		WithDestination("/tmp"),
		WithHashAlgorithm(inventory.MD5Hash),
	)
	got, err := p.CreateHashes([]string{"testdata/archives/archive.tar.gz"})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(
		t,
		[]config.HashSet{
			{
				Filename: "testdata/archives/archive.tar.gz",
				Hash:     "165f3b4fca62b435900aed352165875c",
			},
		},
		got,
	)
}

func TestPorterCreateHashesFail(t *testing.T) {
	p := New(WithHashAlgorithm(inventory.MD5Hash))
	got, err := p.CreateHashes([]string{"testdata/archives/archive.tar.gz"})
	require.Error(t, err)
	require.EqualError(t, err, "must set Destination in porter before using CreateHashes")
	require.Nil(t, got)

	p = New(WithDestination("/tmp"))
	got, err = p.CreateHashes([]string{"testdata/archives/archive.tar.gz"})
	require.Error(t, err)
	require.EqualError(t, err, "must set HashAlgorithm in porter before using CreateHashes")
	require.Nil(t, got)
}

func TestCreateOrReadInventoryCreate(t *testing.T) {
	cmd := inventory.NewInventoryCmd()
	cmd.Execute()
	p := New(
		WithCmdArgs(cmd, []string{"testdata/limit-dir"}),
	)
	got, err := p.CreateOrReadInventory("")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Greater(t, len(got.Files), 1)
}

func TestCreateOrReadInventoryRead(t *testing.T) {
	cmd := inventory.NewInventoryCmd()
	cmd.Execute()
	p := New(
		WithCmdArgs(cmd, []string{"testdata/limit-dir"}),
	)
	got, err := p.CreateOrReadInventory("testdata/inventories/example-inventory.yaml")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "bad.tar", got.Files[0].Name)
}

func TestWriteInventoryFile(t *testing.T) {
	f := t.TempDir()
	cmd := inventory.NewInventoryCmd()
	cmd.Execute()
	ptr := New(
		WithDestination(f),
		WithCmdArgs(cmd, []string{"./testdata/fake-dir"}),
		WithUserOverrides(viper.New()),
	)
	i, gf, err := ptr.WriteInventory() // (v, cmd, []string{"../testdata/fake-dir"}, "testing")
	require.NoError(t, err)
	require.FileExists(t, gf.Name())
	require.NotNil(t, i)
}
