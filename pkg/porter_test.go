package porter

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/rclone"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/travelagent"
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

func TestPorterNew(t *testing.T) {
	got := New(
		WithVersion("0.1.2"),
		WithCLIMeta(&CLIMeta{Username: "joebob"}),
		WithLogger(&log.Logger),
	)
	require.Equal(t, "0.1.2", got.Version)
	require.Equal(t, "joebob", got.CLIMeta.Username)
}

func TestCalculateHash(t *testing.T) {
	text := []byte("Lorem Ipsum dolor sit Amet")
	tests := map[string]string{
		"md5":    "4db45e622c0ae3157bdcb53e436c96c5",
		"sha1":   "8f2f2e214fc4dfbc6bc0a2b0aced48146c7fd12c",
		"sha256": "eb7a03c377c28da97ae97884582e6bd07fa44724af99798b42593355e39f82cb",
		"sha512": "5cdaf0d2f162f55ccc04a8639ee490c94f2faeab3ba57d3c50d41930a67b5fa6915a73d6c78048729772390136efed25b11858e7fc0eed1aa7a464163bd44b1c",
	}
	for h, expect := range tests {
		got, eerr := CalculateHash(bytes.NewReader(text), h)
		require.NoError(t, eerr, h)
		require.Equal(t, expect, got, h)
	}

	got, err := CalculateHash(bytes.NewReader(text), "bogus-hash")
	require.Error(t, err)
	require.EqualError(t, err, "unexpected hash type: bogus-hash")
	require.Equal(t, "", got)
}

type fakeTa struct{}

func (f fakeTa) StatusURL() string {
	return "https://www.example.com/api/v1/5"
}

func (f fakeTa) Update(s travelagent.StatusUpdate) (*travelagent.StatusUpdateResponse, error) {
	return &travelagent.StatusUpdateResponse{
		Messages: []string{
			"updated fields: some_fake_field",
		},
	}, nil
}

type fakeTrans struct {
	attempt int
}

func (ft fakeTrans) Check() error {
	return nil
}

func (ft *fakeTrans) Send(s, u string) error {
	if ft.attempt == 3 {
		return nil
	}
	ft.attempt++
	return errors.New("some fake error")
}

func (ft *fakeTrans) SendWithChannel(s, u string, c chan rclone.TransferStatus) error {
	if ft.attempt == 3 {
		return nil
	}
	ft.attempt++
	return errors.New("some fake error")
}

func TestRetryTransport(t *testing.T) {
	i, err := inventory.NewDirectoryInventory(&inventory.Options{
		Directories:     []string{"testdata/archives"},
		TransportPlugin: &fakeTrans{},
	})
	require.NoError(t, err)
	p := New(
		WithTravelAgent(fakeTa{}),
		WithInventory(i),
	)
	c := make(chan rclone.TransferStatus)
	go func() {
		for {
			<-c
		}
	}()
	// Make sure that it still fails when retry count is zero
	require.EqualError(
		t,
		p.RetryTransport("testdata/archives/archive.tar.gz", c, 0, time.Millisecond*1),
		"could not transport suitcasefile even with retries",
	)
	// Make sure that it still fails on just 2 attempts
	require.EqualError(
		t,
		p.RetryTransport("testdata/archives/archive.tar.gz", c, 2, time.Millisecond*1),
		"could not transport suitcasefile even with retries",
	)
	// Make sure it DOES work on 3 attempts
	require.NoError(
		t,
		p.RetryTransport("testdata/archives/archive.tar.gz", c, 3, time.Millisecond*1),
	)
}

func TestSendUpdate(t *testing.T) {
	p := New(WithTravelAgent(fakeTa{}))

	err := p.SendUpdate(travelagent.StatusUpdate{})
	require.NoError(t, err)
}
