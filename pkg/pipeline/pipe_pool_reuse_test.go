package pipeline

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBufferedPipePool_StaleWriterCannotReachReusedPipe pins #914: Put reset
// the returned writer's closed flag, so a writer still held by the previous job
// could write into, or close, the next job's reused pipe.
func TestBufferedPipePool_StaleWriterCannotReachReusedPipe(t *testing.T) {
	pool := NewBufferedPipePool(1, 1024, 64)
	r1, w1 := pool.Get()
	_, err := w1.Write([]byte("job A"))
	require.NoError(t, err)
	require.NoError(t, w1.Close())
	_, err = io.ReadAll(r1)
	require.NoError(t, err)
	pool.Put(r1, w1)

	r2, w2 := pool.Get() // the same pipe, reused by job B
	_, staleErr := w1.Write([]byte("STALE"))
	assert.Error(t, staleErr, "the previous job's writer must stay closed")

	_, err = w2.Write([]byte("job B"))
	require.NoError(t, err)
	require.NoError(t, w2.Close())
	got, err := io.ReadAll(r2)
	require.NoError(t, err)
	assert.Equal(t, "job B", string(got), "job B must read only its own bytes")
}

// TestJobReleasePipe_WaitsForArchiver pins #914's ordering: the pipe goes back
// to the pool only after the archiver goroutine has exited.
func TestJobReleasePipe_WaitsForArchiver(t *testing.T) {
	pool := NewBufferedPipePool(1, 1024, 64)
	r, w := pool.Get()
	done := make(chan struct{})
	job := &Job{pipeReader: r, pipeWriter: w, pipePool: pool, archiveDone: done}

	released := make(chan struct{})
	go func() { job.releasePipe(); close(released) }()

	select {
	case <-released:
		t.Fatal("the pipe was returned while the archiver was still running")
	case <-time.After(50 * time.Millisecond):
	}
	close(done)
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("releasePipe did not return after the archiver exited")
	}
	job.releasePipe() // a second release is a no-op, not a double Put

	got, _ := pool.Get()
	assert.NotNil(t, got, "the pipe is back in the pool exactly once")
}
