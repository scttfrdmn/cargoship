//go:build !unix

package pipeline

import "testing"

// makeFIFO skips the test: this platform has no FIFOs.
func makeFIFO(t *testing.T, _ string) {
	t.Helper()
	t.Skip("FIFOs are not supported on this platform")
}
