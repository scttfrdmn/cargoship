package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

func TestPipelineConfig_DataKMSKey(t *testing.T) {
	assert.Empty(t, (&PipelineConfig{}).dataKMSKeyID())
	assert.Equal(t, types.ServerSideEncryption(""), (&PipelineConfig{}).dataSSE(), "no key: the bucket default")

	c := &PipelineConfig{KMSKeyID: "k1", S3SSEKMSKeyId: "legacy"}
	assert.Equal(t, "k1", c.dataKMSKeyID(), "KMSKeyID wins")
	assert.Equal(t, types.ServerSideEncryptionAwsKms, c.dataSSE())

	legacy := &PipelineConfig{S3SSEKMSKeyId: "legacy"}
	assert.Equal(t, "legacy", legacy.dataKMSKeyID(), "the deprecated field still encrypts rather than being ignored")
}

// TestDirectUploader_AppliesKMSKeyAndStorageClass pins #946 for the direct path:
// PutObject set neither SSE-KMS nor the storage class.
func TestDirectUploader_AppliesKMSKeyAndStorageClass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))

	for _, tc := range []struct {
		name, key string
		class     types.StorageClass
		wantSSE   types.ServerSideEncryption
	}{
		{"kms key and class", "arn:aws:kms:us-east-1:000000000000:key/k", types.StorageClassGlacierIr, types.ServerSideEncryptionAwsKms},
		{"neither", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &mockS3Client{}
			pool := NewAdaptiveWorkerPool(context.Background(), &AdaptiveWorkerPoolConfig{InitialWorkers: 1, MaxWorkers: 1})
			stage, err := NewDirectUploaderStage(&DirectUploaderConfig{
				S3Client: client, Bucket: "b", Prefix: "p", UploadID: "u", SourcePath: dir,
				Workers: 1, MaxRetries: 1, SSEKMSKeyID: tc.key, StorageClass: tc.class, WorkerPool: pool,
			}, make(chan *Job), make(chan *Job))
			require.NoError(t, err)
			require.NoError(t, stage.uploadFile(context.Background(), chunking.File{Path: path, Size: 4}))

			require.NotNil(t, client.last)
			assert.Equal(t, tc.wantSSE, client.last.ServerSideEncryption)
			assert.Equal(t, tc.key, aws.ToString(client.last.SSEKMSKeyId))
			assert.Equal(t, tc.class, client.last.StorageClass)
		})
	}
}
