//go:build integration

package manifest

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/substrate"
)

const (
	testBucketEnv  = "CARGOSHIP_TEST_BUCKET"
	testRegionEnv  = "AWS_REGION"
	testProfileEnv = "AWS_PROFILE"
)

var (
	substrateURL    string
	integTestBucket = "cargoship-manifest-test"
)

func TestMain(m *testing.M) {
	if bucket := os.Getenv(testBucketEnv); bucket != "" {
		integTestBucket = bucket // real AWS
	} else {
		url, cancel, err := launchSubstrate()
		if err != nil {
			fmt.Fprintf(os.Stderr, "substrate: %v\n", err)
			os.Exit(1)
		}
		defer cancel()
		substrateURL = url
		os.Setenv("AWS_ENDPOINT_URL", url)
		os.Setenv("AWS_ACCESS_KEY_ID", "test")
		os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
		os.Setenv("AWS_REGION", "us-east-1")
		if err := createSubstrateBucket(url, integTestBucket); err != nil {
			fmt.Fprintf(os.Stderr, "create bucket: %v\n", err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

// launchSubstrate starts an in-process Substrate server for use in TestMain.
func launchSubstrate() (string, context.CancelFunc, error) {
	cfg := substrate.DefaultConfig()
	cfg.Server.Address = "127.0.0.1:0"
	cfg.EventStore.Enabled = false
	cfg.Log.Level = "error"

	state := substrate.NewMemoryStateManager()
	tc := substrate.NewTimeController(time.Now())
	registry := substrate.NewPluginRegistry()
	logger := substrate.NewDefaultLogger(slog.LevelError, false)
	store := substrate.NewEventStore(cfg.EventStore.ToEventStoreConfig(), substrate.WithTimeController(tc))

	ctx := context.Background()
	if err := substrate.RegisterDefaultPlugins(ctx, registry, state, tc, logger, store, nil); err != nil {
		return "", nil, fmt.Errorf("register plugins: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	srv := substrate.NewServer(*cfg, registry, store, state, tc, logger)
	srvCtx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(srvCtx, ln) }()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, pingErr := http.Get(baseURL + "/health") //nolint:noctx
		if pingErr == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return baseURL, cancel, nil
}

// createSubstrateBucket creates an S3 bucket on the Substrate server.
func createSubstrateBucket(baseURL, bucket string) error {
	cfg := aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(baseURL),
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true })
	_, err := client.CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	return err
}

func getTestConfig(t *testing.T) (bucket, region string, s3Client *s3.Client) {
	t.Helper()

	bucket = integTestBucket

	region = os.Getenv(testRegionEnv)
	if region == "" {
		region = "us-east-1"
	}

	profile := os.Getenv(testProfileEnv)

	// Load AWS config
	ctx := context.Background()
	var cfg aws.Config
	var err error

	if profile != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithSharedConfigProfile(profile),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
		)
	}

	require.NoError(t, err, "Failed to load AWS config")

	var s3Opts []func(*s3.Options)
	if substrateURL != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) { o.UsePathStyle = true })
	}
	s3Client = s3.NewFromConfig(cfg, s3Opts...)
	return bucket, region, s3Client
}

func cleanupS3Object(t *testing.T, s3Client *s3.Client, bucket, key string) {
	t.Helper()
	ctx := context.Background()
	_, err := s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		t.Logf("Warning: Failed to cleanup S3 object %s: %v", key, err)
	}
}

// TestManifest_UploadToS3_Integration tests uploading manifest to real S3 (Issue #94)
func TestManifest_UploadToS3_Integration(t *testing.T) {
	bucket, region, s3Client := getTestConfig(t)

	uploadID := fmt.Sprintf("test-upload-%d", time.Now().Unix())
	prefix := "cargoship-test"

	// Create test manifest
	builder, err := NewBuilder(uploadID, "/test/data", bucket, prefix, region)
	require.NoError(t, err)

	builder.AddFile(FileEntry{
		Path:    "file1.txt",
		Size:    1024,
		ModTime: time.Now(),
		ChunkID: 0,
		ShardID: 0,
		S3Key:   fmt.Sprintf("%s/uploads/%s/shard-0/chunk-0.tar.zst", prefix, uploadID),
	})

	builder.AddFile(FileEntry{
		Path:    "dir/file2.txt",
		Size:    2048,
		ModTime: time.Now(),
		ChunkID: 0,
		ShardID: 0,
		S3Key:   fmt.Sprintf("%s/uploads/%s/shard-0/chunk-0.tar.zst", prefix, uploadID),
	})

	builder.SetCompression("zstd", 3, 0.45)
	builder.SetShardCount(8)

	manifest := builder.Finalize()

	ctx := context.Background()

	// Test uncompressed upload
	t.Run("uncompressed", func(t *testing.T) {
		err := manifest.UploadToS3(ctx, s3Client, false)
		require.NoError(t, err)

		// Verify object exists
		key := fmt.Sprintf("%s/uploads/%s/%s", prefix, uploadID, ManifestFileName)
		defer cleanupS3Object(t, s3Client, bucket, key)

		result, err := s3Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "application/json", aws.ToString(result.ContentType))
	})

	// Test compressed upload
	t.Run("compressed", func(t *testing.T) {
		err := manifest.UploadToS3(ctx, s3Client, true)
		require.NoError(t, err)

		// Verify object exists
		key := fmt.Sprintf("%s/uploads/%s/%s", prefix, uploadID, ManifestFileNameGZ)
		defer cleanupS3Object(t, s3Client, bucket, key)

		result, err := s3Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "application/json", aws.ToString(result.ContentType))
		assert.Equal(t, "gzip", aws.ToString(result.ContentEncoding))
	})
}

// TestManifest_DownloadFromS3_Integration tests downloading manifest from real S3 (Issue #94)
func TestManifest_DownloadFromS3_Integration(t *testing.T) {
	bucket, region, s3Client := getTestConfig(t)

	uploadID := fmt.Sprintf("test-download-%d", time.Now().Unix())
	prefix := "cargoship-test"

	// Create and upload test manifest
	builder, err := NewBuilder(uploadID, "/test/data", bucket, prefix, region)
	require.NoError(t, err)

	testFiles := []FileEntry{
		{Path: "file1.txt", Size: 1024, ChunkID: 0, ShardID: 0},
		{Path: "file2.txt", Size: 2048, ChunkID: 0, ShardID: 0},
		{Path: "file3.txt", Size: 4096, ChunkID: 1, ShardID: 1},
	}

	for _, file := range testFiles {
		builder.AddFile(file)
	}

	builder.SetCompression("zstd", 3, 0.45)
	builder.SetShardCount(8)

	manifest := builder.Finalize()

	ctx := context.Background()

	// Test downloading compressed manifest
	t.Run("compressed", func(t *testing.T) {
		// Upload compressed version
		err := manifest.UploadToS3(ctx, s3Client, true)
		require.NoError(t, err)

		key := fmt.Sprintf("%s/uploads/%s/%s", prefix, uploadID, ManifestFileNameGZ)
		defer cleanupS3Object(t, s3Client, bucket, key)

		// Download and verify
		downloaded, err := DownloadFromS3(ctx, s3Client, bucket, prefix, uploadID)
		require.NoError(t, err)
		require.NotNil(t, downloaded)

		// Verify content matches
		assert.Equal(t, manifest.UploadID, downloaded.UploadID)
		assert.Equal(t, manifest.TotalFiles, downloaded.TotalFiles)
		assert.Equal(t, len(manifest.Files), len(downloaded.Files))

		// Verify files match
		for i, file := range manifest.Files {
			assert.Equal(t, file.Path, downloaded.Files[i].Path)
			assert.Equal(t, file.Size, downloaded.Files[i].Size)
		}
	})

	// Test downloading uncompressed manifest
	t.Run("uncompressed", func(t *testing.T) {
		uploadID2 := fmt.Sprintf("test-download-uncomp-%d", time.Now().Unix())
		builder2, err := NewBuilder(uploadID2, "/test/data", bucket, prefix, region)
		require.NoError(t, err)

		builder2.AddFile(FileEntry{Path: "test.txt", Size: 100})
		manifest2 := builder2.Finalize()

		// Upload uncompressed version
		err = manifest2.UploadToS3(ctx, s3Client, false)
		require.NoError(t, err)

		key := fmt.Sprintf("%s/uploads/%s/%s", prefix, uploadID2, ManifestFileName)
		defer cleanupS3Object(t, s3Client, bucket, key)

		// Download and verify
		downloaded, err := DownloadFromS3(ctx, s3Client, bucket, prefix, uploadID2)
		require.NoError(t, err)
		require.NotNil(t, downloaded)

		assert.Equal(t, manifest2.UploadID, downloaded.UploadID)
		assert.Equal(t, manifest2.TotalFiles, downloaded.TotalFiles)
	})

	// Test fallback from compressed to uncompressed
	t.Run("fallback_to_uncompressed", func(t *testing.T) {
		uploadID3 := fmt.Sprintf("test-download-fallback-%d", time.Now().Unix())
		builder3, err := NewBuilder(uploadID3, "/test/data", bucket, prefix, region)
		require.NoError(t, err)

		builder3.AddFile(FileEntry{Path: "fallback.txt", Size: 100})
		manifest3 := builder3.Finalize()

		// Upload ONLY uncompressed version (no compressed version exists)
		err = manifest3.UploadToS3(ctx, s3Client, false)
		require.NoError(t, err)

		key := fmt.Sprintf("%s/uploads/%s/%s", prefix, uploadID3, ManifestFileName)
		defer cleanupS3Object(t, s3Client, bucket, key)

		// Download should fall back to uncompressed
		downloaded, err := DownloadFromS3(ctx, s3Client, bucket, prefix, uploadID3)
		require.NoError(t, err)
		require.NotNil(t, downloaded)

		assert.Equal(t, manifest3.UploadID, downloaded.UploadID)
	})

	// Test error case: non-existent manifest
	t.Run("not_found", func(t *testing.T) {
		nonExistentID := "nonexistent-upload-id"
		downloaded, err := DownloadFromS3(ctx, s3Client, bucket, prefix, nonExistentID)
		assert.Error(t, err)
		assert.Nil(t, downloaded)
	})
}

// TestManifest_DownloadPartialManifestFromS3_Integration tests downloading partial manifest (Issue #94, Issue #157)
func TestManifest_DownloadPartialManifestFromS3_Integration(t *testing.T) {
	bucket, region, s3Client := getTestConfig(t)

	uploadID := fmt.Sprintf("test-partial-%d", time.Now().Unix())
	prefix := "cargoship-test"

	// Create partial manifest (simulating in-progress upload)
	builder, err := NewBuilder(uploadID, "/test/data", bucket, prefix, region)
	require.NoError(t, err)

	builder.AddFile(FileEntry{Path: "partial1.txt", Size: 1024, ChunkID: 0, ShardID: 0})
	builder.AddFile(FileEntry{Path: "partial2.txt", Size: 2048, ChunkID: 0, ShardID: 0})
	builder.SetCompression("zstd", 3, 0.5)

	partialManifest := builder.Build() // Use Build() for partial (not finalized)

	ctx := context.Background()

	// Manually upload as partial manifest (compressed only)
	data, err := partialManifest.ToJSONCompressed()
	require.NoError(t, err)

	key := fmt.Sprintf("%s/uploads/%s/manifest.partial.json.gz", prefix, uploadID)
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:          aws.String(bucket),
		Key:             aws.String(key),
		Body:            bytes.NewReader(data),
		ContentType:     aws.String("application/json"),
		ContentEncoding: aws.String("gzip"),
	})
	require.NoError(t, err)
	defer cleanupS3Object(t, s3Client, bucket, key)

	// Download partial manifest
	downloaded, err := DownloadPartialManifestFromS3(ctx, s3Client, bucket, prefix, uploadID)
	require.NoError(t, err)
	require.NotNil(t, downloaded)

	// Verify content
	assert.Equal(t, partialManifest.UploadID, downloaded.UploadID)
	assert.Equal(t, len(partialManifest.Files), len(downloaded.Files))

	for i, file := range partialManifest.Files {
		assert.Equal(t, file.Path, downloaded.Files[i].Path)
		assert.Equal(t, file.Size, downloaded.Files[i].Size)
	}
}

// TestManifest_RoundTrip_S3_Integration tests full upload/download cycle (Issue #94)
func TestManifest_RoundTrip_S3_Integration(t *testing.T) {
	bucket, region, s3Client := getTestConfig(t)

	uploadID := fmt.Sprintf("test-roundtrip-%d", time.Now().Unix())
	prefix := "cargoship-test"

	// Create comprehensive manifest
	builder, err := NewBuilder(uploadID, "/test/data", bucket, prefix, region)
	require.NoError(t, err)

	// Add multiple files across shards
	for i := 0; i < 100; i++ {
		builder.AddFile(FileEntry{
			Path:    fmt.Sprintf("dir%d/file%d.txt", i/10, i),
			Size:    int64(i * 1024),
			ModTime: time.Now().Add(-time.Duration(i) * time.Hour),
			ChunkID: i / 10,
			ShardID: i / 25,
		})
	}

	// Add chunks
	for i := 0; i < 10; i++ {
		builder.AddChunk(ChunkEntry{
			ID:               i,
			ShardID:          i / 3,
			S3Key:            fmt.Sprintf("%s/uploads/%s/shard-%d/chunk-%d.tar.zst", prefix, uploadID, i/3, i),
			FileCount:        10,
			UncompressedSize: 10240,
			CompressedSize:   5120,
			CreatedAt:        time.Now().Add(-time.Duration(i) * time.Minute),
			UploadedAt:       time.Now(),
		})
	}

	builder.SetCompression("zstd", 3, 0.5)
	builder.SetShardCount(8)

	original := builder.Finalize()

	ctx := context.Background()

	tests := []struct {
		name     string
		compress bool
	}{
		{"compressed", true},
		{"uncompressed", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Upload
			err := original.UploadToS3(ctx, s3Client, tt.compress)
			require.NoError(t, err)

			var key string
			if tt.compress {
				key = fmt.Sprintf("%s/uploads/%s/%s", prefix, uploadID, ManifestFileNameGZ)
			} else {
				key = fmt.Sprintf("%s/uploads/%s/%s", prefix, uploadID, ManifestFileName)
			}
			defer cleanupS3Object(t, s3Client, bucket, key)

			// Download
			downloaded, err := DownloadFromS3(ctx, s3Client, bucket, prefix, uploadID)
			require.NoError(t, err)
			require.NotNil(t, downloaded)

			// Verify all fields match
			assert.Equal(t, original.Version, downloaded.Version)
			assert.Equal(t, original.UploadID, downloaded.UploadID)
			assert.Equal(t, original.SourcePath, downloaded.SourcePath)
			assert.Equal(t, original.Bucket, downloaded.Bucket)
			assert.Equal(t, original.Prefix, downloaded.Prefix)
			assert.Equal(t, original.Region, downloaded.Region)
			assert.Equal(t, original.TotalFiles, downloaded.TotalFiles)
			assert.Equal(t, original.TotalBytes, downloaded.TotalBytes)
			assert.Equal(t, original.TotalChunks, downloaded.TotalChunks)
			assert.Equal(t, original.ShardCount, downloaded.ShardCount)
			assert.Equal(t, original.CompressionType, downloaded.CompressionType)
			assert.Equal(t, original.CompressionLevel, downloaded.CompressionLevel)
			assert.InDelta(t, original.CompressionRatio, downloaded.CompressionRatio, 0.0001)

			// Verify files match
			assert.Equal(t, len(original.Files), len(downloaded.Files))
			for i := range original.Files {
				assert.Equal(t, original.Files[i].Path, downloaded.Files[i].Path)
				assert.Equal(t, original.Files[i].Size, downloaded.Files[i].Size)
				assert.Equal(t, original.Files[i].ChunkID, downloaded.Files[i].ChunkID)
				assert.Equal(t, original.Files[i].ShardID, downloaded.Files[i].ShardID)
			}

			// Verify chunks match
			assert.Equal(t, len(original.Chunks), len(downloaded.Chunks))
			for i := range original.Chunks {
				assert.Equal(t, original.Chunks[i].ID, downloaded.Chunks[i].ID)
				assert.Equal(t, original.Chunks[i].ShardID, downloaded.Chunks[i].ShardID)
				assert.Equal(t, original.Chunks[i].S3Key, downloaded.Chunks[i].S3Key)
				assert.Equal(t, original.Chunks[i].FileCount, downloaded.Chunks[i].FileCount)
			}

			// Verify query operations work on downloaded manifest
			query := NewManifestQuery(downloaded)
			file := query.FindFile("dir5/file50.txt")
			require.NotNil(t, file)
			assert.Equal(t, int64(50*1024), file.Size)
		})
	}
}
