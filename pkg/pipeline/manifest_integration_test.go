//go:build integration

package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestManifestIntegration_Generation tests manifest generation during upload
//
// Prerequisites:
// - AWS credentials configured
// - S3 bucket available (CARGOSHIP_TEST_BUCKET or defaults to cargoship-pipeline-test)
// - Run with: go test -tags=integration -run TestManifestIntegration_Generation
func TestManifestIntegration_Generation(t *testing.T) {
	// Get S3 bucket from environment or use default
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		bucket = "cargoship-pipeline-test"
	}

	// Get AWS region from environment or use default
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	// Create AWS config
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	require.NoError(t, err, "Failed to load AWS config")

	// Create S3 client
	var s3Opts []func(*s3.Options)
	if substrateURL != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) { o.UsePathStyle = true })
	}
	s3Client := s3.NewFromConfig(cfg, s3Opts...)
	ctx := context.Background()

	// Create test directory with files. This test validates *chunked* manifest
	// generation, so the files must be large enough to stay out of direct-upload
	// mode (avg >= 5 MB, see shouldUseDirectUpload). 6 files @ 6 MB → chunked.
	const wantFiles = 6
	tmpDir, cleanup := createTestFiles(t, wantFiles, 6*1024*1024) // 6 files @ 6MB each
	defer cleanup()

	// Create pipeline config with manifest enabled
	testPrefix := fmt.Sprintf("manifest-test-%d", time.Now().Unix())
	uploadID := fmt.Sprintf("%d-test", time.Now().Unix())
	pipelineConfig := &PipelineConfig{
		ScannerWorkers:    2,
		ArchiverWorkers:   4,
		UploaderWorkers:   2,
		S3Bucket:          bucket,
		S3Prefix:          testPrefix,
		S3Region:          region,
		UseRealS3:         true,
		S3Client:          s3Client,
		S3PartSize:        5 * 1024 * 1024,
		EnableManifest:    true, // Enable manifest generation
		SourcePath:        tmpDir,
		UploadID:          uploadID,
		EnableMultiPrefix: true,
		ShardCount:        4,
		FileChecksums:     true, // #271: capture per-file content checksums
	}

	// Create and run pipeline
	pipeline, err := NewPipeline(pipelineConfig)
	require.NoError(t, err)

	result, err := pipeline.Run(ctx, tmpDir)
	require.NoError(t, err)
	assert.True(t, result.Success)

	t.Logf("Upload completed: %d files, %d chunks", result.TotalFiles, result.ChunksCreated)

	// Download and verify manifest from S3
	manifestKey := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", testPrefix, uploadID)
	t.Logf("Downloading manifest: s3://%s/%s", bucket, manifestKey)

	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(manifestKey),
	}

	manifestResult, err := s3Client.GetObject(ctx, getObjectInput)
	require.NoError(t, err, "Manifest should be uploaded to S3")
	defer manifestResult.Body.Close()

	// Read manifest bytes
	manifestBytes, err := readAll(manifestResult.Body)
	require.NoError(t, err)
	t.Logf("Manifest size: %d bytes", len(manifestBytes))

	// Deserialize manifest
	m, err := manifest.FromJSONCompressed(manifestBytes)
	require.NoError(t, err, "Manifest should deserialize successfully")

	// #274: a REAL uploaded manifest must satisfy the published JSON Schema.
	// This is the "real output complies with the spec" compliance check —
	// distinct from the struct↔schema drift test, which only checks shapes.
	realJSON, err := m.ToJSON()
	require.NoError(t, err)
	schemaViolations, err := manifest.ValidateAgainstSchema(realJSON)
	require.NoError(t, err)
	assert.Empty(t, schemaViolations, "uploaded manifest must satisfy schema.json; violations: %v", schemaViolations)

	// Verify manifest contents
	assert.Equal(t, uploadID, m.UploadID, "Upload ID should match")
	assert.Equal(t, bucket, m.Bucket, "Bucket should match")
	assert.Equal(t, testPrefix, m.Prefix, "Prefix should match")
	assert.Equal(t, region, m.Region, "Region should match")
	assert.Equal(t, tmpDir, m.SourcePath, "Source path should match")
	assert.Equal(t, int64(wantFiles), m.TotalFiles, "Total files should match")
	assert.Equal(t, wantFiles, len(m.Files), "Files array should have all entries")
	assert.Greater(t, len(m.Chunks), 0, "Should have at least 1 chunk (chunked mode)")
	assert.Equal(t, 4, m.ShardCount, "Should have 4 shards")
	assert.Equal(t, 4, len(m.Shards), "Shards array should have 4 entries")

	// Verify all files have S3 keys and shard IDs
	for i, file := range m.Files {
		assert.NotEmpty(t, file.Path, "File %d should have a path", i)
		assert.NotEmpty(t, file.S3Key, "File %d should have an S3 key", i)
		assert.GreaterOrEqual(t, file.ShardID, 0, "File %d should have a valid shard ID", i)
		assert.GreaterOrEqual(t, file.ChunkID, 0, "File %d should have a valid chunk ID", i)
		// #273: S3Key must be a portable relative key, not a full URL.
		assert.NotContains(t, file.S3Key, "://", "File %d S3Key must not be a URL (portability): %s", i, file.S3Key)
	}

	// Verify all chunks have valid metadata
	for i, chunk := range m.Chunks {
		assert.NotEmpty(t, chunk.S3Key, "Chunk %d should have an S3 key", i)
		assert.GreaterOrEqual(t, chunk.ShardID, 0, "Chunk %d should have a valid shard ID", i)
		assert.Greater(t, chunk.FileCount, 0, "Chunk %d should contain files", i)
		assert.Greater(t, chunk.UncompressedSize, int64(0), "Chunk %d should have uncompressed size", i)
		assert.Greater(t, chunk.CompressedSize, int64(0), "Chunk %d should have compressed size", i)
		// #273: chunk S3Key must be a portable relative key, not a full URL.
		assert.NotContains(t, chunk.S3Key, "://", "Chunk %d S3Key must not be a URL (portability): %s", i, chunk.S3Key)
	}

	// #271: chunk-level checksum capture. The manifest must record the
	// algorithm, and every chunk must carry a checksum populated during upload.
	assert.Equal(t, manifest.ChecksumAlgorithmSHA256, m.ChecksumAlgorithm,
		"manifest should record the checksum algorithm")
	for i, chunk := range m.Chunks {
		assert.NotEmpty(t, chunk.Checksum, "Chunk %d should have a checksum captured on upload", i)
		assert.Len(t, chunk.Checksum, 64, "Chunk %d checksum should be a hex SHA-256", i)
	}

	// #271: deep verify should PASS against the freshly-uploaded objects —
	// re-download each chunk, recompute SHA-256, compare to the manifest.
	deepRes, err := manifest.NewDeepVerifier(m, s3Client).VerifyChunks(ctx)
	require.NoError(t, err)
	assert.True(t, deepRes.Passed(),
		"deep verify should pass on fresh upload: %d OK, %d mismatched, %d missing, %d unverifiable",
		deepRes.OK, deepRes.Mismatched, deepRes.Missing, deepRes.Unverifiable)
	assert.Equal(t, len(m.Chunks), deepRes.OK, "every chunk should verify OK")

	// #271 part 2: per-file content checksums. Every non-duplicate file should
	// carry a checksum captured during archiving, and file-level deep verify
	// (extract each file, recompute its hash) should PASS on the fresh upload.
	for i, f := range m.Files {
		if f.IsDuplicate {
			continue
		}
		assert.NotEmpty(t, f.Checksum, "file %d (%s) should have a content checksum", i, f.Path)
		assert.Len(t, f.Checksum, 64, "file %d checksum should be a hex SHA-256", i)
	}
	fileRes, err := manifest.NewDeepVerifier(m, s3Client).VerifyFiles(ctx)
	require.NoError(t, err)
	assert.True(t, fileRes.Passed(),
		"file-level deep verify should pass on fresh upload: %d OK, %d mismatched, %d missing, %d unverifiable",
		fileRes.OK, fileRes.Mismatched, fileRes.Missing, fileRes.Unverifiable)

	// #271: corruption is detected. Overwrite one chunk object with garbage and
	// confirm deep verify now FAILS with a mismatch on that chunk. Derive the
	// object key the same way deep verify does (S3Key may be a full URL).
	corruptKey := manifest.ResolveObjectKey(m.Prefix, bucket, m.Chunks[0].S3Key)
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(corruptKey),
		Body:   bytes.NewReader([]byte("this is not the original archive")),
	})
	require.NoError(t, err, "should be able to overwrite chunk object for corruption test")

	corruptRes, err := manifest.NewDeepVerifier(m, s3Client).VerifyChunks(ctx)
	require.NoError(t, err)
	assert.False(t, corruptRes.Passed(), "deep verify must fail after a chunk is corrupted")
	assert.GreaterOrEqual(t, corruptRes.Mismatched, 1, "the corrupted chunk should be flagged as a mismatch")

	// Verify shard statistics
	totalFilesInShards := int64(0)
	totalCompressedInShards := int64(0)
	shardsWithChunks := 0
	for i, shard := range m.Shards {
		// Only check shards that have chunks (small datasets may not use all shards)
		if shard.ChunkCount > 0 {
			shardsWithChunks++
			assert.NotEmpty(t, shard.ChunkKeys, "Shard %d should have chunk keys", i)
			assert.Greater(t, shard.FileCount, int64(0), "Shard %d should have files", i)
		}
		totalFilesInShards += shard.FileCount
		totalCompressedInShards += shard.CompressedSize
	}
	assert.Greater(t, shardsWithChunks, 0, "At least one shard should have chunks")
	assert.Equal(t, m.TotalFiles, totalFilesInShards, "Shard file counts should sum to total files")
	assert.Greater(t, totalCompressedInShards, int64(0), "Total compressed size should be positive")

	t.Logf("✅ Manifest validation passed:")
	t.Logf("  Upload ID: %s", m.UploadID)
	t.Logf("  Files: %d", m.TotalFiles)
	t.Logf("  Chunks: %d", len(m.Chunks))
	t.Logf("  Shards: %d", len(m.Shards))
	t.Logf("  Compression ratio: %.2f%%", m.CompressionRatio*100)
}

// TestManifestIntegration_QueryAPI tests the manifest query functionality
func TestManifestIntegration_QueryAPI(t *testing.T) {
	// Get S3 bucket from environment or use default
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		bucket = "cargoship-pipeline-test"
	}

	// Get AWS region from environment or use default
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	// Create AWS config
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	require.NoError(t, err)

	// Create S3 client
	var s3Opts []func(*s3.Options)
	if substrateURL != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) { o.UsePathStyle = true })
	}
	s3Client := s3.NewFromConfig(cfg, s3Opts...)
	ctx := context.Background()

	// Create test directory with known file structure
	tmpDir, cleanup := createTestFiles(t, 15, 3*1024) // 15 files @ 3KB each
	defer cleanup()

	// Create pipeline config
	testPrefix := fmt.Sprintf("manifest-query-test-%d", time.Now().Unix())
	uploadID := fmt.Sprintf("%d-query", time.Now().Unix())
	pipelineConfig := &PipelineConfig{
		ScannerWorkers:    2,
		ArchiverWorkers:   4,
		UploaderWorkers:   2,
		S3Bucket:          bucket,
		S3Prefix:          testPrefix,
		S3Region:          region,
		UseRealS3:         true,
		S3Client:          s3Client,
		S3PartSize:        5 * 1024 * 1024,
		EnableManifest:    true,
		SourcePath:        tmpDir,
		UploadID:          uploadID,
		EnableMultiPrefix: true,
		ShardCount:        2,
	}

	// Create and run pipeline
	pipeline, err := NewPipeline(pipelineConfig)
	require.NoError(t, err)

	result, err := pipeline.Run(ctx, tmpDir)
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Download manifest
	manifestKey := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", testPrefix, uploadID)
	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(manifestKey),
	}

	manifestResult, err := s3Client.GetObject(ctx, getObjectInput)
	require.NoError(t, err)
	defer manifestResult.Body.Close()

	manifestBytes, err := readAll(manifestResult.Body)
	require.NoError(t, err)

	m, err := manifest.FromJSONCompressed(manifestBytes)
	require.NoError(t, err)

	// Create query interface
	query := manifest.NewManifestQuery(m)

	// Test FindFile - find first file
	firstFilePath := m.Files[0].Path
	foundFile := query.FindFile(firstFilePath)
	require.NotNil(t, foundFile, "Should find file by exact path")
	assert.Equal(t, firstFilePath, foundFile.Path)

	// Test FilesInShard
	shard0Files := query.FilesInShard(0)
	assert.Greater(t, len(shard0Files), 0, "Shard 0 should have files")
	for _, file := range shard0Files {
		assert.Equal(t, 0, file.ShardID, "All files should be in shard 0")
	}

	// Test FilesInChunk
	chunk0Files := query.FilesInChunk(0)
	assert.Greater(t, len(chunk0Files), 0, "Chunk 0 should have files")
	for _, file := range chunk0Files {
		assert.Equal(t, 0, file.ChunkID, "All files should be in chunk 0")
	}

	// Test GetSummary
	summary := query.GetSummary()
	assert.Equal(t, m.TotalFiles, summary.TotalFiles)
	assert.Equal(t, m.TotalBytes, summary.TotalBytes)
	assert.Equal(t, m.TotalChunks, summary.TotalChunks)
	assert.Equal(t, m.ShardCount, summary.ShardCount)
	assert.Equal(t, uploadID, summary.UploadID)

	t.Logf("✅ Query API validation passed:")
	t.Logf("  FindFile: found %s", firstFilePath)
	t.Logf("  FilesInShard(0): %d files", len(shard0Files))
	t.Logf("  FilesInChunk(0): %d files", len(chunk0Files))
	t.Logf("  Summary: %d files, %d chunks", summary.TotalFiles, summary.TotalChunks)
}

// Helper function to read all bytes from a reader (io.ReadAll equivalent)
func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	buf := make([]byte, 0, 512)
	for {
		if len(buf) == cap(buf) {
			buf = append(buf, 0)[:len(buf)]
		}
		n, err := r.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, err
		}
	}
}
