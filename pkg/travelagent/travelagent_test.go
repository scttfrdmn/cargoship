package travelagent

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/rclone"
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

func TestStatusUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	ta, err = New(WithCmd(&cmd))
	require.NoError(t, err)
	require.Equal(t, ta.URL.String(), "https://www.example.com/api/v1/suitcase_transfers/2")
	require.Equal(t, ta.Token, "another-token")
	require.Equal(t, "https://www.example.com/suitcase_transfers/2", ta.StatusURL())
}

func TestNewStatusUpdate(t *testing.T) {
	got := NewStatusUpdate(rclone.TransferStatus{
		Name: "foo",
	})
	require.Equal(t, "foo", got.ComponentName)
}

/*
func copyResp(f string, w io.Writer) {
	b, err := os.ReadFile(f)
	panicIfErr(err)
	_, err = io.Copy(w, bytes.NewReader(b))
	panicIfErr(err)
}
*/

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}
