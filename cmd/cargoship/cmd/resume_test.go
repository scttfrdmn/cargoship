package cmd

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/scttfrdmn/cargoship/pkg/resume"
)

// TestNewResumePipelineConfig_EnablesResume guards #119: rebuilding a pipeline
// from saved state must set ResumeMode + ResumeUploadID + reuse the prior
// UploadID, or the pipeline would re-upload everything instead of skipping the
// chunks already done. Fails if any of those regress.
func TestNewResumePipelineConfig_EnablesResume(t *testing.T) {
	client := &s3.Client{}
	state := &resume.UploadState{
		UploadID:     "20260101-deadbeef",
		SourceDir:    "/data/src",
		Bucket:       "bucket",
		Prefix:       "prefix",
		Region:       "us-west-2",
		StorageClass: "GLACIER",
		KMSKeyID:     "kms-1",
		ShardCount:   6,
	}

	cfg := newResumePipelineConfig(state, client)

	if !cfg.ResumeMode {
		t.Fatal("#119: resume config must set ResumeMode=true or it re-uploads everything")
	}
	if cfg.ResumeUploadID != state.UploadID {
		t.Fatalf("ResumeUploadID = %q, want %q", cfg.ResumeUploadID, state.UploadID)
	}
	if cfg.UploadID != state.UploadID {
		t.Fatalf("UploadID = %q, want the prior run's %q", cfg.UploadID, state.UploadID)
	}
	if !cfg.UseRealS3 {
		t.Error("resume must use real S3")
	}
	if got, ok := cfg.S3Client.(*s3.Client); !ok || got != client {
		t.Errorf("S3Client = %v, want the provided client", cfg.S3Client)
	}

	// Field mapping from the saved state.
	if cfg.S3Bucket != "bucket" || cfg.S3Prefix != "prefix" || cfg.S3Region != "us-west-2" {
		t.Errorf("bucket/prefix/region not mapped: %+v", cfg)
	}
	if cfg.SourcePath != "/data/src" {
		t.Errorf("SourcePath = %q, want /data/src", cfg.SourcePath)
	}
	if cfg.S3StorageClass != "GLACIER" || cfg.S3SSEKMSKeyId != "kms-1" {
		t.Errorf("storage class / KMS not mapped: class=%q kms=%q", cfg.S3StorageClass, cfg.S3SSEKMSKeyId)
	}
	if cfg.ShardCount != 6 {
		t.Errorf("ShardCount = %d, want 6", cfg.ShardCount)
	}
	if !cfg.EnablePartialManifest {
		t.Error("EnablePartialManifest must be true — it is the resume skip source")
	}

	// Zero shard count in state falls back to a sane default.
	if got := newResumePipelineConfig(&resume.UploadState{}, client).ShardCount; got <= 0 {
		t.Errorf("ShardCount fallback should be > 0, got %d", got)
	}
}
