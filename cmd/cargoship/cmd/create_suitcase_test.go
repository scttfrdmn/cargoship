package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	porter "github.com/scttfrdmn/cargoship/pkg"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
	"github.com/scttfrdmn/cargoship/pkg/rclone"
	"github.com/scttfrdmn/cargoship/pkg/travelagent"
)

func TestNewSuitcaseWithDir(t *testing.T) {
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	// cmd := NewRootCmd(os.Stderr)
	cmd.SetArgs([]string{"create", "suitcase", testD})
	err := cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
}

func TestNewSuitcaseWithBadPrefix(t *testing.T) {
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", testD, "--prefix=foo/bar"})
	err := cmd.ExecuteContext(context.Background())
	require.EqualError(t, err, "prefix cannot contain a /")
}

func TestNewSuitcaseDuplicateDir(t *testing.T) {
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", testD, testD})
	err := cmd.ExecuteContext(context.Background())
	require.EqualError(t, err, "duplicate path found in arguments")
}

func TestNewSuitcaseWithLimit(t *testing.T) {
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{
		"create", "suitcase", "../../../pkg/testdata/limit-dir/",
		"--only-inventory", "--destination", testD,
		"--limit-file-count", "5",
	})
	// err := cmd.ExecuteContext(context.Background())
	err := cmd.Execute()
	require.NoError(t, err)
	i, err := inventory.NewInventoryWithFilename(path.Join(testD, "inventory.yaml"))
	require.NoError(t, err)
	require.Equal(t, 5, len(i.Files))

	// Make sure we get a logfile with actual entries
	lfn := path.Join(testD, "suitcasectl.log")
	require.FileExists(t, lfn)
	lfStat, err := os.Stat(lfn)
	require.NoError(t, err)
	require.Greater(t, lfStat.Size(), int64(0))
}

func TestNewSuitcaseOverflow(t *testing.T) {
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{
		"create", "suitcase", "../../../pkg/testdata/overflow-queue/",
		"--max-suitcase-size", "2.1Mb", "--concurrency", "1", "--destination", testD,
	})
	err := cmd.Execute()
	require.NoError(t, err)
	i, err := inventory.NewInventoryWithFilename(path.Join(testD, "inventory.yaml"))
	require.NoError(t, err)
	require.Equal(t, 2, len(i.Files))
}

func TestNewSuitcaseCloudDest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cloud destination test in short mode (requires rclone operations)")
	}
	
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{
		"create", "suitcase", "../../../pkg/testdata/overflow-queue/",
		"--max-suitcase-size", "2.1Mb", "--concurrency", "2", "--cloud-destination", testD,
	})
	// err := cmd.ExecuteContext(context.Background())
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestMultiCreateRuns(t *testing.T) {
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{
		"create", "suitcase", "../../../pkg/testdata/limit-dir/",
		"--destination", testD,
	})
	err := cmd.Execute()
	require.NoError(t, err)

	cmd = NewRootCmd(io.Discard)
	cmd.SetArgs([]string{
		"create", "suitcase", "../../../pkg/testdata/limit-dir/",
		"--destination", testD,
	})
	err = cmd.Execute()
	require.NoError(t, err)
}

func TestNewSuitcaseOuterHash(t *testing.T) {
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{
		"create", "suitcase", "../../../pkg/testdata/limit-dir/",
		"--destination", testD,
		"--user", "kimuser",
		"--hash-outer",
		"--hash-algorithm", "sha1",
	})
	err := cmd.Execute()
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(testD, "suitcasectl.sha1"))

	// Try md5
	testD = t.TempDir()
	cmd = NewRootCmd(io.Discard)
	cmd.SetArgs([]string{
		"create", "suitcase", "../../../pkg/testdata/limit-dir/",
		"--destination", testD,
		"--user", "kimuser",
		"--hash-outer",
		"--hash-algorithm", "md5",
	})
	err = cmd.Execute()
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(testD, "suitcasectl.md5"))
}

func TestNewSuitcaseWithProfiling(t *testing.T) {
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", testD, "--profile"})
	err := cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	require.FileExists(t, cpufile.Name())
}

func TestNewSuitcaseWithViper(t *testing.T) {
	// testD := t.TempDir()
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "--destination", testD, "../../../pkg/testdata/viper-enabled-target", "--user", "joebob"})
	err := cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	require.FileExists(t, path.Join(testD, "snakey-thing-joebob-01-of-01.tar.zst"))
}

// Ensure that if we set a value on the CLI that it gets preference over whatever is in the user overrides
func TestNewSuitcaseWithViperFlag(t *testing.T) {
	// testD, err := os.MkdirTemp("", "")
	// require.NoError(t, err)
	// defer os.RemoveAll(testD)
	testD := t.TempDir()
	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "--destination", testD, "--user", "darcy", "../../../pkg/testdata/viper-enabled-target"})
	err := cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	require.FileExists(t, path.Join(testD, "snakey-thing-darcy-01-of-01.tar.zst"))
}

func TestNewSuitcaseWithInventory(t *testing.T) {
	toutDir := t.TempDir()
	i, err := inventory.NewDirectoryInventory(inventory.NewOptions(
		inventory.WithDirectories([]string{"../../../pkg/testdata/fake-dir"}),
		inventory.WithSuitcaseFormat("tar"),
	))
	require.NoError(t, err)
	outF, err := os.Create(path.Join(toutDir, "inventory.yaml"))
	require.NoError(t, err)
	ir, err := inventory.NewInventoryerWithFilename(outF.Name())
	require.NoError(t, err)

	err = ir.Write(outF, i)
	require.NoError(t, err)

	cmd := NewRootCmd(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "--destination", toutDir, "--inventory-file", outF.Name()})
	err = cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
}

func TestNewSuitcaseWithInventoryAndDir(t *testing.T) {
	cmd := NewRootCmd(io.Discard)
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"create", "suitcase", "--destination", t.TempDir(), "--inventory-file", "doesnt-matter", t.TempDir()})
	err := cmd.ExecuteContext(context.Background())
	require.Error(t, err, "Did NOT get an error when executing command")
	require.EqualError(t, err, "error: You can't specify an inventory file and target dir arguments at the same time", "Got an unexpected error")
}

func TestInventoryFormatComplete(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(io.Discard)
	cmd.SetOut(b)
	cmd.SetArgs([]string{"__complete", "create", "suitcase", "--inventory-format", ""})
	err := cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	// require.Equal(t, "json\tJSON inventory is not very readable, but could allow for faster machine parsing under certain conditions\nyaml\tYAML is the preferred format. It allows for easy human readable inventories that can also be easily parsed by machines\n:4\n", b.String())
	require.Contains(t, b.String(), "yaml\t")
	require.Contains(t, b.String(), "json\t")
}

func TestSuitcaseFormatComplete(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(io.Discard)
	cmd.SetOut(b)
	cmd.SetArgs([]string{"__complete", "create", "suitcase", "--suitcase-format", ""})
	err := cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	require.Equal(t, "tar\ntar.gpg\ntar.gz\ntar.gz.gpg\ntar.zst\ntar.zst.gpg\n:4\n", b.String())
}

func BenchmarkSuitcaseCreate(b *testing.B) {
	benchmarks := map[string]struct {
		format  string
		tarargs string
	}{
		"tar": {
			format:  "tar",
			tarargs: "c",
		},
		"targz": {
			format:  "tar.gz",
			tarargs: "cz",
		},
	}
	cmd := NewRootCmd(io.Discard)
	// formats := []string{"tar", "tar.gz"}
	datasets := map[string]struct {
		path string
	}{
		"672M-american-gut": {
			path: "American-Gut",
		},
		"3.3G-Synthetic-cell-images": {
			path: "BBBC005_v1_images",
		},
	}
	bdd := os.Getenv("BENCHMARK_DATA_DIR")
	if bdd == "" {
		bdd = "../../../benchmark_data/"
	}
	for desc, opts := range benchmarks {
		for dataDesc, dataSet := range datasets {
			location := path.Join(bdd, dataSet.path)
			if _, err := os.Stat(location); err == nil {
				b.Run(fmt.Sprintf("suitcase_format_golang_%v_%v", dataDesc, desc), func(b *testing.B) {
					out := b.TempDir()
					cmd.SetArgs([]string{"create", "suitcase", location, "--destination", out, "--suitcase-format", opts.format})
					_ = cmd.ExecuteContext(context.Background()) // Benchmark test, error not critical
				})
				//				b.Run(fmt.Sprintf("suitcase_format_gtar_%v_%v", dataDesc, desc), func(b *testing.B) {
				//				out := b.TempDir()
				//			exec.Command("tar", fmt.Sprintf("%vvf", opts.tarargs), path.Join(out, fmt.Sprintf("gnutar.%v", opts.format)), location).Output()
				//	})
			}
		}
	}
}

func TestValidateCmdArgs(t *testing.T) {
	require.EqualError(
		t,
		validateCmdArgs("", false, cobra.Command{}, []string{}),
		"error: You must specify an inventory file or target dirs",
	)

	require.EqualError(
		t,
		validateCmdArgs("inventory.yaml", true, cobra.Command{}, []string{}),
		"you can't specify an inventory file and only-inventory at the same time",
	)
}

func TestNowPtr(t *testing.T) {
	// Test that nowPtr returns a valid time pointer
	timePtr := nowPtr()
	require.NotNil(t, timePtr)
	
	// Test that the time is recent (within last few seconds)
	now := time.Now()
	diff := now.Sub(*timePtr)
	require.True(t, diff >= 0, "Returned time should not be in the future")
	require.True(t, diff < 5*time.Second, "Returned time should be very recent")
	
	// Test that multiple calls return different times
	timePtr1 := nowPtr()
	time.Sleep(1 * time.Millisecond) // Small delay
	timePtr2 := nowPtr()
	
	require.NotEqual(t, timePtr1, timePtr2, "Different calls should return different pointers")
	require.True(t, timePtr2.After(*timePtr1), "Second call should return later time")
}

func TestUploadMetaNilTravelAgent(t *testing.T) {
	// Test uploadMeta with nil TravelAgent (should return nil without error)
	porter := &porter.Porter{
		TravelAgent: nil,
	}
	
	files := []string{"file1.txt", "file2.txt"}
	err := uploadMeta(porter, files)
	require.NoError(t, err)
}

// MockTravelAgent for testing uploadMeta
type MockTravelAgent struct {
	uploadCalls    []string
	uploadResults  map[string]int64
	uploadErrors   map[string]error
	totalUploaded  int64
	mu             sync.Mutex // Add mutex for thread safety
}

func (m *MockTravelAgent) Upload(filePath string, c chan rclone.TransferStatus) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.uploadCalls = append(m.uploadCalls, filePath)
	
	if err, exists := m.uploadErrors[filePath]; exists {
		return 0, err
	}
	
	if size, exists := m.uploadResults[filePath]; exists {
		m.totalUploaded += size
		return size, nil
	}
	
	// Default: return file size based on filename for testing
	defaultSize := int64(len(filePath) * 100) // Simple size calculation
	m.totalUploaded += defaultSize
	return defaultSize, nil
}

func (m *MockTravelAgent) PostMetaData(data string) error {
	// Mock implementation - just return nil for testing
	return nil
}

func (m *MockTravelAgent) StatusURL() string {
	return "http://mock-status-url"
}

func (m *MockTravelAgent) Update(update travelagent.StatusUpdate) (*travelagent.StatusUpdateResponse, error) {
	// Mock implementation 
	return &travelagent.StatusUpdateResponse{}, nil
}

func TestUploadMetaSuccess(t *testing.T) {
	// Test successful uploads
	mockAgent := &MockTravelAgent{
		uploadResults: map[string]int64{
			"file1.txt": 1024,
			"file2.txt": 2048,
			"file3.txt": 512,
		},
		uploadErrors: make(map[string]error),
	}
	
	porter := &porter.Porter{
		TravelAgent: mockAgent,
	}
	
	files := []string{"file1.txt", "file2.txt", "file3.txt"}
	err := uploadMeta(porter, files)
	require.NoError(t, err)
	
	// Verify all files were uploaded
	require.Len(t, mockAgent.uploadCalls, 3)
	require.Contains(t, mockAgent.uploadCalls, "file1.txt")
	require.Contains(t, mockAgent.uploadCalls, "file2.txt")
	require.Contains(t, mockAgent.uploadCalls, "file3.txt")
	
	// Verify total transferred was updated
	expectedTotal := int64(1024 + 2048 + 512)
	require.Equal(t, expectedTotal, porter.TotalTransferred)
}

func TestUploadMetaWithErrors(t *testing.T) {
	// Test uploads with some failures
	mockAgent := &MockTravelAgent{
		uploadResults: map[string]int64{
			"file1.txt": 1024,
			"file3.txt": 512,
		},
		uploadErrors: map[string]error{
			"file2.txt": fmt.Errorf("upload failed for file2"),
		},
	}
	
	porter := &porter.Porter{
		TravelAgent: mockAgent,
	}
	
	files := []string{"file1.txt", "file2.txt", "file3.txt"}
	err := uploadMeta(porter, files)
	
	// Should return error due to file2 failure
	require.Error(t, err)
	require.Contains(t, err.Error(), "upload failed for file2")
	
	// Verify all files were attempted
	require.Len(t, mockAgent.uploadCalls, 3)
}

func TestUploadMetaEmptyFileList(t *testing.T) {
	// Test with empty file list
	mockAgent := &MockTravelAgent{
		uploadResults: make(map[string]int64),
		uploadErrors:  make(map[string]error),
	}
	
	porter := &porter.Porter{
		TravelAgent: mockAgent,
	}
	
	files := []string{} // Empty list
	err := uploadMeta(porter, files)
	require.NoError(t, err)
	
	// No files should be uploaded
	require.Len(t, mockAgent.uploadCalls, 0)
	require.Equal(t, int64(0), porter.TotalTransferred)
}

func TestUploadMetaConcurrentUploads(t *testing.T) {
	// Test concurrent upload handling with many files
	mockAgent := &MockTravelAgent{
		uploadResults: make(map[string]int64),
		uploadErrors:  make(map[string]error),
	}
	
	// Create many files for concurrent testing
	files := make([]string, 20)
	expectedTotal := int64(0)
	for i := 0; i < 20; i++ {
		filename := fmt.Sprintf("file%d.txt", i)
		files[i] = filename
		size := int64((i + 1) * 100)
		mockAgent.uploadResults[filename] = size
		expectedTotal += size
	}
	
	porter := &porter.Porter{
		TravelAgent: mockAgent,
	}
	
	err := uploadMeta(porter, files)
	require.NoError(t, err)
	
	// Due to concurrent execution, we should expect all files to be uploaded
	// The uploadMeta function uses a pool.Wait() which ensures all goroutines complete
	require.Equal(t, 20, len(mockAgent.uploadCalls), "All files should be uploaded")
	
	// Verify all expected files were uploaded
	for _, expectedFile := range files {
		require.Contains(t, mockAgent.uploadCalls, expectedFile, "File %s should be uploaded", expectedFile)
	}
	
	// Verify total transferred matches expected (since all uploads succeed)
	require.Equal(t, expectedTotal, porter.TotalTransferred, "Total transferred should match expected")
}
