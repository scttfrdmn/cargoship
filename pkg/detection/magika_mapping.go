// Package detection provides Magika content type mapping to compression types (Issue #30)
package detection

import (
	"github.com/scttfrdmn/cargoship/pkg/compression"
)

// MagikaToCompressionType maps Magika content type labels to CargoShip compression types
var MagikaToCompressionType = map[string]compression.ContentType{
	// Text formats - best compression
	"txt":      compression.ContentTypeText,
	"markdown": compression.ContentTypeText,
	"csv":      compression.ContentTypeText,
	"tsv":      compression.ContentTypeText,
	"log":      compression.ContentTypeText,
	"ini":      compression.ContentTypeText,
	"conf":     compression.ContentTypeText,
	"toml":     compression.ContentTypeText,

	// Code formats - best compression
	"python":     compression.ContentTypeCode,
	"javascript": compression.ContentTypeCode,
	"typescript": compression.ContentTypeCode,
	"go":         compression.ContentTypeCode,
	"rust":       compression.ContentTypeCode,
	"c":          compression.ContentTypeCode,
	"cpp":        compression.ContentTypeCode,
	"java":       compression.ContentTypeCode,
	"kotlin":     compression.ContentTypeCode,
	"swift":      compression.ContentTypeCode,
	"ruby":       compression.ContentTypeCode,
	"php":        compression.ContentTypeCode,
	"perl":       compression.ContentTypeCode,
	"shell":      compression.ContentTypeCode,
	"bash":       compression.ContentTypeCode,
	"powershell": compression.ContentTypeCode,
	"sql":        compression.ContentTypeCode,
	"json":       compression.ContentTypeCode,
	"xml":        compression.ContentTypeCode,
	"yaml":       compression.ContentTypeCode,
	"html":       compression.ContentTypeCode,
	"css":        compression.ContentTypeCode,
	"scss":       compression.ContentTypeCode,
	"sass":       compression.ContentTypeCode,
	"less":       compression.ContentTypeCode,
	"jsx":        compression.ContentTypeCode,
	"tsx":        compression.ContentTypeCode,
	"vue":        compression.ContentTypeCode,
	"svelte":     compression.ContentTypeCode,

	// Document formats - good compression
	"pdf":  compression.ContentTypeDocument,
	"docx": compression.ContentTypeDocument,
	"doc":  compression.ContentTypeDocument,
	"xlsx": compression.ContentTypeDocument,
	"xls":  compression.ContentTypeDocument,
	"pptx": compression.ContentTypeDocument,
	"ppt":  compression.ContentTypeDocument,
	"odt":  compression.ContentTypeDocument,
	"ods":  compression.ContentTypeDocument,
	"odp":  compression.ContentTypeDocument,
	"rtf":  compression.ContentTypeDocument,
	"epub": compression.ContentTypeDocument,

	// Image formats - already compressed, minimal benefit
	"jpeg": compression.ContentTypeImage,
	"jpg":  compression.ContentTypeImage,
	"png":  compression.ContentTypeImage,
	"gif":  compression.ContentTypeImage,
	"webp": compression.ContentTypeImage,
	"svg":  compression.ContentTypeImage,
	"bmp":  compression.ContentTypeImage,
	"tiff": compression.ContentTypeImage,
	"ico":  compression.ContentTypeImage,
	"heic": compression.ContentTypeImage,
	"heif": compression.ContentTypeImage,
	"avif": compression.ContentTypeImage,
	"jxl":  compression.ContentTypeImage,
	"raw":  compression.ContentTypeImage,
	"cr2":  compression.ContentTypeImage,
	"nef":  compression.ContentTypeImage,
	"arw":  compression.ContentTypeImage,

	// Video formats - already compressed, skip compression
	"mp4":  compression.ContentTypeVideo,
	"avi":  compression.ContentTypeVideo,
	"mkv":  compression.ContentTypeVideo,
	"mov":  compression.ContentTypeVideo,
	"webm": compression.ContentTypeVideo,
	"flv":  compression.ContentTypeVideo,
	"wmv":  compression.ContentTypeVideo,
	"m4v":  compression.ContentTypeVideo,
	"mpg":  compression.ContentTypeVideo,
	"mpeg": compression.ContentTypeVideo,
	"3gp":  compression.ContentTypeVideo,
	"ogv":  compression.ContentTypeVideo,

	// Audio formats - already compressed, skip compression
	"mp3":  compression.ContentTypeAudio,
	"flac": compression.ContentTypeAudio,
	"wav":  compression.ContentTypeAudio,
	"ogg":  compression.ContentTypeAudio,
	"m4a":  compression.ContentTypeAudio,
	"aac":  compression.ContentTypeAudio,
	"wma":  compression.ContentTypeAudio,
	"opus": compression.ContentTypeAudio,
	"ape":  compression.ContentTypeAudio,
	"aiff": compression.ContentTypeAudio,

	// Archive formats - already compressed, skip re-compression
	"zip":   compression.ContentTypeArchive,
	"gzip":  compression.ContentTypeArchive,
	"tar":   compression.ContentTypeArchive,
	"7zip":  compression.ContentTypeArchive,
	"7z":    compression.ContentTypeArchive,
	"rar":   compression.ContentTypeArchive,
	"zstd":  compression.ContentTypeArchive,
	"bzip2": compression.ContentTypeArchive,
	"xz":    compression.ContentTypeArchive,
	"lz4":   compression.ContentTypeArchive,
	"lzma":  compression.ContentTypeArchive,
	"cab":   compression.ContentTypeArchive,
	"iso":   compression.ContentTypeArchive,
	"dmg":   compression.ContentTypeArchive,

	// Binary formats - fast compression
	"elf":           compression.ContentTypeBinary,
	"pe":            compression.ContentTypeBinary,
	"macho":         compression.ContentTypeBinary,
	"java_class":    compression.ContentTypeBinary,
	"pyc":           compression.ContentTypeBinary,
	"wasm":          compression.ContentTypeBinary,
	"dll":           compression.ContentTypeBinary,
	"so":            compression.ContentTypeBinary,
	"dylib":         compression.ContentTypeBinary,
	"exe":           compression.ContentTypeBinary,
	"app":           compression.ContentTypeBinary,
	"obj":           compression.ContentTypeBinary,
	"o":             compression.ContentTypeBinary,
	"a":             compression.ContentTypeBinary,
	"lib":           compression.ContentTypeBinary,
	"firmware":      compression.ContentTypeBinary,
	"rom":           compression.ContentTypeBinary,
	"bootloader":    compression.ContentTypeBinary,
	"kernel_module": compression.ContentTypeBinary,

	// Database formats - mixed compression benefit
	"sqlite":   compression.ContentTypeBinary,
	"db":       compression.ContentTypeBinary,
	"mdb":      compression.ContentTypeBinary,
	"accdb":    compression.ContentTypeBinary,
	"dbf":      compression.ContentTypeBinary,
	"ldf":      compression.ContentTypeBinary,
	"mdf":      compression.ContentTypeBinary,
	"postgres": compression.ContentTypeBinary,
	"mysql":    compression.ContentTypeBinary,

	// Font formats
	"ttf":   compression.ContentTypeBinary,
	"otf":   compression.ContentTypeBinary,
	"woff":  compression.ContentTypeArchive, // Already compressed
	"woff2": compression.ContentTypeArchive, // Already compressed
	"eot":   compression.ContentTypeBinary,

	// CAD/Design formats
	"dwg":    compression.ContentTypeBinary,
	"dxf":    compression.ContentTypeText, // Text-based CAD format
	"blend":  compression.ContentTypeBinary,
	"fbx":    compression.ContentTypeBinary,
	"obj_3d": compression.ContentTypeText, // Wavefront OBJ is text
	"stl":    compression.ContentTypeBinary,
	"3ds":    compression.ContentTypeBinary,
	"max":    compression.ContentTypeBinary,
	"skp":    compression.ContentTypeBinary,
	"revit":  compression.ContentTypeBinary,

	// Crypto/Security formats
	"certificate": compression.ContentTypeText, // PEM format
	"private_key": compression.ContentTypeText, // PEM format
	"public_key":  compression.ContentTypeText, // PEM format
	"keystore":    compression.ContentTypeBinary,
	"pkcs12":      compression.ContentTypeBinary,
	"gpg":         compression.ContentTypeBinary,
	"pgp":         compression.ContentTypeBinary,

	// Email formats
	"eml":  compression.ContentTypeText,
	"msg":  compression.ContentTypeBinary,
	"mbox": compression.ContentTypeText,
	"pst":  compression.ContentTypeBinary,
	"ost":  compression.ContentTypeBinary,

	// Generic binary
	"binary":  compression.ContentTypeBinary,
	"data":    compression.ContentTypeBinary,
	"unknown": compression.ContentTypeUnknown,
}

// MapMagikaToCompression maps a Magika content type label to a compression content type
// Falls back to ContentTypeUnknown if no mapping exists
func MapMagikaToCompression(magikaLabel string) compression.ContentType {
	if contentType, exists := MagikaToCompressionType[magikaLabel]; exists {
		return contentType
	}
	return compression.ContentTypeUnknown
}

// GetMappingStats returns statistics about the mapping table
func GetMappingStats() map[string]int {
	counts := map[string]int{
		"total":    len(MagikaToCompressionType),
		"text":     0,
		"code":     0,
		"document": 0,
		"image":    0,
		"video":    0,
		"audio":    0,
		"archive":  0,
		"binary":   0,
		"unknown":  0,
	}

	for _, contentType := range MagikaToCompressionType {
		switch contentType {
		case compression.ContentTypeText:
			counts["text"]++
		case compression.ContentTypeCode:
			counts["code"]++
		case compression.ContentTypeDocument:
			counts["document"]++
		case compression.ContentTypeImage:
			counts["image"]++
		case compression.ContentTypeVideo:
			counts["video"]++
		case compression.ContentTypeAudio:
			counts["audio"]++
		case compression.ContentTypeArchive:
			counts["archive"]++
		case compression.ContentTypeBinary:
			counts["binary"]++
		case compression.ContentTypeUnknown:
			counts["unknown"]++
		}
	}

	return counts
}
