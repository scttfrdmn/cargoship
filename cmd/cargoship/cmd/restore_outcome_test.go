package cmd

import (
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestRestoreOutcomeError pins the exit-code contract for restore (#336).
//
// BatchRestore is deliberately partially tolerant: it counts a per-file failure
// and continues rather than returning an error, so `err == nil` even when
// nothing was restored. Before this, the command printed "✅ Restore complete!"
// with "Files failed: 2" and exited 0 — which any script reads as success.
func TestRestoreOutcomeError(t *testing.T) {
	tests := []struct {
		name    string
		stats   manifest.RestoreStats
		wantErr bool
		wantMsg string
	}{
		{
			name:    "all restored",
			stats:   manifest.RestoreStats{Restored: 3},
			wantErr: false,
		},
		{
			name:    "nothing requested",
			stats:   manifest.RestoreStats{},
			wantErr: false,
		},
		{
			// The case that motivated the fix: a total failure must not exit 0.
			name:    "total failure",
			stats:   manifest.RestoreStats{Restored: 0, Failed: 2},
			wantErr: true,
			wantMsg: "restore failed: 0 of 2 file(s) restored",
		},
		{
			// A partial restore is still an error — the user asked for files they
			// did not get, and only they can decide whether that is acceptable.
			name:    "partial failure",
			stats:   manifest.RestoreStats{Restored: 1, Failed: 1},
			wantErr: true,
			wantMsg: "restore incomplete: 1 of 2 file(s) failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := restoreOutcomeError(&tt.stats)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("restored=%d failed=%d: got nil error, want non-nil — the exit code must reflect the failure",
						tt.stats.Restored, tt.stats.Failed)
				}
				if err.Error() != tt.wantMsg {
					t.Errorf("message mismatch:\n got %q\nwant %q", err.Error(), tt.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("restored=%d failed=%d: got error %v, want nil",
					tt.stats.Restored, tt.stats.Failed, err)
			}
		})
	}
}
