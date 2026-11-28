package manifest

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilderSnapshotJSON(t *testing.T) {
	b, err := NewBuilder("u-1", "/src", "bucket", "prefix", "us-east-1")
	require.NoError(t, err)
	assert.Zero(t, b.ChunkCount())

	b.AddFileBatch([]FileEntry{{Path: "/src/a", Size: 3}, {Path: "/src/b", Size: 4}})
	b.AddChunk(ChunkEntry{ID: 0, S3Key: "prefix/uploads/u-1/shard-0/chunk-0.tar.zst", FilePaths: []string{"/src/a", "/src/b"}})
	assert.Equal(t, 1, b.ChunkCount())

	snap, err := b.SnapshotJSON()
	require.NoError(t, err)
	assert.Equal(t, "u-1", snap.UploadID)
	assert.Equal(t, 2, snap.Files)
	assert.Equal(t, 1, snap.Chunks)

	want, err := b.Build().ToJSON()
	require.NoError(t, err)
	assert.Equal(t, want, snap.JSON, "the snapshot is the manifest's ToJSON form")

	// The snapshot doesn't change when the builder does.
	b.AddChunk(ChunkEntry{ID: 1})
	var m Manifest
	require.NoError(t, json.Unmarshal(snap.JSON, &m))
	assert.Len(t, m.Chunks, 1)

	gz, err := snap.Compressed()
	require.NoError(t, err)
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	require.NoError(t, err)
	plain, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.Equal(t, snap.JSON, plain)

	_, err = snap.Encrypted(context.Background(), nil, "")
	assert.ErrorContains(t, err, "a KMS client and key are required")
}

// TestBuilderSnapshotJSON_ConcurrentWriters pins #1008 at the builder: a
// snapshot taken while chunks and file bindings are being recorded is race-free
// (run with -race) and internally consistent.
func TestBuilderSnapshotJSON_ConcurrentWriters(t *testing.T) {
	b, err := NewBuilder("u-race", "/src", "bucket", "prefix", "us-east-1")
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			path := fmt.Sprintf("/src/f%05d", i)
			key := fmt.Sprintf("prefix/uploads/u-race/shard-0/chunk-%d.tar.zst", i)
			b.AddFileBatch([]FileEntry{{Path: path, Size: 1, ChunkID: i}})
			b.AddChunk(ChunkEntry{ID: i, S3Key: key, FilePaths: []string{path}})
			b.BindChunkFiles(i, 0, key, []ChunkFile{{Path: path, Size: 1}})
		}
	}()
	for i := 0; i < 50; i++ {
		snap, err := b.SnapshotJSON()
		require.NoError(t, err)
		var m Manifest
		require.NoError(t, json.Unmarshal(snap.JSON, &m))
		assert.Equal(t, len(m.Chunks), m.TotalChunks)
		assert.Equal(t, snap.Chunks, len(m.Chunks))
	}
	wg.Wait()
}
