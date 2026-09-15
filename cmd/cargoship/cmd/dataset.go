package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// NewDatasetCmd creates the 'dataset' command group for inspecting the version
// history that incremental `sync` builds (Issue #521). A dataset is a chain of
// manifests linked by PreviousManifestID; DatasetID names the chain (see
// manifest.DatasetIDOf), and each upload is one version.
func NewDatasetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dataset",
		Short: "Inspect dataset versions (incremental sync history)",
		Long: `Inspect the version history that incremental sync builds under an S3 prefix.

A "dataset" is a chain of manifests linked by their previous-manifest reference;
each 'cargoship sync' adds a version. These commands are read-only.`,
	}
	cmd.AddCommand(newDatasetListCmd(), newDatasetVersionsCmd(), newDatasetDiffCmd(), newDatasetPruneCmd())
	return cmd
}

func newDatasetPruneCmd() *cobra.Command {
	var region, datasetID string
	var keepLast int
	var dryRun, force bool
	cmd := &cobra.Command{
		Use:   "prune S3_URL --dataset-id ID --keep-last N",
		Short: "Delete old versions of a dataset, keeping the newest N (garbage collection)",
		Long: `Reclaim storage by deleting old versions of a dataset, keeping the newest N.

Only objects no kept version still references are removed. The oldest kept
version is first rewritten self-contained (compaction) so the pruned versions'
manifests can be removed without breaking it; the original is backed up to a
".pre-compact.bak" object first, and the rewrite is verified before anything is
deleted.

WARNING: deletion is IRREVERSIBLE. Use --dry-run to preview. Encrypted-manifest
datasets and mixed direct/chunked chains are not yet supported.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			if datasetID == "" {
				return fmt.Errorf("--dataset-id is required (see 'cargoship dataset list')")
			}
			if keepLast < 1 {
				return fmt.Errorf("--keep-last must be >= 1")
			}
			bucket, prefix, err := parseS3URL(args[0])
			if err != nil {
				return fmt.Errorf("invalid S3 URL: %w", err)
			}
			s3Client, kmsClient, err := datasetClients(ctx, region)
			if err != nil {
				return err
			}
			all, err := listUploadManifests(ctx, s3Client, kmsClient, bucket, prefix)
			if err != nil {
				return err
			}
			fetch := inMemoryFetch(all)

			var members []*manifest.Manifest
			for _, m := range all {
				id, derr := manifest.DatasetIDOf(ctx, m, fetch)
				if derr != nil {
					id = m.UploadID
				}
				if id != datasetID {
					continue
				}
				if m.Encryption != nil && m.Encryption.ManifestEncrypted {
					return fmt.Errorf("prune does not yet support encrypted-manifest datasets (version %s)", m.UploadID)
				}
				members = append(members, m)
			}

			plan, err := manifest.PlanKeepLast(ctx, members, keepLast, fetch)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(plan.Prune) == 0 {
				_, _ = fmt.Fprintf(out, "Nothing to prune: dataset %s has %d version(s); keeping last %d.\n",
					datasetID, len(plan.Keep), keepLast)
				return nil
			}

			prunableObjs := plan.PrunableObjectKeys()
			var manifestKeys []string
			for _, m := range plan.Prune {
				manifestKeys = append(manifestKeys,
					fmt.Sprintf("%s/uploads/%s/manifest.json.gz", prefix, m.UploadID),
					fmt.Sprintf("%s/uploads/%s/manifest.json", prefix, m.UploadID),
				)
			}
			deletionKeys := append(append([]string{}, prunableObjs...), manifestKeys...)

			_, _ = fmt.Fprintf(out, "Dataset %s: keep %d newest, prune %d older version(s).\n", datasetID, len(plan.Keep), len(plan.Prune))
			_, _ = fmt.Fprintf(out, "  Keep:  %s\n", versionLabels(plan.Keep))
			_, _ = fmt.Fprintf(out, "  Prune: %s\n", versionLabels(plan.Prune))
			if plan.CompactID != "" {
				_, _ = fmt.Fprintf(out, "  Compact: upload %s is rewritten self-contained (backup .pre-compact.bak) before its ancestors are removed.\n", plan.CompactID)
			}
			_, _ = fmt.Fprintf(out, "  Delete: %d data object(s) + %d pruned manifest(s).\n", len(prunableObjs), len(plan.Prune))

			if dryRun {
				_, _ = fmt.Fprintln(out, "\n🔍 Dry run — nothing deleted.")
				return nil
			}
			if !force {
				_, _ = fmt.Fprintf(out, "\n⚠️  This permanently deletes %d objects. Type 'yes' to confirm: ", len(deletionKeys))
				reader := bufio.NewReader(os.Stdin)
				resp, rerr := reader.ReadString('\n')
				if rerr != nil {
					return fmt.Errorf("failed to read confirmation: %w", rerr)
				}
				if strings.TrimSpace(strings.ToLower(resp)) != "yes" {
					_, _ = fmt.Fprintln(out, "❌ Prune cancelled")
					return nil
				}
			}

			// 1. Compact the oldest kept version FIRST (verified) so a mid-run
			//    failure never strands a kept version.
			if plan.CompactID != "" {
				if err := compactVersion(ctx, s3Client, bucket, prefix, plan.CompactID, fetch); err != nil {
					return fmt.Errorf("compaction failed — nothing deleted: %w", err)
				}
				_, _ = fmt.Fprintf(out, "✅ Compacted upload %s (self-contained).\n", plan.CompactID)
			}
			// 2. Concurrency guard: refuse if a new version appeared meanwhile.
			after, err := listUploadManifests(ctx, s3Client, kmsClient, bucket, prefix)
			if err != nil {
				return fmt.Errorf("re-list before delete: %w", err)
			}
			if datasetHead(ctx, after, inMemoryFetch(after), datasetID) != datasetHead(ctx, all, fetch, datasetID) {
				return fmt.Errorf("a new version appeared during prune; aborting before any delete")
			}
			// 3. Delete pruned objects (safe: none are in the kept mark set).
			deleted, err := deleteS3Objects(ctx, s3Client, bucket, deletionKeys)
			_, _ = fmt.Fprintf(out, "✅ Pruned %d version(s); deleted %d object(s).\n", len(plan.Prune), deleted)
			return err
		},
	}
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().StringVar(&datasetID, "dataset-id", "", "Dataset ID to prune (from 'cargoship dataset list')")
	cmd.Flags().IntVar(&keepLast, "keep-last", 0, "Keep the newest N versions; delete older ones (required, >= 1)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview what would be pruned without deleting")
	cmd.Flags().BoolVar(&force, "force", false, "Skip the confirmation prompt")
	return cmd
}

// compactVersion rewrites the upload's manifest as a self-contained (full)
// manifest — the merged effective view of its chain — so pruning its ancestors
// won't strand it. The original manifest object is copied to a
// ".pre-compact.bak" key first, and the rewrite is re-read and checked before
// the caller deletes anything. Plaintext manifests only (callers refuse
// encrypted datasets).
func compactVersion(ctx context.Context, s3Client *s3.Client, bucket, prefix, uploadID string, fetch manifest.ChainFetcher) error {
	target, err := fetch(ctx, uploadID)
	if err != nil {
		return fmt.Errorf("load compaction target %s: %w", uploadID, err)
	}
	chain, err := manifest.ResolveChain(ctx, target, fetch)
	if err != nil {
		return fmt.Errorf("resolve chain of %s: %w", uploadID, err)
	}
	merged := manifest.MergeChain(chain)
	merged.PreviousManifestID = ""
	merged.SyncType = manifest.SyncTypeFull
	merged.Bucket, merged.Prefix, merged.UploadID = bucket, prefix, uploadID

	origKey := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", prefix, uploadID)
	bakKey := origKey + ".pre-compact.bak"
	if _, err := s3Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: aws.String(bucket + "/" + origKey),
		Key:        aws.String(bakKey),
	}); err != nil {
		return fmt.Errorf("back up manifest before compaction: %w", err)
	}
	if err := merged.UploadToS3(ctx, s3Client, true); err != nil {
		return fmt.Errorf("write compacted manifest: %w", err)
	}
	// Verify: the rewrite must re-read and be self-contained.
	check, err := manifest.DownloadFromS3(ctx, s3Client, bucket, prefix, uploadID)
	if err != nil {
		return fmt.Errorf("re-read compacted manifest: %w", err)
	}
	if check.PreviousManifestID != "" {
		return fmt.Errorf("compacted manifest %s is still chained after rewrite", uploadID)
	}
	return nil
}

// datasetHead returns the upload ID of the highest-ordinal (then most recent)
// version of datasetID among all, or "" if none — used as a concurrency guard.
func datasetHead(ctx context.Context, all []*manifest.Manifest, fetch manifest.ChainFetcher, datasetID string) string {
	head, err := selectDatasetVersion(ctx, all, fetch, datasetID, 0, time.Time{})
	if err != nil {
		return ""
	}
	return head.UploadID
}

// versionLabels renders a compact "vN(uploadID)" list for reporting.
func versionLabels(ms []*manifest.Manifest) string {
	parts := make([]string, len(ms))
	for i, m := range ms {
		parts[i] = fmt.Sprintf("v%d(%s)", m.VersionOrdinal, m.UploadID)
	}
	return strings.Join(parts, ", ")
}

// selectDatasetVersion picks the manifest for a version selector within `all`
// (filtered to datasetID): version > 0 → the exact VersionOrdinal; a non-zero
// asOf → the newest version created at or before it; otherwise HEAD (highest
// ordinal, then most recent). Shared by `dataset diff` and version-aware restore.
func selectDatasetVersion(ctx context.Context, all []*manifest.Manifest, fetch manifest.ChainFetcher, datasetID string, version int, asOf time.Time) (*manifest.Manifest, error) {
	var members []*manifest.Manifest
	for _, m := range all {
		id, derr := manifest.DatasetIDOf(ctx, m, fetch)
		if derr != nil {
			id = m.UploadID
		}
		if id == datasetID {
			members = append(members, m)
		}
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("no dataset %q found under this prefix", datasetID)
	}
	switch {
	case version > 0:
		for _, m := range members {
			if m.VersionOrdinal == version {
				return m, nil
			}
		}
		return nil, fmt.Errorf("dataset %q has no version %d", datasetID, version)
	case !asOf.IsZero():
		var best *manifest.Manifest
		for _, m := range members {
			if !m.CreatedAt.After(asOf) && (best == nil || m.CreatedAt.After(best.CreatedAt)) {
				best = m
			}
		}
		if best == nil {
			return nil, fmt.Errorf("dataset %q has no version at or before %s", datasetID, asOf.Format("2006-01-02"))
		}
		return best, nil
	default: // HEAD
		best := members[0]
		for _, m := range members[1:] {
			if m.VersionOrdinal > best.VersionOrdinal ||
				(m.VersionOrdinal == best.VersionOrdinal && m.CreatedAt.After(best.CreatedAt)) {
				best = m
			}
		}
		return best, nil
	}
}

func newDatasetDiffCmd() *cobra.Command {
	var region, datasetID, fromSel, toSel string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "diff S3_URL --dataset-id ID --from V --to V",
		Short: "Compare two versions of a dataset (added/removed/modified files)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			if datasetID == "" {
				return fmt.Errorf("--dataset-id is required (see 'cargoship dataset list')")
			}
			if fromSel == "" {
				return fmt.Errorf("--from is required (a version number like 2, or a date YYYY-MM-DD)")
			}
			bucket, prefix, err := parseS3URL(args[0])
			if err != nil {
				return fmt.Errorf("invalid S3 URL: %w", err)
			}
			s3Client, kmsClient, err := datasetClients(ctx, region)
			if err != nil {
				return err
			}
			all, err := listUploadManifests(ctx, s3Client, kmsClient, bucket, prefix)
			if err != nil {
				return err
			}
			fetch := inMemoryFetch(all)

			fromM, err := resolveSelector(ctx, all, fetch, datasetID, fromSel)
			if err != nil {
				return fmt.Errorf("--from: %w", err)
			}
			toM, err := resolveSelector(ctx, all, fetch, datasetID, toSel) // toSel "" → HEAD
			if err != nil {
				return fmt.Errorf("--to: %w", err)
			}

			fromEff, err := manifest.ResolveEffective(ctx, fromM, fetch)
			if err != nil {
				return fmt.Errorf("resolve --from effective view: %w", err)
			}
			toEff, err := manifest.ResolveEffective(ctx, toM, fetch)
			if err != nil {
				return fmt.Errorf("resolve --to effective view: %w", err)
			}
			d := manifest.DiffFiles(fromEff.Files, toEff.Files)

			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"dataset_id": datasetID,
					"from":       fromM.UploadID, "from_version": fromM.VersionOrdinal,
					"to": toM.UploadID, "to_version": toM.VersionOrdinal,
					"added": d.Added, "removed": d.Removed, "modified": d.Modified, "unchanged": d.Unchanged,
				})
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Dataset %s: v%d → v%d\n", datasetID, fromM.VersionOrdinal, toM.VersionOrdinal)
			_, _ = fmt.Fprintf(out, "  +%d added  -%d removed  ~%d modified  =%d unchanged\n",
				len(d.Added), len(d.Removed), len(d.Modified), d.Unchanged)
			for _, p := range d.Added {
				_, _ = fmt.Fprintf(out, "  + %s\n", p)
			}
			for _, p := range d.Removed {
				_, _ = fmt.Fprintf(out, "  - %s\n", p)
			}
			for _, p := range d.Modified {
				_, _ = fmt.Fprintf(out, "  ~ %s\n", p)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().StringVar(&datasetID, "dataset-id", "", "Dataset ID (from 'cargoship dataset list')")
	cmd.Flags().StringVar(&fromSel, "from", "", "Baseline version: a number (v2 → 2) or a date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&toSel, "to", "", "Target version: a number or date; default HEAD (latest)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

// resolveSelector parses a CLI version selector — "" (HEAD), an ordinal like "2"
// or "v2", or a date "YYYY-MM-DD" — and resolves it to a manifest via
// selectDatasetVersion.
func resolveSelector(ctx context.Context, all []*manifest.Manifest, fetch manifest.ChainFetcher, datasetID, sel string) (*manifest.Manifest, error) {
	sel = strings.TrimSpace(sel)
	if sel == "" || strings.EqualFold(sel, "head") {
		return selectDatasetVersion(ctx, all, fetch, datasetID, 0, time.Time{})
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(sel, "v")); err == nil {
		return selectDatasetVersion(ctx, all, fetch, datasetID, n, time.Time{})
	}
	if d, err := time.Parse("2006-01-02", sel); err == nil {
		return selectDatasetVersion(ctx, all, fetch, datasetID, 0, d.Add(24*time.Hour-time.Nanosecond))
	}
	return nil, fmt.Errorf("invalid version selector %q (use a number like 2, v2, or a date YYYY-MM-DD)", sel)
}

// datasetClients loads S3 + KMS clients for a region.
func datasetClients(ctx context.Context, region string) (*s3.Client, *kms.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return s3.NewFromConfig(cfg), kms.NewFromConfig(cfg), nil
}

// inMemoryFetch returns a ChainFetcher backed by the already-enumerated
// manifests, so DatasetIDOf/chain resolution needs no extra network calls.
func inMemoryFetch(all []*manifest.Manifest) manifest.ChainFetcher {
	byID := make(map[string]*manifest.Manifest, len(all))
	for _, m := range all {
		byID[m.UploadID] = m
	}
	return func(_ context.Context, id string) (*manifest.Manifest, error) {
		if m, ok := byID[id]; ok {
			return m, nil
		}
		return nil, fmt.Errorf("manifest %s not found under this prefix", id)
	}
}

type datasetSummary struct {
	DatasetID    string    `json:"dataset_id"`
	Versions     int       `json:"versions"`
	HeadUploadID string    `json:"head_upload_id"`
	HeadOrdinal  int       `json:"head_version"`
	Files        int64     `json:"files"`
	Bytes        int64     `json:"bytes"`
	Updated      time.Time `json:"updated"`
}

func newDatasetListCmd() *cobra.Command {
	var region string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list S3_URL",
		Short: "List datasets under an S3 prefix",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			bucket, prefix, err := parseS3URL(args[0])
			if err != nil {
				return fmt.Errorf("invalid S3 URL: %w", err)
			}
			s3Client, kmsClient, err := datasetClients(ctx, region)
			if err != nil {
				return err
			}
			all, err := listUploadManifests(ctx, s3Client, kmsClient, bucket, prefix)
			if err != nil {
				return err
			}
			fetch := inMemoryFetch(all)

			// Group by dataset identity; track the head (highest version, then
			// most recent) per dataset.
			byDataset := map[string]*datasetSummary{}
			for _, m := range all {
				id, derr := manifest.DatasetIDOf(ctx, m, fetch)
				if derr != nil {
					id = m.UploadID // fall back to the upload's own id if the chain can't resolve
				}
				s := byDataset[id]
				if s == nil {
					s = &datasetSummary{DatasetID: id}
					byDataset[id] = s
				}
				s.Versions++
				if m.VersionOrdinal > s.HeadOrdinal ||
					(m.VersionOrdinal == s.HeadOrdinal && m.CreatedAt.After(s.Updated)) {
					s.HeadUploadID, s.HeadOrdinal = m.UploadID, m.VersionOrdinal
					s.Files, s.Bytes, s.Updated = m.TotalFiles, m.TotalBytes, m.CreatedAt
				}
			}
			summaries := make([]*datasetSummary, 0, len(byDataset))
			for _, s := range byDataset {
				summaries = append(summaries, s)
			}
			sort.Slice(summaries, func(i, j int) bool { return summaries[i].Updated.After(summaries[j].Updated) })

			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(summaries)
			}
			if len(summaries) == 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No datasets found under s3://%s/%s\n", bucket, prefix)
				return nil
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-40s %8s %6s %10s  %s\n", "DATASET", "VERSIONS", "FILES", "SIZE", "UPDATED")
			for _, s := range summaries {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-40s %8d %6d %10s  %s\n",
					s.DatasetID, s.Versions, s.Files, humanize.Bytes(uint64(s.Bytes)), humanize.Time(s.Updated))
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

type datasetVersion struct {
	Version   int       `json:"version"`
	UploadID  string    `json:"upload_id"`
	SyncType  string    `json:"sync_type"`
	Files     int64     `json:"files"`
	Bytes     int64     `json:"bytes"`
	CreatedAt time.Time `json:"created_at"`
}

func newDatasetVersionsCmd() *cobra.Command {
	var region, datasetID string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "versions S3_URL --dataset-id ID",
		Short: "List the versions of one dataset, newest first",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			if datasetID == "" {
				return fmt.Errorf("--dataset-id is required (see 'cargoship dataset list')")
			}
			bucket, prefix, err := parseS3URL(args[0])
			if err != nil {
				return fmt.Errorf("invalid S3 URL: %w", err)
			}
			s3Client, kmsClient, err := datasetClients(ctx, region)
			if err != nil {
				return err
			}
			all, err := listUploadManifests(ctx, s3Client, kmsClient, bucket, prefix)
			if err != nil {
				return err
			}
			fetch := inMemoryFetch(all)

			var versions []datasetVersion
			for _, m := range all {
				id, derr := manifest.DatasetIDOf(ctx, m, fetch)
				if derr != nil {
					id = m.UploadID
				}
				if id != datasetID {
					continue
				}
				versions = append(versions, datasetVersion{
					Version: m.VersionOrdinal, UploadID: m.UploadID, SyncType: m.SyncType,
					Files: m.TotalFiles, Bytes: m.TotalBytes, CreatedAt: m.CreatedAt,
				})
			}
			if len(versions) == 0 {
				return fmt.Errorf("no dataset %q found under s3://%s/%s", datasetID, bucket, prefix)
			}
			// Newest first: by version ordinal, then creation time.
			sort.Slice(versions, func(i, j int) bool {
				if versions[i].Version != versions[j].Version {
					return versions[i].Version > versions[j].Version
				}
				return versions[i].CreatedAt.After(versions[j].CreatedAt)
			})

			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(versions)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Dataset %s — %d version(s):\n", datasetID, len(versions))
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-6s %-28s %-12s %6s %10s  %s\n", "VER", "UPLOAD ID", "TYPE", "FILES", "SIZE", "CREATED")
			for _, v := range versions {
				ver := fmt.Sprintf("v%d", v.Version)
				if v.Version == 0 {
					ver = "—" // legacy upload without a recorded ordinal
				}
				st := v.SyncType
				if st == "" {
					st = "full"
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-6s %-28s %-12s %6d %10s  %s\n",
					ver, v.UploadID, st, v.Files, humanize.Bytes(uint64(v.Bytes)), humanize.Time(v.CreatedAt))
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().StringVar(&datasetID, "dataset-id", "", "Dataset ID to list versions of (from 'cargoship dataset list')")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}
