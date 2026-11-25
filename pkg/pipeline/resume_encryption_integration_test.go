//go:build integration

package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestResume_EncryptedUploadStaysEncrypted pins #913. A resumed --encrypt-manifest
// upload wrote its final manifest in plaintext (the resumed builder dropped the
// Encryption block, and the upload fell back to plaintext without it), and the
// partial manifest was always plaintext, so an interrupted encrypted upload left
// its full file list readable in S3.
func TestResume_EncryptedUploadStaysEncrypted(t *testing.T) {
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		bucket = "cargoship-pipeline-test"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	require.NoError(t, err)
	pathStyle := func(o *s3.Options) { o.UsePathStyle = substrateURL != "" }
	plain := s3.NewFromConfig(cfg, pathStyle)
	flaky := s3.NewFromConfig(cfg, pathStyle, func(o *s3.Options) {
		o.HTTPClient = &http.Client{Transport: &failOneChunk{next: http.DefaultTransport}}
	})
	kmsClient := kms.NewFromConfig(cfg)
	key, err := kmsClient.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyARN := aws.ToString(key.KeyMetadata.Arn)

	src := t.TempDir()
	writeRandomFiles(t, src, "secret-name-", 200, 30*1024)
	prefix := fmt.Sprintf("resume-encrypted-%d", time.Now().UnixNano())
	uploadID := fmt.Sprintf("u-%d", time.Now().UnixNano())
	base := fmt.Sprintf("%s/uploads/%s/", prefix, uploadID)

	newPipe := func(client *s3.Client, resume, encrypt bool) (*Pipeline, error) {
		pc := &PipelineConfig{
			ScannerWorkers: 2, ArchiverWorkers: 2, UploaderWorkers: 2,
			S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
			UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
			UploadID: uploadID, EnableManifest: true, EnablePartialManifest: true,
			PartialManifestSaveInterval: time.Hour, SourcePath: src, DirectUploadMaxFiles: 1,
			ChunkingConfig: &chunking.ChunkingConfig{
				Workers: 2, AvailableMemory: 1 << 30,
				TargetChunkSize: 1 << 20, MinChunkSize: 512 << 10, MaxChunkSize: 1 << 20,
			},
			EnableMultiPrefix: true, ShardCount: 4, WorkersPerPrefix: 2,
		}
		if encrypt {
			pc.KMSKeyID, pc.EncryptManifest, pc.KMSClient = keyARN, true, kmsClient
		}
		if resume {
			pc.ResumeMode, pc.ResumeUploadID = true, uploadID
		}
		return NewPipeline(pc)
	}
	exists := func(name string) bool {
		_, err := plain.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(base + name)})
		return err == nil
	}

	// Run 1 is interrupted by a failed chunk and saves its partial manifest.
	p1, err := newPipe(flaky, false, true)
	require.NoError(t, err)
	r1, err := p1.Run(ctx, src)
	require.NoError(t, err)
	require.False(t, r1.Success)

	assert.False(t, exists(manifest.PartialManifestFileNameGZ), "the partial manifest must not be written in plaintext")
	require.True(t, exists(manifest.PartialEncryptedManifestFileNameGZ), "the partial manifest must be saved encrypted")
	obj, err := plain.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(base + manifest.PartialEncryptedManifestFileNameGZ)})
	require.NoError(t, err)
	raw, err := readAll(obj.Body)
	require.NoError(t, err)
	_ = obj.Body.Close()
	assert.False(t, bytes.Contains(raw, []byte("secret-name-")), "file names must not be readable")
	partial, err := manifest.DecryptPartialManifest(ctx, kmsClient, raw, uploadID)
	require.NoError(t, err)
	require.NotEmpty(t, partial.Chunks)

	// Resuming without the encryption settings is refused, not downgraded.
	_, err = newPipe(plain, true, false)
	require.Error(t, err)

	// Run 2 resumes with them and finishes encrypted.
	p2, err := newPipe(plain, true, true)
	require.NoError(t, err)
	r2, err := p2.Run(ctx, src)
	require.NoError(t, err)
	require.True(t, r2.Success, "errors: %v", r2.Errors)

	assert.False(t, exists(manifest.ManifestFileNameGZ), "a resumed encrypted upload must not write a plaintext manifest")
	assert.True(t, exists("manifest.encrypted.json.gz"))
	assert.False(t, exists(manifest.PartialEncryptedManifestFileNameGZ), "the partial is cleaned up after success")

	m, err := manifest.DownloadFromS3WithDecryption(ctx, plain, kmsClient, bucket, prefix, uploadID)
	require.NoError(t, err)
	require.NotNil(t, m.Encryption)
	assert.True(t, m.Encryption.ManifestEncrypted)
	assert.Equal(t, partial.DatasetID, m.DatasetID, "resume must keep the dataset identity")
	assert.Len(t, m.Files, 200)

	var targets []string
	for _, f := range m.Files {
		targets = append(targets, f.Path)
	}
	stats, err := manifest.NewSelectiveExtractor(m, plain, 0).SetBucket(bucket).BatchRestore(ctx, targets, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, int64(200), stats.Restored, "reasons: %v", stats.Errors)
}
