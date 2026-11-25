package cmd

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms"
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
		ChunkSizeMB:  48,
	}

	cfg := newResumePipelineConfig(state, client, nil)

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
	if cfg.S3StorageClass != "GLACIER" || cfg.KMSKeyID != "kms-1" {
		t.Errorf("storage class / KMS not mapped: class=%q kms=%q", cfg.S3StorageClass, cfg.KMSKeyID)
	}
	if cfg.ShardCount != 6 {
		t.Errorf("ShardCount = %d, want 6", cfg.ShardCount)
	}
	if cfg.ForceChunkSizeMB != 48 {
		t.Errorf("#904: ForceChunkSizeMB = %d, want the saved 48", cfg.ForceChunkSizeMB)
	}
	if !cfg.EnablePartialManifest {
		t.Error("EnablePartialManifest must be true — it is the resume skip source")
	}

	// Zero shard count in state falls back to a sane default.
	if got := newResumePipelineConfig(&resume.UploadState{}, client, nil).ShardCount; got <= 0 {
		t.Errorf("ShardCount fallback should be > 0, got %d", got)
	}
}

// TestNewResumePipelineConfig_KeepsManifestEncryption pins #913: resume kept the
// data key but not --encrypt-manifest, so the resumed manifest was plaintext.
func TestNewResumePipelineConfig_KeepsManifestEncryption(t *testing.T) {
	state := &resume.UploadState{UploadID: "u", KMSKeyID: "arn:aws:kms:us-east-1:1:key/k", EncryptManifest: true}
	k := &kms.Client{}
	cfg := newResumePipelineConfig(state, &s3.Client{}, k)
	if !cfg.EncryptManifest || cfg.KMSKeyID != state.KMSKeyID {
		t.Fatalf("EncryptManifest=%v KMSKeyID=%q, want true and the saved key", cfg.EncryptManifest, cfg.KMSKeyID)
	}
	if got, ok := cfg.KMSClient.(*kms.Client); !ok || got != k {
		t.Errorf("KMSClient = %v, want the provided client", cfg.KMSClient)
	}
	if cfg := newResumePipelineConfig(&resume.UploadState{}, &s3.Client{}, nil); cfg.KMSClient != nil {
		t.Error("a nil KMS client must stay unset, not a typed nil that looks configured")
	}
}

func TestResumeEncryption(t *testing.T) {
	const key = "arn:aws:kms:us-east-1:1:key/k"
	tests := []struct {
		name        string
		state       resume.UploadState
		flagKey     string
		flagEncrypt bool
		wantKey     string
		wantEncrypt bool
		wantErr     bool
	}{
		{name: "unencrypted upload", state: resume.UploadState{UploadID: "u"}},
		{name: "adopts saved key and manifest encryption", state: resume.UploadState{UploadID: "u", KMSKeyID: key, EncryptManifest: true}, wantKey: key, wantEncrypt: true},
		{name: "same flags", state: resume.UploadState{UploadID: "u", KMSKeyID: key, EncryptManifest: true}, flagKey: key, flagEncrypt: true, wantKey: key, wantEncrypt: true},
		{name: "data key only", state: resume.UploadState{UploadID: "u", KMSKeyID: key}, wantKey: key},
		{name: "conflicting key", state: resume.UploadState{UploadID: "u", KMSKeyID: key}, flagKey: "other", wantErr: true},
		{name: "encrypted state without a key", state: resume.UploadState{UploadID: "u", EncryptManifest: true}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k, enc, err := resumeEncryption(&tc.state, tc.flagKey, tc.flagEncrypt)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want an error")
				}
				return
			}
			if err != nil || k != tc.wantKey || enc != tc.wantEncrypt {
				t.Errorf("got (%q, %v, %v), want (%q, %v, nil)", k, enc, err, tc.wantKey, tc.wantEncrypt)
			}
		})
	}
}

func TestTruncateKey(t *testing.T) {
	if got := truncateKey("alias/x"); got != "alias/x" {
		t.Errorf("a short key must print whole (it used to panic): %q", got)
	}
	if got := truncateKey("arn:aws:kms:us-east-1:111122223333:key/abc"); got != "arn:aws:kms:us-east-..." {
		t.Errorf("got %q", got)
	}
}
