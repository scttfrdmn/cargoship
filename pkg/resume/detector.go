package resume

import (
	"fmt"
	"path/filepath"
)

// DetectInterruptedUpload checks if there's a resumable upload for the given parameters
// Returns the state if found, nil if not found
func DetectInterruptedUpload(sourceDir, bucket, prefix string) (*UploadState, error) {
	// Normalize source directory path
	sourceDir = filepath.Clean(sourceDir)

	// Search for matching state
	state, err := FindStateBySource(sourceDir, bucket, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to search for interrupted upload: %w", err)
	}

	// No match found
	if state == nil {
		return nil, nil
	}

	// Found a match - return it
	return state, nil
}

// ShouldPromptForResume determines if we should prompt the user to resume
// Returns true if:
// - An interrupted upload is found
// - The upload is not complete
// - The upload is recent (not too old)
func ShouldPromptForResume(state *UploadState) bool {
	if state == nil {
		return false
	}

	// Don't prompt for completed uploads
	if state.IsComplete() {
		return false
	}

	// Don't prompt for very old uploads (>7 days)
	maxAge := 7 * 24 * 60 * 60 // 7 days in seconds
	if state.Age().Seconds() > float64(maxAge) {
		return false
	}

	// Don't prompt if upload has barely started (<1%)
	if state.Progress() < 1.0 {
		return false
	}

	return true
}

// ResumeDecision represents the user's choice about resuming
type ResumeDecision int

const (
	ResumeYes       ResumeDecision = iota // Resume the upload
	ResumeNo                              // Start fresh upload
	ResumeValidate                        // Validate files first, then resume
	ResumeCanceled                        // Cancel the operation
)

// String returns a human-readable representation of the decision
func (d ResumeDecision) String() string {
	switch d {
	case ResumeYes:
		return "Resume"
	case ResumeNo:
		return "Start Fresh"
	case ResumeValidate:
		return "Validate & Resume"
	case ResumeCanceled:
		return "Canceled"
	default:
		return "Unknown"
	}
}
