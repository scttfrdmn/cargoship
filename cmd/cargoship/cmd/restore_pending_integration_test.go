//go:build integration

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pendingExitCode is the documented exit code for "retrieval requested, not
// restored yet" (docs/reference/exit-codes.md).
const pendingExitCode = 75

// lastJSONLine returns the last line of out that parses as a JSON object.
func lastJSONLine(t *testing.T, out string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var obj map[string]any
		if json.Unmarshal([]byte(lines[i]), &obj) == nil {
			return obj
		}
	}
	t.Fatalf("no JSON object in output:\n%s", out)
	return nil
}

// TestRestoreCmd_PendingGlacierExitsNonZero pins #979: with chunks still in
// Glacier and no --wait, restore requested the retrieval, saved a job and
// exited 0 with nothing restored, so `restore ... && rm -rf ./local` would
// delete the only copy. It now exits with the distinct pending code, and
// --json reports a pending status object.
func TestRestoreCmd_PendingGlacierExitsNonZero(t *testing.T) {
	for _, jsonOut := range []bool{false, true} {
		name := "text"
		if jsonOut {
			name = "json"
		}
		t.Run(name, func(t *testing.T) {
			s3URL, filePath := seedFrozenArchive(t)
			proxy := newGlacierProxy(t)
			t.Setenv("AWS_ENDPOINT_URL", proxy.url)
			t.Setenv("AWS_ACCESS_KEY_ID", "test")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

			dest := t.TempDir()
			args := []string{s3URL, dest, "--region", testRegion, "--file", filePath}
			if jsonOut {
				args = append(args, "--json")
			}
			cmd := NewRestoreCmd()
			var errBuf bytes.Buffer
			cmd.SetOut(&errBuf)
			cmd.SetErr(&errBuf)
			cmd.SetArgs(args)
			var err error
			out := captureStdout(t, func() { err = cmd.Execute() })

			assert.Equal(t, int64(1), proxy.restores.Load(), "the frozen chunk is requested")
			require.Equal(t, pendingExitCode, ExitCode(err),
				"a restore that only requested a retrieval must not exit 0 (err: %v)", err)
			entries, rerr := os.ReadDir(dest)
			require.NoError(t, rerr)
			assert.Empty(t, entries, "nothing is restored while the chunks are frozen")

			if jsonOut {
				obj := lastJSONLine(t, out)
				assert.Equal(t, "pending", obj["status"], "json: %v", obj)
				assert.NotEmpty(t, obj["job_id"], "json: %v", obj)
				assert.EqualValues(t, 1, obj["chunks_pending"], "json: %v", obj)
			}
		})
	}
}
