package pipeline

import (
	"fmt"
	"io"
	"sync/atomic"
)

// countingReadCloser counts the bytes read through it. Installed once per job
// over the archive stream, so the uploader can tell whether a failed attempt
// consumed any of it (#901).
type countingReadCloser struct {
	rc io.ReadCloser
	n  atomic.Int64
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	n, err := c.rc.Read(p)
	c.n.Add(int64(n))
	return n, err
}

func (c *countingReadCloser) Close() error { return c.rc.Close() }

// trackArchiveReads wraps job.Archive in a byte counter, once. Every uploader
// calls it before reading the archive; later calls are no-ops, so wrapping on
// each attempt doesn't stack counters.
func (j *Job) trackArchiveReads() {
	if j.Archive == nil || j.archiveCounter != nil {
		return
	}
	j.archiveCounter = &countingReadCloser{rc: j.Archive}
	j.Archive = j.archiveCounter
}

// ArchiveBytesConsumed reports how many bytes of the archive stream have been
// read by upload attempts so far.
func (j *Job) ArchiveBytesConsumed() int64 {
	if j.archiveCounter == nil {
		return 0
	}
	return j.archiveCounter.n.Load()
}

// retryableAfter decides whether a failed upload attempt may be retried, and
// returns the error to give up with when it may not.
//
// job.Archive is a one-shot pipe from the archiver: bytes read by a failed
// attempt are gone. Retrying then uploads only what is left, often nothing,
// and because the S3 upload carries no Content-Length, S3 stores the short
// object and the attempt "succeeds". That was #901: an empty chunk recorded
// with its full size and checksum, and the upload reported success. A retry is
// safe only while nothing has been read (e.g. the connection failed before the
// body was sent). Otherwise the job fails, and the chunk is uploaded again from
// source on the next run.
func retryableAfter(job *Job, attemptErr error) (bool, error) {
	consumed := job.ArchiveBytesConsumed()
	if consumed == 0 {
		return true, nil
	}
	return false, fmt.Errorf("upload of %s failed after %d bytes of its one-shot archive stream were read; "+
		"not retrying, since a retry could send only the remainder (#901): %w", job.S3Key, consumed, attemptErr)
}
