package chunking

import (
	"fmt"
	"io"
)

// ArchivePadder adds zero-byte padding to archives to reach target sizes.
// This ensures uniform compressed chunk sizes for optimal S3 load balancing.
type ArchivePadder struct {
	useLowEntropyPadding bool // If true, use zero bytes (default); if false, use random data
}

// NewArchivePadder creates a new archive padder with default settings
func NewArchivePadder() *ArchivePadder {
	return &ArchivePadder{
		useLowEntropyPadding: true, // Default to zero-byte padding (S3-optimized)
	}
}

// NewArchivePadderWithConfig creates a padder with custom configuration
func NewArchivePadderWithConfig(useLowEntropyPadding bool) *ArchivePadder {
	return &ArchivePadder{
		useLowEntropyPadding: useLowEntropyPadding,
	}
}

// PadToTarget pads the writer to reach the target size
// Returns the number of padding bytes written
func (ap *ArchivePadder) PadToTarget(w io.Writer, currentSize int64, targetSize int64) (int64, error) {
	if currentSize >= targetSize {
		// Already at or above target, no padding needed
		return 0, nil
	}

	paddingNeeded := targetSize - currentSize
	if paddingNeeded < 0 {
		return 0, fmt.Errorf("padding would be negative: current=%d, target=%d", currentSize, targetSize)
	}

	if ap.useLowEntropyPadding {
		return ap.writeLowEntropyPadding(w, paddingNeeded)
	}

	return ap.writeHighEntropyPadding(w, paddingNeeded)
}

// writeLowEntropyPadding writes zero bytes (highly compressible, S3-optimized)
func (ap *ArchivePadder) writeLowEntropyPadding(w io.Writer, size int64) (int64, error) {
	// Write in chunks to avoid allocating huge buffers
	const chunkSize = 1024 * 1024 // 1MB chunks
	zeros := make([]byte, chunkSize)

	totalWritten := int64(0)
	remaining := size

	for remaining > 0 {
		toWrite := remaining
		if toWrite > chunkSize {
			toWrite = chunkSize
		}

		n, err := w.Write(zeros[:toWrite])
		totalWritten += int64(n)
		remaining -= int64(n)

		if err != nil {
			return totalWritten, fmt.Errorf("failed to write padding: %w", err)
		}
	}

	return totalWritten, nil
}

// writeHighEntropyPadding writes pseudo-random data (less compressible)
// Note: This is intentionally simple - not cryptographically secure
func (ap *ArchivePadder) writeHighEntropyPadding(w io.Writer, size int64) (int64, error) {
	// For high-entropy padding, we use a simple pattern that's less compressible
	// Using a deterministic pattern for reproducibility
	const chunkSize = 1024 * 1024 // 1MB chunks
	pattern := make([]byte, chunkSize)

	// Create a pattern that's reasonably random-looking
	for i := range pattern {
		pattern[i] = byte((i * 7) ^ (i >> 3) ^ (i >> 11))
	}

	totalWritten := int64(0)
	remaining := size

	for remaining > 0 {
		toWrite := remaining
		if toWrite > chunkSize {
			toWrite = chunkSize
		}

		n, err := w.Write(pattern[:toWrite])
		totalWritten += int64(n)
		remaining -= int64(n)

		if err != nil {
			return totalWritten, fmt.Errorf("failed to write padding: %w", err)
		}
	}

	return totalWritten, nil
}

// CalculatePaddingSize calculates how much padding is needed
func (ap *ArchivePadder) CalculatePaddingSize(currentSize int64, targetSize int64) int64 {
	if currentSize >= targetSize {
		return 0
	}
	return targetSize - currentSize
}

// PaddingInfo contains information about padding operation
type PaddingInfo struct {
	OriginalSize     int64   // Size before padding
	TargetSize       int64   // Desired size after padding
	PaddingBytes     int64   // Bytes of padding added
	FinalSize        int64   // Actual size after padding
	PaddingRatio     float64 // Percentage of padding (0.0-1.0)
	LowEntropyPadding bool   // Whether zero-byte padding was used
}

// PadToTargetWithInfo pads to target and returns detailed information
func (ap *ArchivePadder) PadToTargetWithInfo(w io.Writer, currentSize int64, targetSize int64) (*PaddingInfo, error) {
	paddingWritten, err := ap.PadToTarget(w, currentSize, targetSize)
	if err != nil {
		return nil, err
	}

	finalSize := currentSize + paddingWritten
	paddingRatio := float64(paddingWritten) / float64(finalSize)

	return &PaddingInfo{
		OriginalSize:      currentSize,
		TargetSize:        targetSize,
		PaddingBytes:      paddingWritten,
		FinalSize:         finalSize,
		PaddingRatio:      paddingRatio,
		LowEntropyPadding: ap.useLowEntropyPadding,
	}, nil
}

// ValidatePaddingRatio checks if padding ratio is within acceptable limits
// Returns true if padding is reasonable, false if too much padding
func ValidatePaddingRatio(info *PaddingInfo, maxRatio float64) (bool, string) {
	if info.PaddingRatio > maxRatio {
		return false, fmt.Sprintf(
			"Padding ratio %.2f%% exceeds maximum %.2f%%. "+
				"Original: %d bytes, Padding: %d bytes (%.2f%% overhead)",
			info.PaddingRatio*100, maxRatio*100,
			info.OriginalSize, info.PaddingBytes, info.PaddingRatio*100,
		)
	}

	return true, fmt.Sprintf(
		"Padding ratio %.2f%% is acceptable (max: %.2f%%). "+
			"Added %d bytes padding to %d bytes original.",
		info.PaddingRatio*100, maxRatio*100,
		info.PaddingBytes, info.OriginalSize,
	)
}
