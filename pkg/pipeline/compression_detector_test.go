package pipeline

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShouldCompress_GenomicsExtensionsAreFrameless guards #511: already-compressed
// genomics formats must be recognized by extension so they are stored frameless
// (plain .tar), not wrapped in a pointless zstd frame.
func TestShouldCompress_GenomicsExtensionsAreFrameless(t *testing.T) {
	d := NewCompressionDetector()
	for _, name := range []string{"sample.cram", "reads.bam", "calls.bcf"} {
		should, reason := d.ShouldCompress(name)
		assert.False(t, should, "%s must not be compressed", name)
		assert.Contains(t, reason, "already_compressed_extension", "%s", name)
	}
	// A plainly compressible file is still compressed.
	should, _ := d.ShouldCompress("notes.txt")
	assert.True(t, should)
}

// TestShouldCompress_CRAMMagic covers a .cram renamed to an unknown extension:
// the CRAM magic bytes still route it frameless.
func TestShouldCompress_CRAMMagic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mystery.xyz")
	body := append([]byte("CRAM"), make([]byte, 1024)...) // CRAM magic + padding
	require.NoError(t, os.WriteFile(p, body, 0o600))

	should, reason := NewCompressionDetector().ShouldCompress(p)
	assert.False(t, should)
	assert.Contains(t, reason, "already_compressed_magic:CRAM")
}

// TestShouldCompress_LargeIncompressibleProbe guards #511 ask 2: a large file with
// an unknown extension and no magic match, but incompressible content, is probed
// and stored frameless; a compressible one of the same shape is still compressed;
// a SMALL incompressible one is below the probe threshold and left compressible.
func TestShouldCompress_LargeIncompressibleProbe(t *testing.T) {
	dir := t.TempDir()

	// 9 MiB of cryptographically random bytes → incompressible, and ≥ probeMinSize.
	incompressible := filepath.Join(dir, "big.xyz")
	buf := make([]byte, 9<<20)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(incompressible, buf, 0o600))

	should, reason := NewCompressionDetector().ShouldCompress(incompressible)
	assert.False(t, should, "a large incompressible unknown-ext file must be frameless")
	assert.Equal(t, "incompressible_probe", reason)

	// 9 MiB of zeros → highly compressible.
	compressible := filepath.Join(dir, "zeros.xyz")
	require.NoError(t, os.WriteFile(compressible, make([]byte, 9<<20), 0o600))
	should, _ = NewCompressionDetector().ShouldCompress(compressible)
	assert.True(t, should, "a large compressible unknown-ext file must still be compressed")

	// Small incompressible file: below the probe threshold, left as compressible
	// (a small file never creates the giant frame the probe guards against).
	small := filepath.Join(dir, "small.xyz")
	sbuf := make([]byte, 64<<10)
	_, err = rand.Read(sbuf)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(small, sbuf, 0o600))
	should, reason = NewCompressionDetector().ShouldCompress(small)
	assert.True(t, should, "a small file is not probed")
	assert.Equal(t, "compressible", reason)
}
