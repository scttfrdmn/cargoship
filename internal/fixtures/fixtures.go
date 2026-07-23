// Package fixtures provides shared, deterministic test fixtures for CargoShip:
// a fluent builder for manifests and on-disk file trees, plus golden-file
// helpers with output normalization. It exists to replace the ad-hoc,
// per-test fixture code that drifted between suites (#238 Phase 3).
//
// Everything here is deterministic: fixed timestamps, seeded content, stable
// ordering — so a fixture used in a golden test produces identical bytes on
// every run and every machine.
package fixtures

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// FixedTime is the canonical timestamp stamped into fixtures. Using one fixed
// instant (rather than time.Now) keeps manifests and golden output stable.
var FixedTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// ManifestBuilder builds a *manifest.Manifest with sensible, deterministic
// defaults. Chain the With* methods and call Build.
type ManifestBuilder struct {
	m manifest.Manifest
}

// NewManifest returns a ManifestBuilder seeded with deterministic defaults:
// version 2.0, a fixed upload ID and timestamps, and an example bucket/prefix.
func NewManifest() *ManifestBuilder {
	return &ManifestBuilder{
		m: manifest.Manifest{
			Version:     manifest.ManifestVersion,
			UploadID:    "20260102-testfixture",
			CreatedAt:   FixedTime,
			CompletedAt: FixedTime,
			SourcePath:  "/src",
			Hostname:    "test-host",
			Bucket:      "test-bucket",
			Prefix:      "archives",
			Region:      "us-east-1",
			Files:       []manifest.FileEntry{},
			Chunks:      []manifest.ChunkEntry{},
			Shards:      []manifest.ShardEntry{},
		},
	}
}

// WithUploadID overrides the upload ID.
func (b *ManifestBuilder) WithUploadID(id string) *ManifestBuilder {
	b.m.UploadID = id
	return b
}

// WithBucket overrides bucket, prefix, and region.
func (b *ManifestBuilder) WithBucket(bucket, prefix, region string) *ManifestBuilder {
	b.m.Bucket, b.m.Prefix, b.m.Region = bucket, prefix, region
	return b
}

// WithFile appends a FileEntry with a deterministic ModTime and updates the
// TotalFiles/TotalBytes statistics. chunkID/shardID place the file in the
// archive layout.
func (b *ManifestBuilder) WithFile(path string, size int64, chunkID, shardID int) *ManifestBuilder {
	b.m.Files = append(b.m.Files, manifest.FileEntry{
		Path:    path,
		Size:    size,
		ModTime: FixedTime,
		ChunkID: chunkID,
		ShardID: shardID,
		S3Key:   fmt.Sprintf("%s/uploads/%s/shard-%d/chunk-%d.tar.zst", b.m.Prefix, b.m.UploadID, shardID, chunkID),
	})
	b.m.TotalFiles = int64(len(b.m.Files))
	b.m.TotalBytes += size
	return b
}

// WithSimpleFiles appends n files named file-000.dat, file-001.dat, … each of
// the given size, all in chunk 0 / shard 0. Convenience for the common case.
func (b *ManifestBuilder) WithSimpleFiles(n int, size int64) *ManifestBuilder {
	for i := 0; i < n; i++ {
		b.WithFile(fmt.Sprintf("file-%03d.dat", i), size, 0, 0)
	}
	return b
}

// Build returns the assembled manifest. Files are sorted by path so the result
// is stable regardless of insertion order.
func (b *ManifestBuilder) Build() *manifest.Manifest {
	sort.Slice(b.m.Files, func(i, j int) bool { return b.m.Files[i].Path < b.m.Files[j].Path })
	m := b.m
	return &m
}

// Tree describes an on-disk file tree to materialize under a temp directory.
// Keys are relative paths (forward slashes); values are file contents.
type Tree map[string]string

// TinyTree is a minimal two-file tree.
func TinyTree() Tree {
	return Tree{
		"greeting.txt": "hello\n",
		"notes.txt":    "second file\n",
	}
}

// MixedTree is a small tree spanning nested dirs and a couple of content types,
// useful for exercising path handling and compression selection.
func MixedTree() Tree {
	return Tree{
		"readme.md":         "# Fixture\n\nmixed content tree.\n",
		"data/a.csv":        "id,name\n1,alpha\n2,beta\n",
		"data/b.json":       `{"k":"v","n":42}` + "\n",
		"src/main.go":       "package main\n\nfunc main() {}\n",
		"nested/deep/x.txt": "deep file\n",
	}
}

// Write materializes the tree under a fresh temp directory and returns its root.
// Files are created with deterministic mtimes so downstream metadata is stable.
func (t Tree) Write(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	// Deterministic order for reproducibility.
	names := make([]string, 0, len(t))
	for name := range t {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			tb.Fatalf("fixtures: mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(p, []byte(t[name]), 0o644); err != nil {
			tb.Fatalf("fixtures: write %s: %v", name, err)
		}
		if err := os.Chtimes(p, FixedTime, FixedTime); err != nil {
			tb.Fatalf("fixtures: chtimes %s: %v", name, err)
		}
	}
	return root
}
