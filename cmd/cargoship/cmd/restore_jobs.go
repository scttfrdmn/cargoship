package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	s3pkg "github.com/scttfrdmn/cargoship/pkg/aws/s3"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/restore"
)

// newRestoreJobsCmd returns the 'restore jobs' subcommand group (Issue #202).
func newRestoreJobsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "Manage queued Glacier restore jobs",
		Long: `List, check, download, and clean restore jobs created when Glacier/Deep Archive
objects need time to be retrieved before they can be downloaded.

When 'cargoship restore' requests a Glacier restore without --wait, it saves a
job to ~/.cargoship/restore-jobs/ and prints a job ID. Use these subcommands to
track the job and trigger the download once the objects are ready.`,
	}
	cmd.AddCommand(
		newRestoreJobsListCmd(),
		newRestoreJobsCheckCmd(),
		newRestoreJobsDownloadCmd(),
		newRestoreJobsCleanCmd(),
	)
	return cmd
}

// newRestoreJobsListCmd lists all saved restore jobs.
func newRestoreJobsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all restore jobs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := restore.NewDefaultStore()
			if err != nil {
				return err
			}
			jobs, err := store.List()
			if err != nil {
				return err
			}
			if len(jobs) == 0 {
				fmt.Println("No restore jobs found.")
				return nil
			}
			fmt.Printf("%-18s  %-16s  %-10s  %s\n", "JOB ID", "STATUS", "AGE", "SOURCE")
			fmt.Println(strings.Repeat("─", 80))
			for _, j := range jobs {
				age := humanize.RelTime(j.CreatedAt, time.Now(), "ago", "from now")
				fmt.Printf("%-18s  %-16s  %-10s  %s\n", j.ID, j.Status, age, j.S3URL)
				if j.Selection.DVCStage != "" {
					fmt.Printf("    stage: %s\n", j.Selection.DVCStage)
				}
				if j.Selection.GitCommit != "" {
					fmt.Printf("    commit: %s\n", j.Selection.GitCommit)
				}
				if len(j.Selection.FilePaths) > 0 {
					fmt.Printf("    files: %s\n", strings.Join(j.Selection.FilePaths, ", "))
				}
				if j.EstimatedCostUSD > 0 {
					fmt.Printf("    est. cost: $%.4f USD\n", j.EstimatedCostUSD)
				}
				if j.Status == restore.JobStatusComplete {
					fmt.Printf("    restored: %d files, %s\n",
						j.FilesRestored, humanize.Bytes(uint64(j.BytesWritten)))
				}
			}
			return nil
		},
	}
}

// newRestoreJobsCheckCmd polls S3 and updates status for pending jobs.
func newRestoreJobsCheckCmd() *cobra.Command {
	var jobID string

	cmd := &cobra.Command{
		Use:   "check [job-id]",
		Short: "Check Glacier restore status for pending jobs",
		Long: `Poll S3 for each pending job and mark jobs as 'ready' when all their chunks
are accessible. If a job ID is given, only that job is checked.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				jobID = args[0]
			}

			store, err := restore.NewDefaultStore()
			if err != nil {
				return err
			}

			var jobs []*restore.Job
			if jobID != "" {
				j, err := store.Load(jobID)
				if err != nil {
					return err
				}
				jobs = []*restore.Job{j}
			} else {
				jobs, err = store.List()
				if err != nil {
					return err
				}
			}

			pending := make([]*restore.Job, 0)
			for _, j := range jobs {
				if j.Status == restore.JobStatusPendingGlacier {
					pending = append(pending, j)
				}
			}

			if len(pending) == 0 {
				fmt.Println("No pending Glacier restore jobs.")
				return nil
			}

			fmt.Printf("Checking %d pending job(s)…\n", len(pending))

			for _, j := range pending {
				cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(j.Region))
				if err != nil {
					fmt.Printf("  [%s] ⚠️  could not load AWS config: %v\n", j.ID, err)
					continue
				}
				s3Client := s3.NewFromConfig(cfg)
				gr := s3pkg.NewGlacierRestorer(s3Client, j.RestoreDays)

				// The job's retrieval was approved when it was saved, so a chunk
				// found frozen again (its restored copy expired) is re-requested.
				report, err := gr.CheckAndRestore(context.Background(), j.Bucket, j.ChunkKeys, s3pkg.RestoreTier(j.Tier))
				if err != nil {
					fmt.Printf("  [%s] ⚠️  check failed: %v\n", j.ID, err)
					continue
				}

				if report.AllAccessible() {
					now := time.Now()
					j.Status = restore.JobStatusReady
					j.ReadyAt = &now
					if saveErr := store.Save(j); saveErr != nil {
						fmt.Printf("  [%s] ⚠️  could not save status: %v\n", j.ID, saveErr)
					} else {
						fmt.Printf("  [%s] ✅ ready — run: cargoship restore jobs download %s\n", j.ID, j.ID)
					}
				} else {
					inProg := len(report.InProgress) + len(report.Frozen)
					fmt.Printf("  [%s] ⏳ %d chunk(s) still pending (%s tier)\n", j.ID, inProg, j.Tier)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&jobID, "job-id", "", "Check only this specific job ID")
	return cmd
}

// newRestoreJobsDownloadCmd downloads files from a ready restore job.
func newRestoreJobsDownloadCmd() *cobra.Command {
	var cacheGB int64
	var outputOverride string
	var existing existingFileFlags // #915

	cmd := &cobra.Command{
		Use:   "download <job-id>",
		Short: "Download files from a ready restore job",
		Long: `Download the files for a restore job whose Glacier restore has completed.
The job must be in 'ready' status (run 'restore jobs check' first if unsure).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobID := args[0]
			ctx := context.Background()

			store, err := restore.NewDefaultStore()
			if err != nil {
				return err
			}
			job, err := store.Load(jobID)
			if err != nil {
				return err
			}

			switch job.Status {
			case restore.JobStatusPendingGlacier:
				return fmt.Errorf("job %s is still pending Glacier restoration — run 'restore jobs check' first", jobID)
			case restore.JobStatusComplete:
				fmt.Printf("Job %s already completed (%d files, %s).\n",
					jobID, job.FilesRestored, humanize.Bytes(uint64(job.BytesWritten)))
				return nil
			case restore.JobStatusFailed:
				return fmt.Errorf("job %s previously failed: %s", jobID, job.Error)
			case restore.JobStatusDownloading:
				return fmt.Errorf("job %s appears to be downloading already (or was interrupted — delete and re-run if stuck)", jobID)
			}

			if job.OutputDir, err = jobOutputDir(job, outputOverride); err != nil {
				return err
			}

			// Mark downloading.
			job.Status = restore.JobStatusDownloading
			if err := store.Save(job); err != nil {
				return err
			}

			cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(job.Region))
			if err != nil {
				return fmt.Errorf("load AWS config: %w", err)
			}
			s3Client := s3.NewFromConfig(cfg)
			kmsClient := kms.NewFromConfig(cfg)

			bucket, actualPrefix, uploadID, err := jobUploadLocation(job)
			if err != nil {
				return err
			}

			// #917: resolve the chain like `restore` does (#552); a plain download
			// of an incremental version restored only its own changed files.
			m, err := loadEffectiveManifest(ctx, s3Client, kmsClient, bucket, actualPrefix, uploadID)
			if err != nil {
				job.Status = restore.JobStatusFailed
				job.Error = err.Error()
				_ = store.Save(job)
				return fmt.Errorf("load manifest: %w", err)
			}

			maxCacheBytes := cacheGB * 1024 * 1024 * 1024
			// SetBucket/SetPrefix: fetch from where the job's manifest was read,
			// not the location recorded in the manifest (#335, #1032).
			se := manifest.NewSelectiveExtractor(m, s3Client, maxCacheBytes).SetBucket(bucket).SetPrefix(actualPrefix).
				SetVerify(!job.NoVerify).
				SetFlatten(job.Flatten).
				SetExisting(existing.policy())

			fmt.Printf("🚀 Restoring files to %s…\n", job.OutputDir)

			var stats *manifest.RestoreStats
			sel := job.Selection

			switch {
			case sel.Hash != "":
				stats, err = se.ExtractFileByHash(ctx, sel.Hash, job.OutputDir)
			case sel.DVCStage != "":
				stats, err = se.BatchRestoreByDVCStage(ctx, sel.DVCStage, job.OutputDir)
			case sel.GitCommit != "":
				stats, err = se.BatchRestoreByCommit(ctx, sel.GitCommit, job.OutputDir)
			case len(sel.FilePaths) > 0:
				stats, err = se.BatchRestore(ctx, sel.FilePaths, job.OutputDir)
			default:
				err = fmt.Errorf("no selection criteria in job %s", jobID)
			}

			if err != nil {
				if errors.Is(err, manifest.ErrExistingFiles) {
					// #915: nothing was written; leave the job ready to retry with
					// --overwrite or --skip-existing rather than marking it failed.
					job.Status = restore.JobStatusReady
					_ = store.Save(job)
					return err
				}
				job.Status = restore.JobStatusFailed
				job.Error = err.Error()
				_ = store.Save(job)
				return fmt.Errorf("restore failed: %w", err)
			}

			now := time.Now()
			job.Status = restore.JobStatusComplete
			job.CompletedAt = &now
			job.FilesRestored = stats.Restored
			job.BytesWritten = stats.Bytes
			// #905: a job that restored some files and failed others is not complete.
			if outcome := restoreOutcomeError(stats); outcome != nil {
				job.Status = restore.JobStatusFailed
				job.Error = outcome.Error()
			}
			if err := store.Save(job); err != nil {
				fmt.Printf("⚠️  could not save final job status: %v\n", err)
			}

			// #905: the banner and exit code must agree with the counts.
			switch {
			case stats.Failed == 0:
				fmt.Printf("✅ Restore complete!\n")
			case stats.Restored == 0:
				fmt.Printf("❌ Restore failed: no files were restored\n")
			default:
				fmt.Printf("⚠️  Restore incomplete\n")
			}
			fmt.Printf("   Files restored:    %d\n", stats.Restored)
			if stats.Failed > 0 {
				fmt.Printf("   Files failed:      %d\n", stats.Failed)
			}
			fmt.Printf("   Data written:      %s\n", humanize.Bytes(uint64(stats.Bytes)))
			fmt.Printf("   Chunks downloaded: %d\n", stats.ChunksDownloaded)
			fmt.Printf("   Output directory:  %s\n", job.OutputDir)
			return restoreOutcomeError(stats)
		},
	}

	cmd.Flags().Int64Var(&cacheGB, "cache-gb", 10, "LRU chunk cache size in GB")
	cmd.Flags().StringVar(&outputOverride, "output", "", "Restore into this directory instead of the one saved with the job")
	existing.add(cmd)
	return cmd
}

// newRestoreJobsCleanCmd removes old completed/failed jobs.
func newRestoreJobsCleanCmd() *cobra.Command {
	var olderThan string

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove completed and failed restore jobs",
		Long:  `Delete completed and failed restore jobs older than the given duration (default: 24h).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			age := 24 * time.Hour
			if olderThan != "" {
				var err error
				age, err = time.ParseDuration(olderThan)
				if err != nil {
					return fmt.Errorf("invalid --older-than %q: %w", olderThan, err)
				}
			}

			store, err := restore.NewDefaultStore()
			if err != nil {
				return err
			}
			removed, err := store.CleanCompleted(age)
			if err != nil {
				return err
			}
			if removed == 0 {
				fmt.Println("No jobs to clean.")
			} else {
				fmt.Printf("Removed %d job(s).\n", removed)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&olderThan, "older-than", "24h", "Remove jobs older than this duration (e.g. 72h, 7d)")
	return cmd
}

// jobUploadLocation returns where a job's upload lives: the location the
// restore resolved (#917), or, for a job saved before that, the one parsed from
// its S3 URL.
func jobUploadLocation(job *restore.Job) (bucket, prefix, uploadID string, err error) {
	bucket, urlPrefix, err := parseS3URL(job.S3URL)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid S3 URL in job: %w", err)
	}
	if job.Bucket != "" {
		bucket = job.Bucket
	}
	if job.UploadID != "" {
		return bucket, job.Prefix, job.UploadID, nil
	}
	if idx := strings.Index(urlPrefix, "/uploads/"); idx != -1 {
		return bucket, urlPrefix[:idx], urlPrefix[idx+9:], nil
	}
	return bucket, "", urlPrefix, nil
}

// jobOutputDir is where a job's download writes: override if given, else the
// saved directory. It never resolves a relative directory against the current
// one (#917); a relative path saved by an earlier version is refused.
func jobOutputDir(job *restore.Job, override string) (string, error) {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve --output: %w", err)
		}
		return abs, nil
	}
	if !filepath.IsAbs(job.OutputDir) {
		return "", fmt.Errorf("job %s was saved with the relative output directory %q (before v0.43.0), so where it "+
			"meant is unknown; re-run with --output <dir>", job.ID, job.OutputDir)
	}
	return job.OutputDir, nil
}
