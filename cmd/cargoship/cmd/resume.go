package cmd

import (
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/resume"
)

// NewResumeCmd creates the 'resume' command for managing resumable uploads
func NewResumeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resume [upload-id]",
		Short: "Manage resumable uploads",
		Long: `Manage resumable uploads including listing, resuming, and cleaning up old states.

The resume command allows you to:
- List all resumable uploads with their progress
- Resume a specific interrupted upload by ID
- Clean up old state files to free disk space

Examples:
  # List all resumable uploads
  cargoship resume list

  # Resume a specific upload
  cargoship resume 20250115-143052-a3b4c5d6

  # Clean up state files older than 24 hours
  cargoship resume clean --older-than 24h

  # Clean up all completed uploads
  cargoship resume clean --completed

State files are stored in ~/.cargoship/state/
Each state file contains upload progress, configuration, and file hashes.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}

			// Direct resume by upload ID
			uploadID := args[0]
			fmt.Printf("🔄 Resuming upload: %s\n\n", uploadID)

			// Load state
			state, err := resume.LoadState(uploadID)
			if err != nil {
				return fmt.Errorf("failed to load upload state: %w", err)
			}

			// Display upload info
			displayUploadInfo(state)

			// TODO: Actually resume the upload by calling pipeline
			// This requires refactoring upload command to support resume
			fmt.Println("\n⚠️  Direct resume not yet implemented")
			fmt.Println("💡 Use: cargoship upload <path> <destination> --resume --upload-id", uploadID)

			return nil
		},
	}

	// Add subcommands
	cmd.AddCommand(newResumeListCmd())
	cmd.AddCommand(newResumeCleanCmd())

	return cmd
}

// newResumeListCmd creates the 'resume list' subcommand
func newResumeListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all resumable uploads",
		Long: `List all resumable uploads with their current progress.

Displays:
- Upload ID and status
- Source directory and S3 destination
- Progress (files and bytes completed)
- Time since upload started
- Estimated completion (if applicable)

State files are read from ~/.cargoship/state/
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			states, err := resume.ListStates()
			if err != nil {
				return fmt.Errorf("failed to list upload states: %w", err)
			}

			if len(states) == 0 {
				fmt.Println("No resumable uploads found")
				fmt.Println("\nState files are stored in ~/.cargoship/state/")
				return nil
			}

			fmt.Printf("Found %d resumable upload(s):\n\n", len(states))

			for i, state := range states {
				fmt.Printf("[%d] %s\n", i+1, state.UploadID)
				displayUploadInfo(state)
				fmt.Println()
			}

			return nil
		},
	}

	return cmd
}

// newResumeCleanCmd creates the 'resume clean' subcommand
func newResumeCleanCmd() *cobra.Command {
	var (
		olderThan     string
		cleanCompleted bool
	)

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean up old or completed upload states",
		Long: `Remove old state files to free up disk space.

By default, cleans up state files older than 24 hours.
Use --completed to remove only fully completed uploads.
Use --older-than to specify a custom age threshold.

Examples:
  # Clean states older than 24 hours (default)
  cargoship resume clean

  # Clean states older than 1 week
  cargoship resume clean --older-than 168h

  # Clean only completed uploads
  cargoship resume clean --completed

State files are stored in ~/.cargoship/state/
Each file is typically 10-50 KB.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var maxAge time.Duration
			var err error

			if olderThan != "" {
				maxAge, err = time.ParseDuration(olderThan)
				if err != nil {
					return fmt.Errorf("invalid duration format: %w (example: 24h, 7d, 168h)", err)
				}
			} else {
				maxAge = 24 * time.Hour // Default: 24 hours
			}

			fmt.Printf("🧹 Cleaning up upload states...\n")
			if cleanCompleted {
				fmt.Printf("   Removing: Completed uploads\n")
			} else {
				fmt.Printf("   Removing: States older than %s\n", maxAge)
			}
			fmt.Println()

			if cleanCompleted {
				// Clean only completed uploads
				count, err := cleanCompletedStates()
				if err != nil {
					return fmt.Errorf("failed to clean completed states: %w", err)
				}
				fmt.Printf("✅ Removed %d completed upload state(s)\n", count)
			} else {
				// Clean by age
				count, err := resume.CleanupOldStates(maxAge)
				if err != nil {
					return fmt.Errorf("failed to clean old states: %w", err)
				}
				fmt.Printf("✅ Removed %d old upload state(s)\n", count)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&olderThan, "older-than", "", "Clean states older than duration (e.g., 24h, 7d, 168h)")
	cmd.Flags().BoolVar(&cleanCompleted, "completed", false, "Clean only completed uploads")

	return cmd
}

// displayUploadInfo displays formatted information about an upload state
func displayUploadInfo(state *resume.UploadState) {
	// Progress percentage
	progress := state.Progress()
	progressBar := makeProgressBar(progress, 40)

	fmt.Printf("   📂 Source:       %s\n", state.SourceDir)
	fmt.Printf("   🪣 Destination:  s3://%s/%s\n", state.Bucket, state.Prefix)
	if state.Region != "" {
		fmt.Printf("   🌍 Region:       %s\n", state.Region)
	}
	fmt.Printf("   📊 Progress:     %s (%.1f%%)\n", progressBar, progress)

	if state.TotalFiles > 0 {
		fmt.Printf("   📄 Files:        %d / %d completed\n",
			state.CompletedFiles, state.TotalFiles)
	}
	if state.TotalBytes > 0 {
		fmt.Printf("   💾 Data:         %s / %s\n",
			humanize.Bytes(uint64(state.CompletedBytes)),
			humanize.Bytes(uint64(state.TotalBytes)))
	}

	fmt.Printf("   ⏰ Started:      %s (%s ago)\n",
		state.StartTime.Format("2006-01-02 15:04:05"),
		humanize.Time(state.StartTime))

	fmt.Printf("   💾 Last saved:   %s ago\n",
		humanize.Time(state.LastSave))

	// Configuration details
	if state.StorageClass != "" {
		fmt.Printf("   📦 Storage:      %s\n", state.StorageClass)
	}
	if state.KMSKeyID != "" {
		fmt.Printf("   🔐 Encryption:   KMS (key: %s...)\n", state.KMSKeyID[:20])
	}
	if state.ShardCount > 0 {
		fmt.Printf("   🎯 Shards:       %d\n", state.ShardCount)
	}

	// Status
	if state.IsComplete() {
		fmt.Printf("   ✅ Status:       Complete\n")
	} else {
		fmt.Printf("   🔄 Status:       In Progress\n")
	}
}

// makeProgressBar creates a text-based progress bar
func makeProgressBar(percent float64, width int) string {
	filled := int(percent * float64(width) / 100)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	bar := "["
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "="
		} else if i == filled {
			bar += ">"
		} else {
			bar += " "
		}
	}
	bar += "]"

	return bar
}

// cleanCompletedStates removes state files for completed uploads
func cleanCompletedStates() (int, error) {
	states, err := resume.ListStates()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, state := range states {
		if state.IsComplete() {
			if err := resume.DeleteState(state.UploadID); err != nil {
				fmt.Printf("⚠️  Warning: Failed to delete state %s: %v\n", state.UploadID, err)
				continue
			}
			count++
		}
	}

	return count, nil
}
