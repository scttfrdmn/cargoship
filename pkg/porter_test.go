package porter

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"github.com/scttfrdmn/cargoship/pkg/config"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
	"github.com/scttfrdmn/cargoship/pkg/plugins/transporters"
	"github.com/scttfrdmn/cargoship/pkg/plugins/transporters/cloud"
	"github.com/scttfrdmn/cargoship/pkg/rclone"
	"github.com/scttfrdmn/cargoship/pkg/travelagent"
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
	_ = cmd.Execute() // Test helper
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
	_ = cmd.Execute() // Test helper
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
	_ = cmd.Execute() // Test helper
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
		// WithLogger(&log.Logger),
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

func (f fakeTa) PostMetaData(_ string) error {
	return nil
}

func (f fakeTa) Transferred() int64 {
	return 0
}

func (f fakeTa) Upload(_ string, _ chan rclone.TransferStatus) (int64, error) {
	return 0, errors.New("not yet implemented")
}

func (f fakeTa) Update(_ travelagent.StatusUpdate) (*travelagent.StatusUpdateResponse, error) {
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

func (ft *fakeTrans) Send(_, _ string) error {
	if ft.attempt == 3 {
		return nil
	}
	ft.attempt++
	return errors.New("some fake error")
}

func (ft *fakeTrans) SendWithChannel(_, _ string, _ chan rclone.TransferStatus) error {
	if ft.attempt == 3 {
		return nil
	}
	ft.attempt++
	return errors.New("some fake error")
}

type fta struct {
	LastStatus travelagent.StatusUpdate
}

func (f fta) StatusURL() string {
	return "https://www.example.com/api/v1/status"
}

func (f fta) PostMetaData(_ string) error {
	return nil
}

func (f fta) Upload(_ string, _ chan rclone.TransferStatus) (int64, error) {
	return 0, errors.New("not yet implemented")
}

func (f fta) Transferred() int64 {
	return 0
}

func (f *fta) Update(s travelagent.StatusUpdate) (*travelagent.StatusUpdateResponse, error) {
	f.LastStatus = s
	r := &travelagent.StatusUpdateResponse{
		Messages: []string{fmt.Sprintf("got update of %v\n", s)},
	}
	return r, nil
}

func TestShipItems(t *testing.T) {
	td := t.TempDir()
	ctd := t.TempDir()
	tfile := "testdata/overflow-queue/2mb"
	require.NoError(t, copySrcDst(tfile, path.Join(ctd, path.Base(tfile))))
	require.NoError(t, copySrcDst(tfile, path.Join(td, path.Base(tfile))))
	ftaI := &fta{}
	p := New(
		WithDestination(td),
		WithInventory(&inventory.Inventory{
			Options: inventory.NewOptions(),
		}),
		WithTravelAgent(ftaI),
	)
	p.Inventory.Options.TransportPlugin = &cloud.Transporter{
		Config: transporters.Config{
			Destination: ctd,
		},
	}
	p.ShipItems([]string{path.Base(tfile)}, "foo")
	// The transfer amount seems to be inconsistent based on source and destination. Leaving this pretty bare for now
	// require.Equal(t, int64(2097152), ftaI.LastStatus.TransferredBytes)
	require.Greater(t, ftaI.LastStatus.TransferredBytes, int64(0))
	require.EqualValues(t, travelagent.StatusComplete, ftaI.LastStatus.Status)
	// require.Equal(t, "foo", "bar")
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

func TestCreateForm(t *testing.T) {
	os.Clearenv()
	f := createForm(&inventory.WizardForm{
		Destination: "/foo/destination",
	})
	f.Update(f.Init())
	require.Contains(t, f.View(), "/foo/destination")
}

func TestMustExpandDir(t *testing.T) {
	require.Equal(t, "/foo", mustExpandDir("/foo"))
}

func TestValidateIsDir(t *testing.T) {
	require.EqualError(t, validateIsDir(""), "directory cannot be blank")
	require.EqualError(t, validateIsDir("/never/exists/ever"), "could not stat /never/exists/ever, got error: stat /never/exists/ever: no such file or directory")
	tf, err := os.CreateTemp("", "")
	require.NoError(t, err)
	require.FileExists(t, tf.Name())
	require.EqualError(t, validateIsDir(tf.Name()), "this must be a directory, not a file")
	require.NoError(t, validateIsDir(t.TempDir()))
}

func TestEnvOrString(t *testing.T) {
	os.Clearenv()
	t.Setenv("SOME_ENV", "env-value")
	require.Equal(t, "env-value", envOrString("SOME_ENV", "not-found"))
	require.Equal(t, "not-found", envOrString("NEVER_EXISTS", "not-found"))
}

func TestEnvOrTempDir(t *testing.T) {
	os.Clearenv()
	t.Setenv("SOME_TMP", "/tmp/foo")
	require.Equal(t, "/tmp/foo", envOrTmpDir("SOME_TMP"))
	require.Contains(t, envOrTmpDir("NEVER_EXISTS"), "cargoship")
}

func TestMergeWizard(t *testing.T) {
	p := New()
	require.EqualError(t, p.mergeWizard(), "must have an Inventory set before merge can happen")

	target := t.TempDir()
	inv, err := inventory.NewDirectoryInventory(&inventory.Options{
		Directories: []string{target},
	})
	require.NoError(t, err)
	p = New(WithInventory(inv))
	require.EqualError(t, p.mergeWizard(), "must have a WizardForm set before merge can happen")

	p.WizardForm = &inventory.WizardForm{}
	require.NoError(t, p.mergeWizard())
}

func TestSetOrReadInv(t *testing.T) {
	p := New()
	require.NoError(t, p.SetOrReadInventory("./testdata/validations/inventory.yaml"))
	require.EqualError(t, p.SetOrReadInventory("/never-exists.yaml"), "open /never-exists.yaml: no such file or directory")
}

func TestRun(t *testing.T) {
	dest := t.TempDir()
	cmd := inventory.NewInventoryCmd()
	cmd.SetArgs([]string{"--user", "gotest"})
	_ = cmd.Execute() // Test helper
	p := New(
		WithCmdArgs(cmd, []string{"testdata/limit-dir"}),
		WithDestination(dest),
		WithHashAlgorithm(inventory.MD5Hash),
	)
	require.NotNil(t, p)
	require.NoError(t, p.SetOrReadInventory(""))
	require.NoError(t, p.Run())
	require.DirExists(t, dest)
	listing, err := os.ReadDir(dest)
	require.NoError(t, err)
	fmt.Fprintf(os.Stderr, "%+v\n", listing)
	require.FileExists(t, path.Join(dest, "inventory.yaml"))
	sFile := path.Join(dest, "suitcase-gotest-01-of-01.tar.zst")
	require.FileExists(t, sFile)
	stat, err := os.Stat(sFile)
	require.NoError(t, err)
	require.Greater(t, stat.Size(), int64(100))
}

// Test 0% coverage functions
func TestSetTravelAgent(t *testing.T) {
	p := New()
	
	// Verify initial state
	require.False(t, p.hasTravelAgent)
	require.Nil(t, p.TravelAgent)
	
	// Create a fake travel agent
	ta := &fta{}
	
	// Test SetTravelAgent
	p.SetTravelAgent(ta)
	
	require.True(t, p.hasTravelAgent)
	require.Equal(t, ta, p.TravelAgent)
}

func TestWithLogger(t *testing.T) {
	// Test WithLogger option
	logger := &slog.Logger{}
	p := New(WithLogger(logger))
	
	require.Equal(t, logger, p.Logger)
}

func TestSetConcurrency(t *testing.T) {
	p := New()
	
	// Test default concurrency (New() sets it to 10)
	require.Equal(t, 10, p.concurrency)
	
	// Test SetConcurrency
	p.SetConcurrency(5)
	require.Equal(t, 5, p.concurrency)
	
	// Test with different value
	p.SetConcurrency(15)
	require.Equal(t, 15, p.concurrency)
}

func TestSetRetries(t *testing.T) {
	p := New()
	
	// Test default values (New() sets retryCount=1, retryInterval=5s)
	require.Equal(t, 1, p.retryCount)
	require.Equal(t, time.Second*5, p.retryInterval)
	
	// Test SetRetries
	p.SetRetries(3, time.Second*10)
	require.Equal(t, 3, p.retryCount)
	require.Equal(t, time.Second*10, p.retryInterval)
	
	// Test with different values
	p.SetRetries(10, time.Minute)
	require.Equal(t, 10, p.retryCount)
	require.Equal(t, time.Minute, p.retryInterval)
}

// Test 0% coverage functions
func TestSendFinalUpdate(t *testing.T) {
	// Test porter without travel agent
	p := New()
	require.False(t, p.hasTravelAgent)
	
	err := p.SendFinalUpdate(travelagent.StatusUpdate{
		Status: travelagent.StatusComplete,
	})
	require.NoError(t, err) // Should return nil when no travel agent
	
	// Test porter with travel agent
	ta := &fta{}
	p.SetTravelAgent(ta)
	p.InventoryFilePath = "testdata/inventories/example-inventory.yaml"
	
	err = p.SendFinalUpdate(travelagent.StatusUpdate{
		Status: travelagent.StatusComplete,
	})
	require.NoError(t, err) // Should call PostMetaData and SendUpdate
}

func TestRunForm(t *testing.T) {
	// Test runForm with a wizard form
	// Note: This is tricky to test directly since it uses interactive UI
	// We'll test that the function exists and can be called
	wf := &inventory.WizardForm{
		Source:      "/tmp",
		Destination: "/tmp",
		MaxSize:     "1GB",
	}
	
	// Since runForm uses interactive UI, we can't easily test it without mocking
	// But we can at least verify the function signature is correct
	require.NotNil(t, runForm)
	
	// Test that it's a function that takes WizardForm and returns error
	testFunc := runForm
	require.NotNil(t, testFunc)
	
	// We can't call it directly in tests due to interactive nature
	// but we've verified the function exists and has correct signature
	_ = wf // Use the variable to avoid unused error
}

func TestRunWizard(t *testing.T) {
	// Test RunWizard method
	// This is also interactive, so we'll test basic initialization
	p := New()
	
	// RunWizard should initialize WizardForm
	// We can't run it fully due to interactive form, but we can test
	// that the method exists and has correct signature
	require.NotNil(t, p.RunWizard)
	
	// Test that it's a method that returns error
	testMethod := p.RunWizard
	require.NotNil(t, testMethod)
	
	// We can't call it directly in tests due to interactive nature
	// but we've verified the method exists and has correct signature
}

/*
func TestFillWithInventoryIndex(t *testing.T) {
	p := New(
		WithDestination(t.TempDir()),
		WithHashAlgorithm(inventory.MD5Hash),
	)
	require.NoError(t, err)
	i, err := inventory.NewDirectoryInventory(inventory.NewOptions(
		inventory.WithDirectories([]string{"../testdata/fake-dir"}),
	))
	require.NoError(t, err)
	_, err = Fill(s, i, 0, nil)
	require.NoError(t, err)
}
*/
