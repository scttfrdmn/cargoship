package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	porter "gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg"
)

func BenchmarkGetSha256(b *testing.B) {
	tf := os.Getenv("BENCHMARK_SHA256_FILE")
	if tf == "" {
		tf = "../../../pkg/testdata/fakey-private.key"
	}
	for _, ha := range []string{"sha256", "sha512", "md5", "sha1"} {
		b.Run(fmt.Sprintf("suitcase_%v_%v", ha, tf), func(b *testing.B) {
			tfh, err := os.Open(tf)
			require.NoError(b, err)
			defer dclose(tfh)
			got := porter.MustCalculateHash(tfh, ha)
			require.NotEmpty(b, got)
		})
	}
}

func TestGetSha256(t *testing.T) {
	got, err := getSha256("../../../pkg/testdata/fakey-private.key")
	require.NoError(t, err)
	require.Equal(t, "07a115fe11a34882f3b54bee5a5ac2262405e33bf066b8d70b281fa2b4c01edb", got)
}

func BenchmarkCalculateHashes(b *testing.B) {
	bdd := os.Getenv("BENCHMARK_DATA_DIR")
	if bdd == "" {
		bdd = "../../../benchmark_data/"
	}

	var allFiles []string
	err := filepath.Walk(bdd, func(p string, info os.FileInfo, _ error) error {
		if !info.IsDir() {
			allFiles = append(allFiles, p)
		}
		return nil
	})
	require.NoError(b, err)

	if _, err := os.Stat(bdd); err == nil {
		for _, format := range []string{"md5", "sha1", "sha256", "sha512"} {
			b.Run(fmt.Sprintf("suitcase_calculate_hashes_%v", format), func(b *testing.B) {
				for _, location := range allFiles {
					fh, oerr := os.Open(location)
					require.NoError(b, oerr)
					defer fh.Close()
					porter.CalculateHash(fh, format)
				}
			})
		}
	} else {
		logger.Warn("error encountered while benchmarking", "error", err)
	}
}
