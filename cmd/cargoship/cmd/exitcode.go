package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Process exit codes. These are the CLI's contract with scripts and CI (see the
// exit-code table in the command reference), so they must not drift.
const (
	// ExitOK is returned when the command succeeded.
	ExitOK = 0
	// ExitError is returned when the command ran and failed.
	ExitError = 1
	// ExitUsage is returned when the invocation itself was wrong: an unknown
	// command or flag, a bad flag value, a missing required flag, or the wrong
	// number of arguments.
	ExitUsage = 2
)

// ErrUsage marks an error as the caller's mistake rather than a runtime failure,
// so main can exit ExitUsage instead of ExitError.
//
// Wrap with it in any RunE that rejects its own arguments:
//
//	return fmt.Errorf("%w: --bucket is required", ErrUsage)
var ErrUsage = errors.New("usage error")

// ErrSilent reports failure through the exit code alone, for commands that have
// already printed their own diagnostics.
//
// It exists so such a command can return normally — letting deferred cleanup and
// PersistentPostRun run — instead of calling os.Exit mid-RunE, without main
// logging a second, redundant error line on top of the output the command
// already produced.
var ErrSilent = errors.New("")

// IsSilent reports whether an error should be surfaced by exit code only.
func IsSilent(err error) bool {
	return errors.Is(err, ErrSilent)
}

// usagePatterns are cobra's own wordings for the malformed-invocation errors
// that no hook can intercept.
//
// Matching on message text is normally a mistake, and it is confined to this one
// place for that reason. Cobra builds these with fmt.Errorf and exports no
// sentinel or type to test against, and it raises them inside Command.execute:
// argument-count validation at the ValidateArgs call and required-flag
// validation at ValidateRequiredFlags, neither of which is routed through
// SetFlagErrorFunc or any other extension point. The alternative —
// reimplementing cobra's argument handling in order to classify it — would be a
// second copy of that logic, drifting silently against the first.
//
// Flag-parse errors are deliberately absent: attachUsageClassifier tags those at
// the source via SetFlagErrorFunc, which is exact rather than textual. Listing
// them here as well would be unreachable duplication.
//
// Every pattern is pinned by a subtest in exitcode_test.go that drives a real
// command and asserts the resulting code, so a cobra rewording fails the suite
// instead of quietly reverting that case to ExitError.
var usagePatterns = []string{
	"unknown command ",  // args.go legacyArgs / NoArgs
	"required flag(s) ", // command.go ValidateRequiredFlags
	"accepts ",          // args.go ExactArgs / MaximumNArgs / RangeArgs
	"requires at least", // args.go MinimumNArgs
}

// ExitCode maps an error returned by command execution to a process exit code.
//
// Anything not recognised as a usage problem is ExitError: a command that ran
// and failed is the common case, and misreporting it as a usage error would tell
// a caller to check its command line when the real problem was elsewhere.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, ErrUsage) {
		return ExitUsage
	}
	msg := err.Error()
	for _, p := range usagePatterns {
		if strings.Contains(msg, p) {
			return ExitUsage
		}
	}
	return ExitError
}

// attachUsageClassifier marks flag-parse failures as usage errors. Cobra's
// FlagErrorFunc walks up to the parent when a command has none of its own, so
// setting it on the root covers the whole tree.
//
// This makes the classification explicit for the errors that can be hooked;
// usagePatterns covers the ones cobra raises where no hook can see them.
func attachUsageClassifier(root *cobra.Command) {
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w: %w", ErrUsage, err)
	})
}
