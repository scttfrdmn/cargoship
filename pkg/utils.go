package porter

import (
	"errors"
	"fmt"
	"os"
	"path"
	"reflect"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/scttfrdmn/cargoship/pkg/config"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
	"github.com/scttfrdmn/cargoship/pkg/suitcase"
	"github.com/spf13/cobra"
)

// getCmd uses generics to get a given flag with the appropriate Type from a cobra.Command
func getCmd[T []int | int | string | bool | time.Duration](cmd *cobra.Command, s string) (T, error) {
	switch any(new(T)).(type) {
	case *int:
		item, err := cmd.Flags().GetInt(s)
		if err != nil {
			var zero T
			return zero, err
		}
		return any(item).(T), nil
	case *string:
		item, err := cmd.Flags().GetString(s)
		if err != nil {
			var zero T
			return zero, err
		}
		return any(item).(T), nil
	case *bool:
		item, err := cmd.Flags().GetBool(s)
		if err != nil {
			var zero T
			return zero, err
		}
		return any(item).(T), nil
	case *[]int:
		item, err := cmd.Flags().GetIntSlice(s)
		if err != nil {
			var zero T
			return zero, err
		}
		return any(item).(T), nil
	case *time.Duration:
		item, err := cmd.Flags().GetDuration(s)
		if err != nil {
			var zero T
			return zero, err
		}
		return any(item).(T), nil
	default:
		var zero T
		return zero, fmt.Errorf("unexpected use of getCmd: %v", reflect.TypeOf(s))
	}
}

func expandDir(s string) (string, error) {
	expanded, err := homedir.Expand(s)
	if err != nil {
		return "", fmt.Errorf("failed to expand directory %s: %w", s, err)
	}
	return expanded, nil
}

func validateIsDir(s string) error {
	if s == "" {
		return errors.New("directory cannot be blank")
	}
	expanded, err := homedir.Expand(s)
	if err != nil {
		return err
	}
	st, err := os.Stat(expanded)
	if err != nil {
		return fmt.Errorf("could not stat %v, got error: %v", expanded, err)
	}
	if !st.IsDir() {
		return errors.New("this must be a directory, not a file")
	}
	return nil
}

func inProcessName(s string) string {
	return path.Join(path.Dir(s), fmt.Sprintf(".__creating-%v", path.Base(s)))
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func hashInner(targetFn string, ha inventory.HashAlgorithm, hashes []config.HashSet) error {
	hashF, err := os.Create(fmt.Sprintf("%v.%v", targetFn, ha)) // nolint:gosec
	if err != nil {
		return err
	}
	defer dclose(hashF)
	if err := suitcase.WriteHashFile(hashes, hashF); err != nil {
		return err
	}
	return nil
}

func int64ToUint64(i int64) uint64 {
	if i < 0 {
		// Return 0 for negative values instead of panicking
		// This is defensive - callers should validate before calling
		return 0
	}
	return uint64(i)
}

func intToUint64(i int) uint64 {
	if i < 0 {
		// Return 0 for negative values instead of panicking
		// This is defensive - callers should validate before calling
		return 0
	}
	return uint64(i)
}
