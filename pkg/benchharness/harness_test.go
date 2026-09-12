package benchharness

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/pricingfallback"
	"github.com/scttfrdmn/cargoship/pkg/corpus"
)

func sha256hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestVerifyByteIdentical(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) corpus.File {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), b, 0o600))
		return corpus.File{RelPath: name, Base: name, Size: len(b), Sum: sha256hex(b)}
	}
	files := []corpus.File{
		write("a.txt", []byte("hello")),
		write("b.txt", []byte("world")),
	}

	t.Run("all match", func(t *testing.T) {
		ok, err := verifyByteIdentical(files, dir)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("content mismatch", func(t *testing.T) {
		bad := append([]corpus.File(nil), files...)
		bad[0].Sum = sha256hex([]byte("HELLO")) // right file, wrong expected bytes
		ok, err := verifyByteIdentical(bad, dir)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("missing file", func(t *testing.T) {
		bad := append([]corpus.File(nil), files...)
		bad = append(bad, corpus.File{RelPath: "c.txt", Base: "c.txt", Sum: sha256hex([]byte("x"))})
		ok, err := verifyByteIdentical(bad, dir)
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

func TestMBPerSec(t *testing.T) {
	assert.Equal(t, 0.0, mbPerSec(1<<20, 0), "zero duration is guarded")
	assert.Equal(t, 0.0, mbPerSec(1<<20, -time.Second), "negative duration is guarded")
	// 10 MiB in 1s == 10 MB/s (MiB-based, matching the harness).
	assert.InDelta(t, 10.0, mbPerSec(10*(1<<20), time.Second), 1e-9)
}

func TestComputeCost(t *testing.T) {
	const std = config.StorageClassStandard
	upload := map[string]int{"PutObject": 1, "CreateMultipartUpload": 1, "UploadPart": 5, "CompleteMultipartUpload": 1}
	restore := map[string]int{"GetObject": 3}
	const storedBytes = 25 * 1024 * 1024

	got := computeCost(storedBytes, upload, restore)

	// Upload requests: 8 PUT-tier ops, priced from the canonical fallback table.
	wantUpload := (8.0 / 1000.0) * pricingfallback.RequestPrice("PUT", std)
	assert.InDelta(t, wantUpload, got.UploadRequestsUSD, 1e-12)

	// Restore requests: 3 GET-tier ops.
	wantRestoreReq := (3.0 / 1000.0) * pricingfallback.RequestPrice("GET", std)
	assert.InDelta(t, wantRestoreReq, got.RestoreRequestsUSD, 1e-12)

	// Monthly storage from stored (compressed) bytes.
	storedGB := float64(storedBytes) / (1024 * 1024 * 1024)
	wantStorage := storedGB * pricingfallback.StoragePrice(std)
	assert.InDelta(t, wantStorage, got.MonthlyStorageUSD, 1e-12)

	// Egress at $0.09/GB on the bytes leaving S3 (stored/compressed bytes).
	wantEgress := storedGB * 0.09
	assert.InDelta(t, wantEgress, got.RestoreEgressUSD, 1e-12)

	// #451: upload data transfer (ingress) is never billed — only requests appear.
	assert.Positive(t, got.UploadRequestsUSD)
	assert.Positive(t, got.MonthlyStorageUSD)
}
