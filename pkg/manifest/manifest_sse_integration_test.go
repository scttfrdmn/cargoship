//go:build integration

package manifest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManifestUpload_SSEKMSUnderDataKey pins #1016. With --kms-key-id the data
// objects are SSE-KMS under that key, but the manifest objects, which hold
// every path, size and checksum, were written with no SSE fields and so sat
// under the bucket default (often SSE-S3). Every manifest write, plaintext or
// envelope-encrypted, must use the upload's data key.
func TestManifestUpload_SSEKMSUnderDataKey(t *testing.T) {
	if substrateURL == "" {
		t.Skip("needs the emulator: it creates KMS keys and uses a placeholder key ARN, which real AWS (the CI role) refuses")
	}
	bucket, region, s3Client := getTestConfig(t)
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	require.NoError(t, err)
	kmsClient := kms.NewFromConfig(cfg)
	k, err := kmsClient.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)
	key := aws.ToString(k.KeyMetadata.Arn)

	head := func(t *testing.T, objKey string) *s3.HeadObjectOutput {
		t.Helper()
		h, err := s3Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(objKey)})
		require.NoError(t, err, objKey)
		return h
	}
	newManifest := func(prefix string, enc *EncryptionMetadata) *Manifest {
		return &Manifest{Version: ManifestVersion, UploadID: fmt.Sprintf("u-%d", time.Now().UnixNano()),
			Bucket: bucket, Prefix: prefix, Encryption: enc,
			Files: []FileEntry{{Path: "/src/secret-name.txt", Size: 1}}}
	}

	for _, compress := range []bool{true, false} {
		t.Run(fmt.Sprintf("plaintext manifest compress=%v", compress), func(t *testing.T) {
			m := newManifest("sse-plain", &EncryptionMetadata{Enabled: true, DataKMSKeyID: key})
			require.NoError(t, m.UploadToS3(ctx, s3Client, compress))
			name := ManifestFileName
			if compress {
				name = ManifestFileNameGZ
			}
			h := head(t, fmt.Sprintf("sse-plain/uploads/%s/%s", m.UploadID, name))
			assert.Equal(t, types.ServerSideEncryptionAwsKms, h.ServerSideEncryption, "the manifest must be SSE-KMS")
			assert.Equal(t, key, aws.ToString(h.SSEKMSKeyId), "under the upload's data key")
		})
	}

	t.Run("envelope-encrypted manifest", func(t *testing.T) {
		m := newManifest("sse-enc", &EncryptionMetadata{Enabled: true, DataKMSKeyID: key, ManifestEncrypted: true, ManifestKMSKeyID: key})
		require.NoError(t, m.UploadToS3WithEncryption(ctx, s3Client, kmsClient, true))
		h := head(t, fmt.Sprintf("sse-enc/uploads/%s/manifest.encrypted.json.gz", m.UploadID))
		assert.Equal(t, types.ServerSideEncryptionAwsKms, h.ServerSideEncryption)
		assert.Equal(t, key, aws.ToString(h.SSEKMSKeyId))
	})

	t.Run("no data key keeps the bucket default", func(t *testing.T) {
		m := newManifest("sse-none", nil)
		require.NoError(t, m.UploadToS3(ctx, s3Client, true))
		h := head(t, fmt.Sprintf("sse-none/uploads/%s/%s", m.UploadID, ManifestFileNameGZ))
		assert.NotEqual(t, types.ServerSideEncryptionAwsKms, h.ServerSideEncryption)
	})
}
