package pipeline

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// partialSink is a minimal S3 endpoint that accepts single-part PUTs and keeps
// the last body it was sent.
type partialSink struct {
	mu   sync.Mutex
	last []byte
	puts atomic.Int64
}

func (s *partialSink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if r.Method == http.MethodPut {
		s.mu.Lock()
		s.last = body
		s.mu.Unlock()
		s.puts.Add(1)
	}
	w.Header().Set("ETag", `"cargoship-test"`)
	w.WriteHeader(http.StatusOK)
}

// TestSavePartialManifest_NoRaceWithTheUpload pins #1008. Builder.Build returns
// the live manifest, and savePartialManifest serialised it after releasing the
// lock while the uploaders went on appending chunks and binding files: a data
// race that could write a torn resume snapshot, or crash on a torn slice. The
// suite's 30 s interval hid it. Run with -race: saving every millisecond while
// chunks are added must be race-free, and each saved snapshot must be whole.
func TestSavePartialManifest_NoRaceWithTheUpload(t *testing.T) {
	sink := &partialSink{}
	srv := httptest.NewServer(sink)
	defer srv.Close()
	client := s3.New(s3.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(srv.URL),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
	})

	builder, err := manifest.NewBuilder("u-race", "/src", "b", "p", "us-east-1")
	require.NoError(t, err)
	p := &Pipeline{
		config: &PipelineConfig{
			S3Client: client, S3Bucket: "b", S3Prefix: "p", UploadID: "u-race",
			UseRealS3: true, EnablePartialManifest: true,
			PartialManifestSaveInterval: time.Millisecond,
		},
		manifestBuilder: builder,
	}

	ctx, cancel := context.WithCancel(context.Background())
	saved := make(chan struct{})
	go func() {
		defer close(saved)
		p.savePartialManifestPeriodically(ctx)
	}()

	// Record chunks the way the uploader does while the saves run: first adding
	// them, then re-binding files in place, until enough saves have overlapped.
	const chunks = 1000
	deadline := time.Now().Add(30 * time.Second)
	for i := 0; i < chunks || sink.puts.Load() < 20; i++ {
		require.True(t, time.Now().Before(deadline), "the periodic save never ran")
		id := i % chunks
		path := fmt.Sprintf("/src/f%06d", id)
		key := fmt.Sprintf("p/uploads/u-race/shard-0/chunk-%d.tar.zst", id)
		if i < chunks {
			builder.AddFileBatch([]manifest.FileEntry{{Path: path, Size: 1, ChunkID: id, ShardID: -1}})
			builder.AddChunk(manifest.ChunkEntry{ID: id, S3Key: key, FileCount: 1, FilePaths: []string{path}})
		}
		builder.BindChunkFiles(id, 0, key, []manifest.ChunkFile{{Path: path, Size: int64(i)}})
	}
	cancel()
	<-saved

	sink.mu.Lock()
	last := sink.last
	sink.mu.Unlock()
	zr, err := gzip.NewReader(bytes.NewReader(last))
	require.NoError(t, err)
	var m manifest.Manifest
	require.NoError(t, json.NewDecoder(zr).Decode(&m))
	assert.NotEmpty(t, m.Chunks)
	assert.Equal(t, len(m.Chunks), m.TotalChunks, "a snapshot is taken at one instant")
}
