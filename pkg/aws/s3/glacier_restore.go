// Package s3 provides S3 transport utilities including Glacier/Deep Archive
// restoration orchestration (Issue #200).
package s3

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// RestoreTier controls Glacier retrieval speed and cost.
type RestoreTier string

const (
	// RestoreTierExpedited retrieves data in 1–5 minutes (most expensive).
	// Not available for DEEP_ARCHIVE.
	RestoreTierExpedited RestoreTier = "Expedited"
	// RestoreTierStandard retrieves data in 3–5 hours.
	RestoreTierStandard RestoreTier = "Standard"
	// RestoreTierBulk retrieves data in 5–12 hours (cheapest).
	RestoreTierBulk RestoreTier = "Bulk"
)

// DefaultRestoreTier is used when no tier is explicitly specified.
const DefaultRestoreTier = RestoreTierStandard

// DefaultRestoreDays is the number of days a restored copy remains available.
const DefaultRestoreDays int32 = 7

// DefaultGlacierPollInterval is how often WaitForRestore checks object status.
const DefaultGlacierPollInterval = 30 * time.Second

// retrievalCostPerGB contains approximate retrieval fees in USD/GB by tier.
// Sources: AWS pricing as of 2026 (us-east-1 reference prices).
var retrievalCostPerGB = map[string]map[RestoreTier]float64{
	"GLACIER": {
		RestoreTierExpedited: 0.030,
		RestoreTierStandard:  0.010,
		RestoreTierBulk:      0.0025,
	},
	"DEEP_ARCHIVE": {
		// Expedited is not supported for DEEP_ARCHIVE; fall back to Standard.
		RestoreTierExpedited: 0.020,
		RestoreTierStandard:  0.020,
		RestoreTierBulk:      0.0025,
	},
}

// estimatedRestoreDuration returns the approximate wait time for a restore by
// storage class and tier.
func estimatedRestoreDuration(storageClass string, tier RestoreTier) string {
	switch storageClass {
	case "DEEP_ARCHIVE":
		if tier == RestoreTierBulk {
			return "12–48 hours"
		}
		return "9–12 hours"
	default: // GLACIER
		switch tier {
		case RestoreTierExpedited:
			return "1–5 minutes"
		case RestoreTierBulk:
			return "5–12 hours"
		default:
			return "3–5 hours"
		}
	}
}

// GlacierS3Client is the subset of the S3 API required by GlacierRestorer.
type GlacierS3Client interface {
	HeadObject(ctx context.Context, input *s3.HeadObjectInput, opts ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	RestoreObject(ctx context.Context, input *s3.RestoreObjectInput, opts ...func(*s3.Options)) (*s3.RestoreObjectOutput, error)
}

// GlacierObjectRestoreStatus describes the current accessibility of a single S3 object.
type GlacierObjectRestoreStatus int

const (
	// GlacierObjectAccessible means the object can be downloaded immediately.
	GlacierObjectAccessible GlacierObjectRestoreStatus = iota
	// GlacierObjectRestoreInProgress means a restore was requested but is not yet complete.
	GlacierObjectRestoreInProgress
	// GlacierObjectFrozen means the object is in Glacier/Deep Archive with no pending restore.
	GlacierObjectFrozen
)

// GlacierObjectInfo describes the storage state of one S3 object key.
type GlacierObjectInfo struct {
	Key          string
	StorageClass string
	Status       GlacierObjectRestoreStatus
	// ExpiresAt is when the restored copy will expire (only set when the
	// object was recently restored from Glacier).
	ExpiresAt *time.Time
	// SizeBytes is the content length from HeadObject.
	SizeBytes int64
}

// AccessibilityReport summarises the Glacier state of a set of S3 object keys.
type AccessibilityReport struct {
	// Objects is the per-key status.
	Objects []GlacierObjectInfo
	// Accessible contains keys that can be downloaded immediately.
	Accessible []string
	// InProgress contains keys whose restore has already been initiated.
	InProgress []string
	// Frozen contains keys that need a restore request before they can be read.
	Frozen []string
	// JustRequested contains keys for which this call issued RestoreObject.
	JustRequested []string
	// EstimatedCostUSD is the approximate retrieval fee for frozen objects (USD).
	EstimatedCostUSD float64
	// TotalSizeGB is the sum of all object sizes in GB.
	TotalSizeGB float64
}

// AllAccessible returns true when every key is immediately downloadable.
func (r *AccessibilityReport) AllAccessible() bool {
	return len(r.Frozen) == 0 && len(r.InProgress) == 0
}

// GlacierRestorer checks S3 object accessibility and orchestrates
// Glacier/Deep Archive restorations (Issue #200).
type GlacierRestorer struct {
	s3Client GlacierS3Client
	// Days is how long a restored copy remains available after retrieval.
	Days int32
}

// NewGlacierRestorer creates a GlacierRestorer using the provided S3 client.
// Pass 0 for days to use DefaultRestoreDays.
func NewGlacierRestorer(client GlacierS3Client, days int32) *GlacierRestorer {
	if days <= 0 {
		days = DefaultRestoreDays
	}
	return &GlacierRestorer{s3Client: client, Days: days}
}

// CheckAndRestore inspects each key's storage state and, for frozen objects,
// issues a RestoreObject request at the requested tier. It returns an
// AccessibilityReport describing what was found and what actions were taken.
//
// If a key is already in-progress (restore requested previously) it is
// reported in InProgress without issuing a duplicate request.
func (gr *GlacierRestorer) CheckAndRestore(ctx context.Context, bucket string, keys []string, tier RestoreTier) (*AccessibilityReport, error) {
	if tier == "" {
		tier = DefaultRestoreTier
	}

	report := &AccessibilityReport{}

	for _, key := range keys {
		info, err := gr.headObject(ctx, bucket, key)
		if err != nil {
			return nil, fmt.Errorf("HeadObject %q: %w", key, err)
		}
		report.Objects = append(report.Objects, *info)
		report.TotalSizeGB += float64(info.SizeBytes) / (1024 * 1024 * 1024)

		switch info.Status {
		case GlacierObjectAccessible:
			report.Accessible = append(report.Accessible, key)

		case GlacierObjectRestoreInProgress:
			report.InProgress = append(report.InProgress, key)

		case GlacierObjectFrozen:
			// Estimate retrieval cost.
			costPerGB := retrievalCostPerGB[info.StorageClass][tier]
			if costPerGB == 0 {
				costPerGB = retrievalCostPerGB[info.StorageClass][RestoreTierStandard]
			}
			sizeGB := float64(info.SizeBytes) / (1024 * 1024 * 1024)
			report.EstimatedCostUSD += sizeGB * costPerGB

			if err := gr.requestRestore(ctx, bucket, key, tier); err != nil {
				return nil, fmt.Errorf("RestoreObject %q: %w", key, err)
			}
			report.JustRequested = append(report.JustRequested, key)
			report.Frozen = append(report.Frozen, key)
		}
	}

	return report, nil
}

// WaitForRestore polls each key in keys until all are accessible or ctx is
// cancelled. It calls progressFn (if non-nil) before each sleep with the
// number of keys still pending.
func (gr *GlacierRestorer) WaitForRestore(ctx context.Context, bucket string, keys []string, pollInterval time.Duration, progressFn func(pending int)) error {
	if pollInterval <= 0 {
		pollInterval = DefaultGlacierPollInterval
	}
	pending := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		pending[k] = struct{}{}
	}

	for len(pending) > 0 {
		if progressFn != nil {
			progressFn(len(pending))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}

		for key := range pending {
			info, err := gr.headObject(ctx, bucket, key)
			if err != nil {
				return fmt.Errorf("HeadObject %q: %w", key, err)
			}
			if info.Status == GlacierObjectAccessible {
				delete(pending, key)
			}
		}
	}

	return nil
}

// headObject calls HeadObject and parses the result into a GlacierObjectInfo.
func (gr *GlacierRestorer) headObject(ctx context.Context, bucket, key string) (*GlacierObjectInfo, error) {
	out, err := gr.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}

	// StorageClass is a typed string alias; convert to plain string for map lookup.
	sc := string(out.StorageClass)

	info := &GlacierObjectInfo{
		Key:          key,
		StorageClass: sc,
		Status:       GlacierObjectAccessible,
	}
	if out.ContentLength != nil {
		info.SizeBytes = *out.ContentLength
	}

	if isGlacierClass(sc) {
		if out.Restore == nil || *out.Restore == "" {
			info.Status = GlacierObjectFrozen
		} else {
			restore := *out.Restore
			switch {
			case strings.Contains(restore, `ongoing-request="true"`):
				info.Status = GlacierObjectRestoreInProgress
			case strings.Contains(restore, `ongoing-request="false"`):
				info.Status = GlacierObjectAccessible
				if t := parseRestoreExpiry(restore); t != nil {
					info.ExpiresAt = t
				}
			}
		}
	}

	return info, nil
}

// requestRestore issues a RestoreObject API call for a single key.
func (gr *GlacierRestorer) requestRestore(ctx context.Context, bucket, key string, tier RestoreTier) error {
	s3Tier := s3types.TierStandard
	switch tier {
	case RestoreTierExpedited:
		s3Tier = s3types.TierExpedited
	case RestoreTierBulk:
		s3Tier = s3types.TierBulk
	}

	_, err := gr.s3Client.RestoreObject(ctx, &s3.RestoreObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		RestoreRequest: &s3types.RestoreRequest{
			Days: aws.Int32(gr.Days),
			GlacierJobParameters: &s3types.GlacierJobParameters{
				Tier: s3Tier,
			},
		},
	})
	return err
}

// isGlacierClass returns true for storage classes that require a restore request.
func isGlacierClass(sc string) bool {
	return sc == "GLACIER" || sc == "DEEP_ARCHIVE"
}

// parseRestoreExpiry extracts the expiry time from a Restore header value like:
// ongoing-request="false", expiry-date="Fri, 21 Dec 2012 00:00:00 GMT"
func parseRestoreExpiry(header string) *time.Time {
	const prefix = `expiry-date="`
	idx := strings.Index(header, prefix)
	if idx < 0 {
		return nil
	}
	rest := header[idx+len(prefix):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return nil
	}
	t, err := time.Parse(time.RFC1123, rest[:end])
	if err != nil {
		return nil
	}
	return &t
}

// EstimateRetrievalCost returns the approximate retrieval cost in USD for
// sizeGB bytes at the given storage class and tier.
func EstimateRetrievalCost(sizeGB float64, storageClass string, tier RestoreTier) float64 {
	tiers, ok := retrievalCostPerGB[storageClass]
	if !ok {
		return 0
	}
	rate, ok := tiers[tier]
	if !ok {
		rate = tiers[RestoreTierStandard]
	}
	return sizeGB * rate
}

// FormatAccessibilityReport returns a human-readable summary of an
// AccessibilityReport for display in CLI output.
func FormatAccessibilityReport(report *AccessibilityReport, tier RestoreTier) string {
	var sb strings.Builder

	if len(report.JustRequested) > 0 {
		sc := ""
		if len(report.Objects) > 0 {
			sc = report.Objects[0].StorageClass
		}
		eta := estimatedRestoreDuration(sc, tier)
		fmt.Fprintf(&sb, "🧊 Restore requested for %d chunk(s) — estimated ready in %s\n", len(report.JustRequested), eta)
	}
	if len(report.InProgress) > 0 {
		fmt.Fprintf(&sb, "⏳ %d chunk(s) already being restored (in-progress)\n", len(report.InProgress))
	}
	if report.EstimatedCostUSD > 0 {
		fmt.Fprintf(&sb, "💰 Estimated retrieval cost: $%.4f USD (%.2f GB at %s tier)\n",
			report.EstimatedCostUSD, report.TotalSizeGB, tier)
	}

	return sb.String()
}
