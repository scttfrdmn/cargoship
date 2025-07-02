package travelagent

import (
	"encoding/json"
	"fmt"
	"net"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	token := "some-token"
	dest := t.TempDir()
	srv := NewServer(
		WithAdminToken(token),
		WithStaticTransfers([]credentialResponse{
			{
				AuthType:      map[string]string{},
				Destination:   dest,
				ExpireSeconds: 60,
			},
		}),
	)
	require.NotNil(t, srv)

	go func() { _ = srv.Run() }() // Background server for test
	// Give it a sec to spin up
	time.Sleep(time.Second * 1)

	// Now test with a good token
	c, err := New(
		WithURL(fmt.Sprintf("http://localhost:%v/api/v1/suitcase_transfers/0", srv.Port())),
		WithToken(token),
	)
	require.NoError(t, err)
	require.NotNil(t, c)
	// Get credentials
	creds, err := c.getCredentials()
	require.NoError(t, err)
	require.Equal(t, 60, creds.ExpireSeconds)

	// Send a status update
	resp, err := c.Update(StatusUpdate{Status: StatusInProgress})
	require.NoError(t, err)
	require.Equal(t,
		&StatusUpdateResponse{
			Messages: []string{"updated fields: status"},
		},
		resp,
	)

	// Same status as before
	resp, err = c.Update(StatusUpdate{Status: StatusInProgress})
	require.NoError(t, err)
	require.Equal(t,
		&StatusUpdateResponse{
			Messages: []string{"updated fields: "},
		},
		resp,
	)
	// Now update a size
	resp, err = c.Update(StatusUpdate{
		Status:    StatusInProgress,
		SizeBytes: 5,
	})
	require.NoError(t, err)
	require.Equal(t,
		&StatusUpdateResponse{
			Messages: []string{"updated fields: size"},
		},
		resp,
	)

	// Update a component
	resp, err = c.Update(StatusUpdate{
		Name:   "some-thing.tar.gz",
		Status: StatusInProgress,
	})
	require.NoError(t, err)
	require.Equal(t,
		&StatusUpdateResponse{
			Messages: []string{"updated fields: status"},
		},
		resp,
	)
}

func TestBadTokenServer(t *testing.T) {
	token := "some-token"
	srv := NewServer(
		WithAdminToken(token),
		WithStaticTransfers([]credentialResponse{
			{
				AuthType:      map[string]string{},
				Destination:   t.TempDir(),
				ExpireSeconds: 60,
			},
		}),
	)
	require.NotNil(t, srv)

	go func() { _ = srv.Run() }() // Background server for test
	// Give it a sec to spin up
	time.Sleep(time.Second * 1)

	// Test with a bad token
	bc, err := New(
		WithURL(fmt.Sprintf("http://localhost:%v/api/v1/suitcase_transfers/0/", srv.Port())),
		WithToken("bad-token"),
	)
	require.NoError(t, err)
	require.NotNil(t, bc)
	_, err = bc.Update(StatusUpdate{
		Status: StatusPending,
	})
	require.Error(t, err)
	require.EqualError(t, err, "invalid token")
}

func TestStatusUnmarshal(t *testing.T) {
	var updatedState StatusUpdate
	require.NoError(t, json.Unmarshal([]byte(`{"status":"pending"}`), &updatedState))
}

func TestDB(t *testing.T) {
	td := t.TempDir()
	tf := path.Join(td, "test-db")

	got := NewServer(
		WithDBFile(tf),
	)
	require.NotNil(t, got)
	require.FileExists(t, tf)

	require.NoError(t, got.setState(5, "foo.txt", suitcaseTransferState{Status: StatusInProgress}))

	gotS, gotE := got.getState(5, "foo.txt")
	require.NoError(t, gotE)
	require.Equal(t,
		&suitcaseTransferState{Status: StatusInProgress},
		gotS,
	)
}

// Test 0% coverage function
func TestWithListener(t *testing.T) {
	// Create a custom listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	
	// Test WithListener option
	server := NewServer(WithListener(listener))
	require.NotNil(t, server)
	
	// Verify the listener was set correctly
	require.Equal(t, listener, server.listener)
	require.Equal(t, listener.Addr(), server.Addr())
}
