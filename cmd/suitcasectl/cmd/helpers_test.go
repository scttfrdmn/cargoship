package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkGetSha256(b *testing.B) {
	tf := os.Getenv("BENCHMARK_SHA256_FILE")
	if tf == "" {
		tf = "../../../pkg/testdata/fakey-private.key"
	}
	for _, ha := range []string{"sha256", "sha512", "md5", "sha1"} {
		ha := ha
		b.Run(fmt.Sprintf("suitcase_%v_%v", ha, tf), func(b *testing.B) {
			tfh, err := os.Open(tf)
			require.NoError(b, err)
			defer dclose(tfh)
			got := calculateHash(tfh, ha)
			require.NotEmpty(b, got)
		})
	}
}

func TestGetSha256(t *testing.T) {
	got, err := getSha256("../../../pkg/testdata/fakey-private.key")
	require.NoError(t, err)
	require.Equal(t, "07a115fe11a34882f3b54bee5a5ac2262405e33bf066b8d70b281fa2b4c01edb", got)
}

func TestCalculateHash(t *testing.T) {
	tests := map[string]string{
		"md5":    "efef19d2f597c0cf55cc0cd7f746d594",
		"sha1":   "9b71a8bf3b141240155dc285a37c08f0465d0bf1",
		"sha256": "07a115fe11a34882f3b54bee5a5ac2262405e33bf066b8d70b281fa2b4c01edb",
		"sha512": "26aba150bbf61007372092e7509b8f33d7bc45cdcfb5296bc7cb2cbfb9cd3dc26b3d9313541bec7cd78b74f14adc68c446c935313c38dbbdec93a914a21657dd",
	}
	for hash, want := range tests {
		fh, err := os.Open("../../../pkg/testdata/fakey-private.key")
		require.NoError(t, err)
		require.Equal(t, want, calculateHash(fh, hash))
		// Make sure bad hashes fail
		require.Panics(t, func() { calculateHash(fh, "non-existent-hash") })
		err = fh.Close()
		require.NoError(t, err)
	}
}

func BenchmarkCalculateHashes(b *testing.B) {
	datasets := map[string]struct {
		path string
	}{
		"672M-american-gut": {
			path: "American-Gut",
		},
		"3.3G-Synthetic-cell-images": {
			path: "BBBC005_v1_images",
		},
	}
	bdd := os.Getenv("BENCHMARK_DATA_DIR")
	if bdd == "" {
		bdd = "../../../benchmark_data/"
	}

	for desc, dataset := range datasets {
		location := path.Join(bdd, dataset.path)
		if _, err := os.Stat(location); err == nil {
			for _, format := range []string{"md5", "sha1", "sha256", "sha512"} {
				format := format
				location := location
				b.Run(fmt.Sprintf("suitcase_calculate_hashes_%v_%v", format, desc), func(b *testing.B) {
					err = filepath.Walk(location+"/", func(path string, info fs.FileInfo, err error) error {
						if info.IsDir() {
							return filepath.SkipDir
						}
						fh, oerr := os.Open(location)
						require.NoError(b, oerr)
						defer fh.Close()
						calculateHash(fh, format)
						return nil
					})
				})
			}
		}
	}
}
