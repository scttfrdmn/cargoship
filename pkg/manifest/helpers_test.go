package manifest

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

// Shared fixtures for the manifest package's tests. These lived in
// extract_test.go until the superseded manifest.Extractor was removed (#308);
// the restore tests that still use them are in batch_restore_test.go and
// direct_restore_test.go.

// mockS3Client serves object bytes from an in-memory map, keyed by S3 key. It
// records the keys it was asked for so tests can assert WHICH object was
// fetched, not merely that the fetch succeeded (#334), and the buckets so they
// can assert WHERE it was fetched from (#335).
//
// When onlyBucket is set, a request for any other bucket fails the way a real
// missing/foreign bucket would — that is what lets a test prove the fetch went
// to the intended bucket rather than the one recorded in the manifest.
type mockS3Client struct {
	chunks     map[string][]byte // S3 key -> compressed tar data
	onlyBucket string            // when set, requests to other buckets fail
	mu         sync.Mutex
	requested  []string
	buckets    []string
}

func (m *mockS3Client) GetObject(ctx context.Context, input *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	bucket := ""
	if input.Bucket != nil {
		bucket = *input.Bucket
	}

	m.mu.Lock()
	m.requested = append(m.requested, *input.Key)
	m.buckets = append(m.buckets, bucket)
	m.mu.Unlock()

	if m.onlyBucket != "" && bucket != m.onlyBucket {
		return nil, fmt.Errorf("NoSuchBucket: %s", bucket)
	}

	data, ok := m.chunks[*input.Key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", *input.Key)
	}

	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(data)),
	}, nil
}

// requestedBuckets returns the deduplicated set of buckets GetObject addressed.
func (m *mockS3Client) requestedBuckets() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := make(map[string]struct{}, len(m.buckets))
	var out []string
	for _, b := range m.buckets {
		if _, ok := seen[b]; !ok {
			seen[b] = struct{}{}
			out = append(out, b)
		}
	}
	return out
}

// requestedKeys returns the deduplicated set of keys GetObject was called with.
func (m *mockS3Client) requestedKeys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := make(map[string]struct{}, len(m.requested))
	var out []string
	for _, k := range m.requested {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	return out
}

// createMockChunks builds a zstd-compressed tar per manifest chunk, keyed by the
// chunk's S3 key, with dummy content sized to match each file entry.
func createMockChunks(t *testing.T, manifest *Manifest) map[string][]byte {
	t.Helper()
	chunks := make(map[string][]byte)

	// Create mock tar.zst archives for each chunk
	for _, chunk := range manifest.Chunks {
		// Get files in this chunk
		var files []FileEntry
		for _, file := range manifest.Files {
			if file.ChunkID == chunk.ID {
				files = append(files, file)
			}
		}

		// Create tar archive
		tarBuf := new(bytes.Buffer)
		tarWriter := tar.NewWriter(tarBuf)

		for _, file := range files {
			header := &tar.Header{
				Name: file.Path,
				Size: file.Size,
				Mode: 0644,
			}

			err := tarWriter.WriteHeader(header)
			require.NoError(t, err)

			// Write dummy file content
			content := bytes.Repeat([]byte("test data\n"), int(file.Size/10)+1)
			_, err = tarWriter.Write(content[:file.Size])
			require.NoError(t, err)
		}

		err := tarWriter.Close()
		require.NoError(t, err)

		// Compress with zstd
		compressedBuf := new(bytes.Buffer)
		encoder, err := zstd.NewWriter(compressedBuf)
		require.NoError(t, err)

		_, err = encoder.Write(tarBuf.Bytes())
		require.NoError(t, err)

		err = encoder.Close()
		require.NoError(t, err)

		chunks[chunk.S3Key] = compressedBuf.Bytes()
	}

	return chunks
}
