package travelagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/scttfrdmn/cargoship/pkg/rclone"
)

func TestNew(t *testing.T) {
	got, err := New(
		WithURL("https://example.com"),
		WithToken("foo"),
	)
	require.NoError(t, err)
	require.NotNil(t, got)

	got, err = New(WithToken("foo"))
	require.EqualError(t, err, "must set a URL")
	require.Nil(t, got)

	got, err = New(WithURL("http://example.com"))
	require.EqualError(t, err, "must set a token")
	require.Nil(t, got)

	got, err = New(WithURL("%zzzzz"), WithToken("foo"))
	require.EqualError(t, err, "parse \"%zzzzz\": invalid URL escape \"%zz\"")
	require.Nil(t, got)
}

func TestStatusStrings(t *testing.T) {
	tests := map[Status]string{
		StatusPending:    "pending",
		StatusFailed:     "failed",
		StatusComplete:   "complete",
		StatusInProgress: "in_progress",
	}
	for status, statusS := range tests {
		require.Equal(t, statusS, status.String())
	}
}

func TestStatusUpdateFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, err := io.Copy(w, bytes.NewReader([]byte(`{"errors":["suitcase cannot be updated since it is complete"]}`)))
		panicIfErr(err)
	}))
	c, err := New(
		WithToken("foo"),
		WithURL(srv.URL),
	)
	require.NotNil(t, c)
	require.NoError(t, err)
	_, err = c.Update(StatusUpdate{
		Status: StatusPending,
	})
	// require.Nil(t, update)
	require.Error(t, err)
	require.EqualError(t, err, "suitcase cannot be updated since it is complete")
}

func TestTravelAgentUpload(t *testing.T) {
	fakeDest := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := io.Copy(w, bytes.NewReader([]byte(fmt.Sprintf(`{"destination":"%v"}`, fakeDest))))
		require.NoError(t, err)
		// panicIfErr(err)
	}))
	c, err := New(
		WithToken("foo"),
		WithURL(srv.URL+"/api/v1/suitcase_transfers/1"),
	)
	require.NoError(t, err)
	require.NotNil(t, c)
	_, uerr := c.Upload("../testdata/archives/archive.tar.gz", nil)
	require.NoError(t, uerr)

	require.FileExists(t, path.Join(fakeDest, "archive.tar.gz"))
}

func TestPostMetaData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	c, err := New(
		WithToken("foo"),
		WithURL(srv.URL+"/api/v1/suitcase_transfers/1"),
	)
	require.NoError(t, err)
	require.NotNil(t, c)

	assert.EqualError(t, c.PostMetaData("never-exists.yaml"), "open never-exists.yaml: no such file or directory")
	require.NoError(t, c.PostMetaData("../testdata/inventories/example-inventory.yaml"))
}

func TestStatusUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := io.Copy(w, bytes.NewReader([]byte(`{"messages":["updated fields: status and updated_at"]}`)))
		panicIfErr(err)
	}))
	c, err := New(
		WithToken("foo"),
		WithURL(srv.URL),
	)
	require.NoError(t, err)
	require.NotNil(t, c)

	got, err := c.Update(StatusUpdate{
		Status: StatusPending,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"updated fields: status and updated_at"}, got.Messages)
}

func TestStatusMarshal(t *testing.T) {
	su := StatusUpdate{
		Status: StatusFailed,
	}
	got, err := json.Marshal(su)
	// got, err := su.Status.MarshalJSON()
	require.NoError(t, err)
	require.Equal(
		t,
		`{"status":"failed"}`,
		string(got),
	)
}

func TestBindCmd(t *testing.T) {
	cmd := cobra.Command{}
	BindCobra(&cmd)

	// Test with credential blob
	cmd.SetArgs([]string{"--travel-agent", "ewogICJ1cmwiOiAiaHR0cHM6Ly9leGFtcGxlLmNvbS9hcGkvdjEvc3VpdGNhc2VfdHJhbnNmZXJzLzEiLAogICJwYXNzd29yZCI6ICJzZWNyZXQtdG9rZW4iCn0K"})
	cmd.Execute()
	ta, err := New(WithCmd(&cmd))
	require.NoError(t, err)
	require.Equal(t, ta.URL.String(), "https://example.com/api/v1/suitcase_transfers/1")
	require.Equal(t, ta.Token, "secret-token")

	// Test with url/token
	cmd = cobra.Command{}
	BindCobra(&cmd)
	cmd.SetArgs([]string{
		"--travel-agent-url", "https://www.example.com/api/v1/suitcase_transfers/2",
		"--travel-agent-token", "another-token",
	})
	cmd.Execute()
	ta, err = New(
		WithCmd(&cmd),
		WithMetaTokenExpiration(5*time.Minute),
		WithTokenExpiration(48*time.Hour),
	)
	require.NoError(t, err)
	require.Equal(t, ta.URL.String(), "https://www.example.com/api/v1/suitcase_transfers/2")
	require.Equal(t, ta.Token, "another-token")
	require.Equal(t, "https://www.example.com/suitcase_transfers/2", ta.StatusURL())
	require.Equal(t, "https://www.example.com/api/v1/suitcase_transfers/2/credentials", ta.credentialURL())
	require.Equal(t, "https://www.example.com/api/v1/suitcase_transfers/2/suitcase_components/foo", ta.componentURL("foo"))
}

func TestNewStatusUpdate(t *testing.T) {
	got := NewStatusUpdate(rclone.TransferStatus{
		Name: "foo",
	})
	require.Equal(t, "foo", got.Name)
}

/*
func copyResp(f string, w io.Writer) {
	b, err := os.ReadFile(f)
	panicIfErr(err)
	_, err = io.Copy(w, bytes.NewReader(b))
	panicIfErr(err)
}
*/

func TestCredentialConnectionStrings(t *testing.T) {
	tests := map[string]struct {
		given  credentialResponse
		expect string
	}{
		"azure-blob-sas-url": {
			given: credentialResponse{
				AuthType: map[string]string{
					"type":    "azureblob",
					"sas_url": "https://foo.blob.core.windows.net/test?sp=racwdli&st=2023-10-13T13:05:07Z&se=2023-10-20T21:05:07Z&spr=https&sv=2022-11-02&sr=c&sig=some-token",
				},
				Destination: "some-container/",
			},
			expect: `:azureblob,sas_url='https://foo.blob.core.windows.net/test?sp=racwdli&st=2023-10-13T13:05:07Z&se=2023-10-20T21:05:07Z&spr=https&sv=2022-11-02&sr=c&sig=some-token':`,
		},
		"assume-local": {
			given: credentialResponse{
				Destination: "/tmp",
			},
			expect: ":local:",
		},
	}
	for desc, tt := range tests {
		require.Equal(
			t,
			tt.expect,
			tt.given.connectionString(),
			desc,
		)
	}
}
