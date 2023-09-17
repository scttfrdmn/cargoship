package rclone

import (
	"path"
	"testing"

	"github.com/rclone/rclone/fs/rc"
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

func TestCopyParamsWithSrcDest(t *testing.T) {
	got := copyParamsWithSrcDest("/tmp/foo.txt", "cloud/foo/bar/")
	require.Equal(
		t,
		/*
			rc.Params{
				"srcFs":     "/tmp",
				"srcRemote": "foo.txt",
				"dstFs":     "cloud/foo/bar/",
				"dstRemote": "foo.txt",
				"_async":    true,
				"_group":    "foo.txt",
			},
		*/
		rc.Params{"_async": true, "_group": "foo.txt", "dstFs": "cloud/foo/bar/", "srcFs": "/tmp/foo.txt"},
		got,
	)
}

func TestCopy(t *testing.T) {
	d := t.TempDir()
	// err := Copy("testdata/file-1mb.txt", d, nil)
	c := make(chan TransferStatus)
	var status TransferStatus
	go func() {
		for {
			item := <-c
			status = item
			/*
				if item.Stats.Bytes > 0 {
					fmt.Fprintf(os.Stderr, "ITEM::: %+v\n", item)
				}
			*/
		}
	}()
	err := Copy("../testdata/archives/self-tarred.tar", d, c)
	// err := Copy("testdata/fake-file.txt", d, nil)
	require.NoError(t, err)
	require.FileExists(t, path.Join(d, "self-tarred.tar"))
	close(c)
	require.Equal(t, int64(3154625), status.Stats.TotalBytes)
}

func TestCopyFail(t *testing.T) {
	err := Copy("testdata/fake-file.txt", "never-exists:/foo", nil)
	require.Error(t, err)
	// require.EqualError(t, err, "didn't find section in config file")
	require.EqualError(t, err, "didn't find section in config file")
}
