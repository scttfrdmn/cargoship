package cmd

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/scttfrdmn/cargoship/pkg/resume"
)

// TestNewResumePipelineConfig_EncryptedNeverDirect pins #1037 on resume: a
// resumed --encrypt-manifest upload must not switch to direct mode, which names
// objects after files.
func TestNewResumePipelineConfig_EncryptedNeverDirect(t *testing.T) {
	enc := newResumePipelineConfig(&resume.UploadState{UploadID: "u", KMSKeyID: "k", EncryptManifest: true}, &s3.Client{}, nil)
	if enc.EnableDirectUpload || enc.ForceDirectUpload || enc.DirectUploadMaxFiles >= 0 {
		t.Fatalf("encrypted resume can still take direct mode: enable=%v force=%v maxFiles=%d",
			enc.EnableDirectUpload, enc.ForceDirectUpload, enc.DirectUploadMaxFiles)
	}
	plain := newResumePipelineConfig(&resume.UploadState{UploadID: "u"}, &s3.Client{}, nil)
	if plain.DirectUploadMaxFiles < 0 {
		t.Fatal("a plaintext resume keeps the default direct-upload choice")
	}
}
