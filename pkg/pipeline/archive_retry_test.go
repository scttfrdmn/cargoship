package pipeline

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrackArchiveReads_CountsOnceAndDoesNotStack(t *testing.T) {
	job := &Job{S3Key: "uploads/u/shard-0/chunk-0.tar.zst", Archive: io.NopCloser(strings.NewReader("0123456789"))}
	job.trackArchiveReads()
	first := job.Archive
	job.trackArchiveReads() // a retry's call must not wrap again
	assert.Same(t, first, job.Archive, "trackArchiveReads must be idempotent")

	buf := make([]byte, 4)
	n, err := job.Archive.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, int64(4), job.ArchiveBytesConsumed())
	require.NoError(t, job.Archive.Close())

	assert.Equal(t, int64(0), (&Job{}).ArchiveBytesConsumed(), "no counter means nothing read")
	(&Job{}).trackArchiveReads() // nil Archive is a no-op
}

// TestRetryableAfter pins #901: retry only while the one-shot stream is
// untouched; once bytes are read, give up with an error that says why.
func TestRetryableAfter(t *testing.T) {
	attemptErr := errors.New("InternalError: injected")

	fresh := &Job{S3Key: "k", Archive: io.NopCloser(strings.NewReader("data"))}
	fresh.trackArchiveReads()
	ok, giveUp := retryableAfter(fresh, attemptErr)
	assert.True(t, ok, "a failure before any byte was read is safe to retry")
	assert.NoError(t, giveUp)

	consumed := &Job{S3Key: "uploads/u/chunk-0.tar.zst", Archive: io.NopCloser(strings.NewReader("data"))}
	consumed.trackArchiveReads()
	_, _ = io.ReadAll(consumed.Archive)
	ok, giveUp = retryableAfter(consumed, attemptErr)
	assert.False(t, ok, "a retry after the stream was read would send only the remainder")
	require.Error(t, giveUp)
	assert.ErrorIs(t, giveUp, attemptErr, "the original failure must stay in the chain")
	assert.Contains(t, giveUp.Error(), "uploads/u/chunk-0.tar.zst")
	assert.Contains(t, giveUp.Error(), "4 bytes")
	assert.Contains(t, giveUp.Error(), "#901")
}
