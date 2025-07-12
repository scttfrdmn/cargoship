package launch

import (
	"io/fs"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileWatcher(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	watchPaths := []WatchPath{
		{
			Path:      "/tmp",
			Recursive: true,
			MinAge:    time.Hour,
		},
	}

	watcher, err := NewFileWatcher(watchPaths, logger)
	require.NoError(t, err)
	require.NotNil(t, watcher)

	assert.Len(t, watcher.watchPaths, 1)
	assert.Len(t, watcher.detectors, 4) // GenomicsDetector, ImagingDetector, ComputationalDetector, GeneralDatasetDetector
}

func TestGenomicsDetector(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	detector := &GenomicsDetector{logger: logger}

	tests := []struct {
		name     string
		filename string
		fileSize int64
		wantHit  bool
	}{
		{
			name:     "fastq.gz file",
			filename: "sample.fastq.gz",
			fileSize: 1024 * 1024 * 100, // 100MB
			wantHit:  true,
		},
		{
			name:     "bam file",
			filename: "alignment.bam",
			fileSize: 1024 * 1024 * 500, // 500MB
			wantHit:  true,
		},
		{
			name:     "vcf.gz file",
			filename: "variants.vcf.gz",
			fileSize: 1024 * 1024 * 50, // 50MB
			wantHit:  true,
		},
		{
			name:     "non-genomics file",
			filename: "document.pdf",
			fileSize: 1024 * 1024, // 1MB
			wantHit:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock FileInfo
			info := &mockFileInfo{
				name:    tt.filename,
				size:    tt.fileSize,
				modTime: time.Now().Add(-time.Hour * 24), // 1 day old
			}

			candidate, err := detector.Detect("/test/"+tt.filename, info)
			assert.NoError(t, err)

			if tt.wantHit {
				assert.NotNil(t, candidate)
				assert.Equal(t, CandidateTypeFile, candidate.Type)
				assert.Equal(t, "genomics", candidate.DetectedBy)
				assert.Equal(t, "genomics", candidate.Metadata["data_type"])
				assert.Equal(t, tt.fileSize, candidate.Size)

				// Test confidence calculation
				confidence := detector.GetConfidence(candidate)
				assert.Greater(t, confidence, 0.8) // Should be high confidence
			} else {
				assert.Nil(t, candidate)
			}
		})
	}
}

func TestImagingDetector(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	detector := &ImagingDetector{logger: logger}

	tests := []struct {
		name     string
		filename string
		fileSize int64
		wantHit  bool
	}{
		{
			name:     "tiff file",
			filename: "microscopy.tiff",
			fileSize: 1024 * 1024 * 200, // 200MB
			wantHit:  true,
		},
		{
			name:     "czi file",
			filename: "confocal.czi",
			fileSize: 1024 * 1024 * 800, // 800MB
			wantHit:  true,
		},
		{
			name:     "lsm file",
			filename: "zeiss.lsm",
			fileSize: 1024 * 1024 * 300, // 300MB
			wantHit:  true,
		},
		{
			name:     "non-imaging file",
			filename: "data.csv",
			fileSize: 1024 * 1024, // 1MB
			wantHit:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &mockFileInfo{
				name:    tt.filename,
				size:    tt.fileSize,
				modTime: time.Now().Add(-time.Hour * 24),
			}

			candidate, err := detector.Detect("/test/"+tt.filename, info)
			assert.NoError(t, err)

			if tt.wantHit {
				assert.NotNil(t, candidate)
				assert.Equal(t, CandidateTypeFile, candidate.Type)
				assert.Equal(t, "imaging", candidate.DetectedBy)
				assert.Equal(t, "imaging", candidate.Metadata["data_type"])

				confidence := detector.GetConfidence(candidate)
				assert.Greater(t, confidence, 0.8)
			} else {
				assert.Nil(t, candidate)
			}
		})
	}
}

func TestComputationalDetector(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	detector := &ComputationalDetector{logger: logger}

	tests := []struct {
		name     string
		filename string
		wantHit  bool
	}{
		{
			name:     "csv file",
			filename: "results.csv",
			wantHit:  true,
		},
		{
			name:     "h5 file",
			filename: "simulation.h5",
			wantHit:  true,
		},
		{
			name:     "log file",
			filename: "job.log",
			wantHit:  true,
		},
		{
			name:     "out file",
			filename: "program.out",
			wantHit:  true,
		},
		{
			name:     "non-computational file",
			filename: "image.jpg",
			wantHit:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &mockFileInfo{
				name:    tt.filename,
				size:    1024 * 1024 * 10, // 10MB
				modTime: time.Now().Add(-time.Hour * 24),
			}

			candidate, err := detector.Detect("/test/"+tt.filename, info)
			assert.NoError(t, err)

			if tt.wantHit {
				assert.NotNil(t, candidate)
				assert.Equal(t, CandidateTypeFile, candidate.Type)
				assert.Equal(t, "computational", candidate.DetectedBy)
				assert.Equal(t, "computational", candidate.Metadata["data_type"])
			} else {
				assert.Nil(t, candidate)
			}
		})
	}
}

func TestGeneralDatasetDetector(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	detector := &GeneralDatasetDetector{logger: logger}

	tests := []struct {
		name     string
		dirname  string
		wantHit  bool
	}{
		{
			name:    "data directory",
			dirname: "experiment-data",
			wantHit: true,
		},
		{
			name:    "results directory",
			dirname: "analysis-results",
			wantHit: true,
		},
		{
			name:    "completed directory",
			dirname: "job-completed",
			wantHit: true,
		},
		{
			name:    "random directory",
			dirname: "random-stuff",
			wantHit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &mockFileInfo{
				name:    tt.dirname,
				size:    0,
				modTime: time.Now().Add(-time.Hour * 24),
				isDir:   true,
			}

			candidate, err := detector.Detect("/test/"+tt.dirname, info)
			assert.NoError(t, err)

			if tt.wantHit {
				assert.NotNil(t, candidate)
				assert.Equal(t, CandidateTypeDataset, candidate.Type)
				assert.Equal(t, "general", candidate.DetectedBy)
				assert.Equal(t, "dataset", candidate.Metadata["data_type"])
			} else {
				assert.Nil(t, candidate)
			}
		})
	}
}

func TestMatchesPatterns(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	watcher := &FileWatcher{logger: logger}

	tests := []struct {
		name            string
		path            string
		includePatterns []string
		excludePatterns []string
		want            bool
	}{
		{
			name:            "matches include pattern",
			path:            "/data/test.fastq.gz",
			includePatterns: []string{"*.fastq.gz"},
			excludePatterns: []string{},
			want:            true,
		},
		{
			name:            "excluded by exclude pattern",
			path:            "/data/test.tmp",
			includePatterns: []string{"*"},
			excludePatterns: []string{"*.tmp"},
			want:            false,
		},
		{
			name:            "no include patterns (include all)",
			path:            "/data/test.bam",
			includePatterns: []string{},
			excludePatterns: []string{},
			want:            true,
		},
		{
			name:            "does not match include pattern",
			path:            "/data/test.pdf",
			includePatterns: []string{"*.fastq.gz", "*.bam"},
			excludePatterns: []string{},
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := watcher.matchesPatterns(tt.path, tt.includePatterns, tt.excludePatterns)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestPriorityCalculation(t *testing.T) {
	tests := []struct {
		name     string
		ext      string
		function func(string) int
		want     int
	}{
		{
			name:     "fastq genomics priority",
			ext:      ".fastq.gz",
			function: getGenomicsPriority,
			want:     1,
		},
		{
			name:     "bam genomics priority",
			ext:      ".bam",
			function: getGenomicsPriority,
			want:     2,
		},
		{
			name:     "vcf genomics priority",
			ext:      ".vcf.gz",
			function: getGenomicsPriority,
			want:     3,
		},
		{
			name:     "czi imaging priority",
			ext:      ".czi",
			function: getImagingPriority,
			want:     1,
		},
		{
			name:     "tiff imaging priority",
			ext:      ".tiff",
			function: getImagingPriority,
			want:     2,
		},
		{
			name:     "csv computational priority",
			ext:      ".csv",
			function: getComputationalPriority,
			want:     2,
		},
		{
			name:     "log computational priority",
			ext:      ".log",
			function: getComputationalPriority,
			want:     5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.function(tt.ext)
			assert.Equal(t, tt.want, result)
		})
	}
}

// mockFileInfo implements fs.FileInfo for testing
type mockFileInfo struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() fs.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return m.modTime }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() interface{}   { return nil }