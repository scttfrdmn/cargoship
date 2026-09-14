package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

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
	cmd.AddCommand(newDatasetListCmd(), newDatasetVersionsCmd())
	return cmd
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
