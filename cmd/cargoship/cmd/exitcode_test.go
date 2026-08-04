package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// runForExitCode drives the real root command and returns the code main would
// exit with. Going through NewRootCmd rather than calling ExitCode on a
// hand-made error is the point: it pins the classification against the errors
// cobra actually produces, so a cobra upgrade that rewords one of them fails
// here instead of silently downgrading that case to ExitError.
func runForExitCode(t *testing.T, args ...string) int {
	t.Helper()
	b := bytes.NewBufferString("")
	c := NewRootCmd(b)
	c.SetArgs(args)
	c.SetOut(b)
	c.SetErr(b)
	return ExitCode(c.Execute())
}

func TestExitCode_UsageErrors(t *testing.T) {
	// Each case is a way of getting the invocation wrong. All must be
	// distinguishable from a runtime failure, which is the whole point of #401.
	tests := []struct {
		name string
		args []string
	}{
		{"unknown command", []string{"not-a-command"}},
		{"unknown flag", []string{"create", "keys", "--bogus-flag"}},
		{"unknown shorthand flag", []string{"create", "keys", "-Z"}},
		{"flag missing its value", []string{"create", "keys", "--name"}},
		{"bad flag value", []string{"create", "keys", "--name", "x", "--email", "y@z.com", "--bits", "notanint"}},
		{"missing required flag", []string{"create", "keys"}},
		{"wrong arg count", []string{"estimate"}},
		{"too few args (MinimumNArgs)", []string{"create", "upload"}},
		{"parent command without subcommand", []string{"create"}},
		{"unknown subcommand of a parent", []string{"create", "bogus-sub"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, ExitUsage, runForExitCode(t, tt.args...),
				"a malformed invocation must exit %d so scripts can tell it from a runtime failure", ExitUsage)
		})
	}
}

// TestExitCode_RuntimeFailureIsNotUsage guards the other direction: a command
// that parsed fine and then failed must NOT be reported as a usage error, or
// callers would be told to check their command line when the real problem was
// elsewhere. `create keys` with no passphrase available fails inside RunE.
func TestExitCode_RuntimeFailureIsNotUsage(t *testing.T) {
	t.Setenv(passphraseEnvVar, "")
	require.NoError(t, os.Unsetenv(passphraseEnvVar))
	code := runForExitCode(t, "create", "keys",
		"--name", "x", "--email", "y@z.com", "--bits", "1024",
		"--destination", t.TempDir())
	require.Equal(t, ExitError, code, "a command that ran and failed must exit %d, not %d", ExitError, ExitUsage)
}

func TestExitCode_SuccessIsZero(t *testing.T) {
	require.Equal(t, ExitOK, runForExitCode(t, "--version"))
	require.Equal(t, ExitOK, runForExitCode(t, "--help"))
}

// TestExitCode_NeverReturnsThree pins the actual regression: every failure mode
// used to come back as 3, which is outside the documented 0/1/2 range.
func TestExitCode_NeverReturnsThree(t *testing.T) {
	for _, args := range [][]string{
		{"not-a-command"},
		{"create"},
		{"create", "keys"},
		{"estimate"},
		{"create", "keys", "--bogus-flag"},
	} {
		code := runForExitCode(t, args...)
		require.Contains(t, []int{ExitOK, ExitError, ExitUsage}, code,
			"args %v produced undocumented exit code %d", args, code)
	}
}

func TestExitCode_NilIsZero(t *testing.T) {
	require.Equal(t, ExitOK, ExitCode(nil))
}

func TestExitCode_ErrUsageWrapping(t *testing.T) {
	// The sentinel survives wrapping, so a RunE can add context to a usage error.
	err := fmt.Errorf("resolving destination: %w", fmt.Errorf("%w: --bucket is required", ErrUsage))
	require.Equal(t, ExitUsage, ExitCode(err))
}

func TestExitCode_PlainErrorIsExitError(t *testing.T) {
	require.Equal(t, ExitError, ExitCode(errors.New("disk on fire")))
}

// TestIsSilent covers the verify path: failure is reported by exit code alone,
// because the command already printed its own diagnostics.
func TestIsSilent(t *testing.T) {
	require.True(t, IsSilent(ErrSilent))
	require.True(t, IsSilent(fmt.Errorf("wrapped: %w", ErrSilent)))
	require.False(t, IsSilent(errors.New("ordinary failure")))
	require.False(t, IsSilent(nil))

	// A silent error is still a failure, so it must not map to ExitOK.
	require.Equal(t, ExitError, ExitCode(ErrSilent))
}
