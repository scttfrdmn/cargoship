package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/aws/costs"
	"github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

func TestNewEstimateCmd(t *testing.T) {
	cmd := NewEstimateCmd()

	assert.NotNil(t, cmd)
	assert.Equal(t, "estimate [path]", cmd.Use)
	assert.Equal(t, "Estimate AWS costs for archiving data", cmd.Short)
	assert.NotEmpty(t, cmd.Long)

	// Verify flags are properly set
	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("format"))
	assert.NotNil(t, flags.Lookup("storage-class"))
	assert.NotNil(t, flags.Lookup("show-recommendations"))
	assert.NotNil(t, flags.Lookup("region"))
	assert.NotNil(t, flags.Lookup("real-time-pricing"))
	assert.NotNil(t, flags.Lookup("show-parallel"))
	assert.NotNil(t, flags.Lookup("max-prefixes"))
	assert.NotNil(t, flags.Lookup("show-upload-optimization"))
	assert.NotNil(t, flags.Lookup("bandwidth"))
}

func TestRunEstimate_NonexistentPath(t *testing.T) {
	cmd := NewEstimateCmd()
	cmd.SetArgs([]string{"/nonexistent/path"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path does not exist")
}

func TestRunEstimate_EmptyDirectory(t *testing.T) {
	// Create temporary empty directory
	tempDir, err := os.MkdirTemp("", "estimate_test_empty")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	cmd := NewEstimateCmd()
	cmd.SetArgs([]string{tempDir})

	// Capture stdout since the command writes directly to os.Stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = cmd.Execute()
	assert.NoError(t, err)

	// Restore stdout and read output
	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// For empty directory, it should still show cost estimate with 0 size
	assert.Contains(t, output, "Cost Estimate")
	assert.Contains(t, output, "$0.00")
}

func TestRunEstimate_WithFiles(t *testing.T) {
	// Create temporary directory with test files
	tempDir, err := os.MkdirTemp("", "estimate_test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create test files
	testFile1 := filepath.Join(tempDir, "file1.txt")
	err = os.WriteFile(testFile1, []byte("test data 1"), 0644)
	require.NoError(t, err)

	testFile2 := filepath.Join(tempDir, "file2.txt")
	err = os.WriteFile(testFile2, []byte("test data 2"), 0644)
	require.NoError(t, err)

	cmd := NewEstimateCmd()
	cmd.SetArgs([]string{tempDir, "--format", "table"})

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = cmd.Execute()
	assert.NoError(t, err)

	// Restore stdout and read output
	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	assert.Contains(t, output, "Cost Estimate")
	assert.Contains(t, output, "Storage")
	assert.Contains(t, output, "Transfer")
}

func TestRunEstimate_JSONFormat(t *testing.T) {
	// Create temporary directory with test files
	tempDir, err := os.MkdirTemp("", "estimate_test_json")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create test file
	testFile := filepath.Join(tempDir, "file.txt")
	err = os.WriteFile(testFile, []byte("test data"), 0644)
	require.NoError(t, err)

	cmd := NewEstimateCmd()
	cmd.SetArgs([]string{tempDir, "--format", "json"})

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = cmd.Execute()
	assert.NoError(t, err)

	// Restore stdout and read output
	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	// Verify JSON output
	var result map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	assert.NoError(t, err)

	assert.Contains(t, result, "cost_estimate")
	assert.Contains(t, result, "parallel_optimization")
	assert.Contains(t, result, "upload_optimization")
}

func TestRunEstimate_StorageClassFlag(t *testing.T) {
	// Create temporary directory with test files
	tempDir, err := os.MkdirTemp("", "estimate_test_storage")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create test file
	testFile := filepath.Join(tempDir, "file.txt")
	err = os.WriteFile(testFile, []byte("test data"), 0644)
	require.NoError(t, err)

	cmd := NewEstimateCmd()
	cmd.SetArgs([]string{tempDir, "--storage-class", "glacier", "--format", "table"})

	// Capture stdout since the command writes directly to os.Stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = cmd.Execute()
	assert.NoError(t, err)

	// Restore stdout and read output
	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Check for glacier in the output table
	assert.Contains(t, output, "Glacier")
}

func TestRunEstimate_NoRecommendations(t *testing.T) {
	// Create temporary directory with test files
	tempDir, err := os.MkdirTemp("", "estimate_test_no_rec")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create test file
	testFile := filepath.Join(tempDir, "file.txt")
	err = os.WriteFile(testFile, []byte("test data"), 0644)
	require.NoError(t, err)

	cmd := NewEstimateCmd()
	cmd.SetArgs([]string{tempDir, "--show-recommendations=false", "--show-parallel=false", "--show-upload-optimization=false"})

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = cmd.Execute()
	assert.NoError(t, err)

	// Restore stdout and read output
	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Should not contain recommendation sections
	assert.NotContains(t, output, "Parallel Upload")
	assert.NotContains(t, output, "Upload Optimization")
}

func TestCreateMockArchives(t *testing.T) {
	// Create temporary directory with test files
	tempDir, err := os.MkdirTemp("", "mock_archives_test")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create test files of different sizes
	testFile1 := filepath.Join(tempDir, "small.txt")
	err = os.WriteFile(testFile1, []byte("small file"), 0644)
	require.NoError(t, err)

	testFile2 := filepath.Join(tempDir, "large.txt")
	largeData := strings.Repeat("large file data ", 1000)
	err = os.WriteFile(testFile2, []byte(largeData), 0644)
	require.NoError(t, err)

	archives, err := createMockArchives(tempDir)
	assert.NoError(t, err)
	// createMockArchives always creates exactly 1 archive with total size
	assert.Len(t, archives, 1)

	// Verify archive has proper structure
	archive := archives[0]
	assert.NotEmpty(t, archive.Key)
	assert.Greater(t, archive.Size, int64(0))
	assert.NotEmpty(t, archive.StorageClass)
	assert.Contains(t, archive.Key, ".tar.zst")
}

func TestCreateMockArchives_EmptyDirectory(t *testing.T) {
	// Create temporary empty directory
	tempDir, err := os.MkdirTemp("", "mock_archives_empty")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	archives, err := createMockArchives(tempDir)
	assert.NoError(t, err)
	// createMockArchives always creates 1 archive even for empty directory
	assert.Len(t, archives, 1)
	// The archive should have 0 size for empty directory
	assert.Equal(t, int64(0), archives[0].Size)
}

func TestCreateMockArchives_NonexistentPath(t *testing.T) {
	archives, err := createMockArchives("/nonexistent/path")
	assert.Error(t, err)
	assert.Nil(t, archives)
}

func TestCreateCalculatorWithRealTimePricing(t *testing.T) {
	ctx := context.Background()
	calculator := createCalculatorWithRealTimePricing(ctx, "us-east-1")
	assert.NotNil(t, calculator)
}

func TestGenerateParallelOptimization(t *testing.T) {
	archives := []s3.Archive{
		{Key: "file1.txt", Size: 1024, StorageClass: "STANDARD"},
		{Key: "file2.txt", Size: 2048, StorageClass: "STANDARD"},
		{Key: "subdir/file3.txt", Size: 512, StorageClass: "STANDARD"},
	}

	optimization := generateParallelOptimization(archives)
	assert.NotNil(t, optimization)
	assert.Greater(t, optimization.RecommendedPrefixes, 0)
	assert.Greater(t, optimization.RecommendedConcurrency, 0)
}

func TestGenerateParallelOptimization_EmptyArchives(t *testing.T) {
	archives := []s3.Archive{}

	optimization := generateParallelOptimization(archives)
	assert.NotNil(t, optimization)
	assert.Equal(t, 1, optimization.RecommendedPrefixes)
	assert.Greater(t, optimization.RecommendedConcurrency, 0)
}

func TestGenerateUploadOptimization(t *testing.T) {
	archives := []s3.Archive{
		{Key: "small.txt", Size: 1024, StorageClass: "STANDARD"},
		{Key: "medium.txt", Size: 10 * 1024 * 1024, StorageClass: "STANDARD"},
		{Key: "large.txt", Size: 100 * 1024 * 1024, StorageClass: "STANDARD"},
	}

	optimization := generateUploadOptimization(archives)
	assert.NotNil(t, optimization)
	assert.Greater(t, optimization.OptimalChunkSize, int64(0))
	assert.Greater(t, optimization.EstimatedDuration, time.Duration(0))
}

func TestGenerateUploadOptimization_EmptyArchives(t *testing.T) {
	archives := []s3.Archive{}

	optimization := generateUploadOptimization(archives)
	// generateUploadOptimization returns nil for empty archives
	assert.Nil(t, optimization)
}

func TestOutputJSON(t *testing.T) {
	// Create test data
	estimate := &costs.CostEstimate{
		TotalUploadCost:  2.40,
		TotalMonthlyCost: 10.50,
		TotalAnnualCost:  126.00,
	}

	parallelOpt := &s3.PrefixOptimization{
		RecommendedPrefixes:    4,
		RecommendedConcurrency: 8,
		TotalSize:              1000000,
		ArchiveCount:           10,
	}

	uploadOpt := &s3.UploadRecommendations{
		OptimalChunkSize:   5 * 1024 * 1024,
		OptimalConcurrency: 8,
		EstimatedDuration:  30 * time.Minute,
		ConfidenceLevel:    0.95,
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputJSON(estimate, parallelOpt, uploadOpt)

	// Restore stdout
	_ = w.Close()
	os.Stdout = oldStdout

	assert.NoError(t, err)

	// Read captured output
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	// Verify JSON structure
	var result map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	assert.NoError(t, err)

	assert.Contains(t, result, "cost_estimate")
	assert.Contains(t, result, "parallel_optimization")
	assert.Contains(t, result, "upload_optimization")
}

func TestOutputTable(t *testing.T) {
	// Create test data
	estimate := &costs.CostEstimate{
		TotalUploadCost:  2.40,
		TotalMonthlyCost: 10.50,
		TotalAnnualCost:  126.00,
	}

	parallelOpt := &s3.PrefixOptimization{
		RecommendedPrefixes:    4,
		RecommendedConcurrency: 8,
		TotalSize:              1000000,
		ArchiveCount:           10,
	}

	uploadOpt := &s3.UploadRecommendations{
		OptimalChunkSize:   5 * 1024 * 1024,
		OptimalConcurrency: 8,
		EstimatedDuration:  30 * time.Minute,
		ConfidenceLevel:    0.95,
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputTable(estimate, parallelOpt, uploadOpt, "/test/path")

	// Restore stdout
	_ = w.Close()
	os.Stdout = oldStdout

	assert.NoError(t, err)

	// Read captured output
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Verify table output contains expected sections
	assert.Contains(t, output, "Cost Estimate")
	assert.Contains(t, output, "Storage")
	assert.Contains(t, output, "Transfer")
	assert.Contains(t, output, "Request")
}

func TestEstimateCmd_FlagDefaults(t *testing.T) {
	cmd := NewEstimateCmd()

	// Test default flag values
	flags := cmd.Flags()

	format, _ := flags.GetString("format")
	assert.Equal(t, "table", format)

	region, _ := flags.GetString("region")
	assert.Equal(t, "us-east-1", region)

	showRec, _ := flags.GetBool("show-recommendations")
	assert.True(t, showRec)

	showParallel, _ := flags.GetBool("show-parallel")
	assert.True(t, showParallel)

	showUpload, _ := flags.GetBool("show-upload-optimization")
	assert.True(t, showUpload)

	realTime, _ := flags.GetBool("real-time-pricing")
	assert.False(t, realTime)

	maxPrefixes, _ := flags.GetInt("max-prefixes")
	assert.Equal(t, 0, maxPrefixes)

	bandwidth, _ := flags.GetFloat64("bandwidth")
	assert.Equal(t, float64(0), bandwidth)
}

func TestEstimateCmd_InvalidFormat(t *testing.T) {
	// Create temporary directory with test files
	tempDir, err := os.MkdirTemp("", "estimate_test_invalid_format")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create test file
	testFile := filepath.Join(tempDir, "file.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	cmd := NewEstimateCmd()
	cmd.SetArgs([]string{tempDir, "--format", "xml"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.Execute()
	// Should return error for unsupported format
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

func TestEstimateCmd_WithSubdirectories(t *testing.T) {
	// Create temporary directory with subdirectories
	tempDir, err := os.MkdirTemp("", "estimate_test_subdir")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create subdirectory with files
	subDir := filepath.Join(tempDir, "subdir")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	testFile1 := filepath.Join(tempDir, "file1.txt")
	err = os.WriteFile(testFile1, []byte("root file"), 0644)
	require.NoError(t, err)

	testFile2 := filepath.Join(subDir, "file2.txt")
	err = os.WriteFile(testFile2, []byte("sub file"), 0644)
	require.NoError(t, err)

	cmd := NewEstimateCmd()
	cmd.SetArgs([]string{tempDir})

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = cmd.Execute()
	assert.NoError(t, err)

	// Restore stdout and read output
	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()
	assert.Contains(t, output, "Cost Estimate")
}

// Benchmark tests
func BenchmarkCreateMockArchives(b *testing.B) {
	// Create temporary directory with test files
	tempDir, err := os.MkdirTemp("", "benchmark_archives")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create multiple test files
	for i := 0; i < 100; i++ {
		testFile := filepath.Join(tempDir, fmt.Sprintf("file%d.txt", i))
		err = os.WriteFile(testFile, []byte("test data"), 0644)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = createMockArchives(tempDir)
	}
}

func BenchmarkGenerateParallelOptimization(b *testing.B) {
	archives := make([]s3.Archive, 1000)
	for i := range archives {
		archives[i] = s3.Archive{
			Key:          fmt.Sprintf("file%d.txt", i),
			Size:         int64(1024 + i),
			StorageClass: "STANDARD",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = generateParallelOptimization(archives)
	}
}
