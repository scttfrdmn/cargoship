// Package access implements read-only access-control posture checks for the S3
// buckets CargoShip writes to and reads from (Issue #529).
//
// The goal is narrow and deliberate: surface whether an archive would be
// world- or account-wide-readable, and whether the encryption key protecting it
// is scoped to the intended principals — NOT a general security audit (that is
// tracked separately in #323). Every check is read-only and degrades
// gracefully: a check that lacks permission reports an "unknown" posture with
// the IAM action it needs, and never aborts the rest of the report.
package access

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	smithy "github.com/aws/smithy-go"
)

// Severity ranks a finding from benign to actionable.
type Severity string

const (
	// SeverityOK means the posture is as locked-down as expected.
	SeverityOK Severity = "ok"
	// SeverityInfo is a neutral observation, not a problem.
	SeverityInfo Severity = "info"
	// SeverityWarn is a posture worth a second look but not clearly wrong.
	SeverityWarn Severity = "warn"
	// SeverityCritical means the archive is (or can be) broadly readable.
	SeverityCritical Severity = "critical"
	// SeverityUnknown means the check could not run (usually missing permission).
	SeverityUnknown Severity = "unknown"
)

// rank orders severities so a report can roll up to its worst finding.
func (s Severity) rank() int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityWarn:
		return 3
	case SeverityUnknown:
		return 2
	case SeverityInfo:
		return 1
	default: // SeverityOK
		return 0
	}
}

// AtLeast reports whether s is at least as severe as o (by rank), so callers
// can gate on a threshold (e.g. "fail if worst finding is at least warn").
func (s Severity) AtLeast(o Severity) bool { return s.rank() >= o.rank() }

// Finding is the result of a single posture check.
type Finding struct {
	Check       string   `json:"check"`                 // stable machine key, e.g. "block-public-access"
	Severity    Severity `json:"severity"`              // ok | info | warn | critical | unknown
	Summary     string   `json:"summary"`               // one-line human verdict
	Detail      string   `json:"detail,omitempty"`      // supporting evidence
	Remediation string   `json:"remediation,omitempty"` // what to do about it (incl. the IAM action for unknowns)
}

// Report is the full posture report for one bucket target.
type Report struct {
	Bucket    string    `json:"bucket"`
	Prefix    string    `json:"prefix,omitempty"`
	Region    string    `json:"region,omitempty"`
	Account   string    `json:"account,omitempty"`
	CallerARN string    `json:"caller_arn,omitempty"`
	Findings  []Finding `json:"findings"`
}

// Worst returns the highest severity across all findings (SeverityOK if empty).
func (r *Report) Worst() Severity {
	worst := SeverityOK
	for _, f := range r.Findings {
		if f.Severity.rank() > worst.rank() {
			worst = f.Severity
		}
	}
	return worst
}

// S3PolicyAPI is the subset of the S3 API the posture checks call. It is an
// interface so tests can supply a mock instead of a live client.
type S3PolicyAPI interface {
	GetPublicAccessBlock(context.Context, *s3.GetPublicAccessBlockInput, ...func(*s3.Options)) (*s3.GetPublicAccessBlockOutput, error)
	GetBucketPolicyStatus(context.Context, *s3.GetBucketPolicyStatusInput, ...func(*s3.Options)) (*s3.GetBucketPolicyStatusOutput, error)
	GetBucketPolicy(context.Context, *s3.GetBucketPolicyInput, ...func(*s3.Options)) (*s3.GetBucketPolicyOutput, error)
	GetBucketAcl(context.Context, *s3.GetBucketAclInput, ...func(*s3.Options)) (*s3.GetBucketAclOutput, error)
	GetBucketOwnershipControls(context.Context, *s3.GetBucketOwnershipControlsInput, ...func(*s3.Options)) (*s3.GetBucketOwnershipControlsOutput, error)
	GetBucketEncryption(context.Context, *s3.GetBucketEncryptionInput, ...func(*s3.Options)) (*s3.GetBucketEncryptionOutput, error)
}

// STSAPI resolves the calling identity for report context. Optional.
type STSAPI interface {
	GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// KMSAPI inspects a KMS key's policy. Optional. GetKeyPolicy accepts a key ID,
// ARN, or alias directly, so no DescribeKey resolution step is needed.
type KMSAPI interface {
	GetKeyPolicy(context.Context, *kms.GetKeyPolicyInput, ...func(*kms.Options)) (*kms.GetKeyPolicyOutput, error)
}

// Checker runs the posture checks against one bucket. S3 is required; STS and
// KMS are optional (nil disables the caller-identity and KMS-key checks).
type Checker struct {
	S3  S3PolicyAPI
	STS STSAPI
	KMS KMSAPI
	// KMSKeyID, when set, forces the KMS key-policy check against this key
	// instead of auto-detecting the bucket's default SSE-KMS key.
	KMSKeyID string
}

// Check runs every posture check and returns the assembled report. It never
// returns an error for a check that merely lacks permission — that surfaces as
// an "unknown" finding. A non-nil error is reserved for a caller that wants to
// treat total failure as fatal; today Check always returns nil.
func (c *Checker) Check(ctx context.Context, bucket, prefix string) (*Report, error) {
	r := &Report{Bucket: bucket, Prefix: prefix}

	if c.STS != nil {
		if out, err := c.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}); err == nil {
			r.Account = aws.ToString(out.Account)
			r.CallerARN = aws.ToString(out.Arn)
		}
	}

	r.Findings = append(r.Findings,
		c.checkPublicAccessBlock(ctx, bucket),
		c.checkPolicyStatus(ctx, bucket),
		c.checkBucketPolicy(ctx, bucket),
		c.checkBucketACL(ctx, bucket),
		c.checkOwnership(ctx, bucket),
	)
	r.Findings = append(r.Findings, c.checkKMS(ctx, bucket))

	return r, nil
}

// apiErrCode extracts the AWS API error code (e.g. "AccessDenied") from err,
// or "" if err is not a recognized API error.
func apiErrCode(err error) string {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode()
	}
	return ""
}

// isAccessDenied reports whether err is an access-denied API error (S3 uses
// "AccessDenied", KMS "AccessDeniedException").
func isAccessDenied(err error) bool {
	switch apiErrCode(err) {
	case "AccessDenied", "AccessDeniedException", "Forbidden":
		return true
	}
	return false
}

// denied builds the standard "couldn't check — missing permission" finding.
func denied(check, action string) Finding {
	return Finding{
		Check:       check,
		Severity:    SeverityUnknown,
		Summary:     "Could not determine posture (access denied)",
		Remediation: fmt.Sprintf("Grant %s to the calling principal to include this check.", action),
	}
}

func (c *Checker) checkPublicAccessBlock(ctx context.Context, bucket string) Finding {
	const check = "block-public-access"
	out, err := c.S3.GetPublicAccessBlock(ctx, &s3.GetPublicAccessBlockInput{Bucket: aws.String(bucket)})
	if err != nil {
		if isAccessDenied(err) {
			return denied(check, "s3:GetBucketPublicAccessBlock")
		}
		if apiErrCode(err) == "NoSuchPublicAccessBlockConfiguration" {
			return Finding{
				Check:       check,
				Severity:    SeverityWarn,
				Summary:     "No bucket-level Block Public Access configuration",
				Detail:      "The bucket has no BPA config of its own; only account-level BPA (if any) protects it.",
				Remediation: "Enable Block Public Access on the bucket (all four settings) unless public access is intended.",
			}
		}
		return Finding{Check: check, Severity: SeverityUnknown, Summary: "Block Public Access check failed", Detail: err.Error()}
	}
	cfg := out.PublicAccessBlockConfiguration
	if cfg != nil && aws.ToBool(cfg.BlockPublicAcls) && aws.ToBool(cfg.IgnorePublicAcls) &&
		aws.ToBool(cfg.BlockPublicPolicy) && aws.ToBool(cfg.RestrictPublicBuckets) {
		return Finding{Check: check, Severity: SeverityOK, Summary: "Block Public Access fully enabled"}
	}
	return Finding{
		Check:       check,
		Severity:    SeverityWarn,
		Summary:     "Block Public Access is not fully enabled",
		Detail:      fmt.Sprintf("BlockPublicAcls=%t IgnorePublicAcls=%t BlockPublicPolicy=%t RestrictPublicBuckets=%t", aws.ToBool(cfg.BlockPublicAcls), aws.ToBool(cfg.IgnorePublicAcls), aws.ToBool(cfg.BlockPublicPolicy), aws.ToBool(cfg.RestrictPublicBuckets)),
		Remediation: "Enable all four Block Public Access settings unless public access is intended.",
	}
}

func (c *Checker) checkPolicyStatus(ctx context.Context, bucket string) Finding {
	const check = "bucket-public-status"
	out, err := c.S3.GetBucketPolicyStatus(ctx, &s3.GetBucketPolicyStatusInput{Bucket: aws.String(bucket)})
	if err != nil {
		if isAccessDenied(err) {
			return denied(check, "s3:GetBucketPolicyStatus")
		}
		// No policy → not public via policy.
		if apiErrCode(err) == "NoSuchBucketPolicy" {
			return Finding{Check: check, Severity: SeverityOK, Summary: "Bucket is not public (no policy)"}
		}
		return Finding{Check: check, Severity: SeverityUnknown, Summary: "Public-status check failed", Detail: err.Error()}
	}
	if out.PolicyStatus != nil && aws.ToBool(out.PolicyStatus.IsPublic) {
		return Finding{
			Check:       check,
			Severity:    SeverityCritical,
			Summary:     "Bucket is PUBLIC per its policy status",
			Remediation: "Remove the public grant from the bucket policy or enable Block Public Access.",
		}
	}
	return Finding{Check: check, Severity: SeverityOK, Summary: "Bucket is not public"}
}

func (c *Checker) checkBucketPolicy(ctx context.Context, bucket string) Finding {
	const check = "bucket-policy"
	out, err := c.S3.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: aws.String(bucket)})
	if err != nil {
		if isAccessDenied(err) {
			return denied(check, "s3:GetBucketPolicy")
		}
		if apiErrCode(err) == "NoSuchBucketPolicy" {
			return Finding{Check: check, Severity: SeverityOK, Summary: "No bucket policy (no policy-based grants)"}
		}
		return Finding{Check: check, Severity: SeverityUnknown, Summary: "Bucket policy check failed", Detail: err.Error()}
	}
	wildcards := wildcardPrincipalStatements(aws.ToString(out.Policy))
	if len(wildcards) > 0 {
		return Finding{
			Check:       check,
			Severity:    SeverityCritical,
			Summary:     "Bucket policy grants access to a wildcard principal",
			Detail:      fmt.Sprintf("%d Allow statement(s) with Principal \"*\": %s", len(wildcards), strings.Join(wildcards, ", ")),
			Remediation: "Scope the bucket policy Principal to specific accounts/roles, or add a Condition that restricts it.",
		}
	}
	return Finding{Check: check, Severity: SeverityOK, Summary: "Bucket policy has no wildcard-principal Allow statements"}
}

func (c *Checker) checkBucketACL(ctx context.Context, bucket string) Finding {
	const check = "bucket-acl"
	out, err := c.S3.GetBucketAcl(ctx, &s3.GetBucketAclInput{Bucket: aws.String(bucket)})
	if err != nil {
		if isAccessDenied(err) {
			return denied(check, "s3:GetBucketAcl")
		}
		return Finding{Check: check, Severity: SeverityUnknown, Summary: "Bucket ACL check failed", Detail: err.Error()}
	}
	const (
		allUsers  = "http://acs.amazonaws.com/groups/global/AllUsers"
		authUsers = "http://acs.amazonaws.com/groups/global/AuthenticatedUsers"
	)
	var public, authenticated []string
	for _, g := range out.Grants {
		if g.Grantee == nil || g.Grantee.Type != types.TypeGroup {
			continue
		}
		switch aws.ToString(g.Grantee.URI) {
		case allUsers:
			public = append(public, string(g.Permission))
		case authUsers:
			authenticated = append(authenticated, string(g.Permission))
		}
	}
	if len(public) > 0 {
		return Finding{
			Check:       check,
			Severity:    SeverityCritical,
			Summary:     "Bucket ACL grants access to everyone (AllUsers)",
			Detail:      "Permissions: " + strings.Join(public, ", "),
			Remediation: "Remove the AllUsers grant; prefer Block Public Access with ACLs disabled (BucketOwnerEnforced).",
		}
	}
	if len(authenticated) > 0 {
		return Finding{
			Check:       check,
			Severity:    SeverityWarn,
			Summary:     "Bucket ACL grants access to any authenticated AWS account",
			Detail:      "Permissions: " + strings.Join(authenticated, ", "),
			Remediation: "Remove the AuthenticatedUsers grant; it allows any AWS account, not just yours.",
		}
	}
	return Finding{Check: check, Severity: SeverityOK, Summary: "Bucket ACL has no public or cross-account group grants"}
}

func (c *Checker) checkOwnership(ctx context.Context, bucket string) Finding {
	const check = "object-ownership"
	out, err := c.S3.GetBucketOwnershipControls(ctx, &s3.GetBucketOwnershipControlsInput{Bucket: aws.String(bucket)})
	if err != nil {
		if isAccessDenied(err) {
			return denied(check, "s3:GetBucketOwnershipControls")
		}
		if apiErrCode(err) == "OwnershipControlsNotFoundError" {
			return Finding{
				Check:    check,
				Severity: SeverityInfo,
				Summary:  "No object-ownership controls set (ACLs enabled by default)",
			}
		}
		return Finding{Check: check, Severity: SeverityUnknown, Summary: "Ownership controls check failed", Detail: err.Error()}
	}
	if out.OwnershipControls != nil {
		for _, rule := range out.OwnershipControls.Rules {
			if rule.ObjectOwnership == types.ObjectOwnershipBucketOwnerEnforced {
				return Finding{Check: check, Severity: SeverityOK, Summary: "Object ownership is BucketOwnerEnforced (ACLs disabled)"}
			}
		}
	}
	return Finding{
		Check:       check,
		Severity:    SeverityInfo,
		Summary:     "Object ACLs are enabled (ownership not BucketOwnerEnforced)",
		Remediation: "Consider setting ObjectOwnership=BucketOwnerEnforced to disable ACLs entirely.",
	}
}

// checkKMS inspects the key policy of the bucket's default SSE-KMS key (or the
// KMSKeyID override). It returns a single finding; when there is nothing to
// inspect (no KMS client, no default KMS encryption) the finding is Info.
func (c *Checker) checkKMS(ctx context.Context, bucket string) Finding {
	const check = "kms-key-policy"
	if c.KMS == nil {
		return Finding{Check: check, Severity: SeverityInfo, Summary: "KMS key-policy check skipped (KMS inspection not enabled)"}
	}

	keyID := c.KMSKeyID
	if keyID == "" {
		var err error
		keyID, err = c.defaultBucketKMSKey(ctx, bucket)
		if err != nil {
			if isAccessDenied(err) {
				return denied(check, "s3:GetEncryptionConfiguration")
			}
			return Finding{Check: check, Severity: SeverityUnknown, Summary: "Could not read bucket encryption configuration", Detail: err.Error()}
		}
		if keyID == "" {
			return Finding{Check: check, Severity: SeverityInfo, Summary: "No default SSE-KMS key on the bucket to inspect"}
		}
	}

	pol, err := c.KMS.GetKeyPolicy(ctx, &kms.GetKeyPolicyInput{KeyId: aws.String(keyID), PolicyName: aws.String("default")})
	if err != nil {
		if isAccessDenied(err) {
			return denied(check, "kms:GetKeyPolicy")
		}
		return Finding{Check: check, Severity: SeverityUnknown, Summary: "KMS key-policy check failed", Detail: err.Error()}
	}
	wildcards := wildcardPrincipalStatements(aws.ToString(pol.Policy))
	if len(wildcards) > 0 {
		return Finding{
			Check:       check,
			Severity:    SeverityWarn,
			Summary:     "KMS key policy has wildcard-principal Allow statements",
			Detail:      fmt.Sprintf("key %s: %d statement(s) with Principal \"*\" — verify a Condition scopes them: %s", keyID, len(wildcards), strings.Join(wildcards, ", ")),
			Remediation: "Ensure any Principal \"*\" statement in the key policy is constrained by a Condition (e.g. kms:ViaService, aws:PrincipalOrgID).",
		}
	}
	return Finding{Check: check, Severity: SeverityOK, Summary: fmt.Sprintf("KMS key %s policy has no unconditioned wildcard-principal grants", keyID)}
}

// defaultBucketKMSKey returns the KMS key ID from the bucket's default SSE-KMS
// encryption rule, or "" if the bucket has no SSE-KMS default.
func (c *Checker) defaultBucketKMSKey(ctx context.Context, bucket string) (string, error) {
	out, err := c.S3.GetBucketEncryption(ctx, &s3.GetBucketEncryptionInput{Bucket: aws.String(bucket)})
	if err != nil {
		if apiErrCode(err) == "ServerSideEncryptionConfigurationNotFoundError" {
			return "", nil
		}
		return "", err
	}
	if out.ServerSideEncryptionConfiguration == nil {
		return "", nil
	}
	for _, rule := range out.ServerSideEncryptionConfiguration.Rules {
		d := rule.ApplyServerSideEncryptionByDefault
		if d != nil && d.SSEAlgorithm == types.ServerSideEncryptionAwsKms {
			return aws.ToString(d.KMSMasterKeyID), nil
		}
	}
	return "", nil
}

// policyDocument is the minimal shape of an IAM/S3/KMS policy we parse. The
// Principal and Action fields are polymorphic in JSON (string or list/object),
// so they are decoded as json.RawMessage and interpreted by helpers.
type policyDocument struct {
	Statement []struct {
		Sid       string          `json:"Sid"`
		Effect    string          `json:"Effect"`
		Principal json.RawMessage `json:"Principal"`
		Condition json.RawMessage `json:"Condition"`
	} `json:"Statement"`
}

// wildcardPrincipalStatements returns the Sid (or index) of every Allow
// statement whose Principal is the wildcard "*" (or {"AWS":"*"}) AND which
// carries no Condition. Statements gated by a Condition are treated as
// intentionally scoped and excluded — a Principal "*" with a Condition is a
// common, legitimate pattern (e.g. kms:ViaService, aws:SourceArn).
func wildcardPrincipalStatements(policyJSON string) []string {
	policyJSON = strings.TrimSpace(policyJSON)
	if policyJSON == "" {
		return nil
	}
	// Bucket-policy JSON may arrive URL-encoded from some APIs; decode best-effort.
	if !strings.HasPrefix(policyJSON, "{") {
		if dec, err := url.QueryUnescape(policyJSON); err == nil {
			policyJSON = dec
		}
	}
	var doc policyDocument
	if err := json.Unmarshal([]byte(policyJSON), &doc); err != nil {
		return nil
	}
	var hits []string
	for i, st := range doc.Statement {
		if !strings.EqualFold(st.Effect, "Allow") {
			continue
		}
		if len(st.Condition) > 0 && string(st.Condition) != "null" && string(st.Condition) != "{}" {
			continue // conditioned wildcard — treated as intentionally scoped
		}
		if principalIsWildcard(st.Principal) {
			label := st.Sid
			if label == "" {
				label = fmt.Sprintf("statement[%d]", i)
			}
			hits = append(hits, label)
		}
	}
	return hits
}

// principalIsWildcard reports whether a policy Principal is the unrestricted
// wildcard, in any of its JSON encodings: "*" or {"AWS":"*"} (or a list
// containing "*").
func principalIsWildcard(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	// Form 1: "Principal": "*"
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s == "*"
	}
	// Form 2: "Principal": {"AWS": "*"} or {"AWS": ["*", ...]}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err == nil {
		for _, v := range m {
			var vs string
			if err := json.Unmarshal(v, &vs); err == nil {
				if vs == "*" {
					return true
				}
				continue
			}
			var vl []string
			if err := json.Unmarshal(v, &vl); err == nil {
				for _, item := range vl {
					if item == "*" {
						return true
					}
				}
			}
		}
	}
	return false
}
