package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
)

// Severity ranks an immutability finding. Higher is worse; Unknown (a check we
// couldn't complete, e.g. AccessDenied) ranks above Warn but below Critical — it is
// uncertainty, not a confirmed gap.
type Severity int

const (
	SevOK Severity = iota
	SevWarn
	SevUnknown
	SevCritical
)

func (s Severity) String() string {
	switch s {
	case SevOK:
		return "OK"
	case SevWarn:
		return "WARN"
	case SevCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// MarshalJSON emits the label so --json output is human-readable.
func (s Severity) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

// UnmarshalJSON parses the label form back (round-trips MarshalJSON).
func (s *Severity) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}
	switch str {
	case "OK":
		*s = SevOK
	case "WARN":
		*s = SevWarn
	case "CRITICAL":
		*s = SevCritical
	default:
		*s = SevUnknown
	}
	return nil
}

// ImmutabilityFinding is one check in the fleet-bucket posture audit (#616).
type ImmutabilityFinding struct {
	Check       string   `json:"check"`
	Severity    Severity `json:"severity"`
	Summary     string   `json:"summary"`
	Detail      string   `json:"detail,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
}

// ImmutabilityReport is the bucket-wide immutability/hygiene posture of a fleet
// bucket: is history protected against a stolen delete-capable credential, and is
// failed-upload hygiene in place. Object Lock, versioning, and lifecycle are all
// bucket-level, so this audits a bucket, not a writer prefix.
type ImmutabilityReport struct {
	Bucket   string                `json:"bucket"`
	Findings []ImmutabilityFinding `json:"findings"`
}

// Worst returns the highest severity across the findings (SevOK for none).
func (r ImmutabilityReport) Worst() Severity {
	worst := SevOK
	for _, f := range r.Findings {
		if f.Severity > worst {
			worst = f.Severity
		}
	}
	return worst
}

// BucketPostureAPI is the minimal read-only S3 surface the audit needs; *s3.Client
// satisfies it. All three calls are GETs — the audit never mutates the bucket.
type BucketPostureAPI interface {
	GetBucketVersioning(ctx context.Context, in *s3.GetBucketVersioningInput, optFns ...func(*s3.Options)) (*s3.GetBucketVersioningOutput, error)
	GetObjectLockConfiguration(ctx context.Context, in *s3.GetObjectLockConfigurationInput, optFns ...func(*s3.Options)) (*s3.GetObjectLockConfigurationOutput, error)
	GetBucketLifecycleConfiguration(ctx context.Context, in *s3.GetBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.GetBucketLifecycleConfigurationOutput, error)
}

// AuditBucketImmutability runs the versioning + Object Lock + lifecycle-hygiene checks
// against a fleet bucket and returns a posture report. It never mutates the bucket and
// never returns a fatal error for a single denied/failed check — that check becomes an
// Unknown finding so the rest of the report still renders.
func AuditBucketImmutability(ctx context.Context, api BucketPostureAPI, bucket string) (ImmutabilityReport, error) {
	return ImmutabilityReport{
		Bucket: bucket,
		Findings: []ImmutabilityFinding{
			checkVersioning(ctx, api, bucket),
			checkObjectLock(ctx, api, bucket),
			checkLifecycleMPU(ctx, api, bucket),
		},
	}, nil
}

func checkVersioning(ctx context.Context, api BucketPostureAPI, bucket string) ImmutabilityFinding {
	out, err := api.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(bucket)})
	if err != nil {
		return unknownFinding("versioning", "s3:GetBucketVersioning", err)
	}
	switch out.Status {
	case s3types.BucketVersioningStatusEnabled:
		return ImmutabilityFinding{Check: "versioning", Severity: SevOK, Summary: "Enabled"}
	case s3types.BucketVersioningStatusSuspended:
		return ImmutabilityFinding{
			Check: "versioning", Severity: SevCritical, Summary: "Suspended",
			Detail:      "versioning was enabled then suspended; new overwrites and deletes are no longer versioned",
			Remediation: "re-enable bucket versioning (it is a prerequisite for the Object Lock backstop)",
		}
	default: // "" — never enabled
		return ImmutabilityFinding{
			Check: "versioning", Severity: SevCritical, Summary: "not enabled",
			Detail:      "without versioning an overwrite or delete is unrecoverable, and Object Lock cannot protect history",
			Remediation: "enable bucket versioning (prerequisite for the Object Lock backstop)",
		}
	}
}

func checkObjectLock(ctx context.Context, api BucketPostureAPI, bucket string) ImmutabilityFinding {
	const notConfigured = "not configured"
	out, err := api.GetObjectLockConfiguration(ctx, &s3.GetObjectLockConfigurationInput{Bucket: aws.String(bucket)})
	if err != nil {
		if code := apiErrCode(err); code == "ObjectLockConfigurationNotFoundError" {
			return objectLockAbsent(notConfigured)
		}
		return unknownFinding("object-lock", "s3:GetObjectLockConfiguration", err)
	}
	cfg := out.ObjectLockConfiguration
	if cfg == nil || cfg.ObjectLockEnabled != s3types.ObjectLockEnabledEnabled {
		return objectLockAbsent(notConfigured)
	}
	if cfg.Rule == nil || cfg.Rule.DefaultRetention == nil {
		return ImmutabilityFinding{
			Check: "object-lock", Severity: SevWarn, Summary: "enabled, no default retention",
			Detail:      "Object Lock is on but no default retention is set, so newly written objects are not automatically protected",
			Remediation: "set a default retention (governance mode) covering your recovery window",
		}
	}
	dr := cfg.Rule.DefaultRetention
	period := retentionPeriod(dr)
	detail := fmt.Sprintf("%s mode, default retention %s", dr.Mode, period)
	if dr.Mode == s3types.ObjectLockRetentionModeGovernance {
		detail += "; note governance mode can be bypassed by a principal with s3:BypassGovernanceRetention"
	} else {
		detail += "; compliance mode cannot be bypassed by any principal, including root"
	}
	return ImmutabilityFinding{
		Check: "object-lock", Severity: SevOK,
		Summary: fmt.Sprintf("%s, %s", dr.Mode, period),
		Detail:  detail,
	}
}

func objectLockAbsent(summary string) ImmutabilityFinding {
	return ImmutabilityFinding{
		Check: "object-lock", Severity: SevCritical, Summary: summary,
		Detail:      "a stolen delete-capable credential can destroy backup history",
		Remediation: "enable S3 Versioning + Object Lock (governance mode) on the fleet bucket; see the fleet immutability guide",
	}
}

func retentionPeriod(dr *s3types.DefaultRetention) string {
	switch {
	case dr.Days != nil && *dr.Days > 0:
		return fmt.Sprintf("%d days", *dr.Days)
	case dr.Years != nil && *dr.Years > 0:
		return fmt.Sprintf("%d years", *dr.Years)
	default:
		return "unspecified"
	}
}

func checkLifecycleMPU(ctx context.Context, api BucketPostureAPI, bucket string) ImmutabilityFinding {
	out, err := api.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: aws.String(bucket)})
	if err != nil {
		if code := apiErrCode(err); code == "NoSuchLifecycleConfiguration" {
			return lifecycleMPUMissing("no lifecycle rules")
		}
		return unknownFinding("lifecycle-mpu", "s3:GetBucketLifecycleConfiguration", err)
	}
	for _, r := range out.Rules {
		if r.AbortIncompleteMultipartUpload != nil && r.Status == s3types.ExpirationStatusEnabled {
			days := aws.ToInt32(r.AbortIncompleteMultipartUpload.DaysAfterInitiation)
			return ImmutabilityFinding{
				Check: "lifecycle-mpu", Severity: SevOK,
				Summary: fmt.Sprintf("AbortIncompleteMultipartUpload after %dd", days),
			}
		}
	}
	return lifecycleMPUMissing("no AbortIncompleteMultipartUpload rule")
}

func lifecycleMPUMissing(summary string) ImmutabilityFinding {
	return ImmutabilityFinding{
		Check: "lifecycle-mpu", Severity: SevWarn, Summary: summary,
		Detail:      "the agent does not clean up failed multipart uploads (CleanupOnFailure is off), so incomplete uploads accrue storage cost indefinitely",
		Remediation: "add a lifecycle rule with AbortIncompleteMultipartUpload (e.g. 7 days)",
	}
}

// unknownFinding is the graceful-degrade result for a check that couldn't complete —
// AccessDenied (missing read permission) or any other API error.
func unknownFinding(check, action string, err error) ImmutabilityFinding {
	if apiErrCode(err) == "AccessDenied" {
		return ImmutabilityFinding{
			Check: check, Severity: SevUnknown, Summary: "unknown (access denied)",
			Detail:      fmt.Sprintf("%s was denied, so this posture could not be determined", action),
			Remediation: fmt.Sprintf("grant read access for %s to the auditing principal", action),
		}
	}
	return ImmutabilityFinding{
		Check: check, Severity: SevUnknown, Summary: "unknown",
		Detail: fmt.Sprintf("%s failed: %v", action, err),
	}
}

func apiErrCode(err error) string {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode()
	}
	return ""
}
