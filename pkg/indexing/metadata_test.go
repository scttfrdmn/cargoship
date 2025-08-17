package indexing

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/scttfrdmn/cargoship/internal/testutil"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
)

func TestEnhancedFile(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Creation_With_Basic_Fields", func(t *testing.T) {
		now := time.Now()
		file := &EnhancedFile{
			File: inventory.File{
				Name:        "test.txt",
				Size:        1024,
				Destination: "/path/to/test.txt",
			},
			StorageClass: "STANDARD",
			ContentType:  "text/plain",
			CreatedAt:    now,
			ModifiedAt:   now,
		}

		assert.Equal(t, "test.txt", file.Name)
		assert.Equal(t, int64(1024), file.Size)
		assert.Equal(t, "STANDARD", file.StorageClass)
		assert.Equal(t, "text/plain", file.ContentType)
		assert.Equal(t, now, file.CreatedAt)
	})

	t.Run("GetHumanSize_Method", func(t *testing.T) {
		file := &EnhancedFile{
			File: inventory.File{
				Size: 1024,
			},
		}

		humanSize := file.GetHumanSize()
		assert.Equal(t, "1.0 KB", humanSize)
	})

	t.Run("IsCompressed_Method", func(t *testing.T) {
		// Test compressed file
		compressedFile := &EnhancedFile{
			CompressionInfo: CompressionInfo{
				Algorithm:        "zstd",
				OriginalSize:     2048,
				CompressedSize:   1024,
				CompressionRatio: 0.5,
			},
		}
		assert.True(t, compressedFile.IsCompressed())

		// Test uncompressed file
		uncompressedFile := &EnhancedFile{}
		assert.False(t, uncompressedFile.IsCompressed())
	})

	t.Run("GetCompressionRatio_Method", func(t *testing.T) {
		file := &EnhancedFile{
			CompressionInfo: CompressionInfo{
				OriginalSize:     2048,
				CompressedSize:   1024,
				CompressionRatio: 0.5,
			},
		}

		ratio := file.GetCompressionRatio()
		assert.Equal(t, 0.5, ratio)
	})

	t.Run("Tags_Management", func(t *testing.T) {
		file := &EnhancedFile{
			Tags: map[string]string{
				"project": "genomics",
				"stage":   "raw-data",
				"owner":   "researcher-1",
			},
		}

		assert.Equal(t, "genomics", file.Tags["project"])
		assert.Equal(t, "raw-data", file.Tags["stage"])
		assert.Equal(t, "researcher-1", file.Tags["owner"])
		assert.Len(t, file.Tags, 3)
	})

	t.Run("S3_Metadata_Integration", func(t *testing.T) {
		now := time.Now()
		s3Meta := &S3ObjectMetadata{
			ETag:         "\"d41d8cd98f00b204e9800998ecf8427e\"",
			Bucket:       "test-bucket",
			Key:          "data/test.txt",
			StorageClass: "STANDARD_IA",
			LastModified: now,
			Metadata: map[string]string{
				"content-encoding": "gzip",
				"cache-control":    "max-age=3600",
			},
		}

		file := &EnhancedFile{
			S3Metadata: s3Meta,
		}

		assert.NotNil(t, file.S3Metadata)
		assert.Equal(t, "test-bucket", file.S3Metadata.Bucket)
		assert.Equal(t, "data/test.txt", file.S3Metadata.Key)
		assert.Equal(t, "STANDARD_IA", file.S3Metadata.StorageClass)
		assert.Equal(t, "gzip", file.S3Metadata.Metadata["content-encoding"])
	})
}

func TestCompressionInfo(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Compression_Ratio_Calculation", func(t *testing.T) {
		// Test various compression scenarios
		testCases := []struct {
			name            string
			original        int64
			compressed      int64
			expectedRatio   float64
			expectedSavings float64
		}{
			{
				name:            "50% compression",
				original:        2048,
				compressed:      1024,
				expectedRatio:   0.5,
				expectedSavings: 0.5,
			},
			{
				name:            "75% compression",
				original:        4096,
				compressed:      1024,
				expectedRatio:   0.25,
				expectedSavings: 0.75,
			},
			{
				name:            "No compression",
				original:        1024,
				compressed:      1024,
				expectedRatio:   1.0,
				expectedSavings: 0.0,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				compression := CompressionInfo{
					Algorithm:        "zstd",
					OriginalSize:     tc.original,
					CompressedSize:   tc.compressed,
					CompressionRatio: float64(tc.compressed) / float64(tc.original),
				}

				assert.Equal(t, tc.expectedRatio, compression.CompressionRatio)

				savings := 1.0 - compression.CompressionRatio
				assert.InDelta(t, tc.expectedSavings, savings, 0.001)
			})
		}
	})

	t.Run("Compression_Algorithms", func(t *testing.T) {
		algorithms := []string{"zstd", "gzip", "bzip2", "lz4", "snappy"}

		for _, algo := range algorithms {
			t.Run(algo, func(t *testing.T) {
				compression := CompressionInfo{
					Algorithm:        algo,
					OriginalSize:     2048,
					CompressedSize:   1024,
					CompressionRatio: 0.5,
					Level:            3,
				}

				assert.Equal(t, algo, compression.Algorithm)
				assert.Equal(t, int64(2048), compression.OriginalSize)
				assert.Equal(t, int64(1024), compression.CompressedSize)
				assert.Equal(t, 0.5, compression.CompressionRatio)
				assert.Equal(t, 3, compression.Level)
			})
		}
	})
}

func TestS3ObjectMetadata(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Complete_S3_Metadata", func(t *testing.T) {
		now := time.Now()
		metadata := &S3ObjectMetadata{
			ETag:                 "\"abc123def456\"",
			Bucket:               "data-archive",
			Key:                  "projects/genomics/sample1.fastq.gz",
			StorageClass:         "GLACIER_IR",
			LastModified:         now,
			ServerSideEncryption: "AES256",
			Metadata: map[string]string{
				"project":      "genomics-study-2024",
				"researcher":   "dr-smith",
				"content-type": "application/gzip",
				"sample-id":    "SAMPLE001",
			},
		}

		assert.Equal(t, "\"abc123def456\"", metadata.ETag)
		assert.Equal(t, "data-archive", metadata.Bucket)
		assert.Equal(t, "projects/genomics/sample1.fastq.gz", metadata.Key)
		assert.Equal(t, "GLACIER_IR", metadata.StorageClass)
		assert.Equal(t, "AES256", metadata.ServerSideEncryption)
		assert.Equal(t, "genomics-study-2024", metadata.Metadata["project"])
		assert.Equal(t, "dr-smith", metadata.Metadata["researcher"])
		assert.Len(t, metadata.Metadata, 4)
	})

	t.Run("S3_Storage_Classes", func(t *testing.T) {
		storageClasses := []string{
			"STANDARD",
			"STANDARD_IA",
			"ONEZONE_IA",
			"REDUCED_REDUNDANCY",
			"GLACIER",
			"GLACIER_IR",
			"DEEP_ARCHIVE",
		}

		for _, class := range storageClasses {
			t.Run(class, func(t *testing.T) {
				metadata := &S3ObjectMetadata{
					Bucket:       "test-bucket",
					Key:          "test-key",
					StorageClass: class,
				}

				assert.Equal(t, class, metadata.StorageClass)
			})
		}
	})
}

func TestArchiveIndex(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Index_Creation_And_Properties", func(t *testing.T) {
		now := time.Now()
		files := []*EnhancedFile{
			{
				File: inventory.File{
					Name: "file1.txt",
					Size: 1024,
				},
				StorageClass: "STANDARD",
				CreatedAt:    now,
				ModifiedAt:   now,
			},
			{
				File: inventory.File{
					Name: "file2.txt",
					Size: 2048,
				},
				StorageClass: "GLACIER",
				CreatedAt:    now,
				ModifiedAt:   now,
			},
		}

		index := &ArchiveIndex{
			Files:        files,
			CreatedAt:    now,
			IndexVersion: "v0.4.2",
			Location:     "s3://test-bucket/archive/",
			FileCount:    len(files),
			TotalSize:    3072,
			Checksums:    make(map[string]string),
		}

		assert.Len(t, index.Files, 2)
		assert.Equal(t, "v0.4.2", index.IndexVersion)
		assert.Equal(t, "s3://test-bucket/archive/", index.Location)
		assert.Equal(t, 2, index.FileCount)
		assert.Equal(t, int64(3072), index.TotalSize)
		assert.Equal(t, now, index.CreatedAt)
	})

	t.Run("Index_File_Statistics", func(t *testing.T) {
		// Create index with various file sizes
		files := []*EnhancedFile{
			{File: inventory.File{Size: 1024}},     // 1KB
			{File: inventory.File{Size: 1048576}},  // 1MB
			{File: inventory.File{Size: 10485760}}, // 10MB
		}

		index := &ArchiveIndex{
			Files:     files,
			FileCount: len(files),
			TotalSize: 1024 + 1048576 + 10485760,
		}

		assert.Equal(t, 3, index.FileCount)
		assert.Equal(t, int64(11535360), index.TotalSize) // ~11MB total

		// Verify individual file sizes
		assert.Equal(t, int64(1024), files[0].Size)
		assert.Equal(t, int64(1048576), files[1].Size)
		assert.Equal(t, int64(10485760), files[2].Size)
	})
}

func TestEnhancedFileMethodsIntegration(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Complete_File_Lifecycle", func(t *testing.T) {
		now := time.Now()
		archivedTime := now.Add(-24 * time.Hour)

		// Create a complete enhanced file representing a genomics data file
		file := &EnhancedFile{
			File: inventory.File{
				Path:         "/project/data/sample1.fastq.gz",
				Destination:  "genomics/raw-data/sample1.fastq.gz",
				Name:         "sample1.fastq.gz",
				Size:         524288000, // ~500MB
				SuitcaseName: "genomics-raw-01-of-05.tar.zst",
			},
			StorageClass: "STANDARD_IA",
			ContentType:  "application/gzip",
			Tags: map[string]string{
				"project":    "genomics-2024",
				"sample-id":  "SAMPLE001",
				"stage":      "raw-sequencing",
				"researcher": "dr-jones",
			},
			Checksum:     "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			ChecksumType: "SHA256",
			CompressionInfo: CompressionInfo{
				Algorithm:        "gzip",
				OriginalSize:     1073741824, // 1GB original
				CompressedSize:   524288000,  // ~500MB compressed
				CompressionRatio: 0.488,
				Level:            6,
			},
			S3Metadata: &S3ObjectMetadata{
				ETag:         "\"d41d8cd98f00b204e9800998ecf8427e\"",
				Bucket:       "genomics-archive",
				Key:          "projects/2024/raw-data/sample1.fastq.gz",
				StorageClass: "STANDARD_IA",
				LastModified: now,
				Metadata: map[string]string{
					"content-encoding": "gzip",
					"sample-metadata":  "paired-end-150bp",
				},
			},
			CreatedAt:  now.Add(-48 * time.Hour),
			ModifiedAt: now.Add(-24 * time.Hour),
			ArchivedAt: &archivedTime,
		}

		// Test all enhanced functionality
		assert.Equal(t, "sample1.fastq.gz", file.Name)
		assert.Equal(t, int64(524288000), file.Size)
		assert.Equal(t, "500.0 MB", file.GetHumanSize())
		assert.True(t, file.IsCompressed())
		assert.InDelta(t, 0.488, file.GetCompressionRatio(), 0.001)
		assert.Equal(t, "STANDARD_IA", file.StorageClass)
		assert.Equal(t, "genomics-2024", file.Tags["project"])
		assert.NotNil(t, file.S3Metadata)
		assert.Equal(t, "genomics-archive", file.S3Metadata.Bucket)
		assert.NotNil(t, file.ArchivedAt)

		// Test compression efficiency
		savings := 1.0 - file.GetCompressionRatio()
		assert.InDelta(t, 0.512, savings, 0.001) // ~51% space savings
	})
}

func TestArchiveIndexOperations(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Index_With_Large_File_Collection", func(t *testing.T) {
		now := time.Now()
		var files []*EnhancedFile
		var totalSize int64

		// Create 100 test files with varying characteristics
		for i := 0; i < 100; i++ {
			size := int64(1024 * (i + 1)) // Varying sizes
			totalSize += size

			file := &EnhancedFile{
				File: inventory.File{
					Name: fmt.Sprintf("test-file-%03d.dat", i),
					Size: size,
				},
				StorageClass: "STANDARD",
				ContentType:  "application/octet-stream",
				CreatedAt:    now.Add(-time.Duration(i) * time.Hour),
				ModifiedAt:   now.Add(-time.Duration(i/2) * time.Hour),
			}

			// Add compression to some files
			if i%3 == 0 {
				file.CompressionInfo = CompressionInfo{
					Algorithm:        "zstd",
					OriginalSize:     size * 2,
					CompressedSize:   size,
					CompressionRatio: 0.5,
				}
			}

			files = append(files, file)
		}

		index := &ArchiveIndex{
			Files:        files,
			CreatedAt:    now,
			IndexVersion: "v0.4.2",
			Location:     "s3://test-bucket/large-archive/",
			FileCount:    len(files),
			TotalSize:    totalSize,
			Checksums:    make(map[string]string),
		}

		// Verify index properties
		assert.Equal(t, 100, index.FileCount)
		assert.Equal(t, len(files), index.FileCount)
		assert.Equal(t, totalSize, index.TotalSize)
		// Verify the total is reasonable (sum of increasing file sizes)
		expectedTotal := int64(1024) * int64((100*(100+1))/2) // Sum of 1024*(1+2+...+100)
		assert.Equal(t, expectedTotal, totalSize)

		// Verify compressed files
		compressedCount := 0
		for _, file := range index.Files {
			if file.IsCompressed() {
				compressedCount++
			}
		}
		assert.Equal(t, 34, compressedCount) // Every 3rd file (0, 3, 6, ..., 99) = 34 files
	})
}

// Benchmark tests for performance validation
func BenchmarkEnhancedFileCreation(b *testing.B) {
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = &EnhancedFile{
			File: inventory.File{
				Name: fmt.Sprintf("file-%d.txt", i),
				Size: int64(i * 1024),
			},
			StorageClass: "STANDARD",
			ContentType:  "text/plain",
			CreatedAt:    now,
			ModifiedAt:   now,
		}
	}
}

func BenchmarkArchiveIndexCreation(b *testing.B) {
	now := time.Now()
	files := make([]*EnhancedFile, 1000)

	// Pre-create files for benchmarking
	for i := 0; i < 1000; i++ {
		files[i] = &EnhancedFile{
			File: inventory.File{
				Name: fmt.Sprintf("file-%d.txt", i),
				Size: int64(i * 1024),
			},
			StorageClass: "STANDARD",
			CreatedAt:    now,
			ModifiedAt:   now,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = &ArchiveIndex{
			Files:        files,
			CreatedAt:    now,
			IndexVersion: "v0.4.2",
			FileCount:    len(files),
			TotalSize:    int64(len(files) * 1024 * 500), // Average
		}
	}
}
