package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
)

// hashingReadCloser wraps an io.ReadCloser and computes a running hash of every
// byte read through it. The digest is only valid once the underlying stream has
// been fully consumed (i.e. after the upload finishes reading it). It adds no
// buffering and a single hash.Write per Read, so it is safe on the streaming
// upload hot path.
type hashingReadCloser struct {
	rc     io.ReadCloser
	hasher hash.Hash
}

// newHashingReadCloser wraps rc so that reads feed a SHA-256 hasher. Passing a
// nil rc returns nil so callers can wrap unconditionally.
func newHashingReadCloser(rc io.ReadCloser) *hashingReadCloser {
	if rc == nil {
		return nil
	}
	return &hashingReadCloser{rc: rc, hasher: sha256.New()}
}

// Read reads from the underlying stream and updates the hash with the bytes
// returned. A hash.Hash never errors on Write, so the read result is passed
// through unchanged.
func (h *hashingReadCloser) Read(p []byte) (int, error) {
	n, err := h.rc.Read(p)
	if n > 0 {
		_, _ = h.hasher.Write(p[:n])
	}
	return n, err
}

// Close closes the underlying stream.
func (h *hashingReadCloser) Close() error {
	return h.rc.Close()
}

// Sum returns the hex-encoded digest of everything read so far. Call it only
// after the stream is fully consumed; a partial read yields a partial digest.
func (h *hashingReadCloser) Sum() string {
	return hex.EncodeToString(h.hasher.Sum(nil))
}
