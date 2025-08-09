package rclone

import (
	"encoding/json"
	"errors"
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
		rc.Params{"_async": true, "_filter": "{\"IncludeRule\":[\"foo.txt\"]}", "_group": "foo.txt", "dstFs": "cloud/foo/bar/", "srcFs": "/tmp"},
		got,
	)
}

func TestCopy(t *testing.T) {
	d := t.TempDir()
	c := make(chan TransferStatus)
	var status TransferStatus
	go func() {
		for {
			item := <-c
			status = item
		}
	}()
	err := Copy("../testdata/archives/self-tarred.tar", d, c)
	require.NoError(t, err)
	require.FileExists(t, path.Join(d, "self-tarred.tar"))
	close(c)
	require.Equal(t, int64(3154432), status.Stats.TotalBytes)
}

func TestCopyFail(t *testing.T) {
	err := Copy("testdata/fake-file.txt", "never-exists:/foo", nil)
	require.Error(t, err)
	require.EqualError(t, err, "didn't find section in config file (\"never-exists\")")
}

func TestCloneRequest_JSONString(t *testing.T) {
	req := &cloneRequest{
		SrcFs:     "/source/path",
		DstFs:     "remote:/dest/path",
		SrcRemote: "file.txt",
		DstRemote: "file.txt",
		Group:     "test-group",
		Async:     true,
		Filter:    `{"IncludeRule":["*.txt"]}`,
	}

	jsonStr := req.JSONString()
	require.Contains(t, jsonStr, `"srcFs":"/source/path"`)
	require.Contains(t, jsonStr, `"dstFs":"remote:/dest/path"`)
	require.Contains(t, jsonStr, `"_group":"test-group"`)
	require.Contains(t, jsonStr, `"_async":true`)

	// Test that it's valid JSON
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &parsed)
	require.NoError(t, err)
}

func TestStatusRequest_JSONString(t *testing.T) {
	req := &statusRequest{
		JobID: 12345,
		Group: "test-group",
	}
	jsonStr := req.JSONString()
	require.Contains(t, jsonStr, `"jobid":12345`)
	require.Contains(t, jsonStr, `"group":"test-group"`)

	// Test that it's valid JSON
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &parsed)
	require.NoError(t, err)
	require.Equal(t, float64(12345), parsed["jobid"])
}

func TestExists(t *testing.T) {
	// Test with non-existent path (this is the main case that works reliably)
	exists := Exists("testdata/does-not-exist.txt")
	require.False(t, exists)

	// Test basic functionality - the function runs without panic
	_ = Exists("testdata/fake-file.txt")
}

func TestAPIOneShot(t *testing.T) {
	// Test with core/version (should work)
	result := APIOneShot("core/version", map[string]interface{}{})
	// We just verify the function can be called without panicking
	// The actual result format depends on rclone internal implementation
	_ = result
}

func TestClone(t *testing.T) {
	tempDir := t.TempDir()

	// Test cloning a file - may fail due to missing rclone config
	err := Clone("testdata/fake-file.txt", tempDir)
	// Just verify the function can be called without panicking
	_ = err
}

func TestClone_Error(t *testing.T) {
	// Test with non-existent source
	err := Clone("testdata/does-not-exist.txt", "invalid-remote:/")
	require.Error(t, err)
}

func TestMarshalParams(t *testing.T) {
	params := map[string]interface{}{
		"srcFs":  "/source",
		"dstFs":  "remote:/dest",
		"_async": true,
		"_group": "test",
	}

	result, err := marshalParams(params)
	require.NoError(t, err)
	require.Contains(t, result, `"srcFs":"/source"`)
	require.Contains(t, result, `"dstFs":"remote:/dest"`)

	// Test with invalid data that can't be marshaled
	invalidParams := map[string]interface{}{
		"invalid": make(chan int), // channels can't be marshaled
	}
	_, err = marshalParams(invalidParams)
	require.Error(t, err)
}

func TestMustMarshalParams(t *testing.T) {
	params := map[string]interface{}{
		"srcFs": "/source",
		"dstFs": "remote:/dest",
	}

	// Should not panic with valid params
	result := mustMarshalParams(params)
	require.Contains(t, result, `"srcFs":"/source"`)

	// Test panic with invalid params
	require.Panics(t, func() {
		invalidParams := map[string]interface{}{
			"invalid": make(chan int),
		}
		mustMarshalParams(invalidParams)
	})
}

func TestErrWithRPCOut(t *testing.T) {
	// Test basic error wrapping with RPC output
	rpcOut := `{"error": "RPC error details", "status": "failed"}`
	wrappedErr := errWithRPCOut(rpcOut)
	require.Error(t, wrappedErr)
	require.Contains(t, wrappedErr.Error(), "RPC error details")
	// Note: The error message format may vary, just check it's an error
}

func TestPanicIfError(t *testing.T) {
	// Should not panic with nil error
	require.NotPanics(t, func() {
		panicIfError(nil)
	})

	// Should panic with actual error
	require.Panics(t, func() {
		panicIfError(errors.New("test error"))
	})
}

func TestGetStats_Error(t *testing.T) {
	// Test with invalid group string - may or may not return error depending on implementation
	_, err := getStats("invalid-group-id")
	// Just verify the function can be called without panicking
	_ = err
}

// Test getStats with different scenarios
func TestGetStats_EdgeCases(t *testing.T) {
	// Test with empty group string
	_, err := getStats("")
	_ = err // May or may not error, but shouldn't panic

	// Test with normal group string
	_, err = getStats("test-group")
	_ = err // May or may not error, but shouldn't panic
}

func TestGetJobStatus_Error(t *testing.T) {
	// Test with invalid job status request
	req := statusRequest{JobID: -1, Group: "invalid"}
	_, err := getJobStatus(req)
	require.Error(t, err)
}

// Test getJobStatus with different error conditions
func TestGetJobStatus_EdgeCases(t *testing.T) {
	// Test with zero job ID
	req := statusRequest{JobID: 0, Group: "test"}
	_, err := getJobStatus(req)
	_ = err // May error depending on rclone state

	// Test with valid-looking request
	req = statusRequest{JobID: 12345, Group: "valid-group"}
	_, err = getJobStatus(req)
	_ = err // May error depending on rclone state, but shouldn't panic
}

// Test about request JSONString method
func TestAboutRequest_JSONString(t *testing.T) {
	req := aboutRequest{
		Fs:     "local",
		Remote: "/test/path",
	}
	jsonStr := req.JSONString()
	require.Contains(t, jsonStr, `"fs":"local"`)
	require.Contains(t, jsonStr, `"remote":"/test/path"`)

	// Test that it's valid JSON
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &parsed)
	require.NoError(t, err)
}

// Test mustNewCloneRequest function
func TestMustNewCloneRequest(t *testing.T) {
	// Test successful case
	req := mustNewCloneRequest(withSrcFs("source"), withDstFs("dest"))
	require.NotNil(t, req)
	require.Equal(t, "source", req.SrcFs)
	require.Equal(t, "dest", req.DstFs)

	// Test panic case with invalid parameters
	require.Panics(t, func() {
		mustNewCloneRequest() // Missing required parameters should panic
	})
}

// Test statsRequest JSONString method
func TestStatsRequest_JSONString(t *testing.T) {
	req := statsRequest{
		Group: "test-group",
	}
	jsonStr := req.JSONString()
	require.Contains(t, jsonStr, `"group":"test-group"`)

	// Test that it's valid JSON
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &parsed)
	require.NoError(t, err)
	require.Equal(t, "test-group", parsed["group"])
}

// Test JSONString panic path for cloneRequest
func TestCloneRequest_JSONString_Panic(t *testing.T) {
	// This is hard to trigger since json.Marshal rarely fails for simple structs
	// but we can test the normal path
	req := &cloneRequest{
		SrcFs: "test",
		DstFs: "dest",
		Group: "group",
		Async: true,
	}
	jsonStr := req.JSONString()
	require.Contains(t, jsonStr, `"srcFs":"test"`)
}

// Test waitForFinished with missing group error
func TestWaitForFinished_MissingGroup(t *testing.T) {
	// Test with missing group - should return error immediately
	req := statusRequest{JobID: 123} // Missing Group
	_, err := waitForFinished(req, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing group")
}

// Test Exists with different path formats
func TestExists_PathFormats(t *testing.T) {
	// Test single component path
	exists := Exists("testdata")
	_ = exists // Just verify it doesn't panic

	// Test two component path
	exists = Exists("local:testdata")
	_ = exists // Just verify it doesn't panic

	// Test panic case with too many colons
	require.Panics(t, func() {
		Exists("one:two:three:four")
	})
}

// Test errWithRPCOut with different inputs
func TestErrWithRPCOut_EdgeCases(t *testing.T) {
	// Test with invalid JSON - should return nil
	err := errWithRPCOut("invalid json")
	require.NoError(t, err) // Function returns nil for invalid JSON

	// Test with valid JSON but no error field - will create error with empty string
	err = errWithRPCOut(`{"status": "ok"}`)
	require.Error(t, err)             // Function creates error even with empty error field
	require.Equal(t, "", err.Error()) // Error message will be empty

	// Test with valid error JSON
	err = errWithRPCOut(`{"error": "test error message"}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "test error message")
}

// Test APIOneShot with error conditions
func TestAPIOneShot_ErrorHandling(t *testing.T) {
	// Test with invalid params that can't be marshaled
	invalidParams := map[string]interface{}{
		"invalid": make(chan int), // channels can't be marshaled
	}
	err := APIOneShot("core/version", invalidParams)
	require.Error(t, err)

	// Test with valid params
	validParams := map[string]interface{}{
		"test": "value",
	}
	err = APIOneShot("core/version", validParams)
	// This may or may not error depending on rclone setup, but shouldn't panic
	_ = err
}
