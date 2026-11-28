package manifest

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// TestApplyManifestSSE pins #1016's rule: a manifest object is SSE-KMS under
// the upload's data key when it has one, and otherwise keeps the bucket default.
func TestApplyManifestSSE(t *testing.T) {
	for _, tc := range []struct {
		name string
		enc  *EncryptionMetadata
		want string
	}{
		{"no encryption", nil, ""},
		{"manifest encryption only", &EncryptionMetadata{ManifestEncrypted: true, ManifestKMSKeyID: "mk"}, ""},
		{"data key", &EncryptionMetadata{Enabled: true, DataKMSKeyID: "dk"}, "dk"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := &s3.PutObjectInput{}
			(&Manifest{Encryption: tc.enc}).applyManifestSSE(in)
			if tc.want == "" {
				if in.ServerSideEncryption != "" || in.SSEKMSKeyId != nil {
					t.Fatalf("SSE set without a data key: %v %v", in.ServerSideEncryption, aws.ToString(in.SSEKMSKeyId))
				}
				return
			}
			if in.ServerSideEncryption != types.ServerSideEncryptionAwsKms || aws.ToString(in.SSEKMSKeyId) != tc.want {
				t.Fatalf("got %v %q, want aws:kms %q", in.ServerSideEncryption, aws.ToString(in.SSEKMSKeyId), tc.want)
			}
		})
	}
}
