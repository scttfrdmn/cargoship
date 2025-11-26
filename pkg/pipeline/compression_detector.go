package pipeline

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CompressionDetector determines if a file is already compressed
type CompressionDetector struct {
	// Configuration
	skipExtensions   map[string]bool
	skipMagicBytes   []magicBytePattern
	entropyThreshold float64
}

// magicBytePattern represents a magic byte signature
type magicBytePattern struct {
	offset int
	bytes  []byte
	desc   string
}

// NewCompressionDetector creates a new compression detector
func NewCompressionDetector() *CompressionDetector {
	return &CompressionDetector{
		skipExtensions:   buildSkipExtensionsMap(),
		skipMagicBytes:   buildSkipMagicBytes(),
		entropyThreshold: 0.85, // High entropy suggests compressed/encrypted data
	}
}

// ShouldCompress returns true if the file should be compressed
func (d *CompressionDetector) ShouldCompress(path string) (bool, string) {
	// Check file extension first (fastest)
	ext := strings.ToLower(filepath.Ext(path))
	if ext != "" && d.skipExtensions[ext] {
		return false, "already_compressed_extension:" + ext
	}

	// For small files or unknown extensions, check magic bytes
	if shouldCheckMagicBytes(path) {
		if compressed, desc := d.checkMagicBytes(path); compressed {
			return false, "already_compressed_magic:" + desc
		}
	}

	return true, "compressible"
}

// checkMagicBytes reads file header and checks for compression signatures
func (d *CompressionDetector) checkMagicBytes(path string) (bool, string) {
	file, err := os.Open(path)
	if err != nil {
		// If we can't read, assume compressible (safer default)
		return false, ""
	}
	defer func() { _ = file.Close() }()

	// Read first 512 bytes for magic byte detection
	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && err != io.EOF {
		return false, ""
	}
	header = header[:n]

	// Check all magic byte patterns
	for _, pattern := range d.skipMagicBytes {
		if pattern.offset+len(pattern.bytes) <= len(header) {
			if bytes.Equal(header[pattern.offset:pattern.offset+len(pattern.bytes)], pattern.bytes) {
				return true, pattern.desc
			}
		}
	}

	return false, ""
}

// shouldCheckMagicBytes determines if magic byte checking is worth the I/O cost
func shouldCheckMagicBytes(path string) bool {
	// Check magic bytes for files without extension or unknown extensions
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" || ext == ".dat" || ext == ".bin" || ext == ".tmp" {
		return true
	}

	// For common text extensions, no need to check (definitely compressible)
	textExtensions := map[string]bool{
		".txt": true, ".log": true, ".md": true, ".csv": true,
		".json": true, ".xml": true, ".yaml": true, ".yml": true,
		".c": true, ".cpp": true, ".h": true, ".go": true,
		".js": true, ".ts": true, ".py": true, ".java": true,
		".sh": true, ".bash": true, ".sql": true, ".html": true,
	}

	return !textExtensions[ext]
}

// buildSkipExtensionsMap creates a map of file extensions that are already compressed
func buildSkipExtensionsMap() map[string]bool {
	extensions := []string{
		// Image formats (compressed)
		".jpg", ".jpeg", ".jpe", ".jfif",
		".png", // Uses DEFLATE compression
		".gif",
		".webp",
		".heic", ".heif",
		".avif",
		".jp2", ".j2k", // JPEG 2000

		// Video formats (compressed)
		".mp4", ".m4v", ".m4a",
		".mov",
		".avi",
		".mkv",
		".webm",
		".flv",
		".wmv",
		".mpg", ".mpeg",
		".3gp",
		".ts", ".mts",

		// Audio formats (compressed)
		".mp3",
		".aac", ".m4a",
		".ogg", ".oga",
		".opus",
		".wma",
		".flac", // Lossless but compressed

		// Archives (already compressed)
		".zip",
		".gz", ".gzip",
		".bz2", ".bzip2",
		".xz",
		".7z",
		".rar",
		".tar.gz", ".tgz",
		".tar.bz2", ".tbz2",
		".tar.xz", ".txz",
		".zst", ".zstd",
		".lz4",
		".lzma",

		// Documents (may be compressed containers)
		".pdf", // Uses internal compression
		".docx", ".xlsx", ".pptx", // ZIP-based
		".odt", ".ods", ".odp",    // ZIP-based
		".epub",                   // ZIP-based

		// Executables (may be compressed)
		".apk", // ZIP-based
		".jar", // ZIP-based
		".war", // ZIP-based
		".ipa", // ZIP-based

		// Other compressed formats
		".dmg",  // macOS disk images
		".iso",  // May contain compressed files
		".vmdk", // Virtual machine disks (often compressed)
		".vhd", ".vhdx",
		".qcow2",
	}

	m := make(map[string]bool, len(extensions))
	for _, ext := range extensions {
		m[ext] = true
	}
	return m
}

// buildSkipMagicBytes creates a list of magic byte patterns for compressed formats
func buildSkipMagicBytes() []magicBytePattern {
	return []magicBytePattern{
		// Archives
		{0, []byte{0x50, 0x4B, 0x03, 0x04}, "ZIP"},
		{0, []byte{0x50, 0x4B, 0x05, 0x06}, "ZIP_empty"},
		{0, []byte{0x50, 0x4B, 0x07, 0x08}, "ZIP_spanned"},
		{0, []byte{0x1F, 0x8B}, "GZIP"},
		{0, []byte{0x42, 0x5A, 0x68}, "BZIP2"},
		{0, []byte{0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00}, "XZ"},
		{0, []byte{0x37, 0x7A, 0xBC, 0xAF, 0x27, 0x1C}, "7Z"},
		{0, []byte{0x52, 0x61, 0x72, 0x21, 0x1A, 0x07}, "RAR"},
		{0, []byte{0x28, 0xB5, 0x2F, 0xFD}, "ZSTD"},
		{0, []byte{0x04, 0x22, 0x4D, 0x18}, "LZ4"},

		// Images
		{0, []byte{0xFF, 0xD8, 0xFF}, "JPEG"},
		{0, []byte{0x89, 0x50, 0x4E, 0x47}, "PNG"},
		{0, []byte{0x47, 0x49, 0x46, 0x38}, "GIF"},
		{0, []byte{0x52, 0x49, 0x46, 0x46}, "WEBP"}, // RIFF container
		{4, []byte{0x66, 0x74, 0x79, 0x70, 0x68, 0x65, 0x69, 0x63}, "HEIC"},

		// Video
		{4, []byte{0x66, 0x74, 0x79, 0x70}, "MP4"},
		{0, []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70}, "MP4_alt"},
		{0, []byte{0x1A, 0x45, 0xDF, 0xA3}, "WEBM/MKV"},
		{0, []byte{0x46, 0x4C, 0x56}, "FLV"},

		// Audio
		{0, []byte{0xFF, 0xFB}, "MP3"},
		{0, []byte{0xFF, 0xF3}, "MP3_alt"},
		{0, []byte{0xFF, 0xF2}, "MP3_alt2"},
		{0, []byte{0x49, 0x44, 0x33}, "MP3_ID3"},
		{0, []byte{0x4F, 0x67, 0x67, 0x53}, "OGG"},
		{0, []byte{0x66, 0x4C, 0x61, 0x43}, "FLAC"},

		// Documents
		{0, []byte{0x25, 0x50, 0x44, 0x46}, "PDF"},

		// Executables
		{0, []byte{0x4D, 0x5A}, "PE/EXE"},
		{0, []byte{0x7F, 0x45, 0x4C, 0x46}, "ELF"},
	}
}
