package cmd

import (

	// nolint:gosec
	// nolint:gosec
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"reflect"
	"strings"
	"time"

	// "github.com/minio/sha256-simd"

	porter "github.com/scttfrdmn/cargoship/pkg"
	"github.com/scttfrdmn/cargoship/pkg/config"
	"github.com/scttfrdmn/cargoship/pkg/travelagent"
	"github.com/spf13/cobra"
)

// newOutDirWithCmd generates a new output directory using cobra.Command options
func newOutDirWithCmd(cmd *cobra.Command) (string, error) {
	o, err := getDestinationWithCobra(cmd)
	if err != nil {
		return "", err
	}
	if o == "" {
		o, err = os.MkdirTemp("", "suitcasectl-")
		if err != nil {
			return "", err
		}
	}

	// Also shove this in to porter. We'll use it later there.
	if ptr, err := porterWithCmd(cmd); err == nil {
		ptr.Destination = o
	}
	return o, nil
}

func dclose(c io.Closer) {
	err := c.Close()
	if err != nil {
		logger.Warn("error closing item", "error", err)
	}
}

// This needs some work. Why do we need it's own context entry instead of just
// using the SuitcaseOpts, which already has it
func getDestinationWithCobra(cmd *cobra.Command) (string, error) {
	d, err := getCmd[string](cmd, "destination")
	if err != nil {
		return "", err
	}
	if d == "" {
		var err error
		if d, err = os.MkdirTemp("", "suitcasectl"); err != nil {
			return "", err
		}
	}

	// Set for later use
	ptr, err := porterWithCmd(cmd)
	if err == nil {
		ptr.Destination = d
	}
	logPath := path.Join(d, "suitcasectl.log")
	lf, err := os.Create(logPath) // nolint:gosec
	if err != nil {
		return "", err
	}

	// If we have a porter, set the logfile here
	if ptr, err := porterWithCmd(cmd); err == nil {
		ptr.LogFile = lf
	}

	return d, nil
}

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

// getSha256 Get sha256 hash from a file
func getSha256(file string) (string, error) {
	f, err := os.Open(file) // nolint:gosec
	if err != nil {
		return "", err
	}
	defer dclose(f)

	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(b)

	return fmt.Sprintf("%x", sum), nil
}

func hasDuplicates(strArr []string) bool {
	seen := make(map[string]bool)
	for _, str := range strArr {
		if seen[str] {
			return true
		}
		seen[str] = true
	}
	return false
}

func porterWithCmd(cmd *cobra.Command) (*porter.Porter, error) {
	p, ok := cmd.Context().Value(porter.PorterKey).(*porter.Porter)
	if !ok {
		return nil, errors.New("could not find Porter in this context")
	}

	return p, nil
}

func porterTravelAgentWithCmd(cmd *cobra.Command, args []string) (*porter.Porter, bool, error) {
	p, err := porterWithCmd(cmd)
	if err != nil {
		return nil, false, err
	}
	retryCount, err := getCmd[int](cmd, "retry-count")
	if err != nil {
		return nil, false, err
	}
	retryInterval, err := getCmd[time.Duration](cmd, "retry-interval")
	if err != nil {
		return nil, false, err
	}
	taOpts := []travelagent.Option{
		travelagent.WithCmd(cmd),
		travelagent.WithUploadRetries(retryCount),
		travelagent.WithUploadRetryTime(retryInterval),
	}
	if Verbose {
		taOpts = append(taOpts, travelagent.WithPrintCurl())
	}
	var terr error
	var ta *travelagent.TravelAgent
	if ta, terr = travelagent.New(taOpts...); terr != nil {
		logger.Debug("no valid travel agent found", "error", terr)
	} else {
		p.SetTravelAgent(ta)
		logger.Info("☀️ Thanks for using a TravelAgent! Check out this URL for full info on your suitcases fun travel", "url", p.TravelAgent.StatusURL())
		if serr := p.SendUpdate(travelagent.StatusUpdate{
			Status: travelagent.StatusPending,
		}); serr != nil {
			return nil, false, serr
		}
	}
	// Get option bits
	inventoryFile, onlyInventory, err := inventoryOptsWithCobra(cmd, args)
	if err != nil {
		return nil, false, err
	}

	// Create an inventory file if one isn't specified
	if p.Inventory, err = p.CreateOrReadInventory(inventoryFile); err != nil {
		return nil, onlyInventory, err
	}
	// Replace the travel agent with one that knows the inventory hash
	// This doesn't work yet, need to find out why
	if ta != nil {
		ta.UniquePrefix = p.InventoryHash
		p.SetTravelAgent(ta)
	}

	// We need options even if we already have the inventory
	p.SuitcaseOpts = &config.SuitCaseOpts{
		// Destination:  p.Destination,
		EncryptInner: p.Inventory.Options.EncryptInner,
		HashInner:    p.Inventory.Options.HashInner,
		Format:       p.Inventory.Options.SuitcaseFormat,
	}

	return p, onlyInventory, nil
}

// validateCmdArgs ensures we are passing valid arguments in
func validateCmdArgs(inventoryFile string, onlyInventory bool, cmd cobra.Command, args []string) error {
	// Figure out if we are using an inventory file, or creating one
	if inventoryFile != "" && len(args) > 0 {
		return errors.New("error: You can't specify an inventory file and target dir arguments at the same time")
	}

	// Make sure we are actually using either an inventory file or target dirs
	if inventoryFile == "" && len(args) == 0 {
		return errors.New("error: You must specify an inventory file or target dirs")
	}

	if onlyInventory && inventoryFile != "" {
		return errors.New("you can't specify an inventory file and only-inventory at the same time")
	}

	if hasDuplicates(args) {
		return errors.New("duplicate path found in arguments")
	}

	prefix, err := getCmd[string](&cmd, "prefix")
	if err != nil {
		return err
	}
	if strings.Contains(prefix, "/") {
		return errors.New("prefix cannot contain a /")
	}

	return nil
}

func uint64ToInt64(u uint64) (int64, error) {
	if u > math.MaxInt64 {
		return 0, errors.New("value out of range for int64")
	}
	return int64(u), nil
}
