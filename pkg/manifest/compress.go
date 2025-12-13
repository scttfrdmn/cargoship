package manifest

import (
	"bytes"
	"compress/gzip"
	"fmt"
)

const (
	// CompressionThreshold is the size threshold (in bytes) for automatic compression
	// Manifests larger than 10MB will be automatically compressed (Issue #92)
	CompressionThreshold = 10 * 1024 * 1024 // 10MB

	// GzipMagicNumber is the magic number for gzip-compressed data
	GzipMagicNumber1 byte = 0x1f
	GzipMagicNumber2 byte = 0x8b
)

// ToJSONAuto serializes the manifest to JSON with automatic compression (Issue #92)
// Compresses if uncompressed size exceeds CompressionThreshold (10MB)
func (m *Manifest) ToJSONAuto() (data []byte, compressed bool, err error) {
	// First serialize to JSON
	jsonData, err := m.ToJSON()
	if err != nil {
		return nil, false, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Check if compression is beneficial
	if len(jsonData) > CompressionThreshold {
		// Compress the data
		compressedData, err := m.ToJSONCompressed()
		if err != nil {
			return nil, false, fmt.Errorf("failed to compress JSON: %w", err)
		}
		return compressedData, true, nil
	}

	// Return uncompressed data
	return jsonData, false, nil
}

// FromJSONAuto deserializes a manifest from JSON with automatic decompression (Issue #92)
// Automatically detects gzip compression and decompresses if needed
func FromJSONAuto(data []byte) (*Manifest, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	// Check if data is gzip-compressed by looking for gzip magic number
	if len(data) >= 2 && data[0] == GzipMagicNumber1 && data[1] == GzipMagicNumber2 {
		// Data is compressed, use compressed deserialization
		return FromJSONCompressed(data)
	}

	// Data is uncompressed, use regular deserialization
	return FromJSON(data)
}

// EstimateCompressedSize estimates the compressed size of a manifest (Issue #92)
// This is useful for understanding compression ratios before actually compressing
func (m *Manifest) EstimateCompressedSize() (uncompressed, compressed int64, ratio float64, err error) {
	// Serialize to JSON
	jsonData, err := m.ToJSON()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	uncompressed = int64(len(jsonData))

	// Compress the data
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)

	if _, err := gzipWriter.Write(jsonData); err != nil {
		return 0, 0, 0, fmt.Errorf("failed to compress JSON: %w", err)
	}

	if err := gzipWriter.Close(); err != nil {
		return 0, 0, 0, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	compressed = int64(buf.Len())
	ratio = float64(compressed) / float64(uncompressed)

	return uncompressed, compressed, ratio, nil
}

// ShouldCompress returns true if the manifest should be compressed based on size (Issue #92)
func (m *Manifest) ShouldCompress() (bool, error) {
	jsonData, err := m.ToJSON()
	if err != nil {
		return false, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return len(jsonData) > CompressionThreshold, nil
}
