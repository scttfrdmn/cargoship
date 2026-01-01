package resume

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDetectInterruptedUpload tests detection of interrupted uploads
func TestDetectInterruptedUpload(t *testing.T) {
	// Create a test state
	testState := &UploadState{
		UploadID:  "test-detect-123",
		StartTime: time.Now(),
		LastSave:  time.Now(),
		SourceDir: "/test/source",
		Bucket:    "test-bucket",
		Prefix:    "test-prefix",
	}

	// Save the test state
	err := SaveState(testState)
	require.NoError(t, err)
	defer func() { _ = DeleteState("test-detect-123") }()

	tests := []struct {
		name      string
		sourceDir string
		bucket    string
		prefix    string
		wantFound bool
		wantErr   bool
	}{
		{
			name:      "find existing state",
			sourceDir: "/test/source",
			bucket:    "test-bucket",
			prefix:    "test-prefix",
			wantFound: true,
			wantErr:   false,
		},
		{
			name:      "no match - different source",
			sourceDir: "/different/source",
			bucket:    "test-bucket",
			prefix:    "test-prefix",
			wantFound: false,
			wantErr:   false,
		},
		{
			name:      "no match - different bucket",
			sourceDir: "/test/source",
			bucket:    "different-bucket",
			prefix:    "test-prefix",
			wantFound: false,
			wantErr:   false,
		},
		{
			name:      "no match - different prefix",
			sourceDir: "/test/source",
			bucket:    "test-bucket",
			prefix:    "different-prefix",
			wantFound: false,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := DetectInterruptedUpload(tt.sourceDir, tt.bucket, tt.prefix)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.wantFound {
				require.NotNil(t, state)
				assert.Equal(t, testState.UploadID, state.UploadID)
			} else {
				assert.Nil(t, state)
			}
		})
	}
}

// TestShouldPromptForResume tests the resume prompt logic
func TestShouldPromptForResume(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name  string
		state *UploadState
		want  bool
	}{
		{
			name:  "nil state",
			state: nil,
			want:  false,
		},
		{
			name: "completed upload",
			state: &UploadState{
				StartTime:      now.Add(-1 * time.Hour),
				TotalFiles:     100,
				CompletedFiles: 100,
				TotalBytes:     1000000,
				CompletedBytes: 1000000,
			},
			want: false,
		},
		{
			name: "very old upload (>7 days)",
			state: &UploadState{
				StartTime:      now.Add(-8 * 24 * time.Hour),
				TotalFiles:     100,
				CompletedFiles: 50,
				TotalBytes:     1000000,
				CompletedBytes: 500000,
			},
			want: false,
		},
		{
			name: "barely started (<1%)",
			state: &UploadState{
				StartTime:      now.Add(-1 * time.Hour),
				TotalFiles:     1000,
				CompletedFiles: 5,
				TotalBytes:     1000000,
				CompletedBytes: 5000, // 0.5%
			},
			want: false,
		},
		{
			name: "valid resume candidate - 50% complete",
			state: &UploadState{
				StartTime:      now.Add(-1 * time.Hour),
				TotalFiles:     100,
				CompletedFiles: 50,
				TotalBytes:     1000000,
				CompletedBytes: 500000,
			},
			want: true,
		},
		{
			name: "valid resume candidate - just over 1%",
			state: &UploadState{
				StartTime:      now.Add(-2 * time.Hour),
				TotalFiles:     100,
				CompletedFiles: 5,
				TotalBytes:     1000000,
				CompletedBytes: 15000, // 1.5%
			},
			want: true,
		},
		{
			name: "valid resume candidate - 99% complete",
			state: &UploadState{
				StartTime:      now.Add(-30 * time.Minute),
				TotalFiles:     100,
				CompletedFiles: 99,
				TotalBytes:     1000000,
				CompletedBytes: 990000,
			},
			want: true,
		},
		{
			name: "valid resume candidate - within 7 days",
			state: &UploadState{
				StartTime:      now.Add(-6 * 24 * time.Hour),
				TotalFiles:     100,
				CompletedFiles: 50,
				TotalBytes:     1000000,
				CompletedBytes: 500000,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldPromptForResume(tt.state)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestResumeDecision_String tests the String method for ResumeDecision
func TestResumeDecision_String(t *testing.T) {
	tests := []struct {
		name     string
		decision ResumeDecision
		expected string
	}{
		{"ResumeYes", ResumeYes, "Resume"},
		{"ResumeNo", ResumeNo, "Start Fresh"},
		{"ResumeValidate", ResumeValidate, "Validate & Resume"},
		{"ResumeCanceled", ResumeCanceled, "Canceled"},
		{"Unknown", ResumeDecision(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.decision.String()
			assert.Equal(t, tt.expected, got)
		})
	}
}
