package travelagent

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
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
