package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExitCode_Pending pins the #979 contract: a pending restore exits 75,
// distinct from success, failure and usage errors, and documented as such.
func TestExitCode_Pending(t *testing.T) {
	require.Equal(t, 75, ExitPending, "docs/reference/exit-codes.md documents 75")
	assert.Equal(t, ExitPending, ExitCode(ErrPending))
	assert.Equal(t, ExitPending, ExitCode(fmt.Errorf("%w%w", ErrSilent, ErrPending)))
	assert.Equal(t, ExitPending, ExitCode(fmt.Errorf("job x: %w", ErrPending)))
	assert.NotContains(t, []int{ExitOK, ExitError, ExitUsage}, ExitPending)
}

func TestRestorePendingOutcome(t *testing.T) {
	p := pendingStatus{JobID: "job-1", ChunksPending: 3, ChunksRequested: 2, Tier: "bulk", RetrievalCostUSD: 0.25}

	t.Run("job saved: pending, silent", func(t *testing.T) {
		var err error
		out := captureStdout(t, func() { err = restorePendingOutcome(false, p, nil) })
		assert.Equal(t, ExitPending, ExitCode(err))
		assert.True(t, IsSilent(err), "the status is already printed")
		assert.Contains(t, out, "3 chunk(s) are not yet accessible; nothing was restored")
	})

	t.Run("json", func(t *testing.T) {
		var err error
		out := captureStdout(t, func() { err = restorePendingOutcome(true, p, nil) })
		assert.Equal(t, ExitPending, ExitCode(err))
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &got), "stdout must be the JSON object alone: %q", out)
		assert.Equal(t, "pending", got["status"])
		assert.Equal(t, "job-1", got["job_id"])
		assert.EqualValues(t, 3, got["chunks_pending"])
		assert.EqualValues(t, 2, got["chunks_requested"])
		assert.Equal(t, "bulk", got["tier"])
		assert.NotContains(t, got, "error")
	})

	t.Run("job not saved: a failure", func(t *testing.T) {
		var err error
		out := captureStdout(t, func() { err = restorePendingOutcome(true, pendingStatus{ChunksPending: 1}, errors.New("disk full")) })
		assert.Equal(t, ExitError, ExitCode(err), "with no job saved, nothing records the restore")
		assert.Contains(t, err.Error(), "disk full")
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out)), &got))
		assert.Equal(t, "pending", got["status"])
		assert.Contains(t, got["error"], "could not save a restore job")
	})
}
