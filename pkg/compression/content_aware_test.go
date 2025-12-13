package compression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     ContentType
	}{
		// Images
		{"JPEG image", "photo.jpg", ContentTypeImage},
		{"PNG image", "icon.png", ContentTypeImage},
		{"GIF animation", "cat.gif", ContentTypeImage},
		{"WebP image", "modern.webp", ContentTypeImage},
		{"SVG vector", "logo.svg", ContentTypeImage},
		{"HEIC photo", "iphone.heic", ContentTypeImage},

		// Video
		{"MP4 video", "movie.mp4", ContentTypeVideo},
		{"MKV video", "film.mkv", ContentTypeVideo},
		{"WebM video", "clip.webm", ContentTypeVideo},
		{"MOV video", "recording.mov", ContentTypeVideo},

		// Audio
		{"MP3 audio", "song.mp3", ContentTypeAudio},
		{"FLAC audio", "lossless.flac", ContentTypeAudio},
		{"WAV audio", "sound.wav", ContentTypeAudio},
		{"OGG audio", "music.ogg", ContentTypeAudio},

		// Documents
		{"PDF document", "report.pdf", ContentTypeDocument},
		{"Word document", "letter.docx", ContentTypeDocument},
		{"Excel spreadsheet", "data.xlsx", ContentTypeDocument},
		{"PowerPoint", "slides.pptx", ContentTypeDocument},

		// Code/Source files
		{"Go source", "main.go", ContentTypeCode},
		{"Python script", "script.py", ContentTypeCode},
		{"JavaScript", "app.js", ContentTypeCode},
		{"TypeScript", "component.ts", ContentTypeCode},
		{"Java source", "Main.java", ContentTypeCode},
		{"C source", "program.c", ContentTypeCode},
		{"C++ source", "engine.cpp", ContentTypeCode},
		{"Rust source", "lib.rs", ContentTypeCode},
		{"JSON data", "config.json", ContentTypeCode},
		{"YAML config", "settings.yaml", ContentTypeCode},
		{"XML data", "data.xml", ContentTypeCode},
		{"HTML page", "index.html", ContentTypeCode},
		{"CSS styles", "theme.css", ContentTypeCode},
		{"SQL script", "schema.sql", ContentTypeCode},
		{"Shell script", "deploy.sh", ContentTypeCode},

		// Text files
		{"Plain text", "notes.txt", ContentTypeText},
		{"Markdown", "README.md", ContentTypeText},
		{"Log file", "app.log", ContentTypeText},
		{"CSV data", "export.csv", ContentTypeText},

		// Archives (already compressed)
		{"ZIP archive", "backup.zip", ContentTypeArchive},
		{"TAR.GZ archive", "files.tar.gz", ContentTypeArchive},
		{"7-Zip archive", "data.7z", ContentTypeArchive},
		{"RAR archive", "old.rar", ContentTypeArchive},
		{"Zstandard archive", "compressed.zst", ContentTypeArchive},
		{"JAR file", "library.jar", ContentTypeArchive},
		{"APK package", "app.apk", ContentTypeArchive},

		// Binary executables
		{"Windows executable", "program.exe", ContentTypeBinary},
		{"Linux binary", "app.out", ContentTypeBinary},
		{"Shared library", "libfoo.so", ContentTypeBinary},
		{"macOS library", "libbar.dylib", ContentTypeBinary},
		{"Java class", "Main.class", ContentTypeBinary},
		{"Python bytecode", "module.pyc", ContentTypeBinary},

		// Unknown/Edge cases
		{"Unknown extension", "file.xyz", ContentTypeUnknown},
		{"No extension", "Makefile", ContentTypeUnknown},
		{"Empty filename", "", ContentTypeUnknown},
		{"Just extension", ".gitignore", ContentTypeText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectContentType(tt.filename)
			assert.Equal(t, tt.want, got, "DetectContentType(%q) = %v, want %v", tt.filename, got, tt.want)
		})
	}
}

func TestDetectContentType_CaseInsensitive(t *testing.T) {
	tests := []struct {
		filename string
		want     ContentType
	}{
		{"FILE.JPG", ContentTypeImage},
		{"Script.PY", ContentTypeCode},
		{"Document.PDF", ContentTypeDocument},
		{"Archive.ZIP", ContentTypeArchive},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := DetectContentType(tt.filename)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultContentAwareConfig(t *testing.T) {
	config := DefaultContentAwareConfig()

	require.NotNil(t, config)

	// Verify compression levels match Issue #105 requirements
	assert.Equal(t, LevelFastest, config.ImageLevel, "Images should use level 1 (already compressed)")
	assert.Equal(t, LevelFastest, config.VideoLevel, "Video should use level 1 (already compressed)")
	assert.Equal(t, LevelFastest, config.AudioLevel, "Audio should use level 1 (already compressed)")
	assert.Equal(t, Level(6), config.DocumentLevel, "Documents should use level 6 (good compression)")
	assert.Equal(t, LevelBest, config.CodeLevel, "Code should use level 9 (best compression)")
	assert.Equal(t, LevelFast, config.BinaryLevel, "Binary should use level 3 (fast)")
	assert.Equal(t, Level(6), config.TextLevel, "Text should use level 6 (good compression)")
	assert.Equal(t, LevelFastest, config.ArchiveLevel, "Archives should use level 1 (already compressed)")
	assert.Equal(t, LevelDefault, config.DefaultLevel, "Default should use level 5")

	// Verify algorithm selection
	assert.Equal(t, AlgorithmZstd, config.ImageAlgorithm)
	assert.Equal(t, AlgorithmNone, config.VideoAlgorithm, "Video should skip compression")
	assert.Equal(t, AlgorithmNone, config.AudioAlgorithm, "Audio should skip compression")
	assert.Equal(t, AlgorithmZstd, config.DocumentAlgorithm)
	assert.Equal(t, AlgorithmZstd, config.CodeAlgorithm)
	assert.Equal(t, AlgorithmZstd, config.BinaryAlgorithm)
	assert.Equal(t, AlgorithmZstd, config.TextAlgorithm)
	assert.Equal(t, AlgorithmNone, config.ArchiveAlgorithm, "Archives should skip compression")
	assert.Equal(t, AlgorithmZstd, config.DefaultAlgorithm)
}

func TestNewContentAwareCompressor(t *testing.T) {
	t.Run("with nil config", func(t *testing.T) {
		compressor := NewContentAwareCompressor(nil)
		require.NotNil(t, compressor)
		require.NotNil(t, compressor.config)

		// Should use default config
		assert.Equal(t, LevelBest, compressor.config.CodeLevel)
	})

	t.Run("with custom config", func(t *testing.T) {
		config := &ContentAwareConfig{
			CodeLevel:     Level(7),
			CodeAlgorithm: AlgorithmZstd,
		}
		compressor := NewContentAwareCompressor(config)
		require.NotNil(t, compressor)
		assert.Equal(t, Level(7), compressor.config.CodeLevel)
	})
}

func TestGetOptimalSettings(t *testing.T) {
	compressor := NewContentAwareCompressor(nil)

	tests := []struct {
		name          string
		filename      string
		wantAlgorithm Algorithm
		wantLevel     Level
	}{
		{"Go source file", "main.go", AlgorithmZstd, LevelBest},
		{"JPEG image", "photo.jpg", AlgorithmZstd, LevelFastest},
		{"PDF document", "report.pdf", AlgorithmZstd, Level(6)},
		{"MP4 video", "movie.mp4", AlgorithmNone, LevelFastest},
		{"Binary executable", "app.exe", AlgorithmZstd, LevelFast},
		{"Text file", "notes.txt", AlgorithmZstd, Level(6)},
		{"ZIP archive", "backup.zip", AlgorithmNone, LevelFastest},
		{"Unknown file", "file.xyz", AlgorithmZstd, LevelDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			algorithm, level := compressor.GetOptimalSettings(tt.filename)
			assert.Equal(t, tt.wantAlgorithm, algorithm, "algorithm mismatch for %s", tt.filename)
			assert.Equal(t, tt.wantLevel, level, "level mismatch for %s", tt.filename)
		})
	}
}

func TestGetSettingsForContentType(t *testing.T) {
	compressor := NewContentAwareCompressor(nil)

	tests := []struct {
		name          string
		contentType   ContentType
		wantAlgorithm Algorithm
		wantLevel     Level
	}{
		{"Image content", ContentTypeImage, AlgorithmZstd, LevelFastest},
		{"Video content", ContentTypeVideo, AlgorithmNone, LevelFastest},
		{"Audio content", ContentTypeAudio, AlgorithmNone, LevelFastest},
		{"Document content", ContentTypeDocument, AlgorithmZstd, Level(6)},
		{"Code content", ContentTypeCode, AlgorithmZstd, LevelBest},
		{"Binary content", ContentTypeBinary, AlgorithmZstd, LevelFast},
		{"Text content", ContentTypeText, AlgorithmZstd, Level(6)},
		{"Archive content", ContentTypeArchive, AlgorithmNone, LevelFastest},
		{"Unknown content", ContentTypeUnknown, AlgorithmZstd, LevelDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			algorithm, level := compressor.GetSettingsForContentType(tt.contentType)
			assert.Equal(t, tt.wantAlgorithm, algorithm)
			assert.Equal(t, tt.wantLevel, level)
		})
	}
}

func TestIsCodeExtension(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{".go", true},
		{".py", true},
		{".js", true},
		{".ts", true},
		{".java", true},
		{".c", true},
		{".cpp", true},
		{".json", true},
		{".yaml", true},
		{".html", true},
		{".css", true},
		{".txt", false},
		{".jpg", false},
		{".pdf", false},
		{".xyz", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := isCodeExtension(tt.ext)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEstimateCompressionBenefit(t *testing.T) {
	tests := []struct {
		name        string
		contentType ContentType
		want        float64
	}{
		{"Code - high benefit", ContentTypeCode, 0.9},
		{"Text - high benefit", ContentTypeText, 0.85},
		{"Document - good benefit", ContentTypeDocument, 0.8},
		{"Binary - moderate benefit", ContentTypeBinary, 0.5},
		{"Image - low benefit", ContentTypeImage, 0.1},
		{"Video - very low benefit", ContentTypeVideo, 0.05},
		{"Audio - very low benefit", ContentTypeAudio, 0.05},
		{"Archive - no benefit", ContentTypeArchive, 0.0},
		{"Unknown - moderate benefit", ContentTypeUnknown, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateCompressionBenefit(tt.contentType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShouldCompress(t *testing.T) {
	tests := []struct {
		name        string
		contentType ContentType
		want        bool
	}{
		{"Code should compress", ContentTypeCode, true},
		{"Text should compress", ContentTypeText, true},
		{"Document should compress", ContentTypeDocument, true},
		{"Binary should compress", ContentTypeBinary, true},
		{"Image should skip", ContentTypeImage, false},
		{"Video should skip", ContentTypeVideo, false},
		{"Audio should skip", ContentTypeAudio, false},
		{"Archive should skip", ContentTypeArchive, false},
		{"Unknown should compress", ContentTypeUnknown, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldCompress(tt.contentType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetCompressionStrategy(t *testing.T) {
	compressor := NewContentAwareCompressor(nil)

	tests := []struct {
		name        string
		filename    string
		wantType    ContentType
		wantSkip    bool
		wantBenefit float64
	}{
		{
			name:        "Go source file",
			filename:    "main.go",
			wantType:    ContentTypeCode,
			wantSkip:    false,
			wantBenefit: 0.9,
		},
		{
			name:        "JPEG image",
			filename:    "photo.jpg",
			wantType:    ContentTypeImage,
			wantSkip:    true,
			wantBenefit: 0.1,
		},
		{
			name:        "PDF document",
			filename:    "report.pdf",
			wantType:    ContentTypeDocument,
			wantSkip:    false,
			wantBenefit: 0.8,
		},
		{
			name:        "MP4 video",
			filename:    "movie.mp4",
			wantType:    ContentTypeVideo,
			wantSkip:    true,
			wantBenefit: 0.05,
		},
		{
			name:        "ZIP archive",
			filename:    "backup.zip",
			wantType:    ContentTypeArchive,
			wantSkip:    true,
			wantBenefit: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := compressor.GetCompressionStrategy(tt.filename)

			assert.Equal(t, tt.wantType, strategy.ContentType)
			assert.Equal(t, tt.wantSkip, strategy.ShouldSkip)
			assert.Equal(t, tt.wantBenefit, strategy.Benefit)
			assert.NotEqual(t, Algorithm(""), strategy.Algorithm)
			assert.NotEqual(t, Level(0), strategy.Level)
		})
	}
}

func TestGetCompressionStrategy_AlgorithmAndLevel(t *testing.T) {
	compressor := NewContentAwareCompressor(nil)

	t.Run("code file strategy", func(t *testing.T) {
		strategy := compressor.GetCompressionStrategy("main.go")
		assert.Equal(t, ContentTypeCode, strategy.ContentType)
		assert.Equal(t, AlgorithmZstd, strategy.Algorithm)
		assert.Equal(t, LevelBest, strategy.Level)
		assert.False(t, strategy.ShouldSkip)
	})

	t.Run("video file strategy", func(t *testing.T) {
		strategy := compressor.GetCompressionStrategy("movie.mp4")
		assert.Equal(t, ContentTypeVideo, strategy.ContentType)
		assert.Equal(t, AlgorithmNone, strategy.Algorithm)
		assert.Equal(t, LevelFastest, strategy.Level)
		assert.True(t, strategy.ShouldSkip)
	})
}

func TestContentAwareCompressor_CustomConfig(t *testing.T) {
	// Create custom config with different levels
	config := &ContentAwareConfig{
		ImageLevel:        Level(5),
		ImageAlgorithm:    AlgorithmZstd,
		CodeLevel:         Level(7),
		CodeAlgorithm:     AlgorithmZstd,
		DocumentLevel:     Level(8),
		DocumentAlgorithm: AlgorithmZstd,
		DefaultLevel:      Level(4),
		DefaultAlgorithm:  AlgorithmZstd,
	}

	compressor := NewContentAwareCompressor(config)

	t.Run("custom image level", func(t *testing.T) {
		algorithm, level := compressor.GetOptimalSettings("photo.jpg")
		assert.Equal(t, AlgorithmZstd, algorithm)
		assert.Equal(t, Level(5), level)
	})

	t.Run("custom code level", func(t *testing.T) {
		algorithm, level := compressor.GetOptimalSettings("main.go")
		assert.Equal(t, AlgorithmZstd, algorithm)
		assert.Equal(t, Level(7), level)
	})

	t.Run("custom document level", func(t *testing.T) {
		algorithm, level := compressor.GetOptimalSettings("report.pdf")
		assert.Equal(t, AlgorithmZstd, algorithm)
		assert.Equal(t, Level(8), level)
	})

	t.Run("custom default level", func(t *testing.T) {
		algorithm, level := compressor.GetOptimalSettings("unknown.xyz")
		assert.Equal(t, AlgorithmZstd, algorithm)
		assert.Equal(t, Level(4), level)
	})
}

func TestContentType_String(t *testing.T) {
	tests := []struct {
		contentType ContentType
		want        string
	}{
		{ContentTypeImage, "image"},
		{ContentTypeVideo, "video"},
		{ContentTypeAudio, "audio"},
		{ContentTypeDocument, "document"},
		{ContentTypeCode, "code"},
		{ContentTypeBinary, "binary"},
		{ContentTypeText, "text"},
		{ContentTypeArchive, "archive"},
		{ContentTypeUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := string(tt.contentType)
			assert.Equal(t, tt.want, got)
		})
	}
}
