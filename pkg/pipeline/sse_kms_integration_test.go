//go:build integration

package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestUpload_KMSKeyEncryptsEveryDataObject: with a KMS key configured, every data
// object the pipeline writes must be stored with SSE-KMS under that key, whichever
// upload path wrote it. Only the optional transporter applied the key: the direct
// fast path (the default for small uploads) and the SDK path (no transporter, and
// every resumed upload) wrote objects without it, while the manifest recorded the
// data as KMS-encrypted.
func TestUpload_KMSKeyEncryptsEveryDataObject(t *testing.T) {
	if substrateURL == "" {
		t.Skip("needs the emulator: it creates KMS keys and uses a placeholder key ARN, which real AWS (the CI role) refuses")
	}
	const key = "arn:aws:kms:us-east-1:000000000000:key/cargoship-test"
	for _, tc := range []struct {
		name   string
		direct bool
	}{{"direct upload", true}, {"chunked upload without a transporter", false}} {
		t.Run(tc.name, func(t *testing.T) {
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

			src := t.TempDir()
			writeRandomFiles(t, src, "f", 5, 4096)
			prefix := fmt.Sprintf("sse-kms-%d", time.Now().UnixNano())
			pc := &PipelineConfig{
				ScannerWorkers: 1, ArchiverWorkers: 1, UploaderWorkers: 1,
				S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
				UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
				EnableManifest: true, SourcePath: src, KMSKeyID: key,
				EnableMultiPrefix: true, ShardCount: 4, WorkersPerPrefix: 1,
			}
			if tc.direct {
				pc.ForceDirectUpload = true
			} else {
				pc.DirectUploadMaxFiles = 1
			}
			p, err := NewPipeline(pc)
			require.NoError(t, err)
			r, err := p.Run(ctx, src)
			require.NoError(t, err)
			require.True(t, r.Success, "errors: %v", r.Errors)

			out, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix + "/")})
			require.NoError(t, err)
			// #1016: manifest objects too. They hold every path, size and
			// checksum, and were left under the bucket default.
			data, manifests := 0, 0
			for _, obj := range out.Contents {
				k := aws.ToString(obj.Key)
				if strings.HasSuffix(k, "/"+manifest.ManifestFileNameGZ) {
					manifests++
				} else {
					data++
				}
				h, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(k)})
				require.NoError(t, err)
				assert.Equal(t, types.ServerSideEncryptionAwsKms, h.ServerSideEncryption, "%s must be SSE-KMS", k)
				assert.Equal(t, key, aws.ToString(h.SSEKMSKeyId), "%s must use the configured key", k)
			}
			require.Positive(t, data, "the upload must have written data objects")
			require.Equal(t, 1, manifests, "the upload must have written its manifest")
		})
	}
}
