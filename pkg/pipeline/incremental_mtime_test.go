package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestShouldUpload_SameSizeEditUsesModTime pins #1002. With no content hash in
// the previous manifest (always the case, #987), the same size counted as
// unchanged and the modification time was never compared, so an in-place edit
// that kept the length (enabled=true -> enabled=fals) was not uploaded and the
// new version claimed the old bytes. The rule is sync's (#918): any
// modification-time difference is a change, allowing for a coarser
// filesystem's time precision.
func TestShouldUpload_SameSizeEditUsesModTime(t *testing.T) {
	recorded := time.Date(2026, 11, 1, 10, 30, 15, 123456789, time.UTC)
	tests := []struct {
		name  string
		mtime time.Time
		want  bool
	}{
		{"same-size edit, newer mtime", recorded.Add(time.Minute), true},
		{"same-size older copy (cp -p, snapshot restore)", recorded.Add(-time.Hour), true},
		{"unchanged", recorded, false},
		{"restored onto a 1s-precision filesystem", recorded.Truncate(time.Second), false},
		{"restored onto FAT (2s)", recorded.Truncate(2 * time.Second), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "config.ini")
			require.NoError(t, os.WriteFile(p, []byte("enabled=fals"), 0o600))
			require.NoError(t, os.Chtimes(p, tt.mtime, tt.mtime))

			s, err := NewIncrementalScanner(&manifest.Manifest{SourcePath: dir, Files: []manifest.FileEntry{
				{Path: p, Size: int64(len("enabled=true")), ModTime: recorded},
			}}, "")
			require.NoError(t, err)
			assert.Equal(t, tt.want, s.ShouldUpload(p, "config.ini"))
		})
	}
}
