package cmd

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkGetSha256(b *testing.B) {
	tf := os.Getenv("BENCHMARK_SHA256_FILE")
	if tf == "" {
		tf = "../../../pkg/testdata/fakey-private.key"
	}
	b.Run(fmt.Sprintf("suitcase_sha256_%v", tf), func(b *testing.B) {
		got := mustGetSha256(tf)
		require.NotEmpty(b, got)
	})
}

func TestGetSha256(t *testing.T) {
	got, err := getSha256("../../../pkg/testdata/fakey-private.key")
	require.NoError(t, err)
	require.Equal(t, "07a115fe11a34882f3b54bee5a5ac2262405e33bf066b8d70b281fa2b4c01edb", got)
}
