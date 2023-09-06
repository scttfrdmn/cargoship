package transporters

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUniqifyDest(t *testing.T) {
	got := UniquifyDest("foo:/bar")
	require.True(t, strings.HasPrefix(got, "foo:/bar/"))
	require.Greater(t, got, "foo:/bar/")
}
