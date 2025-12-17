package pipeline

import (
	"testing"
)

// TestArchiverStage_Name tests the Name method
func TestArchiverStage_Name(t *testing.T) {
	stage := &ArchiverStage{}
	name := stage.Name()
	if name != "archiver" {
		t.Errorf("Expected name 'archiver', got '%s'", name)
	}
}

// TestArchiverStage_GetCompressionStats tests the GetCompressionStats method
func TestArchiverStage_GetCompressionStats(t *testing.T) {
	stage := &ArchiverStage{}
	filesSkipped, timeSaved := stage.GetCompressionStats()

	// Should return zero values for new stage
	if filesSkipped != 0 {
		t.Errorf("Expected filesSkipped=0, got %d", filesSkipped)
	}
	if timeSaved != 0 {
		t.Errorf("Expected timeSaved=0, got %v", timeSaved)
	}
}

// TestArchiverStage_GetPaddingStats tests the GetPaddingStats method
func TestArchiverStage_GetPaddingStats(t *testing.T) {
	stage := &ArchiverStage{}
	paddingBytes := stage.GetPaddingStats()

	// Should return zero for new stage
	if paddingBytes != 0 {
		t.Errorf("Expected paddingBytes=0, got %d", paddingBytes)
	}
}

// TestArchiverStage_Stop tests the Stop method
func TestArchiverStage_Stop(t *testing.T) {
	config := &ArchiverConfig{
		Workers: 2,
	}

	input := make(chan *Job)
	output := make(chan *Job)

	stage, err := NewArchiverStage(config, input, output)
	if err != nil {
		t.Fatalf("Failed to create archiver stage: %v", err)
	}

	// Close channels to allow clean shutdown
	close(input)

	// Stop should not error
	if err := stage.Stop(); err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}

	// Calling Stop again should be safe (idempotent)
	if err := stage.Stop(); err != nil {
		t.Errorf("Second Stop() returned error: %v", err)
	}
}

// TestEncoderPool_Close tests the Close method
func TestEncoderPool_Close(t *testing.T) {
	pool, err := NewEncoderPool(2)
	if err != nil {
		t.Fatalf("Failed to create encoder pool: %v", err)
	}

	// Close should not error
	if err := pool.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Calling Close again should be safe
	if err := pool.Close(); err != nil {
		t.Errorf("Second Close() returned error: %v", err)
	}
}
