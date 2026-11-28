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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// TestUpload_ScannerErrorWritesNoFinalManifest pins #999. A walk error (here an
// unreadable directory) used to leave the final manifest decision to the chunk
// jobs alone: none had failed, so manifest.json.gz was written, listing only the
// files scanned before the error, and the partial manifest and resume state were
// deleted. With CleanupOnFailure the chunks were then deleted too, leaving a
// manifest that points at nothing. Either way the next sync diffed the scanned
// files as unchanged and never uploaded them, and resume was refused because the
// upload looked complete.
//
// The unreadable directory sorts first, so the walk fails before any batch is
// handed to the chunker. That keeps this test about the manifest decision alone,
// clear of the scanner's own exit-path races (#1005, #1006).
func TestUpload_ScannerErrorWritesNoFinalManifest(t *testing.T) {
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

	tests := []struct {
		name             string
		cleanupOnFailure bool
	}{
		// `upload` deletes what it stored when the run fails.
		{name: "cleanup on failure", cleanupOnFailure: true},
		// `sync` keeps it, and the partial manifest, for the next run.
		{name: "keep for resume", cleanupOnFailure: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := t.TempDir()
			lockedDir(t, filepath.Join(src, "000-locked"))
			writePrefixedFiles(t, filepath.Join(src, "data"), "f", 50, 1024)

			prefix := fmt.Sprintf("scanner-error-%d", time.Now().UnixNano())
			uploadID := fmt.Sprintf("u-%d", time.Now().UnixNano())
			p, err := NewPipeline(&PipelineConfig{
				ScannerWorkers: 2, ArchiverWorkers: 2, UploaderWorkers: 2,
				S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
				UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
				UploadID:                    uploadID,
				EnableManifest:              true,
				EnablePartialManifest:       true,
				PartialManifestSaveInterval: time.Hour,
				CleanupOnFailure:            tt.cleanupOnFailure,
				SourcePath:                  src,
				DirectUploadMaxFiles:        1,
				ChunkingConfig: &chunking.ChunkingConfig{
					Workers: 2, AvailableMemory: 1 << 30,
					TargetChunkSize: 1 << 20, MinChunkSize: 512 << 10, MaxChunkSize: 1 << 20,
				},
				EnableMultiPrefix: true, ShardCount: 4, WorkersPerPrefix: 2,
			})
			require.NoError(t, err)

			result, runErr := p.Run(ctx, src)
			require.Error(t, runErr, "a run whose scan failed must report the scan error")
			require.NotNil(t, result)
			assert.False(t, result.Success, "a run whose scan failed must not succeed")

			final := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", prefix, uploadID)
			_, headErr := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(final)})
			assert.True(t, isNoSuchKeyErr(headErr),
				"a run whose scan failed must not write its final manifest (HeadObject err=%v)", headErr)
			assert.True(t, p.GetManifest() == nil, "a run whose scan failed must not finalize a manifest")
		})
	}
}

// TestUpload_ScanFailsWithChunksInFlight is the audit's reproduction for #999,
// #1005 and #1006 together: a full batch of files is chunked and uploading when
// the walk reaches an unreadable directory. Before the fixes the run panicked
// ("send on closed channel"), or lost the walk error and succeeded without the
// rest of the files, or wrote a final manifest whose chunks cleanup then deleted.
func TestUpload_ScanFailsWithChunksInFlight(t *testing.T) {
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
	writePrefixedFiles(t, filepath.Join(src, "a"), "f", 1100, 1024)
	lockedDir(t, filepath.Join(src, "b-locked"))

	for _, cleanup := range []bool{true, false} {
		t.Run(fmt.Sprintf("cleanup=%v", cleanup), func(t *testing.T) {
			prefix := fmt.Sprintf("scan-fails-%d", time.Now().UnixNano())
			uploadID := fmt.Sprintf("u-%d", time.Now().UnixNano())
			p, err := NewPipeline(&PipelineConfig{
				ScannerWorkers: 2, ArchiverWorkers: 2, UploaderWorkers: 2,
				S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
				UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
				UploadID:                    uploadID,
				EnableManifest:              true,
				EnablePartialManifest:       true,
				PartialManifestSaveInterval: time.Hour,
				CleanupOnFailure:            cleanup,
				SourcePath:                  src,
				DirectUploadMaxFiles:        1,
				ChunkingConfig: &chunking.ChunkingConfig{
					Workers: 2, AvailableMemory: 1 << 30,
					TargetChunkSize: 256 << 10, MinChunkSize: 128 << 10, MaxChunkSize: 256 << 10,
				},
				EnableMultiPrefix: true, ShardCount: 4, WorkersPerPrefix: 2,
			})
			require.NoError(t, err)

			result, runErr := p.Run(ctx, src)
			require.Error(t, runErr, "the walk error must fail the run")
			assert.Contains(t, runErr.Error(), "b-locked")
			assert.False(t, result.Success)
			assert.Positive(t, result.ChunksUploaded, "the test needs chunks uploaded before the walk failed")

			objects := map[string]bool{}
			out, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix + "/")})
			require.NoError(t, err)
			chunks := 0
			for _, obj := range out.Contents {
				objects[aws.ToString(obj.Key)] = true
				if strings.Contains(aws.ToString(obj.Key), "/chunk-") {
					chunks++
				}
			}
			base := fmt.Sprintf("%s/uploads/%s/", prefix, uploadID)
			assert.False(t, objects[base+"manifest.json.gz"], "no final manifest after a failed scan; objects: %v", objects)
			if cleanup {
				assert.Zero(t, chunks, "cleanup deletes the run's chunks; objects: %v", objects)
				assert.False(t, objects[base+"manifest.partial.json.gz"], "cleanup deletes the partial manifest")
			} else {
				assert.Positive(t, chunks, "without cleanup the stored chunks stay for resume")
				assert.True(t, objects[base+"manifest.partial.json.gz"],
					"without cleanup the partial manifest stays for resume; objects: %v", objects)
			}
		})
	}
}
