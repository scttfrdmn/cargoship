package resume

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// StateFileExtension is the file extension for state files
	StateFileExtension = ".json"
	// StateTempExtension is used for atomic writes
	StateTempExtension = ".json.tmp"
)

// GetStateDir returns the directory where state files are stored
// Default: ~/.cargoship/state/
func GetStateDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(homeDir, ".cargoship", "state"), nil
}

// SaveState saves the upload state atomically using temp file + rename
// This ensures the state file is never corrupted even if the process crashes
func SaveState(state *UploadState) error {
	if state == nil {
		return fmt.Errorf("state cannot be nil")
	}
	if state.UploadID == "" {
		return fmt.Errorf("upload ID cannot be empty")
	}

	// Get state directory
	stateDir, err := GetStateDir()
	if err != nil {
		return fmt.Errorf("failed to get state directory: %w", err)
	}

	// Create state directory if it doesn't exist
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Update last save time
	state.LastSave = time.Now()

	// Marshal state to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state to JSON: %w", err)
	}

	// Write to temporary file first
	tempFile := filepath.Join(stateDir, state.UploadID+StateTempExtension)
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	// Atomic rename
	finalFile := filepath.Join(stateDir, state.UploadID+StateFileExtension)
	if err := os.Rename(tempFile, finalFile); err != nil {
		// Clean up temp file on failure
		_ = os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp state file: %w", err)
	}

	return nil
}

// LoadState loads an upload state from disk
func LoadState(uploadID string) (*UploadState, error) {
	if uploadID == "" {
		return nil, fmt.Errorf("upload ID cannot be empty")
	}

	stateDir, err := GetStateDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get state directory: %w", err)
	}

	stateFile := filepath.Join(stateDir, uploadID+StateFileExtension)

	// Read state file
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("state file not found for upload ID: %s", uploadID)
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Unmarshal JSON
	var state UploadState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file (may be corrupted): %w", err)
	}

	// Validate basic fields
	if state.UploadID != uploadID {
		return nil, fmt.Errorf("upload ID mismatch: file says %s, expected %s", state.UploadID, uploadID)
	}

	return &state, nil
}

// ListStates returns all resumable upload states, sorted by start time (newest first)
func ListStates() ([]*UploadState, error) {
	stateDir, err := GetStateDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get state directory: %w", err)
	}

	// Check if directory exists
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		return []*UploadState{}, nil // No states yet
	}

	// Read directory
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read state directory: %w", err)
	}

	var states []*UploadState

	// Load each state file
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Skip temp files
		if strings.HasSuffix(entry.Name(), StateTempExtension) {
			continue
		}

		// Only process .json files
		if !strings.HasSuffix(entry.Name(), StateFileExtension) {
			continue
		}

		// Extract upload ID from filename
		uploadID := strings.TrimSuffix(entry.Name(), StateFileExtension)

		// Load state
		state, err := LoadState(uploadID)
		if err != nil {
			// Skip corrupted state files but log warning
			fmt.Fprintf(os.Stderr, "Warning: failed to load state %s: %v\n", uploadID, err)
			continue
		}

		states = append(states, state)
	}

	// Sort by start time (newest first)
	sort.Slice(states, func(i, j int) bool {
		return states[i].StartTime.After(states[j].StartTime)
	})

	return states, nil
}

// DeleteState removes a state file from disk
func DeleteState(uploadID string) error {
	if uploadID == "" {
		return fmt.Errorf("upload ID cannot be empty")
	}

	stateDir, err := GetStateDir()
	if err != nil {
		return fmt.Errorf("failed to get state directory: %w", err)
	}

	stateFile := filepath.Join(stateDir, uploadID+StateFileExtension)

	// Remove state file
	if err := os.Remove(stateFile); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete state file: %w", err)
	}

	return nil
}

// CleanupOldStates removes state files older than the specified duration
func CleanupOldStates(maxAge time.Duration) (int, error) {
	states, err := ListStates()
	if err != nil {
		return 0, fmt.Errorf("failed to list states: %w", err)
	}

	cutoffTime := time.Now().Add(-maxAge)
	deletedCount := 0

	for _, state := range states {
		// Delete if older than cutoff OR already complete
		if state.StartTime.Before(cutoffTime) || state.IsComplete() {
			if err := DeleteState(state.UploadID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to delete old state %s: %v\n", state.UploadID, err)
				continue
			}
			deletedCount++
		}
	}

	return deletedCount, nil
}

// StateExists checks if a state file exists for the given upload ID
func StateExists(uploadID string) bool {
	if uploadID == "" {
		return false
	}

	stateDir, err := GetStateDir()
	if err != nil {
		return false
	}

	stateFile := filepath.Join(stateDir, uploadID+StateFileExtension)
	_, err = os.Stat(stateFile)
	return err == nil
}

// FindStateBySource finds a state file matching the source directory and destination
// Returns nil if no matching state is found
func FindStateBySource(sourceDir, bucket, prefix string) (*UploadState, error) {
	states, err := ListStates()
	if err != nil {
		return nil, err
	}

	// Normalize paths for comparison
	sourceDir = filepath.Clean(sourceDir)

	for _, state := range states {
		stateSrc := filepath.Clean(state.SourceDir)
		if stateSrc == sourceDir && state.Bucket == bucket && state.Prefix == prefix {
			return state, nil
		}
	}

	return nil, nil // No match found
}
