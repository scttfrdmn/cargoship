//go:build integration

package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// TestUpload_StorageClassAppliesOnEveryTransporter pins #1000. With a
// transporter (the CLI default is `--transporter staging`), chunks were stored
// as STANDARD whatever --storage-class said: the uploader built the transporter
// Archive without a class and the CLI built the transporter's S3Config without
// one, so the transporter fell back to "". Only the SDK path (no transporter)
// set it. Every chunk object must carry the requested class on every path.
func TestUpload_StorageClassAppliesOnEveryTransporter(t *testing.T) {
	const want = types.StorageClassStandardIa
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
	client := s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = substrateURL != "" })

	for _, tt := range []string{"none", "basic", "staging", "adaptive", "optimized"} {
		t.Run(tt, func(t *testing.T) {
			src := t.TempDir()
			for i := 0; i < 4; i++ {
				body := fmt.Sprintf("cargoship-storage-class-test %s file %d\n%s", tt, i, strings.Repeat("payload ", 512))
				require.NoError(t, os.WriteFile(filepath.Join(src, fmt.Sprintf("f%02d.txt", i)), []byte(body), 0o600))
			}
			prefix := fmt.Sprintf("storage-class-%s-%d", tt, time.Now().UnixNano())

			pc := &PipelineConfig{
				ScannerWorkers: 1, ArchiverWorkers: 1, UploaderWorkers: 1,
				S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
				UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
				S3StorageClass: string(want),
				EnableManifest: true, SourcePath: src,
				EnableMultiPrefix: true, ShardCount: 4, WorkersPerPrefix: 1,
				DirectUploadMaxFiles: 1, // force the chunked path
			}
			if tt != "none" {
				// Built the way the CLI builds it before #1000: no StorageClass in
				// the transporter's own config, so only the per-archive class can
				// carry the request.
				tr, err := NewPipelineTransporter(TransporterConfig{
					Type:     TransporterType(tt),
					S3Client: client,
					S3Config: awsconfig.S3Config{Bucket: bucket, MultipartChunkSize: 64 * 1024 * 1024, Concurrency: 4},
				})
				require.NoError(t, err)
				pc.Transporter = tr
			}

			p, err := NewPipeline(pc)
			require.NoError(t, err)
			r, err := p.Run(ctx, src)
			require.NoError(t, err)
			require.True(t, r.Success, "errors: %v", r.Errors)

			out, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix + "/")})
			require.NoError(t, err)
			chunks := 0
			for _, obj := range out.Contents {
				k := aws.ToString(obj.Key)
				t.Cleanup(func() {
					_, _ = client.DeleteObject(context.Background(), &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(k)})
				})
				if !strings.HasSuffix(k, ".tar.zst") {
					continue
				}
				chunks++
				h, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(k)})
				require.NoError(t, err)
				assert.Equal(t, want, h.StorageClass, "%s: chunk %s stored in the wrong class", tt, k)
			}
			require.Positive(t, chunks, "the upload must have written chunk objects")
		})
	}
}
