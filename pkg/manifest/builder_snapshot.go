package manifest

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"

	"github.com/scttfrdmn/cargoship/pkg/encryption"
)

// BuildSnapshot is the manifest under construction, serialised at one instant
// (#1008).
type BuildSnapshot struct {
	// JSON is the manifest in its ToJSON form.
	JSON []byte
	// UploadID is the manifest's upload ID.
	UploadID string
	// Files and Chunks count the manifest's entries.
	Files, Chunks int
}

// SnapshotJSON serialises the manifest being built while it holds the builder's
// lock, so the upload can keep adding to it (#1008). Build returns the live
// manifest, and serialising that after Build returned raced the uploaders'
// AddChunk and BindChunkFiles: a torn resume snapshot, or a crash on a torn
// slice or string header.
func (b *Builder) SnapshotJSON() (*BuildSnapshot, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	data, err := b.manifest.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("serialize manifest snapshot: %w", err)
	}
	return &BuildSnapshot{
		JSON:     data,
		UploadID: b.manifest.UploadID,
		Files:    len(b.manifest.Files),
		Chunks:   len(b.manifest.Chunks),
	}, nil
}

// ChunkCount returns how many chunks the manifest records so far (thread-safe).
func (b *Builder) ChunkCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.manifest.Chunks)
}

// Compressed returns the snapshot as a plaintext partial manifest object: the
// same gzip-compressed JSON that ToJSONCompressed produces.
func (s *BuildSnapshot) Compressed() ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(s.JSON); err != nil {
		return nil, fmt.Errorf("failed to compress JSON: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}
	return buf.Bytes(), nil
}

// Encrypted returns the snapshot as an encrypted partial manifest object, the
// form EncryptedPartialBytes produces, sealed under kmsKeyID (#913).
func (s *BuildSnapshot) Encrypted(ctx context.Context, kmsClient encryption.KMSClient, kmsKeyID string) ([]byte, error) {
	return sealPartialJSON(ctx, kmsClient, kmsKeyID, s.UploadID, s.JSON)
}
