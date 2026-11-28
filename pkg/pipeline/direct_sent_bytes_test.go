package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// changingS3Client stores what each PutObject reads from its body. Before
// reading the body of the PUTs listed in changeOn it calls change, standing in
// for a writer that modifies the file while it is being uploaded.
type changingS3Client struct {
	mockS3Client
	change   func()
	changeOn map[int]bool

	mu     sync.Mutex
	calls  int
	stored []byte
}

func (c *changingS3Client) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	c.mu.Lock()
	c.calls++
	n := c.calls
	c.mu.Unlock()
	if c.changeOn[n] {
		c.change()
	}
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.stored = body
	c.mu.Unlock()
	return &s3.PutObjectOutput{}, nil
}

// sentBytesBuilder records what the direct uploader tells the manifest.
type sentBytesBuilder struct {
	mu       sync.Mutex
	checksum string
	size     int64
	modTime  time.Time
	calls    int
}

func (b *sentBytesBuilder) UpdateFileS3KeyByPath(_ string, _ int, _, checksum string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checksum = checksum
	b.calls++
}

func (b *sentBytesBuilder) RecordDirectFile(_ string, _ int, _, checksum string, size int64, modTime time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.checksum, b.size, b.modTime = checksum, size, modTime
	b.calls++
}

// runDirectUpload uploads one file through a direct uploader stage.
func runDirectUpload(t *testing.T, client S3Uploader, builder directManifestBuilder, src string, file chunking.File) error {
	t.Helper()
	ctx := context.Background()
	in, out := make(chan *Job, 1), make(chan *Job, 1)
	up, err := NewDirectUploaderStage(&DirectUploaderConfig{
		S3Client: client, Bucket: "b", SourcePath: src, UploadID: "u", FileChecksums: true,
		ManifestBuilder: builder, Workers: 1, MaxRetries: 3, RetryDelay: time.Millisecond,
		WorkerPool: NewAdaptiveWorkerPool(ctx, &AdaptiveWorkerPoolConfig{InitialWorkers: 1, MaxWorkers: 1}),
	}, in, out)
	require.NoError(t, err)
	require.NoError(t, up.Start(ctx))
	in <- &Job{ID: 1, Chunk: chunking.Chunk{ID: 1, Files: []chunking.File{file}}}
	close(in)
	job := <-out
	_ = up.Stop()
	return job.Error
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// TestDirectUpload_ChecksumMatchesBytesSent pins #1015. Direct upload hashed the
// file in one pass and PUT it in a second, so a file that changed in between
// was stored with a checksum of other bytes, and a verifying restore refused
// it. The checksum must be of the bytes sent, a change during the PUT must be
// detected (and the file sent again), and the manifest must record the size and
// modification time that were sent, not the scan's.
func TestDirectUpload_ChecksumMatchesBytesSent(t *testing.T) {
	src := t.TempDir()
	p := filepath.Join(src, "live.log")
	old := []byte("cargoship-test version 1\n")
	require.NoError(t, os.WriteFile(p, old, 0o600))
	scanned := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(p, scanned, scanned))
	scan := chunking.File{Path: p, Size: int64(len(old)), ModTime: scanned}

	t.Run("file rewritten during the PUT", func(t *testing.T) {
		require.NoError(t, os.WriteFile(p, old, 0o600))
		require.NoError(t, os.Chtimes(p, scanned, scanned))
		newer := bytes.ToUpper(old) // same length, different bytes
		changedAt := scanned.Add(time.Minute)
		client := &changingS3Client{changeOn: map[int]bool{1: true}, change: func() {
			require.NoError(t, os.WriteFile(p, newer, 0o600))
			require.NoError(t, os.Chtimes(p, changedAt, changedAt))
		}}
		b := &sentBytesBuilder{}
		require.NoError(t, runDirectUpload(t, client, b, src, scan))

		assert.Equal(t, newer, client.stored)
		assert.Equal(t, sha256Hex(client.stored), b.checksum, "the recorded checksum must be of the stored bytes")
		assert.Equal(t, int64(len(client.stored)), b.size)
		assert.True(t, b.modTime.Equal(changedAt), "the recorded mtime must be the one that was sent (%v)", b.modTime)
		assert.Equal(t, 2, client.calls, "a change during the PUT must send the file again")
	})

	t.Run("file grew after the scan", func(t *testing.T) {
		grown := append(append([]byte(nil), old...), []byte("appended after the scan\n")...)
		require.NoError(t, os.WriteFile(p, grown, 0o600))
		client := &changingS3Client{}
		b := &sentBytesBuilder{}
		require.NoError(t, runDirectUpload(t, client, b, src, scan))

		assert.Equal(t, grown, client.stored)
		assert.Equal(t, sha256Hex(grown), b.checksum)
		assert.Equal(t, int64(len(grown)), b.size, "the manifest must record the size sent, or restore refuses the object")
	})

	t.Run("empty file", func(t *testing.T) {
		e := filepath.Join(src, "empty")
		require.NoError(t, os.WriteFile(e, nil, 0o600))
		client := &changingS3Client{}
		b := &sentBytesBuilder{}
		require.NoError(t, runDirectUpload(t, client, b, src, chunking.File{Path: e}))
		assert.Empty(t, client.stored)
		assert.Equal(t, sha256Hex(nil), b.checksum)
		assert.Zero(t, b.size)
	})

	t.Run("file keeps changing", func(t *testing.T) {
		require.NoError(t, os.WriteFile(p, old, 0o600))
		n := 0
		client := &changingS3Client{changeOn: map[int]bool{1: true, 2: true, 3: true}, change: func() {
			n++
			at := scanned.Add(time.Duration(n) * time.Minute)
			require.NoError(t, os.Chtimes(p, at, at))
		}}
		b := &sentBytesBuilder{}
		err := runDirectUpload(t, client, b, src, scan)
		require.Error(t, err, "a file that never holds still must fail, not be recorded")
		assert.Contains(t, err.Error(), "changed while")
		assert.Zero(t, b.calls, "nothing may be recorded for it")
	})
}

// TestSentBytesHasher pins how the hasher follows the SDK's passes over a body:
// a rewind restarts the hash, so the last full pass is the one hashed, and a
// pass that doesn't read the body in order from the start is not accepted.
func TestSentBytesHasher(t *testing.T) {
	data := []byte("cargoship-test sent bytes")
	newHasher := func() *sentBytesHasher {
		return &sentBytesHasher{r: bytes.NewReader(data), h: sha256.New()}
	}
	size := int64(len(data))

	// Sign pass, rewind, send pass.
	h := newHasher()
	_, err := io.Copy(io.Discard, h)
	require.NoError(t, err)
	_, err = h.Seek(0, io.SeekStart)
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, h)
	require.NoError(t, err)
	sum, ok := h.sum(size)
	require.True(t, ok)
	assert.Equal(t, sha256Hex(data), sum)

	// A length probe (seek to the end and back) leaves the hash intact.
	h = newHasher()
	_, err = h.Seek(0, io.SeekEnd)
	require.NoError(t, err)
	_, err = h.Seek(0, io.SeekStart)
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, h)
	require.NoError(t, err)
	sum, ok = h.sum(size)
	require.True(t, ok)
	assert.Equal(t, sha256Hex(data), sum)

	// A partial pass is not a whole send.
	h = newHasher()
	_, err = h.Read(make([]byte, 4))
	require.NoError(t, err)
	_, ok = h.sum(size)
	assert.False(t, ok)

	// A pass that skipped ahead is not the bytes in order.
	h = newHasher()
	_, err = h.Read(make([]byte, 4))
	require.NoError(t, err)
	_, err = h.Seek(8, io.SeekStart)
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, h)
	require.NoError(t, err)
	_, ok = h.sum(size)
	assert.False(t, ok)
}
