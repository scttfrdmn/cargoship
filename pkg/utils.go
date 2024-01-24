package porter

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
)

// mustGetCmd uses generics to get a given flag with the appropriate Type from a cobra.Command
func mustGetCmd[T []int | int | string | bool | time.Duration](cmd *cobra.Command, s string) T {
	switch any(new(T)).(type) {
	case *int:
		item, err := cmd.Flags().GetInt(s)
		panicIfErr(err)
		return any(item).(T)
	case *string:
		item, err := cmd.Flags().GetString(s)
		panicIfErr(err)
		return any(item).(T)
	case *bool:
		item, err := cmd.Flags().GetBool(s)
		panicIfErr(err)
		return any(item).(T)
	case *[]int:
		item, err := cmd.Flags().GetIntSlice(s)
		panicIfErr(err)
		return any(item).(T)
	case *time.Duration:
		item, err := cmd.Flags().GetDuration(s)
		panicIfErr(err)
		return any(item).(T)
	default:
		panic(fmt.Sprintf("unexpected use of mustGetCmd: %v", reflect.TypeOf(s)))
	}
}

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}

func mustExpandDir(s string) string {
	expanded, err := homedir.Expand(s)
	if err != nil {
		panic(err)
	}
	return expanded
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
