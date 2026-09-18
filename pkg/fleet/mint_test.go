package fleet

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	smithy "github.com/aws/smithy-go"
)

// fakeIAM records the calls MintWriter/ScuttleWriterIAM make and serves canned
// list responses; per-verb error hooks exercise the failure paths.
type fakeIAM struct {
	calls []string

	createPolicyErr error
	createUserErr   error

	accessKeys       []string // returned by ListAccessKeys
	attachedPolicies []iamtypes.AttachedPolicy
	listKeysErr      error

	deletedKeys     []string
	detached        []string
	deletedPolicies []string
	deletedUser     bool
}

func (f *fakeIAM) CreatePolicy(_ context.Context, in *iam.CreatePolicyInput, _ ...func(*iam.Options)) (*iam.CreatePolicyOutput, error) {
	f.calls = append(f.calls, "CreatePolicy:"+aws.ToString(in.PolicyName))
	if f.createPolicyErr != nil {
		return nil, f.createPolicyErr
	}
	arn := "arn:aws:iam::123456789012:policy" + aws.ToString(in.Path) + aws.ToString(in.PolicyName)
	return &iam.CreatePolicyOutput{Policy: &iamtypes.Policy{Arn: aws.String(arn)}}, nil
}
func (f *fakeIAM) CreateUser(_ context.Context, in *iam.CreateUserInput, _ ...func(*iam.Options)) (*iam.CreateUserOutput, error) {
	f.calls = append(f.calls, "CreateUser:"+aws.ToString(in.UserName))
	if f.createUserErr != nil {
		return nil, f.createUserErr
	}
	return &iam.CreateUserOutput{}, nil
}
func (f *fakeIAM) AttachUserPolicy(_ context.Context, in *iam.AttachUserPolicyInput, _ ...func(*iam.Options)) (*iam.AttachUserPolicyOutput, error) {
	f.calls = append(f.calls, "AttachUserPolicy:"+aws.ToString(in.PolicyArn))
	return &iam.AttachUserPolicyOutput{}, nil
}
func (f *fakeIAM) CreateAccessKey(_ context.Context, in *iam.CreateAccessKeyInput, _ ...func(*iam.Options)) (*iam.CreateAccessKeyOutput, error) {
	f.calls = append(f.calls, "CreateAccessKey:"+aws.ToString(in.UserName))
	return &iam.CreateAccessKeyOutput{AccessKey: &iamtypes.AccessKey{
		AccessKeyId: aws.String("AKIAEXAMPLE"), SecretAccessKey: aws.String("secret123"),
	}}, nil
}
func (f *fakeIAM) ListAccessKeys(_ context.Context, in *iam.ListAccessKeysInput, _ ...func(*iam.Options)) (*iam.ListAccessKeysOutput, error) {
	f.calls = append(f.calls, "ListAccessKeys:"+aws.ToString(in.UserName))
	if f.listKeysErr != nil {
		return nil, f.listKeysErr
	}
	var md []iamtypes.AccessKeyMetadata
	for _, k := range f.accessKeys {
		md = append(md, iamtypes.AccessKeyMetadata{AccessKeyId: aws.String(k)})
	}
	return &iam.ListAccessKeysOutput{AccessKeyMetadata: md}, nil
}
func (f *fakeIAM) DeleteAccessKey(_ context.Context, in *iam.DeleteAccessKeyInput, _ ...func(*iam.Options)) (*iam.DeleteAccessKeyOutput, error) {
	f.deletedKeys = append(f.deletedKeys, aws.ToString(in.AccessKeyId))
	return &iam.DeleteAccessKeyOutput{}, nil
}
func (f *fakeIAM) ListAttachedUserPolicies(_ context.Context, in *iam.ListAttachedUserPoliciesInput, _ ...func(*iam.Options)) (*iam.ListAttachedUserPoliciesOutput, error) {
	return &iam.ListAttachedUserPoliciesOutput{AttachedPolicies: f.attachedPolicies}, nil
}
func (f *fakeIAM) DetachUserPolicy(_ context.Context, in *iam.DetachUserPolicyInput, _ ...func(*iam.Options)) (*iam.DetachUserPolicyOutput, error) {
	f.detached = append(f.detached, aws.ToString(in.PolicyArn))
	return &iam.DetachUserPolicyOutput{}, nil
}
func (f *fakeIAM) DeletePolicy(_ context.Context, in *iam.DeletePolicyInput, _ ...func(*iam.Options)) (*iam.DeletePolicyOutput, error) {
	f.deletedPolicies = append(f.deletedPolicies, aws.ToString(in.PolicyArn))
	return &iam.DeletePolicyOutput{}, nil
}
func (f *fakeIAM) DeleteUser(_ context.Context, in *iam.DeleteUserInput, _ ...func(*iam.Options)) (*iam.DeleteUserOutput, error) {
	f.deletedUser = true
	return &iam.DeleteUserOutput{}, nil
}

func iamAPIErr(code string) error { return &smithy.GenericAPIError{Code: code, Message: code} }

func TestMintWriter_HappyPath(t *testing.T) {
	f := &fakeIAM{}
	res, err := MintWriter(context.Background(), f, "lab-nas-1", `{"Version":"2012-10-17"}`)
	if err != nil {
		t.Fatalf("MintWriter: %v", err)
	}
	if res.UserName != "cargoship-writer-lab-nas-1" {
		t.Errorf("user name = %q", res.UserName)
	}
	if res.AccessKeyID != "AKIAEXAMPLE" || res.SecretAccessKey != "secret123" {
		t.Errorf("key not returned: %+v", res)
	}
	// Order matters: policy → user → attach → key.
	want := []string{
		"CreatePolicy:cargoship-writer-lab-nas-1",
		"CreateUser:cargoship-writer-lab-nas-1",
		"AttachUserPolicy:" + res.PolicyARN,
		"CreateAccessKey:cargoship-writer-lab-nas-1",
	}
	if strings.Join(f.calls, ",") != strings.Join(want, ",") {
		t.Errorf("call sequence = %v, want %v", f.calls, want)
	}
}

func TestMintWriter_RejectsLongID(t *testing.T) {
	long := strings.Repeat("a", maxMintWriterID+1)
	if _, err := MintWriter(context.Background(), &fakeIAM{}, long, "{}"); err == nil {
		t.Fatal("expected error for an over-long writer id")
	}
}

func TestMintWriter_AlreadyExistsFailsClosed(t *testing.T) {
	f := &fakeIAM{createPolicyErr: iamAPIErr("EntityAlreadyExists")}
	_, err := MintWriter(context.Background(), f, "dup", "{}")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already-exists error, got %v", err)
	}
	// Must not have proceeded to create a user.
	for _, c := range f.calls {
		if strings.HasPrefix(c, "CreateUser") {
			t.Error("must not create user when the policy already exists")
		}
	}
}

func TestScuttleWriterIAM_FullTeardown(t *testing.T) {
	name := "cargoship-writer-w1"
	ownARN := "arn:aws:iam::123456789012:policy/cargoship/" + name
	otherARN := "arn:aws:iam::123456789012:policy/SomeOtherPolicy"
	f := &fakeIAM{
		accessKeys: []string{"AKIA1", "AKIA2"},
		attachedPolicies: []iamtypes.AttachedPolicy{
			{PolicyName: aws.String(name), PolicyArn: aws.String(ownARN)},
			{PolicyName: aws.String("SomeOtherPolicy"), PolicyArn: aws.String(otherARN)},
		},
	}
	if err := ScuttleWriterIAM(context.Background(), f, "w1"); err != nil {
		t.Fatalf("ScuttleWriterIAM: %v", err)
	}
	if len(f.deletedKeys) != 2 {
		t.Errorf("want 2 keys deleted, got %v", f.deletedKeys)
	}
	if len(f.detached) != 2 {
		t.Errorf("both attached policies should be detached, got %v", f.detached)
	}
	if !f.deletedUser {
		t.Error("user should be deleted")
	}
	// Only cargoship's own policy is deleted; the operator's other policy is left intact.
	if len(f.deletedPolicies) != 1 || f.deletedPolicies[0] != ownARN {
		t.Errorf("only the own policy should be deleted, got %v", f.deletedPolicies)
	}
}

func TestScuttleWriterIAM_MissingUserIsIdempotent(t *testing.T) {
	f := &fakeIAM{listKeysErr: iamAPIErr("NoSuchEntity")}
	if err := ScuttleWriterIAM(context.Background(), f, "gone"); err != nil {
		t.Fatalf("scuttling an already-gone writer should be a no-op, got %v", err)
	}
	if f.deletedUser {
		t.Error("should not attempt DeleteUser when the user is already gone")
	}
}
