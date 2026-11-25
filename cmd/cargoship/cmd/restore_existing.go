package cmd

import (
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// existingFileFlags holds a restore command's --overwrite / --skip-existing.
type existingFileFlags struct {
	overwrite bool
	skip      bool
}

// add registers the flags on cmd as mutually exclusive.
func (f *existingFileFlags) add(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.overwrite, "overwrite", false, "Replace files that already exist at the destination (by default the restore is refused)")
	cmd.Flags().BoolVar(&f.skip, "skip-existing", false, "Keep files that already exist at the destination and restore the rest")
	cmd.MarkFlagsMutuallyExclusive("overwrite", "skip-existing")
}

// policy is the restore's existing-file policy (#915): refuse unless told.
func (f *existingFileFlags) policy() manifest.ExistingFilePolicy {
	switch {
	case f.overwrite:
		return manifest.ExistingOverwrite
	case f.skip:
		return manifest.ExistingSkip
	default:
		return manifest.ExistingRefuse
	}
}
