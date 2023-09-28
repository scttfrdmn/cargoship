package transporters

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUniqifyDest(t *testing.T) {
	got := UniquifyDest("foo:/bar")
	require.True(t, strings.HasPrefix(got, "foo:/bar/"))
	require.Greater(t, got, "foo:/bar/")
}

func TestToEnv(t *testing.T) {
	c := Config{
		Destination: "/tmp/foo",
	}
	require.NoError(t, c.ToEnv())
	require.Equal(t, "/tmp/foo", os.Getenv("SUITCASECTL_DESTINATION"))
}
