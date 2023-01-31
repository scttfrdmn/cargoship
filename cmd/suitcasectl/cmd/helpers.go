package cmd

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// newOutDirWithCmd generates a new output directory using cobra.Command options
func newOutDirWithCmd(cmd *cobra.Command) (string, error) {
	o, err := getDestination(cmd)
	if err != nil {
		return "", err
	}
	if o == "" {
		o, err = os.MkdirTemp("", "suitcasectl-")
		if err != nil {
			return "", err
		}
	}
	return o, nil
}

func dclose(c io.Closer) {
	err := c.Close()
	if err != nil {
		log.Warn().Err(err).Send()
	}
}

func getDestination(cmd *cobra.Command) (string, error) {
	d, err := cmd.Flags().GetString("destination")
	if err != nil {
		return "", err
	}
	if d != "" {
		return d, nil
	}

	// Fall back to output-dir for now
	o, oerr := cmd.Flags().GetString("output-dir")
	if oerr != nil {
		return "", nil
	}
	if o != "" {
		return o, nil
	}
	return "", nil
}

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
	case *time.Time:
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
