package pipeline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashingReadCloser_HashesStreamedBytes(t *testing.T) {
	data := bytes.Repeat([]byte("cargoship-archive-bytes"), 1000)
	want := sha256.Sum256(data)

	h := newHashingReadCloser(io.NopCloser(bytes.NewReader(data)))
	require.NotNil(t, h)

	// Read in small chunks to exercise the running hash across many Reads.
	got, err := io.CopyBuffer(io.Discard, h, make([]byte, 64))
	require.NoError(t, err)
	assert.Equal(t, int64(len(data)), got)
	assert.NoError(t, h.Close())

	assert.Equal(t, hex.EncodeToString(want[:]), h.Sum())
}

func TestHashingReadCloser_NilPassthrough(t *testing.T) {
	assert.Nil(t, newHashingReadCloser(nil))
}

func TestHashingReadCloser_EmptyStream(t *testing.T) {
	h := newHashingReadCloser(io.NopCloser(bytes.NewReader(nil)))
	_, err := io.Copy(io.Discard, h)
	require.NoError(t, err)
	// SHA-256 of the empty input.
	empty := sha256.Sum256(nil)
	assert.Equal(t, hex.EncodeToString(empty[:]), h.Sum())
}

// TestJob_ArchiveChecksum verifies the Job accessor returns "" without a hasher
// and the digest once one is attached and consumed.
func TestJob_ArchiveChecksum(t *testing.T) {
	j := &Job{}
	assert.Equal(t, "", j.ArchiveChecksum(), "no hasher => empty")

	data := []byte("hello world")
	j.archiveHasher = newHashingReadCloser(io.NopCloser(bytes.NewReader(data)))
	j.Archive = j.archiveHasher
	_, err := io.Copy(io.Discard, j.Archive)
	require.NoError(t, err)

	want := sha256.Sum256(data)
	assert.Equal(t, hex.EncodeToString(want[:]), j.ArchiveChecksum())
}
