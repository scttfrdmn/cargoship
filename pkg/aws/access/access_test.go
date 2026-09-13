package access

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	smithy "github.com/aws/smithy-go"
)

// apiErr builds a smithy API error with the given code, matching how the SDK
// surfaces service errors (AccessDenied, NoSuchBucketPolicy, ...).
func apiErr(code string) error { return &smithy.GenericAPIError{Code: code, Message: code} }

// mockS3 is a configurable S3PolicyAPI. A nil func field yields a "clean,
// locked-down" default so a test overrides only the behavior it exercises.
type mockS3 struct {
	pab func() (*s3.GetPublicAccessBlockOutput, error)
	ps  func() (*s3.GetBucketPolicyStatusOutput, error)
	pol func() (*s3.GetBucketPolicyOutput, error)
	acl func() (*s3.GetBucketAclOutput, error)
	own func() (*s3.GetBucketOwnershipControlsOutput, error)
	enc func() (*s3.GetBucketEncryptionOutput, error)
}

func allTrue() *types.PublicAccessBlockConfiguration {
	return &types.PublicAccessBlockConfiguration{
		BlockPublicAcls:       aws.Bool(true),
		IgnorePublicAcls:      aws.Bool(true),
		BlockPublicPolicy:     aws.Bool(true),
		RestrictPublicBuckets: aws.Bool(true),
	}
}

func (m *mockS3) GetPublicAccessBlock(context.Context, *s3.GetPublicAccessBlockInput, ...func(*s3.Options)) (*s3.GetPublicAccessBlockOutput, error) {
	if m.pab != nil {
		return m.pab()
	}
	return &s3.GetPublicAccessBlockOutput{PublicAccessBlockConfiguration: allTrue()}, nil
}

func (m *mockS3) GetBucketPolicyStatus(context.Context, *s3.GetBucketPolicyStatusInput, ...func(*s3.Options)) (*s3.GetBucketPolicyStatusOutput, error) {
	if m.ps != nil {
		return m.ps()
	}
	return &s3.GetBucketPolicyStatusOutput{PolicyStatus: &types.PolicyStatus{IsPublic: aws.Bool(false)}}, nil
}

func (m *mockS3) GetBucketPolicy(context.Context, *s3.GetBucketPolicyInput, ...func(*s3.Options)) (*s3.GetBucketPolicyOutput, error) {
	if m.pol != nil {
		return m.pol()
	}
	return nil, apiErr("NoSuchBucketPolicy")
}

func (m *mockS3) GetBucketAcl(context.Context, *s3.GetBucketAclInput, ...func(*s3.Options)) (*s3.GetBucketAclOutput, error) {
	if m.acl != nil {
		return m.acl()
	}
	return &s3.GetBucketAclOutput{}, nil
}

func (m *mockS3) GetBucketOwnershipControls(context.Context, *s3.GetBucketOwnershipControlsInput, ...func(*s3.Options)) (*s3.GetBucketOwnershipControlsOutput, error) {
	if m.own != nil {
		return m.own()
	}
	return &s3.GetBucketOwnershipControlsOutput{OwnershipControls: &types.OwnershipControls{
		Rules: []types.OwnershipControlsRule{{ObjectOwnership: types.ObjectOwnershipBucketOwnerEnforced}},
	}}, nil
}

func (m *mockS3) GetBucketEncryption(context.Context, *s3.GetBucketEncryptionInput, ...func(*s3.Options)) (*s3.GetBucketEncryptionOutput, error) {
	if m.enc != nil {
		return m.enc()
	}
	return nil, apiErr("ServerSideEncryptionConfigurationNotFoundError")
}

type mockKMS struct {
	pol func() (*kms.GetKeyPolicyOutput, error)
}

func (m *mockKMS) GetKeyPolicy(context.Context, *kms.GetKeyPolicyInput, ...func(*kms.Options)) (*kms.GetKeyPolicyOutput, error) {
	return m.pol()
}

type mockSTS struct{}

func (m *mockSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return &sts.GetCallerIdentityOutput{Account: aws.String("111122223333"), Arn: aws.String("arn:aws:iam::111122223333:role/writer")}, nil
}

// findingFor returns the finding for a given check key.
func findingFor(r *Report, check string) (Finding, bool) {
	for _, f := range r.Findings {
		if f.Check == check {
			return f, true
		}
	}
	return Finding{}, false
}

func TestChecker_LockedDown(t *testing.T) {
	c := &Checker{S3: &mockS3{}, STS: &mockSTS{}}
	r, err := c.Check(context.Background(), "bucket", "prefix")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if r.Account != "111122223333" || r.CallerARN == "" {
		t.Errorf("caller identity not populated: %+v", r)
	}
	if w := r.Worst(); w.rank() > SeverityInfo.rank() {
		t.Errorf("locked-down bucket should have no warn/critical/unknown, got worst=%s", w)
	}
	for _, check := range []string{"block-public-access", "bucket-public-status", "bucket-policy", "bucket-acl", "object-ownership"} {
		f, ok := findingFor(r, check)
		if !ok {
			t.Fatalf("missing finding %q", check)
		}
		if f.Severity != SeverityOK {
			t.Errorf("check %q: want ok, got %s (%s)", check, f.Severity, f.Summary)
		}
	}
}

func TestChecker_PerCheckSeverity(t *testing.T) {
	publicPolicy := `{"Statement":[{"Sid":"Open","Effect":"Allow","Principal":"*","Action":"s3:GetObject"}]}`
	conditionedPolicy := `{"Statement":[{"Sid":"Scoped","Effect":"Allow","Principal":"*","Action":"s3:GetObject","Condition":{"StringEquals":{"aws:SourceVpce":"vpce-123"}}}]}`

	tests := []struct {
		name    string
		s3      *mockS3
		check   string
		wantSev Severity
		wantRem bool // expect a non-empty Remediation
	}{
		{
			name: "partial BPA -> warn",
			s3: &mockS3{pab: func() (*s3.GetPublicAccessBlockOutput, error) {
				c := allTrue()
				c.BlockPublicPolicy = aws.Bool(false)
				return &s3.GetPublicAccessBlockOutput{PublicAccessBlockConfiguration: c}, nil
			}},
			check:   "block-public-access",
			wantSev: SeverityWarn,
			wantRem: true,
		},
		{
			name: "no BPA config -> warn",
			s3: &mockS3{pab: func() (*s3.GetPublicAccessBlockOutput, error) {
				return nil, apiErr("NoSuchPublicAccessBlockConfiguration")
			}},
			check:   "block-public-access",
			wantSev: SeverityWarn,
		},
		{
			name:    "BPA access denied -> unknown",
			s3:      &mockS3{pab: func() (*s3.GetPublicAccessBlockOutput, error) { return nil, apiErr("AccessDenied") }},
			check:   "block-public-access",
			wantSev: SeverityUnknown,
			wantRem: true,
		},
		{
			name: "public policy status -> critical",
			s3: &mockS3{ps: func() (*s3.GetBucketPolicyStatusOutput, error) {
				return &s3.GetBucketPolicyStatusOutput{PolicyStatus: &types.PolicyStatus{IsPublic: aws.Bool(true)}}, nil
			}},
			check:   "bucket-public-status",
			wantSev: SeverityCritical,
		},
		{
			name: "wildcard bucket policy -> critical",
			s3: &mockS3{pol: func() (*s3.GetBucketPolicyOutput, error) {
				return &s3.GetBucketPolicyOutput{Policy: aws.String(publicPolicy)}, nil
			}},
			check:   "bucket-policy",
			wantSev: SeverityCritical,
		},
		{
			name: "conditioned wildcard policy -> ok",
			s3: &mockS3{pol: func() (*s3.GetBucketPolicyOutput, error) {
				return &s3.GetBucketPolicyOutput{Policy: aws.String(conditionedPolicy)}, nil
			}},
			check:   "bucket-policy",
			wantSev: SeverityOK,
		},
		{
			name: "public ACL (AllUsers) -> critical",
			s3: &mockS3{acl: func() (*s3.GetBucketAclOutput, error) {
				return &s3.GetBucketAclOutput{Grants: []types.Grant{{
					Grantee:    &types.Grantee{Type: types.TypeGroup, URI: aws.String("http://acs.amazonaws.com/groups/global/AllUsers")},
					Permission: types.PermissionRead,
				}}}, nil
			}},
			check:   "bucket-acl",
			wantSev: SeverityCritical,
		},
		{
			name: "authenticated-users ACL -> warn",
			s3: &mockS3{acl: func() (*s3.GetBucketAclOutput, error) {
				return &s3.GetBucketAclOutput{Grants: []types.Grant{{
					Grantee:    &types.Grantee{Type: types.TypeGroup, URI: aws.String("http://acs.amazonaws.com/groups/global/AuthenticatedUsers")},
					Permission: types.PermissionRead,
				}}}, nil
			}},
			check:   "bucket-acl",
			wantSev: SeverityWarn,
		},
		{
			name: "ownership not enforced -> info",
			s3: &mockS3{own: func() (*s3.GetBucketOwnershipControlsOutput, error) {
				return &s3.GetBucketOwnershipControlsOutput{OwnershipControls: &types.OwnershipControls{
					Rules: []types.OwnershipControlsRule{{ObjectOwnership: types.ObjectOwnershipObjectWriter}},
				}}, nil
			}},
			check:   "object-ownership",
			wantSev: SeverityInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Checker{S3: tt.s3}
			r, err := c.Check(context.Background(), "bucket", "")
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			f, ok := findingFor(r, tt.check)
			if !ok {
				t.Fatalf("missing finding %q", tt.check)
			}
			if f.Severity != tt.wantSev {
				t.Errorf("check %q: want %s, got %s (%s / %s)", tt.check, tt.wantSev, f.Severity, f.Summary, f.Detail)
			}
			if tt.wantRem && f.Remediation == "" {
				t.Errorf("check %q: expected a remediation, got none", tt.check)
			}
		})
	}
}

func TestChecker_KMS(t *testing.T) {
	kmsBucket := &mockS3{enc: func() (*s3.GetBucketEncryptionOutput, error) {
		return &s3.GetBucketEncryptionOutput{ServerSideEncryptionConfiguration: &types.ServerSideEncryptionConfiguration{
			Rules: []types.ServerSideEncryptionRule{{ApplyServerSideEncryptionByDefault: &types.ServerSideEncryptionByDefault{
				SSEAlgorithm: types.ServerSideEncryptionAwsKms, KMSMasterKeyID: aws.String("key-abc"),
			}}},
		}}, nil
	}}

	t.Run("no KMS client -> info skipped", func(t *testing.T) {
		c := &Checker{S3: &mockS3{}}
		r, _ := c.Check(context.Background(), "b", "")
		f, _ := findingFor(r, "kms-key-policy")
		if f.Severity != SeverityInfo {
			t.Errorf("want info, got %s", f.Severity)
		}
	})

	t.Run("wildcard key policy -> warn", func(t *testing.T) {
		c := &Checker{S3: kmsBucket, KMS: &mockKMS{pol: func() (*kms.GetKeyPolicyOutput, error) {
			return &kms.GetKeyPolicyOutput{Policy: aws.String(`{"Statement":[{"Effect":"Allow","Principal":"*","Action":"kms:Decrypt"}]}`)}, nil
		}}}
		r, _ := c.Check(context.Background(), "b", "")
		f, _ := findingFor(r, "kms-key-policy")
		if f.Severity != SeverityWarn {
			t.Errorf("want warn, got %s (%s)", f.Severity, f.Summary)
		}
	})

	t.Run("clean key policy -> ok", func(t *testing.T) {
		c := &Checker{S3: kmsBucket, KMS: &mockKMS{pol: func() (*kms.GetKeyPolicyOutput, error) {
			return &kms.GetKeyPolicyOutput{Policy: aws.String(`{"Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::111122223333:root"},"Action":"kms:*"}]}`)}, nil
		}}}
		r, _ := c.Check(context.Background(), "b", "")
		f, _ := findingFor(r, "kms-key-policy")
		if f.Severity != SeverityOK {
			t.Errorf("want ok, got %s (%s)", f.Severity, f.Summary)
		}
	})

	t.Run("no SSE-KMS default -> info", func(t *testing.T) {
		c := &Checker{S3: &mockS3{}, KMS: &mockKMS{pol: func() (*kms.GetKeyPolicyOutput, error) { t.Fatal("should not call KMS"); return nil, nil }}}
		r, _ := c.Check(context.Background(), "b", "")
		f, _ := findingFor(r, "kms-key-policy")
		if f.Severity != SeverityInfo {
			t.Errorf("want info, got %s", f.Severity)
		}
	})
}

func TestAnalyzePolicy(t *testing.T) {
	tests := []struct {
		name    string
		policy  string
		wantHit int
		wantOK  bool
	}{
		{"empty", "", 0, true},
		{"star string", `{"Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:*"}]}`, 1, true},
		{"aws star object", `{"Statement":[{"Effect":"Allow","Principal":{"AWS":"*"},"Action":"s3:*"}]}`, 1, true},
		{"aws star in list", `{"Statement":[{"Effect":"Allow","Principal":{"AWS":["arn:x","*"]},"Action":"s3:*"}]}`, 1, true},
		{"deny star ignored", `{"Statement":[{"Effect":"Deny","Principal":"*","Action":"s3:*"}]}`, 0, true},
		{"scoped principal ok", `{"Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::1:root"},"Action":"s3:*"}]}`, 0, true},

		// F1: a SINGLE-OBJECT Statement (not an array) with a wildcard must be
		// detected — previously it failed to parse and reported clean.
		{"single object statement wildcard", `{"Statement":{"Effect":"Allow","Principal":"*","Action":"s3:GetObject"}}`, 1, true},
		{"single object statement scoped", `{"Statement":{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::1:root"},"Action":"s3:*"}}`, 0, true},

		// F1: an unparseable policy must be reported as UNKNOWN (ok=false), never clean.
		{"not json -> not ok", `not json`, 0, false},
		{"statement wrong type -> not ok", `{"Statement": 42}`, 0, false},

		// F2: a wildcard gated only by a NON-narrowing condition is still public.
		{"secure-transport-only condition still flagged", `{"Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Condition":{"Bool":{"aws:SecureTransport":"true"}}}]}`, 1, true},
		// F2: a principal-narrowing condition genuinely scopes the wildcard.
		{"org-id condition excluded", `{"Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:*","Condition":{"StringEquals":{"aws:PrincipalOrgID":"o-1"}}}]}`, 0, true},
		{"kms viaservice condition excluded", `{"Statement":[{"Effect":"Allow","Principal":"*","Action":"kms:*","Condition":{"StringEquals":{"kms:ViaService":"s3.us-west-2.amazonaws.com"}}}]}`, 0, true},

		// F3: an Allow with NotPrincipal grants to everyone-except and is flagged.
		{"not-principal allow flagged", `{"Statement":[{"Effect":"Allow","NotPrincipal":{"AWS":"arn:aws:iam::1:root"},"Action":"s3:*"}]}`, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits, ok := analyzePolicy(tt.policy)
			if ok != tt.wantOK {
				t.Errorf("ok: want %v, got %v", tt.wantOK, ok)
			}
			if len(hits) != tt.wantHit {
				t.Errorf("want %d broad-grant statements, got %d (%v)", tt.wantHit, len(hits), hits)
			}
		})
	}
}

func TestReportWorst(t *testing.T) {
	r := &Report{Findings: []Finding{
		{Severity: SeverityOK}, {Severity: SeverityInfo}, {Severity: SeverityWarn}, {Severity: SeverityUnknown},
	}}
	if got := r.Worst(); got != SeverityWarn {
		t.Errorf("warn(3) outranks unknown(2): want warn, got %s", got)
	}
	r.Findings = append(r.Findings, Finding{Severity: SeverityCritical})
	if got := r.Worst(); got != SeverityCritical {
		t.Errorf("want critical, got %s", got)
	}
	if got := (&Report{}).Worst(); got != SeverityOK {
		t.Errorf("empty report: want ok, got %s", got)
	}
}
