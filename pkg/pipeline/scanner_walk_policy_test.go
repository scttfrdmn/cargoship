package pipeline

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestScannerWalkError covers each branch of the walk-error policy directly.
func TestScannerWalkError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "src")
	s := &ScannerStage{config: &ScannerConfig{RootPath: root, ExcludePatterns: []string{"#recycle", "*.tmp"}}}
	dir := fakeInfo{dir: true}
	denied := fs.ErrPermission

	tests := []struct {
		name string
		path string
		info os.FileInfo
		err  error
		want error
	}{
		{"root unreadable", root, nil, denied, denied},
		{"root missing", root, nil, fs.ErrNotExist, fs.ErrNotExist},
		{"vanished file", filepath.Join(root, "a.txt"), nil, fs.ErrNotExist, nil},
		{"vanished dir", filepath.Join(root, "sub"), dir, fs.ErrNotExist, nil},
		{"excluded dir", filepath.Join(root, "#recycle"), dir, denied, filepath.SkipDir},
		{"inside excluded dir", filepath.Join(root, "#recycle", "x"), nil, denied, nil},
		{"excluded file", filepath.Join(root, "x.tmp"), nil, denied, nil},
		{"unreadable dir", filepath.Join(root, "private"), dir, denied, denied},
		{"unreadable file", filepath.Join(root, "a.txt"), nil, denied, denied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.walkError(root, tt.path, tt.info, tt.err)
			if tt.want == nil {
				assert.NoError(t, got)
				return
			}
			assert.ErrorIs(t, got, tt.want)
		})
	}
}

// fakeInfo is an os.FileInfo for the walk-error policy test.
type fakeInfo struct{ dir bool }

func (f fakeInfo) Name() string       { return "x" }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() fs.FileMode  { return map[bool]fs.FileMode{true: fs.ModeDir}[f.dir] }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.dir }
func (f fakeInfo) Sys() any           { return nil }
