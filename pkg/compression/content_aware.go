// Package compression provides content-aware compression optimization (Issue #105)
package compression

import (
	"mime"
	"path/filepath"
	"strings"
)

// ContentType represents different content categories for compression
type ContentType string

const (
	ContentTypeImage    ContentType = "image"
	ContentTypeVideo    ContentType = "video"
	ContentTypeAudio    ContentType = "audio"
	ContentTypeDocument ContentType = "document"
	ContentTypeCode     ContentType = "code"
	ContentTypeBinary   ContentType = "binary"
	ContentTypeText     ContentType = "text"
	ContentTypeArchive  ContentType = "archive"
	ContentTypeUnknown  ContentType = "unknown"
)

// ContentAwareConfig configures content-aware compression per content type
type ContentAwareConfig struct {
	// Per-content-type compression levels
	ImageLevel    Level `yaml:"image_level" json:"image_level"`       // Images (already compressed)
	VideoLevel    Level `yaml:"video_level" json:"video_level"`       // Video (already compressed)
	AudioLevel    Level `yaml:"audio_level" json:"audio_level"`       // Audio (already compressed)
	DocumentLevel Level `yaml:"document_level" json:"document_level"` // Documents (good compression)
	CodeLevel     Level `yaml:"code_level" json:"code_level"`         // Source code (best compression)
	BinaryLevel   Level `yaml:"binary_level" json:"binary_level"`     // Binary executables (fast)
	TextLevel     Level `yaml:"text_level" json:"text_level"`         // Plain text (good compression)
	ArchiveLevel  Level `yaml:"archive_level" json:"archive_level"`   // Archives (minimal)
	DefaultLevel  Level `yaml:"default_level" json:"default_level"`   // Unknown types

	// Algorithm selection per content type
	ImageAlgorithm    Algorithm `yaml:"image_algorithm" json:"image_algorithm"`
	VideoAlgorithm    Algorithm `yaml:"video_algorithm" json:"video_algorithm"`
	AudioAlgorithm    Algorithm `yaml:"audio_algorithm" json:"audio_algorithm"`
	DocumentAlgorithm Algorithm `yaml:"document_algorithm" json:"document_algorithm"`
	CodeAlgorithm     Algorithm `yaml:"code_algorithm" json:"code_algorithm"`
	BinaryAlgorithm   Algorithm `yaml:"binary_algorithm" json:"binary_algorithm"`
	TextAlgorithm     Algorithm `yaml:"text_algorithm" json:"text_algorithm"`
	ArchiveAlgorithm  Algorithm `yaml:"archive_algorithm" json:"archive_algorithm"`
	DefaultAlgorithm  Algorithm `yaml:"default_algorithm" json:"default_algorithm"`
}

// DefaultContentAwareConfig returns optimized defaults for content-aware compression
func DefaultContentAwareConfig() *ContentAwareConfig {
	return &ContentAwareConfig{
		// Compression levels (Issue #105 requirements)
		ImageLevel:    LevelFastest, // Level 1: Already compressed, skip high compression
		VideoLevel:    LevelFastest, // Level 1: Already compressed
		AudioLevel:    LevelFastest, // Level 1: Already compressed
		DocumentLevel: 6,            // Level 6: Good compression for text-heavy docs
		CodeLevel:     LevelBest,    // Level 9: Best compression for source code
		BinaryLevel:   LevelFast,    // Level 3: Fast compression for binaries
		TextLevel:     6,            // Level 6: Good compression for text
		ArchiveLevel:  LevelFastest, // Level 1: Already compressed archives
		DefaultLevel:  LevelDefault, // Level 5: Safe default

		// Algorithm selection
		ImageAlgorithm:    AlgorithmZstd, // Fast zstd for already-compressed images
		VideoAlgorithm:    AlgorithmNone, // Skip compression for video
		AudioAlgorithm:    AlgorithmNone, // Skip compression for audio
		DocumentAlgorithm: AlgorithmZstd, // Best compression for documents
		CodeAlgorithm:     AlgorithmZstd, // Best compression for code
		BinaryAlgorithm:   AlgorithmZstd, // Balanced for binaries
		TextAlgorithm:     AlgorithmZstd, // Best compression for text
		ArchiveAlgorithm:  AlgorithmNone, // Skip re-compressing archives
		DefaultAlgorithm:  AlgorithmZstd, // Safe default
	}
}

// ContentAwareCompressor provides content-aware compression optimization
type ContentAwareCompressor struct {
	config *ContentAwareConfig
}

// NewContentAwareCompressor creates a new content-aware compressor
func NewContentAwareCompressor(config *ContentAwareConfig) *ContentAwareCompressor {
	if config == nil {
		config = DefaultContentAwareConfig()
	}
	return &ContentAwareCompressor{
		config: config,
	}
}

// GetOptimalSettings returns the optimal compression algorithm and level for a file
func (cac *ContentAwareCompressor) GetOptimalSettings(filename string) (Algorithm, Level) {
	contentType := DetectContentType(filename)
	return cac.GetSettingsForContentType(contentType)
}

// GetOptimalSettingsWithMetadata returns optimal compression with Magika metadata support (Issue #30)
func (cac *ContentAwareCompressor) GetOptimalSettingsWithMetadata(
	filename string,
	metadata map[string]string,
) (Algorithm, Level) {
	contentType := DetectContentTypeWithMetadata(filename, metadata)
	return cac.GetSettingsForContentType(contentType)
}

// GetSettingsForContentType returns compression settings for a content type
func (cac *ContentAwareCompressor) GetSettingsForContentType(contentType ContentType) (Algorithm, Level) {
	switch contentType {
	case ContentTypeImage:
		return cac.config.ImageAlgorithm, cac.config.ImageLevel
	case ContentTypeVideo:
		return cac.config.VideoAlgorithm, cac.config.VideoLevel
	case ContentTypeAudio:
		return cac.config.AudioAlgorithm, cac.config.AudioLevel
	case ContentTypeDocument:
		return cac.config.DocumentAlgorithm, cac.config.DocumentLevel
	case ContentTypeCode:
		return cac.config.CodeAlgorithm, cac.config.CodeLevel
	case ContentTypeBinary:
		return cac.config.BinaryAlgorithm, cac.config.BinaryLevel
	case ContentTypeText:
		return cac.config.TextAlgorithm, cac.config.TextLevel
	case ContentTypeArchive:
		return cac.config.ArchiveAlgorithm, cac.config.ArchiveLevel
	default:
		return cac.config.DefaultAlgorithm, cac.config.DefaultLevel
	}
}

// DetectContentTypeWithMetadata detects content type using Magika metadata if available,
// falling back to extension-based detection (Issue #30)
func DetectContentTypeWithMetadata(filename string, metadata map[string]string) ContentType {
	// Priority 1: Use Magika detection if available
	if magikaType, ok := metadata["magika_type"]; ok && magikaType != "" {
		contentType := mapMagikaInline(magikaType)
		if contentType != ContentTypeUnknown {
			return contentType
		}
	}

	// Priority 2: Fall back to extension-based detection
	return DetectContentType(filename)
}

// DetectContentType detects the content type from filename/extension
func DetectContentType(filename string) ContentType {
	ext := strings.ToLower(filepath.Ext(filename))

	// Check code extensions first (some have conflicting MIME types like .ts)
	if isCodeExtension(ext) {
		return ContentTypeCode
	}

	// Try MIME type detection
	mimeType := mime.TypeByExtension(ext)
	if mimeType != "" {
		// Parse MIME type
		parts := strings.Split(mimeType, "/")
		if len(parts) >= 1 {
			mainType := parts[0]
			switch mainType {
			case "image":
				return ContentTypeImage
			case "video":
				return ContentTypeVideo
			case "audio":
				return ContentTypeAudio
			case "text":
				return ContentTypeText
			}
		}
	}

	// Fallback to extension-based detection
	switch ext {
	// Images
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".svg", ".ico", ".tiff", ".tif", ".heic", ".heif":
		return ContentTypeImage

	// Video
	case ".mp4", ".avi", ".mkv", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".mpg", ".mpeg", ".3gp":
		return ContentTypeVideo

	// Audio
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".wma", ".m4a", ".opus", ".alac":
		return ContentTypeAudio

	// Documents
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt", ".ods", ".odp", ".rtf":
		return ContentTypeDocument

	// Code/Source files
	case ".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".h", ".hpp", ".rs", ".rb", ".php",
		".cs", ".swift", ".kt", ".scala", ".r", ".m", ".sh", ".bash", ".pl", ".lua", ".vim",
		".sql", ".json", ".yaml", ".yml", ".xml", ".html", ".css", ".scss", ".sass", ".less",
		".jsx", ".tsx", ".vue", ".svelte", ".dart", ".elm", ".ex", ".exs", ".erl", ".hrl",
		".clj", ".cljs", ".hs", ".ml", ".fs", ".f90", ".jl", ".nim", ".cr", ".v", ".vhdl",
		".asm", ".s", ".rkt", ".lisp", ".el", ".diff", ".patch":
		return ContentTypeCode

	// Text files
	case ".txt", ".md", ".markdown", ".log", ".csv", ".tsv", ".ini", ".conf", ".cfg", ".properties",
		".env", ".gitignore", ".dockerignore", ".editorconfig", ".htaccess":
		return ContentTypeText

	// Archives (already compressed)
	case ".zip", ".gz", ".bz2", ".xz", ".7z", ".rar", ".tar", ".tgz", ".tbz2", ".zst", ".lz4",
		".jar", ".war", ".ear", ".apk", ".ipa", ".deb", ".rpm", ".dmg", ".iso":
		return ContentTypeArchive

	// Binary executables
	case ".exe", ".dll", ".so", ".dylib", ".bin", ".app", ".out", ".o", ".a", ".lib", ".class",
		".pyc", ".pyo", ".elc":
		return ContentTypeBinary

	default:
		return ContentTypeUnknown
	}
}

// isCodeExtension checks if an extension represents source code
func isCodeExtension(ext string) bool {
	codeExtensions := map[string]bool{
		".go": true, ".py": true, ".js": true, ".ts": true, ".java": true,
		".c": true, ".cpp": true, ".h": true, ".hpp": true, ".rs": true,
		".rb": true, ".php": true, ".cs": true, ".swift": true, ".kt": true,
		".json": true, ".yaml": true, ".yml": true, ".xml": true,
		".html": true, ".css": true, ".sql": true, ".sh": true,
	}
	return codeExtensions[ext]
}

// mapMagikaInline provides inline Magika mapping to avoid import cycle (Issue #30)
// Full mapping lives in pkg/detection/magika_mapping.go
func mapMagikaInline(magikaLabel string) ContentType {
	// Common mappings (subset for inline use)
	switch magikaLabel {
	case "python", "javascript", "typescript", "go", "rust", "c", "cpp", "java",
		"kotlin", "swift", "ruby", "php", "perl", "shell", "bash", "powershell",
		"sql", "json", "xml", "yaml", "html", "css", "scss", "sass", "less",
		"jsx", "tsx", "vue", "svelte":
		return ContentTypeCode
	case "txt", "markdown", "csv", "tsv", "log", "ini", "conf", "toml":
		return ContentTypeText
	case "jpeg", "jpg", "png", "gif", "webp", "svg", "bmp", "tiff", "ico",
		"heic", "heif", "avif", "jxl", "raw", "cr2", "nef", "arw":
		return ContentTypeImage
	case "mp4", "avi", "mkv", "mov", "webm", "flv", "wmv", "m4v",
		"mpg", "mpeg", "3gp", "ogv":
		return ContentTypeVideo
	case "mp3", "flac", "wav", "ogg", "m4a", "aac", "wma", "opus", "ape", "aiff":
		return ContentTypeAudio
	case "pdf", "docx", "doc", "xlsx", "xls", "pptx", "ppt",
		"odt", "ods", "odp", "rtf", "epub":
		return ContentTypeDocument
	case "zip", "gzip", "tar", "7zip", "7z", "rar", "zstd", "bzip2",
		"xz", "lz4", "lzma", "cab", "iso", "dmg", "woff", "woff2":
		return ContentTypeArchive
	case "elf", "pe", "macho", "java_class", "pyc", "wasm", "dll",
		"so", "dylib", "exe", "app", "obj", "o", "a", "lib",
		"firmware", "rom", "bootloader", "kernel_module", "sqlite", "db",
		"mdb", "accdb", "dbf", "ldf", "mdf", "postgres", "mysql",
		"ttf", "otf", "eot", "dwg", "blend", "fbx", "stl", "3ds",
		"max", "skp", "revit", "keystore", "pkcs12", "gpg", "pgp",
		"msg", "pst", "ost", "binary", "data":
		return ContentTypeBinary
	case "dxf", "obj_3d", "certificate", "private_key", "public_key",
		"eml", "mbox":
		return ContentTypeText // Text-based formats
	default:
		return ContentTypeUnknown
	}
}

// EstimateCompressionBenefit estimates compression benefit for a content type
// Returns a score from 0.0 (no benefit) to 1.0 (maximum benefit)
func EstimateCompressionBenefit(contentType ContentType) float64 {
	switch contentType {
	case ContentTypeCode:
		return 0.9 // High text redundancy
	case ContentTypeText:
		return 0.85 // High text redundancy
	case ContentTypeDocument:
		return 0.8 // Good text redundancy
	case ContentTypeBinary:
		return 0.5 // Moderate benefit
	case ContentTypeImage:
		return 0.1 // Already compressed
	case ContentTypeVideo:
		return 0.05 // Already compressed
	case ContentTypeAudio:
		return 0.05 // Already compressed
	case ContentTypeArchive:
		return 0.0 // Already compressed, no benefit
	default:
		return 0.5 // Unknown, assume moderate
	}
}

// ShouldCompress determines if a file should be compressed based on content type
func ShouldCompress(contentType ContentType) bool {
	benefit := EstimateCompressionBenefit(contentType)
	return benefit > 0.15 // Compress if benefit > 15%
}

// CompressionStrategy describes a compression strategy for a content type
type CompressionStrategy struct {
	ContentType ContentType
	Algorithm   Algorithm
	Level       Level
	Benefit     float64 // Estimated compression benefit (0.0-1.0)
	ShouldSkip  bool    // Whether to skip compression entirely
}

// GetCompressionStrategy returns a complete compression strategy for a file
func (cac *ContentAwareCompressor) GetCompressionStrategy(filename string) CompressionStrategy {
	contentType := DetectContentType(filename)
	algorithm, level := cac.GetSettingsForContentType(contentType)
	benefit := EstimateCompressionBenefit(contentType)
	shouldSkip := !ShouldCompress(contentType)

	return CompressionStrategy{
		ContentType: contentType,
		Algorithm:   algorithm,
		Level:       level,
		Benefit:     benefit,
		ShouldSkip:  shouldSkip,
	}
}
