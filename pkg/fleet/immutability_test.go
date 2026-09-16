package fleet

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
)

// fakePostureAPI returns canned outputs/errors per call.
type fakePostureAPI struct {
	versioning    *s3.GetBucketVersioningOutput
	versioningErr error
	objectLock    *s3.GetObjectLockConfigurationOutput
	objectLockErr error
	lifecycle     *s3.GetBucketLifecycleConfigurationOutput
	lifecycleErr  error
}

func (f *fakePostureAPI) GetBucketVersioning(context.Context, *s3.GetBucketVersioningInput, ...func(*s3.Options)) (*s3.GetBucketVersioningOutput, error) {
	if f.versioningErr != nil {
		return nil, f.versioningErr
	}
	if f.versioning != nil {
		return f.versioning, nil
	}
	return &s3.GetBucketVersioningOutput{}, nil
}

func (f *fakePostureAPI) GetObjectLockConfiguration(context.Context, *s3.GetObjectLockConfigurationInput, ...func(*s3.Options)) (*s3.GetObjectLockConfigurationOutput, error) {
	if f.objectLockErr != nil {
		return nil, f.objectLockErr
	}
	if f.objectLock != nil {
		return f.objectLock, nil
	}
	return &s3.GetObjectLockConfigurationOutput{}, nil
}

func (f *fakePostureAPI) GetBucketLifecycleConfiguration(context.Context, *s3.GetBucketLifecycleConfigurationInput, ...func(*s3.Options)) (*s3.GetBucketLifecycleConfigurationOutput, error) {
	if f.lifecycleErr != nil {
		return nil, f.lifecycleErr
	}
	if f.lifecycle != nil {
		return f.lifecycle, nil
	}
	return &s3.GetBucketLifecycleConfigurationOutput{}, nil
}

func apiErr(code string) error { return &smithy.GenericAPIError{Code: code, Message: code} }

func findingByCheck(r ImmutabilityReport, check string) ImmutabilityFinding {
	for _, f := range r.Findings {
		if f.Check == check {
			return f
		}
	}
	return ImmutabilityFinding{Check: check + " (MISSING)"}
}

func TestAuditBucketImmutability_FullyProtected(t *testing.T) {
	days := int32(30)
	api := &fakePostureAPI{
		versioning: &s3.GetBucketVersioningOutput{Status: s3types.BucketVersioningStatusEnabled},
		objectLock: &s3.GetObjectLockConfigurationOutput{
			ObjectLockConfiguration: &s3types.ObjectLockConfiguration{
				ObjectLockEnabled: s3types.ObjectLockEnabledEnabled,
				Rule: &s3types.ObjectLockRule{DefaultRetention: &s3types.DefaultRetention{
					Mode: s3types.ObjectLockRetentionModeCompliance, Days: aws.Int32(365),
				}},
			},
		},
		lifecycle: &s3.GetBucketLifecycleConfigurationOutput{
			Rules: []s3types.LifecycleRule{{
				Status:                         s3types.ExpirationStatusEnabled,
				AbortIncompleteMultipartUpload: &s3types.AbortIncompleteMultipartUpload{DaysAfterInitiation: &days},
			}},
		},
	}
	r, err := AuditBucketImmutability(context.Background(), api, "b")
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if got := r.Worst(); got != SevOK {
		t.Fatalf("worst = %s, want OK; findings=%+v", got, r.Findings)
	}
	if f := findingByCheck(r, "object-lock"); f.Severity != SevOK {
		t.Errorf("object-lock = %s, want OK", f.Severity)
	}
}

func TestAuditBucketImmutability_NoBackstop(t *testing.T) {
	api := &fakePostureAPI{
		versioning:    &s3.GetBucketVersioningOutput{}, // never enabled
		objectLockErr: apiErr("ObjectLockConfigurationNotFoundError"),
		lifecycleErr:  apiErr("NoSuchLifecycleConfiguration"),
	}
	r, _ := AuditBucketImmutability(context.Background(), api, "b")
	if got := r.Worst(); got != SevCritical {
		t.Fatalf("worst = %s, want CRITICAL", got)
	}
	if f := findingByCheck(r, "versioning"); f.Severity != SevCritical {
		t.Errorf("versioning = %s, want CRITICAL", f.Severity)
	}
	if f := findingByCheck(r, "object-lock"); f.Severity != SevCritical || f.Remediation == "" {
		t.Errorf("object-lock = %+v, want CRITICAL with remediation", f)
	}
	if f := findingByCheck(r, "lifecycle-mpu"); f.Severity != SevWarn {
		t.Errorf("lifecycle-mpu = %s, want WARN", f.Severity)
	}
}

func TestCheckVersioning_Suspended(t *testing.T) {
	api := &fakePostureAPI{versioning: &s3.GetBucketVersioningOutput{Status: s3types.BucketVersioningStatusSuspended}}
	f := checkVersioning(context.Background(), api, "b")
	if f.Severity != SevCritical || f.Summary != "Suspended" {
		t.Errorf("suspended versioning = %+v, want CRITICAL/Suspended", f)
	}
}

func TestCheckObjectLock_EnabledNoDefaultRetention(t *testing.T) {
	api := &fakePostureAPI{objectLock: &s3.GetObjectLockConfigurationOutput{
		ObjectLockConfiguration: &s3types.ObjectLockConfiguration{ObjectLockEnabled: s3types.ObjectLockEnabledEnabled},
	}}
	f := checkObjectLock(context.Background(), api, "b")
	if f.Severity != SevWarn {
		t.Errorf("object-lock enabled/no-retention = %s, want WARN", f.Severity)
	}
}

func TestCheckObjectLock_GovernanceNotesBypass(t *testing.T) {
	api := &fakePostureAPI{objectLock: &s3.GetObjectLockConfigurationOutput{
		ObjectLockConfiguration: &s3types.ObjectLockConfiguration{
			ObjectLockEnabled: s3types.ObjectLockEnabledEnabled,
			Rule: &s3types.ObjectLockRule{DefaultRetention: &s3types.DefaultRetention{
				Mode: s3types.ObjectLockRetentionModeGovernance, Days: aws.Int32(30),
			}},
		},
	}}
	f := checkObjectLock(context.Background(), api, "b")
	if f.Severity != SevOK {
		t.Fatalf("governance object-lock = %s, want OK", f.Severity)
	}
	if !strings.Contains(f.Detail, "BypassGovernanceRetention") {
		t.Errorf("governance detail should note the bypass caveat: %q", f.Detail)
	}
}

func TestAudit_AccessDeniedIsUnknown(t *testing.T) {
	api := &fakePostureAPI{versioningErr: apiErr("AccessDenied")}
	f := checkVersioning(context.Background(), api, "b")
	if f.Severity != SevUnknown {
		t.Errorf("access-denied versioning = %s, want UNKNOWN", f.Severity)
	}
	if !strings.Contains(f.Summary, "access denied") {
		t.Errorf("summary = %q, want it to mention access denied", f.Summary)
	}
}

func TestSeverity_JSONRoundTrip(t *testing.T) {
	for _, s := range []Severity{SevOK, SevWarn, SevUnknown, SevCritical} {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("marshal %v: %v", s, err)
		}
		var got Severity
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", b, err)
		}
		if got != s {
			t.Errorf("round-trip %s -> %s", s, got)
		}
	}
}
