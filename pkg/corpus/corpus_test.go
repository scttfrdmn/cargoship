package corpus

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProfilesReproducible is the core guarantee: planting a profile twice
// yields byte-identical content (same set of relpath→sha256), so a published
// benchmark number can be independently reproduced.
func TestProfilesReproducible(t *testing.T) {
	for _, p := range Profiles() {
		t.Run(p.Name, func(t *testing.T) {
			a, err := p.Plant(t.TempDir())
			require.NoError(t, err)
			b, err := p.Plant(t.TempDir())
			require.NoError(t, err)
			require.NotEmpty(t, a)

			sums := func(fs []File) map[string]string {
				m := make(map[string]string, len(fs))
				for _, f := range fs {
					m[f.RelPath] = f.Sum
				}
				return m
			}
			assert.Equal(t, sums(a), sums(b), "profile %s must be byte-reproducible", p.Name)

			// Basenames must be unique (basename-flattened restore).
			bases := map[string]bool{}
			for _, f := range a {
				assert.False(t, bases[f.Base], "duplicate basename %q in profile %s", f.Base, p.Name)
				bases[f.Base] = true
			}
		})
	}
}

// TestPlantedFilesExistOnDisk confirms the records match what's written.
func TestPlantedFilesExistOnDisk(t *testing.T) {
	p, ok := ProfileByName("mixed")
	require.True(t, ok)
	root := t.TempDir()
	files, err := p.Plant(root)
	require.NoError(t, err)

	idx, err := IndexByBase(root)
	require.NoError(t, err)
	for _, f := range files {
		path, found := idx[f.Base]
		require.True(t, found, "planted file %s missing from disk", f.Base)
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, int64(f.Size), info.Size(), "size mismatch for %s", f.Base)
	}
	assert.Equal(t, len(files), len(idx))
	_ = filepath.Separator
}

// TestProfileByNameUnknown returns not-found for an unknown profile.
func TestProfileByNameUnknown(t *testing.T) {
	_, ok := ProfileByName("does-not-exist")
	assert.False(t, ok)
}
