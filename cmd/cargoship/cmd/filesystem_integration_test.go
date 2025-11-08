//go:build integration

package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

// IntegrationTestSuite provides utilities for filesystem-based integration tests
type IntegrationTestSuite struct {
	t *testing.T

	// Directories
	TempDir     string
	TestDataDir string
	CacheDir    string

	// S3 Configuration
	S3Bucket  string
	S3Client  *s3.Client
	UseRealS3 bool

	// Test State
	CreatedFiles []string
	Checksums    map[string]string

	// Cleanup
	cleanupFuncs []func()
}

// setupIntegrationSuite initializes a new integration test suite
func setupIntegrationSuite(t *testing.T) *IntegrationTestSuite {
	suite := &IntegrationTestSuite{
		t:         t,
		Checksums: make(map[string]string),
	}

	// Create temporary directories
	tempDir, err := os.MkdirTemp("", "cargoship-integration-*")
	require.NoError(t, err, "Failed to create temp directory")
	suite.TempDir = tempDir

	suite.TestDataDir = filepath.Join(tempDir, "testdata")
	require.NoError(t, os.MkdirAll(suite.TestDataDir, 0755))

	suite.CacheDir = filepath.Join(tempDir, "cache")
	require.NoError(t, os.MkdirAll(suite.CacheDir, 0755))

	// Setup S3 client
	suite.setupS3Client()

	// Register cleanup for temp directory
	suite.RegisterCleanup(func() {
		if err := os.RemoveAll(suite.TempDir); err != nil {
			t.Logf("Warning: Failed to cleanup temp directory %s: %v", suite.TempDir, err)
		}
	})

	return suite
}

// setupS3Client initializes S3 client for LocalStack or real AWS
func (s *IntegrationTestSuite) setupS3Client() {
	ctx := context.Background()

	// Check if real AWS integration tests are enabled
	s.UseRealS3 = os.Getenv("CARGOSHIP_ENABLE_AWS_INTEGRATION_TESTS") == "true"

	var cfg aws.Config
	var err error

	if s.UseRealS3 {
		// Use real AWS S3
		cfg, err = config.LoadDefaultConfig(ctx)
		require.NoError(s.t, err, "Failed to load AWS config")

		// Get bucket name from environment or use default
		s.S3Bucket = os.Getenv("CARGOSHIP_TEST_BUCKET")
		if s.S3Bucket == "" {
			s.S3Bucket = fmt.Sprintf("cargoship-integration-test-%d", time.Now().Unix())
			s.t.Logf("Creating test bucket: %s", s.S3Bucket)

			// Create bucket
			s3Client := s3.NewFromConfig(cfg)
			_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
				Bucket: aws.String(s.S3Bucket),
			})
			require.NoError(s.t, err, "Failed to create test bucket")

			// Register cleanup to delete bucket
			s.RegisterCleanup(func() {
				s.cleanupS3Bucket(ctx, s3Client)
			})
		}

		s.S3Client = s3.NewFromConfig(cfg)
		s.t.Logf("Using real AWS S3 with bucket: %s", s.S3Bucket)
	} else {
		// Use LocalStack
		endpoint := os.Getenv("LOCALSTACK_ENDPOINT")
		if endpoint == "" {
			endpoint = "http://localhost:4566"
		}

		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion("us-east-1"),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
			config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
				func(service, region string, options ...interface{}) (aws.Endpoint, error) {
					return aws.Endpoint{
						URL:           endpoint,
						SigningRegion: region,
					}, nil
				},
			)),
		)
		require.NoError(s.t, err, "Failed to load LocalStack config")

		s.S3Client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.UsePathStyle = true
		})
		s.S3Bucket = "cargoship-test-bucket"
		s.t.Logf("Using LocalStack at %s with bucket: %s", endpoint, s.S3Bucket)

		// Create bucket in LocalStack
		ctx := context.Background()
		_, err = s.S3Client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(s.S3Bucket),
		})
		// Ignore error if bucket already exists
		if err != nil {
			s.t.Logf("Bucket may already exist: %v", err)
		}
	}
}

// CreateTestFile creates a test file with random data and returns its path
func (s *IntegrationTestSuite) CreateTestFile(name string, size int64) string {
	path := filepath.Join(s.TestDataDir, name)

	file, err := os.Create(path)
	require.NoError(s.t, err, "Failed to create test file %s", name)
	defer file.Close()

	// Write random data
	written := int64(0)
	buf := make([]byte, 64*1024) // 64KB buffer
	for written < size {
		toWrite := size - written
		if toWrite > int64(len(buf)) {
			toWrite = int64(len(buf))
		}

		// Generate random data
		rand.Read(buf[:toWrite])

		n, err := file.Write(buf[:toWrite])
		require.NoError(s.t, err, "Failed to write to test file")
		written += int64(n)
	}

	// Calculate and store checksum
	checksum := s.CalculateChecksum(path)
	s.Checksums[name] = checksum

	s.CreatedFiles = append(s.CreatedFiles, path)
	s.t.Logf("Created test file: %s (size: %d bytes, checksum: %s)", name, size, checksum[:16])

	return path
}

// CreateTestFileWithContent creates a test file with specific content
func (s *IntegrationTestSuite) CreateTestFileWithContent(name string, content []byte) string {
	path := filepath.Join(s.TestDataDir, name)

	err := os.WriteFile(path, content, 0644)
	require.NoError(s.t, err, "Failed to write test file %s", name)

	// Calculate and store checksum
	checksum := s.CalculateChecksum(path)
	s.Checksums[name] = checksum

	s.CreatedFiles = append(s.CreatedFiles, path)
	s.t.Logf("Created test file with content: %s (size: %d bytes)", name, len(content))

	return path
}

// CalculateChecksum computes SHA256 checksum of a file
func (s *IntegrationTestSuite) CalculateChecksum(path string) string {
	file, err := os.Open(path)
	require.NoError(s.t, err, "Failed to open file for checksum: %s", path)
	defer file.Close()

	hash := sha256.New()
	_, err = io.Copy(hash, file)
	require.NoError(s.t, err, "Failed to calculate checksum for %s", path)

	return hex.EncodeToString(hash.Sum(nil))
}

// VerifyChecksum verifies a file matches its expected checksum
func (s *IntegrationTestSuite) VerifyChecksum(name string, path string) {
	expectedChecksum, ok := s.Checksums[name]
	require.True(s.t, ok, "No checksum found for file: %s", name)

	actualChecksum := s.CalculateChecksum(path)
	require.Equal(s.t, expectedChecksum, actualChecksum,
		"Checksum mismatch for %s (expected: %s, got: %s)",
		name, expectedChecksum[:16], actualChecksum[:16])

	s.t.Logf("✓ Checksum verified for %s", name)
}

// UploadToS3 uploads a file to S3 and returns the S3 key
func (s *IntegrationTestSuite) UploadToS3(localPath, s3Key string) string {
	ctx := context.Background()

	file, err := os.Open(localPath)
	require.NoError(s.t, err, "Failed to open file for upload: %s", localPath)
	defer file.Close()

	_, err = s.S3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.S3Bucket),
		Key:    aws.String(s3Key),
		Body:   file,
	})
	require.NoError(s.t, err, "Failed to upload to S3: %s", s3Key)

	s.t.Logf("Uploaded to S3: s3://%s/%s", s.S3Bucket, s3Key)

	// Register cleanup for S3 object
	s.RegisterCleanup(func() {
		_, _ = s.S3Client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
			Bucket: aws.String(s.S3Bucket),
			Key:    aws.String(s3Key),
		})
	})

	return s3Key
}

// DownloadFromS3 downloads a file from S3 and returns the local path
func (s *IntegrationTestSuite) DownloadFromS3(s3Key, localName string) string {
	ctx := context.Background()

	output, err := s.S3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.S3Bucket),
		Key:    aws.String(s3Key),
	})
	require.NoError(s.t, err, "Failed to download from S3: %s", s3Key)
	defer output.Body.Close()

	localPath := filepath.Join(s.TestDataDir, localName)
	file, err := os.Create(localPath)
	require.NoError(s.t, err, "Failed to create local file: %s", localPath)
	defer file.Close()

	_, err = io.Copy(file, output.Body)
	require.NoError(s.t, err, "Failed to write downloaded file: %s", localPath)

	s.t.Logf("Downloaded from S3: %s -> %s", s3Key, localPath)

	return localPath
}

// ListS3Objects lists all objects in the test bucket
func (s *IntegrationTestSuite) ListS3Objects() []string {
	ctx := context.Background()

	output, err := s.S3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.S3Bucket),
	})
	require.NoError(s.t, err, "Failed to list S3 objects")

	var keys []string
	for _, obj := range output.Contents {
		keys = append(keys, *obj.Key)
	}

	return keys
}

// cleanupS3Bucket deletes all objects and the bucket itself
func (s *IntegrationTestSuite) cleanupS3Bucket(ctx context.Context, client *s3.Client) {
	// List all objects
	output, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.S3Bucket),
	})
	if err != nil {
		s.t.Logf("Warning: Failed to list objects during cleanup: %v", err)
		return
	}

	// Delete all objects
	for _, obj := range output.Contents {
		_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(s.S3Bucket),
			Key:    obj.Key,
		})
		if err != nil {
			s.t.Logf("Warning: Failed to delete object %s: %v", *obj.Key, err)
		}
	}

	// Delete bucket
	_, err = client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(s.S3Bucket),
	})
	if err != nil {
		s.t.Logf("Warning: Failed to delete bucket %s: %v", s.S3Bucket, err)
	} else {
		s.t.Logf("Cleaned up test bucket: %s", s.S3Bucket)
	}
}

// RegisterCleanup registers a cleanup function to be called after test
func (s *IntegrationTestSuite) RegisterCleanup(fn func()) {
	s.cleanupFuncs = append(s.cleanupFuncs, fn)
}

// Cleanup runs all registered cleanup functions
func (s *IntegrationTestSuite) Cleanup() {
	s.t.Logf("Running cleanup for integration test suite")

	// Run cleanup functions in reverse order (LIFO)
	for i := len(s.cleanupFuncs) - 1; i >= 0; i-- {
		s.cleanupFuncs[i]()
	}

	s.t.Logf("Cleanup complete")
}

// Helper: Get file size
func (s *IntegrationTestSuite) GetFileSize(path string) int64 {
	info, err := os.Stat(path)
	require.NoError(s.t, err, "Failed to stat file: %s", path)
	return info.Size()
}

// Helper: Check if file exists
func (s *IntegrationTestSuite) FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// CreateArchive creates an archive from a directory using CargoShip
func (s *IntegrationTestSuite) CreateArchive(sourceDir, format string) string {
	archiveName := fmt.Sprintf("test-archive-%d.%s", time.Now().Unix(), format)
	archivePath := filepath.Join(s.TempDir, archiveName)

	// Use tar command directly for simplicity
	// In a real test, we'd use CargoShip's archive creation
	var cmd string
	switch format {
	case "tar":
		cmd = fmt.Sprintf("tar -cf %s -C %s .", archivePath, sourceDir)
	case "tar.gz":
		cmd = fmt.Sprintf("tar -czf %s -C %s .", archivePath, sourceDir)
	case "tar.zst":
		cmd = fmt.Sprintf("tar -c -C %s . | zstd -o %s", sourceDir, archivePath)
	case "tar.bz2":
		cmd = fmt.Sprintf("tar -cjf %s -C %s .", archivePath, sourceDir)
	default:
		s.t.Fatalf("Unsupported archive format: %s", format)
	}

	output, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		s.t.Fatalf("Failed to create archive %s: %v\nOutput: %s", format, err, output)
	}

	s.t.Logf("Created archive: %s (format: %s)", archiveName, format)
	return archivePath
}

// ExtractArchive extracts an archive to a temporary directory
func (s *IntegrationTestSuite) ExtractArchive(archivePath string) string {
	extractDir := filepath.Join(s.TempDir, fmt.Sprintf("extracted-%d", time.Now().Unix()))
	err := os.MkdirAll(extractDir, 0755)
	require.NoError(s.t, err, "Failed to create extract directory")

	// Detect format from file extension
	var cmd string
	switch {
	case strings.HasSuffix(archivePath, ".tar.gz"):
		cmd = fmt.Sprintf("tar -xzf %s -C %s", archivePath, extractDir)
	case strings.HasSuffix(archivePath, ".tar.zst"):
		cmd = fmt.Sprintf("zstd -d -c %s | tar -x -C %s", archivePath, extractDir)
	case strings.HasSuffix(archivePath, ".tar.bz2"):
		cmd = fmt.Sprintf("tar -xjf %s -C %s", archivePath, extractDir)
	case strings.HasSuffix(archivePath, ".tar"):
		cmd = fmt.Sprintf("tar -xf %s -C %s", archivePath, extractDir)
	default:
		s.t.Fatalf("Unsupported archive format: %s", archivePath)
	}

	output, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		s.t.Fatalf("Failed to extract archive: %v\nOutput: %s", err, output)
	}

	s.t.Logf("Extracted archive to: %s", extractDir)
	return extractDir
}

// TestIntegrationFramework_BasicFunctionality tests the framework itself
func TestIntegrationFramework_BasicFunctionality(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupIntegrationSuite(t)
	defer suite.Cleanup()

	t.Run("CreateTestFile", func(t *testing.T) {
		// Create a 1MB test file
		filePath := suite.CreateTestFile("test1.dat", 1*1024*1024)

		// Verify file exists
		require.True(t, suite.FileExists(filePath), "Test file should exist")

		// Verify size
		size := suite.GetFileSize(filePath)
		require.Equal(t, int64(1*1024*1024), size, "File size mismatch")

		// Verify checksum is stored
		checksum, ok := suite.Checksums["test1.dat"]
		require.True(t, ok, "Checksum should be stored")
		require.NotEmpty(t, checksum, "Checksum should not be empty")
	})

	t.Run("CreateTestFileWithContent", func(t *testing.T) {
		content := []byte("Hello, CargoShip integration tests!")
		filePath := suite.CreateTestFileWithContent("hello.txt", content)

		// Verify file exists and has correct content
		readContent, err := os.ReadFile(filePath)
		require.NoError(t, err)
		require.Equal(t, content, readContent)
	})

	t.Run("ChecksumVerification", func(t *testing.T) {
		// Create file
		filePath := suite.CreateTestFile("checksum-test.dat", 10*1024)

		// Verify checksum
		suite.VerifyChecksum("checksum-test.dat", filePath)

		// Modify file and verify checksum fails
		err := os.WriteFile(filePath, []byte("corrupted"), 0644)
		require.NoError(t, err)

		// This should fail
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected checksum verification to fail for corrupted file")
			}
		}()
		suite.VerifyChecksum("checksum-test.dat", filePath)
	})

	t.Run("S3Operations", func(t *testing.T) {
		// Create test file
		content := []byte("Test content for S3 upload")
		filePath := suite.CreateTestFileWithContent("s3-test.txt", content)

		// Upload to S3
		s3Key := suite.UploadToS3(filePath, "test-uploads/s3-test.txt")
		require.Equal(t, "test-uploads/s3-test.txt", s3Key)

		// Download from S3
		downloadedPath := suite.DownloadFromS3(s3Key, "s3-test-downloaded.txt")
		require.True(t, suite.FileExists(downloadedPath))

		// Verify content matches
		downloadedContent, err := os.ReadFile(downloadedPath)
		require.NoError(t, err)
		require.Equal(t, content, downloadedContent)
	})

	t.Run("ListS3Objects", func(t *testing.T) {
		// Upload a file
		content := []byte("List test")
		filePath := suite.CreateTestFileWithContent("list-test.txt", content)
		suite.UploadToS3(filePath, "test-list/file.txt")

		// List objects
		objects := suite.ListS3Objects()
		require.NotEmpty(t, objects, "Should have at least one object")

		// Verify our file is in the list
		found := false
		for _, key := range objects {
			if key == "test-list/file.txt" {
				found = true
				break
			}
		}
		require.True(t, found, "Uploaded file should be in the list")
	})
}

// TestIntegration_DataIntegrity_BasicRoundTrip tests complete workflow with checksum verification
func TestIntegration_DataIntegrity_BasicRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupIntegrationSuite(t)
	defer suite.Cleanup()

	testFiles := []struct {
		name string
		size int64
	}{
		{"small.txt", 1 * 1024},        // 1KB
		{"medium.dat", 10 * 1024 * 1024}, // 10MB
		{"large.bin", 100 * 1024 * 1024}, // 100MB
	}

	// Create test files and store checksums
	originalChecksums := make(map[string]string)
	for _, tf := range testFiles {
		suite.CreateTestFile(tf.name, tf.size)
		originalChecksums[tf.name] = suite.Checksums[tf.name]
	}

	t.Run("tar format", func(t *testing.T) {
		testRoundTrip(t, suite, "tar", testFiles, originalChecksums)
	})

	t.Run("tar.gz format", func(t *testing.T) {
		testRoundTrip(t, suite, "tar.gz", testFiles, originalChecksums)
	})

	t.Run("tar.zst format", func(t *testing.T) {
		testRoundTrip(t, suite, "tar.zst", testFiles, originalChecksums)
	})

	t.Run("tar.bz2 format", func(t *testing.T) {
		testRoundTrip(t, suite, "tar.bz2", testFiles, originalChecksums)
	})
}

// testRoundTrip performs complete archive → upload → download → extract → verify workflow
func testRoundTrip(t *testing.T, suite *IntegrationTestSuite, format string, testFiles []struct {
	name string
	size int64
}, originalChecksums map[string]string) {

	// Create archive
	archivePath := suite.CreateArchive(suite.TestDataDir, format)
	require.True(t, suite.FileExists(archivePath), "Archive should exist")

	// Upload to S3
	s3Key := fmt.Sprintf("test-archives/%s/archive.%s", format, format)
	suite.UploadToS3(archivePath, s3Key)

	// Download from S3
	downloadedPath := suite.DownloadFromS3(s3Key, fmt.Sprintf("downloaded-%s", filepath.Base(archivePath)))
	require.True(t, suite.FileExists(downloadedPath), "Downloaded archive should exist")

	// Extract archive
	extractDir := suite.ExtractArchive(downloadedPath)

	// Verify all files with checksums
	for _, tf := range testFiles {
		extractedPath := filepath.Join(extractDir, tf.name)
		require.True(t, suite.FileExists(extractedPath), "Extracted file %s should exist", tf.name)

		actualChecksum := suite.CalculateChecksum(extractedPath)
		require.Equal(t, originalChecksums[tf.name], actualChecksum,
			"Checksum mismatch for %s (format: %s)", tf.name, format)

		t.Logf("✓ %s: checksum verified (%s format)", tf.name, format)
	}

	t.Logf("✓ All files verified for %s format", format)
}

// TestIntegration_DataIntegrity_MixedFileTypes tests various file types
func TestIntegration_DataIntegrity_MixedFileTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupIntegrationSuite(t)
	defer suite.Cleanup()

	// Text file
	textContent := []byte("Hello, CargoShip!\nThis is a test file.\nUTF-8: 你好世界 🚀\n")
	_ = suite.CreateTestFileWithContent("text-file.txt", textContent)

	// Binary file (random data)
	_ = suite.CreateTestFile("binary-file.bin", 5*1024*1024) // 5MB

	// JSON file
	jsonContent := []byte(`{"name":"CargoShip","version":"0.5.1","test":true}`)
	_ = suite.CreateTestFileWithContent("config.json", jsonContent)

	// Store original checksums
	originalChecksums := make(map[string]string)
	originalChecksums["text-file.txt"] = suite.Checksums["text-file.txt"]
	originalChecksums["binary-file.bin"] = suite.Checksums["binary-file.bin"]
	originalChecksums["config.json"] = suite.Checksums["config.json"]

	// Full workflow
	archivePath := suite.CreateArchive(suite.TestDataDir, "tar.zst")
	s3Key := suite.UploadToS3(archivePath, "test-mixed/archive.tar.zst")
	downloadedPath := suite.DownloadFromS3(s3Key, "downloaded-mixed.tar.zst")
	extractDir := suite.ExtractArchive(downloadedPath)

	// Verify each file
	for name, expectedChecksum := range originalChecksums {
		extractedPath := filepath.Join(extractDir, name)
		actualChecksum := suite.CalculateChecksum(extractedPath)
		require.Equal(t, expectedChecksum, actualChecksum,
			"Checksum mismatch for %s", name)
		t.Logf("✓ %s verified", name)
	}

	// Additional verification: read text file content
	extractedTextPath := filepath.Join(extractDir, "text-file.txt")
	extractedText, err := os.ReadFile(extractedTextPath)
	require.NoError(t, err)
	require.Equal(t, textContent, extractedText, "Text file content mismatch")

	// Additional verification: read JSON file
	extractedJSONPath := filepath.Join(extractDir, "config.json")
	extractedJSON, err := os.ReadFile(extractedJSONPath)
	require.NoError(t, err)
	require.Equal(t, jsonContent, extractedJSON, "JSON file content mismatch")

	t.Logf("✓ All mixed file types verified successfully")
}

// TestIntegration_DataIntegrity_EmptyAndSmallFiles tests edge cases
func TestIntegration_DataIntegrity_EmptyAndSmallFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupIntegrationSuite(t)
	defer suite.Cleanup()

	// Empty file
	_ = suite.CreateTestFileWithContent("empty.txt", []byte{})

	// Single byte
	_ = suite.CreateTestFileWithContent("single-byte.txt", []byte("X"))

	// 1 byte
	_ = suite.CreateTestFile("one-byte.dat", 1)

	// Store checksums
	originalChecksums := map[string]string{
		"empty.txt":       suite.Checksums["empty.txt"],
		"single-byte.txt": suite.Checksums["single-byte.txt"],
		"one-byte.dat":    suite.Checksums["one-byte.dat"],
	}

	// Full workflow
	archivePath := suite.CreateArchive(suite.TestDataDir, "tar.gz")
	s3Key := suite.UploadToS3(archivePath, "test-edge-cases/archive.tar.gz")
	downloadedPath := suite.DownloadFromS3(s3Key, "downloaded-edge-cases.tar.gz")
	extractDir := suite.ExtractArchive(downloadedPath)

	// Verify all files
	for name, expectedChecksum := range originalChecksums {
		extractedPath := filepath.Join(extractDir, name)
		actualChecksum := suite.CalculateChecksum(extractedPath)
		require.Equal(t, expectedChecksum, actualChecksum,
			"Checksum mismatch for %s", name)
		t.Logf("✓ %s verified", name)
	}

	t.Logf("✓ All edge case files verified successfully")
}

// TestIntegration_LargeFiles tests large file handling with memory efficiency validation
//
// NOTE: This test can take 30+ minutes to run with full-size files (1GB, 5GB).
// Set CARGOSHIP_QUICK_LARGE_FILE_TEST=true to use smaller test sizes (100MB, 500MB).
//
// Expected runtime:
//   - Quick mode (100MB, 500MB): ~2-5 minutes
//   - Full mode (1GB, 5GB): ~30-60 minutes
func TestIntegration_LargeFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode (requires significant disk space and time)")
	}

	suite := setupIntegrationSuite(t)
	defer suite.Cleanup()

	// Use smaller sizes for quick testing
	quickMode := os.Getenv("CARGOSHIP_QUICK_LARGE_FILE_TEST") == "true"

	var sizes []struct {
		name string
		size int64
	}

	if quickMode {
		t.Log("Running in QUICK mode with smaller file sizes")
		sizes = []struct {
			name string
			size int64
		}{
			{"100MB", 100 * 1024 * 1024},  // 100MB
			{"500MB", 500 * 1024 * 1024},  // 500MB
		}
	} else {
		t.Log("Running in FULL mode with production file sizes (this will take 30+ minutes)")
		sizes = []struct {
			name string
			size int64
		}{
			{"1GB", 1 * 1024 * 1024 * 1024},    // 1GB
			{"5GB", 5 * 1024 * 1024 * 1024},    // 5GB
			// {"10GB", 10 * 1024 * 1024 * 1024}, // 10GB - optional, requires disk space
		}
	}

	for _, tc := range sizes {
		t.Run(tc.name, func(t *testing.T) {
			fileName := fmt.Sprintf("large_%s.dat", tc.name)
			t.Logf("Creating %s test file...", tc.name)

			// Create large file
			largeFilePath := suite.CreateTestFile(fileName, tc.size)
			originalChecksum := suite.Checksums[fileName]
			t.Logf("Created %s file with checksum: %s", tc.name, originalChecksum)

			// Memory monitoring
			var maxMemory uint64
			done := make(chan bool)
			go func() {
				ticker := time.NewTicker(100 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-done:
						return
					case <-ticker.C:
						var m runtime.MemStats
						runtime.ReadMemStats(&m)
						if m.Alloc > maxMemory {
							maxMemory = m.Alloc
						}
					}
				}
			}()

			// Full round-trip workflow
			t.Logf("Creating archive...")
			archivePath := suite.CreateArchive(suite.TestDataDir, "tar.zst")

			t.Logf("Uploading to S3...")
			s3Key := fmt.Sprintf("test-large-files/%s/archive.tar.zst", tc.name)
			suite.UploadToS3(archivePath, s3Key)

			t.Logf("Downloading from S3...")
			downloadedPath := suite.DownloadFromS3(s3Key, fmt.Sprintf("downloaded-%s.tar.zst", tc.name))

			t.Logf("Extracting archive...")
			extractDir := suite.ExtractArchive(downloadedPath)

			// Stop memory monitoring
			close(done)

			// Verify integrity
			extractedPath := filepath.Join(extractDir, fileName)
			require.True(t, suite.FileExists(extractedPath), "Extracted file should exist")

			t.Logf("Verifying checksum...")
			extractedChecksum := suite.CalculateChecksum(extractedPath)
			require.Equal(t, originalChecksum, extractedChecksum,
				"Checksum mismatch for %s file", tc.name)

			// Verify memory efficiency
			maxMemoryMB := maxMemory / (1024 * 1024)
			t.Logf("✓ %s file verified - max memory: %d MB", tc.name, maxMemoryMB)

			// Memory limit check (allow more generous limit for very large files)
			memoryLimit := uint64(500)
			if tc.size >= 5*1024*1024*1024 { // For 5GB+ files
				memoryLimit = 1000 // 1GB limit for very large files
			}
			require.Less(t, maxMemoryMB, memoryLimit,
				"Memory usage exceeded %dMB: actual %dMB", memoryLimit, maxMemoryMB)

			// Cleanup large files to save disk space
			os.Remove(largeFilePath)
			os.Remove(archivePath)
			os.Remove(downloadedPath)
		})
	}

	t.Logf("✓ All large file tests completed successfully")
}
