package cmd

import (
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// #724: a direct-upload dataset records no chunks. DeepVerifyResult.Passed()
// requires TotalChunks > 0 so an empty manifest cannot pass vacuously — correct in
// itself, but combined with `chunksPassed && filesPassed` it meant a chunk-less
// dataset could NEVER pass, however cleanly its files verified. The chunk phase
// has to be skipped, not failed.
func TestDeepVerifyPassed(t *testing.T) {
	tests := []struct {
		name   string
		chunks *manifest.DeepVerifyResult
		files  *manifest.FilesVerifyResult
		want   bool
	}{
		{
			name:   "direct mode: no chunks, files all verified",
			chunks: &manifest.DeepVerifyResult{TotalChunks: 0},
			files:  &manifest.FilesVerifyResult{TotalFiles: 4, OK: 4},
			want:   true,
		},
		{
			name:   "chunked: both phases clean",
			chunks: &manifest.DeepVerifyResult{TotalChunks: 6},
			files:  &manifest.FilesVerifyResult{TotalFiles: 410, CoveredByChunk: 410},
			want:   true,
		},
		{
			name:   "direct mode but a file is corrupt",
			chunks: &manifest.DeepVerifyResult{TotalChunks: 0},
			files:  &manifest.FilesVerifyResult{TotalFiles: 4, OK: 3, Mismatched: 1},
			want:   false,
		},
		{
			name:   "direct mode but a file is absent",
			chunks: &manifest.DeepVerifyResult{TotalChunks: 0},
			files:  &manifest.FilesVerifyResult{TotalFiles: 4, OK: 3, Missing: 1},
			want:   false,
		},
		{
			name:   "direct mode with no recorded checksums stays a failure",
			chunks: &manifest.DeepVerifyResult{TotalChunks: 0},
			files:  &manifest.FilesVerifyResult{TotalFiles: 4, Unverifiable: 4},
			want:   false,
		},
		{
			name:   "chunk corruption fails even when files look fine",
			chunks: &manifest.DeepVerifyResult{TotalChunks: 6, Mismatched: 1},
			files:  &manifest.FilesVerifyResult{TotalFiles: 410, CoveredByChunk: 410},
			want:   false,
		},
		{
			// The guard that must survive: nothing described, nothing proven.
			name:   "an empty manifest must not pass vacuously",
			chunks: &manifest.DeepVerifyResult{TotalChunks: 0},
			files:  &manifest.FilesVerifyResult{TotalFiles: 0},
			want:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := deepVerifyPassed(tc.chunks, tc.files); got != tc.want {
				t.Errorf("deepVerifyPassed() = %v, want %v", got, tc.want)
			}
		})
	}
}
