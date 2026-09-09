package cmd

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// TestNewSyncPipelineConfig_UsesRealS3 guards #425: `cargoship sync` must build a
// pipeline that actually uploads. If UseRealS3/S3Client are dropped the pipeline
// silently falls back to the simulated uploader and skips the manifest — the
// command reports success while transferring nothing. This test fails if that
// regresses.
func TestNewSyncPipelineConfig_UsesRealS3(t *testing.T) {
	client := &s3.Client{}
	cfg := newSyncPipelineConfig(syncPipelineParams{
		bucket:           "bucket",
		prefix:           "prefix",
		region:           "us-west-2",
		storageClass:     "STANDARD",
		shardCount:       4,
		shardStrategy:    "round-robin",
		compressionLevel: 0,
		sourcePath:       "/data/src",
		includeFiles:     []string{"a.txt", "b.txt"},
		syncType:         "incremental",
		previousUploadID: "prev-123",
		s3Client:         client,
	})

	if !cfg.UseRealS3 {
		t.Fatal("#425: sync must set UseRealS3=true, else it uploads nothing")
	}
	if cfg.S3Client == nil {
		t.Fatal("#425: sync must set S3Client when UseRealS3 is true")
	}
	if got, ok := cfg.S3Client.(*s3.Client); !ok || got != client {
		t.Errorf("S3Client = %v, want the provided *s3.Client", cfg.S3Client)
	}
	if !cfg.EnableManifest {
		t.Error("sync must enable the manifest")
	}

	// Spot-check the rest of the plumbing so the extraction didn't drop fields.
	if cfg.S3Bucket != "bucket" || cfg.S3Prefix != "prefix" || cfg.S3Region != "us-west-2" {
		t.Errorf("bucket/prefix/region not mapped: %+v", cfg)
	}
	if cfg.ShardCount != 4 || cfg.ShardStrategy != "round-robin" {
		t.Errorf("shard settings not mapped: count=%d strategy=%q", cfg.ShardCount, cfg.ShardStrategy)
	}
	if cfg.SyncType != "incremental" || cfg.PreviousUploadID != "prev-123" {
		t.Errorf("sync/manifest chaining not mapped: type=%q prev=%q", cfg.SyncType, cfg.PreviousUploadID)
	}
	if len(cfg.IncludeOnlyFiles) != 2 {
		t.Errorf("IncludeOnlyFiles = %v, want 2 files", cfg.IncludeOnlyFiles)
	}
	if cfg.CompressionLevel != 0 {
		t.Errorf("CompressionLevel = %d, want 0 (content-aware)", cfg.CompressionLevel)
	}
}
