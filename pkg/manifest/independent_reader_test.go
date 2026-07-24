package manifest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIndependentReader proves the format spec stands alone: a reader using
// ONLY standard tools (gzip, JSON, tar) plus zstd — and NONE of CargoShip's own
// manifest-reading code — can parse a manifest and extract a file's exact bytes,
// following only what docs/reference/format documents (#274). If this test can
// round-trip, a third-party tool in any language can too.
//
// The producer side here also uses only standard tools, mirroring how the real
// pipeline writes the format (tar -> zstd for a chunk; JSON -> gzip for the
// manifest). The point is that reading requires nothing CargoShip-specific.
func TestIndependentReader(t *testing.T) {
	// --- Produce a spec-compliant chunk with standard tools only. ---
	fileContent := []byte("independent reader can extract me exactly\n")
	filePath := "data/hello.txt"
	chunkSum, chunkBytes := buildStdlibChunk(t, map[string][]byte{filePath: fileContent})

	// --- Produce a spec-compliant manifest as plain JSON, then gzip it. ---
	// Field names/shape follow docs/reference/format/manifest.md exactly.
	manifestJSON := map[string]any{
		"version":            "2.0",
		"upload_id":          "20260724-abc123",
		"created_at":         "2026-07-24T00:00:00Z",
		"completed_at":       "2026-07-24T00:01:00Z",
		"source_path":        "/tmp/src",
		"hostname":           "test-host",
		"bucket":             "example-bucket",
		"prefix":             "backups",
		"region":             "us-east-1",
		"total_files":        1,
		"total_bytes":        len(fileContent),
		"total_chunks":       1,
		"shard_count":        1,
		"compression_type":   "zstd",
		"compression_level":  3,
		"compression_ratio":  0.5,
		"checksum_algorithm": "sha256",
		"files": []map[string]any{{
			"path": filePath, "size": len(fileContent), "mod_time": "2026-07-24T00:00:00Z",
			"chunk_id": 0, "shard_id": 0, "s3_key": "backups/uploads/20260724-abc123/shard-0/chunk-0.tar.zst",
			"checksum": sha256hexIndep(fileContent),
		}},
		"chunks": []map[string]any{{
			"id": 0, "shard_id": 0, "s3_key": "backups/uploads/20260724-abc123/shard-0/chunk-0.tar.zst",
			"file_count": 1, "file_paths": []string{filePath},
			"uncompressed_size": len(fileContent), "compressed_size": len(chunkBytes),
			"created_at": "2026-07-24T00:00:00Z", "uploaded_at": "2026-07-24T00:01:00Z",
			"checksum": chunkSum,
		}},
		"shards": []map[string]any{{
			"id": 0, "prefix": "backups/uploads/20260724-abc123/shard-0",
			"chunk_count": 1, "file_count": 1,
			"uncompressed_size": len(fileContent), "compressed_size": len(chunkBytes),
			"chunk_keys": []string{"backups/uploads/20260724-abc123/shard-0/chunk-0.tar.zst"},
		}},
	}
	plain, err := json.Marshal(manifestJSON)
	require.NoError(t, err)
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, err = gw.Write(plain)
	require.NoError(t, err)
	require.NoError(t, gw.Close())
	manifestGz := gzBuf.Bytes()

	// ============================================================
	// READER SIDE — standard tools only, per the spec. No pkg/manifest.
	// ============================================================

	// 1. Manifest: gunzip, then parse JSON generically.
	gr, err := gzip.NewReader(bytes.NewReader(manifestGz))
	require.NoError(t, err, "spec: manifest.json.gz is gzip")
	rawManifest, err := io.ReadAll(gr)
	require.NoError(t, err)
	require.NoError(t, gr.Close())

	var m map[string]any
	require.NoError(t, json.Unmarshal(rawManifest, &m), "spec: manifest is JSON")
	assert.Equal(t, "2.0", m["version"])

	// 2. Find our file's entry and its recorded checksum + chunk key.
	files := m["files"].([]any)
	require.Len(t, files, 1)
	fe := files[0].(map[string]any)
	assert.Equal(t, filePath, fe["path"])
	wantChecksum := fe["checksum"].(string)

	// 3. Extract the file from the chunk: zstd-decompress, then tar-walk,
	//    matching by the tar entry name == FileEntry.path (per split-files spec,
	//    non-split files keep their plain path).
	dec, err := zstd.NewReader(bytes.NewReader(chunkBytes))
	require.NoError(t, err, "spec: chunk is a single zstd frame")
	defer dec.Close()
	tr := tar.NewReader(dec)

	var extracted []byte
	for {
		hdr, terr := tr.Next()
		if terr == io.EOF {
			break
		}
		require.NoError(t, terr)
		if hdr.Name == filePath {
			extracted, err = io.ReadAll(tr)
			require.NoError(t, err)
			break
		}
	}
	require.NotNil(t, extracted, "spec-only reader must find the file in the tar")

	// 4. Byte-identical to the original, and the recorded checksum verifies.
	assert.Equal(t, fileContent, extracted, "extracted bytes must match the source exactly")
	assert.Equal(t, wantChecksum, sha256hexIndep(extracted), "recorded checksum must verify against extracted bytes")
}

// buildStdlibChunk builds a spec-compliant .tar.zst chunk from name->content
// using only stdlib tar + zstd, and returns its SHA-256 (hex) and bytes.
func buildStdlibChunk(t *testing.T, entries map[string][]byte) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf)
	require.NoError(t, err)
	tw := tar.NewWriter(zw)
	for name, content := range entries {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(content))}))
		_, err := tw.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, zw.Close())
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:]), buf.Bytes()
}

func sha256hexIndep(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
