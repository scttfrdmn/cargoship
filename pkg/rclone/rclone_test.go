package rclone

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewCloneRequest(t *testing.T) {
	_, err := newCloneRequest()
	require.EqualError(t, err, "must set at least SrcFs or SrcRemote")

	_, err = newCloneRequest(withSrcFs("foo"))
	require.EqualError(t, err, "must set at least DstFs or DstRemote")

	got, err := newCloneRequest(withSrcFs("foo"), withDstRemote("bar"))
	require.NoError(t, err)
	require.Equal(t, got, &cloneRequest{SrcFs: "foo", DstFs: "", SrcRemote: "", DstRemote: "bar", Group: "SuitcaseCTLTransfer", Async: true})

	got, err = newCloneRequest(withSrcFs("foo"), withDstRemote("bar"), withGroup("FakeGroup"))
	require.NoError(t, err)
	require.Equal(t, got, &cloneRequest{SrcFs: "foo", DstFs: "", SrcRemote: "", DstRemote: "bar", Group: "FakeGroup", Async: true})
}

func TestNewCloneRequestWithSrcDst(t *testing.T) {
	// Test with directory
	gotS, gotR, err := newCloneRequestWithSrcDst("testdata/fake-dir", "fake-dest:/")
	require.NoError(t, err)
	require.Equal(t, "sync/copy", gotS)
	require.Equal(t, &cloneRequest{SrcFs: "testdata/fake-dir", DstFs: "fake-dest:/", SrcRemote: "", DstRemote: "", Group: "testdata-fake-dir", Async: true}, gotR)

	// Test with a file
	gotS, gotR, err = newCloneRequestWithSrcDst("testdata/fake-dir/thing.txt", "fake-dest:/")
	require.NoError(t, err)
	require.Equal(t, "operations/copyfile", gotS)
	require.Equal(t, &cloneRequest{SrcFs: "testdata/fake-dir", DstFs: "fake-dest:/", SrcRemote: "thing.txt", DstRemote: "thing.txt", Group: "testdata-fake-dir-thing-txt", Async: true}, gotR)

	// With something that doesn't exit
	_, _, err = newCloneRequestWithSrcDst("testdata/never-exists.txt", "fake-dest:/")
	require.EqualError(t, err, "stat testdata/never-exists.txt: no such file or directory")
}
