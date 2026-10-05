//go:build integration && !unix

package cmd

import "testing"

func integrationAvailableDiskBytes(t *testing.T, _ string) uint64 {
	t.Helper()
	t.Skip("disk-space integration test requires Unix filesystem statistics")
	return 0
}
