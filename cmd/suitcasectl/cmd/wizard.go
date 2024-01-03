package cmd

import (
	"context"
	"strings"
	"time"

	"github.com/drewstinnett/gout/v2"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	porter "gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/travelagent"
)

// NewWizardCmd creates a new 'find' command
func NewWizardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:      "wizard",
		Short:    "Run a console wizard to do the creation",
		Long:     `This is for users who want a simple command to do some basic stuff. For advanced usage, use 'create suitcase'`,
		Aliases:  []string{"wiz", "easybutton"},
		PreRunE:  wizardPreRunE,
		PostRunE: wizardPostRunE,
		RunE: func(cmd *cobra.Command, args []string) error {
			gout.SetWriter(cmd.OutOrStdout())

			p := mustPorterWithCmd(cmd)
			if err := p.RunForm(); err != nil {
				return err
			}
			if err := p.SendUpdate(travelagent.StatusUpdate{
				SuitcasectlSource:      strings.Join(args, ", "),
				Status:                 travelagent.StatusInProgress,
				StartedAt:              nowPtr(),
				SuitcasectlDestination: p.Destination,
				Metadata:               p.Inventory.MustJSONString(),
				MetadataCheckSum:       p.InventoryHash,
			}); err != nil {
				log.Warn().Err(err).Msg("🧳 error sending status update")
			}

			if err := createSuitcases(p); err != nil {
				log.Warn().Err(err).Msg("🧳 failed to completed createSuitcases")
				return err
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringArray("inventory-directory", []string{"."}, "Directory containing inventories to search. Can be specified multiple times for multiple directories.")
	return cmd
}

func wizardPreRunE(cmd *cobra.Command, args []string) error {
	// Get this first, it'll be important
	globalPersistentPreRun(cmd, args)

	opts := []porter.Option{
		porter.WithLogger(&log.Logger),
		porter.WithHashAlgorithm(hashAlgo),
		porter.WithVersion(version),
		porter.WithCLIMeta(
			porter.NewCLIMeta(
				porter.WithStart(toPTR(time.Now())),
			),
		),
	}

	// Shove porter in to the cmd context so we can use it later
	cmd.SetContext(context.WithValue(cmd.Context(), porter.PorterKey, porter.New(opts...)))

	return nil

	// return setupMultiLoggingWithCmd(cmd)
}

func wizardPostRunE(cmd *cobra.Command, args []string) error {
	ptr := mustPorterWithCmd(cmd)
	metaF, err := ptr.CLIMeta.Complete(ptr.Destination)
	if err != nil {
		return err
	}
	log.Debug().Str("file", metaF).Msg("🧳 Created meta file")

	// Hash the outer items if asked
	var hashes []config.HashSet
	var hashFn, hashFnBin string
	if hashes, hashFn, hashFnBin, err = setOuterHashes(ptr, metaF); err != nil {
		return err
	}

	log.Debug().Str("log-file", ptr.LogFile.Name()).Msg("🧳 Switching back to stderr logger and closing the multi log writer so we can hash it")
	setupLogging(cmd.OutOrStderr())
	// Do we really care if this closes? maybe...
	_ = ptr.LogFile.Close()

	log.Info().
		Str("runtime", ptr.CLIMeta.CompletedAt.Sub(*ptr.CLIMeta.StartedAt).String()).
		Time("start", *ptr.CLIMeta.StartedAt).
		Time("end", *ptr.CLIMeta.CompletedAt).
		Msg("🧳 Completed")

	// opts := suitcase.OptsWithCmd(cmd)
	// Copy files up if needed
	mfiles := appendHashes([]string{
		"inventory.yaml",
		"suitcasectl.log",
		"suitcasectl-invocation-meta.yaml",
	}, hashFn, hashFnBin)
	if ptr.Inventory.Options.TransportPlugin != nil {
		ptr.ShipItems(mfiles, ptr.InventoryHash)
	}

	if err := uploadMeta(ptr, mfiles); err != nil {
		return err
	}

	if serr := ptr.SendUpdate(travelagent.StatusUpdate{
		Status:      travelagent.StatusComplete,
		CompletedAt: nowPtr(),
		SizeBytes:   ptr.TotalTransferred,
	}); serr != nil {
		log.Warn().Err(serr).Msg("🧳 failed to send final status update")
	}

	gout.MustPrint(runsum{
		Destination: ptr.SuitcaseOpts.Destination,
		Suitcases:   ptr.Inventory.UniqueSuitcaseNames(),
		Directories: ptr.Inventory.Options.Directories,
		MetaFiles:   mfiles,
		Hashes:      hashes,
	})
	globalPersistentPostRun(cmd, args)
	return nil
}
