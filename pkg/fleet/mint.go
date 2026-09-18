package fleet

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	smithy "github.com/aws/smithy-go"
)

// iamNamePrefix is the fixed prefix for the IAM user + customer-managed policy that
// back one fleet writer. The writer id is appended; the result must fit IAM's 64-char
// user-name limit, so a writer id longer than maxMintWriterID can't be minted.
const iamNamePrefix = "cargoship-writer-"

// iamPath groups all cargoship-minted identities under one IAM path for easy audit.
const iamPath = "/cargoship/"

// maxMintWriterID is the longest writer id that yields a <=64-char IAM user name.
const maxMintWriterID = 64 - len(iamNamePrefix) // 47

// WriterIdentityName is the IAM user + policy name for a writer (cargoship-writer-<id>).
func WriterIdentityName(writerID string) string { return iamNamePrefix + writerID }

// IAMWriterAPI is the minimal IAM surface for minting and scuttling a fleet writer's
// identity; *iam.Client satisfies it. Narrow so tests can fake it.
type IAMWriterAPI interface {
	CreatePolicy(ctx context.Context, in *iam.CreatePolicyInput, optFns ...func(*iam.Options)) (*iam.CreatePolicyOutput, error)
	CreateUser(ctx context.Context, in *iam.CreateUserInput, optFns ...func(*iam.Options)) (*iam.CreateUserOutput, error)
	AttachUserPolicy(ctx context.Context, in *iam.AttachUserPolicyInput, optFns ...func(*iam.Options)) (*iam.AttachUserPolicyOutput, error)
	CreateAccessKey(ctx context.Context, in *iam.CreateAccessKeyInput, optFns ...func(*iam.Options)) (*iam.CreateAccessKeyOutput, error)
	ListAccessKeys(ctx context.Context, in *iam.ListAccessKeysInput, optFns ...func(*iam.Options)) (*iam.ListAccessKeysOutput, error)
	DeleteAccessKey(ctx context.Context, in *iam.DeleteAccessKeyInput, optFns ...func(*iam.Options)) (*iam.DeleteAccessKeyOutput, error)
	ListAttachedUserPolicies(ctx context.Context, in *iam.ListAttachedUserPoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedUserPoliciesOutput, error)
	DetachUserPolicy(ctx context.Context, in *iam.DetachUserPolicyInput, optFns ...func(*iam.Options)) (*iam.DetachUserPolicyOutput, error)
	DeletePolicy(ctx context.Context, in *iam.DeletePolicyInput, optFns ...func(*iam.Options)) (*iam.DeletePolicyOutput, error)
	DeleteUser(ctx context.Context, in *iam.DeleteUserInput, optFns ...func(*iam.Options)) (*iam.DeleteUserOutput, error)
}

// MintResult is a freshly-minted writer identity. SecretAccessKey is returned exactly
// once by AWS (at creation) — the caller must persist it immediately.
type MintResult struct {
	UserName        string
	PolicyARN       string
	AccessKeyID     string
	SecretAccessKey string
}

// MintWriter provisions a write-only IAM identity for a fleet writer: a
// customer-managed policy (policyDoc, from WriterIAMPolicy) attached to a new user,
// plus an access key. It fails closed — if the user or policy already exists it errors
// rather than clobbering, so an existing writer is never silently re-keyed. On a
// mid-sequence failure the partial identity can be removed with ScuttleWriterIAM.
func MintWriter(ctx context.Context, api IAMWriterAPI, writerID, policyDoc string) (*MintResult, error) {
	if writerID == "" {
		return nil, errors.New("writer id is required to mint an identity")
	}
	if len(writerID) > maxMintWriterID {
		return nil, fmt.Errorf("writer id %q is too long to mint (max %d chars so the IAM user name fits 64)", writerID, maxMintWriterID)
	}
	name := WriterIdentityName(writerID)

	pol, err := api.CreatePolicy(ctx, &iam.CreatePolicyInput{
		PolicyName:     aws.String(name),
		Path:           aws.String(iamPath),
		PolicyDocument: aws.String(policyDoc),
		Description:    aws.String("Write-only CargoShip fleet writer " + writerID + " (cargoship ghostship)"),
	})
	if err != nil {
		if isEntityAlreadyExists(err) {
			return nil, fmt.Errorf("policy %s already exists — scuttle the existing writer first, or use a different --writer-id", name)
		}
		return nil, fmt.Errorf("create policy %s: %w", name, err)
	}
	policyARN := aws.ToString(pol.Policy.Arn)

	if _, err := api.CreateUser(ctx, &iam.CreateUserInput{
		UserName: aws.String(name),
		Path:     aws.String(iamPath),
	}); err != nil {
		if isEntityAlreadyExists(err) {
			return nil, fmt.Errorf("user %s already exists — scuttle the existing writer first, or use a different --writer-id", name)
		}
		return nil, fmt.Errorf("create user %s: %w", name, err)
	}

	if _, err := api.AttachUserPolicy(ctx, &iam.AttachUserPolicyInput{
		UserName:  aws.String(name),
		PolicyArn: aws.String(policyARN),
	}); err != nil {
		return nil, fmt.Errorf("attach policy to %s: %w", name, err)
	}

	key, err := api.CreateAccessKey(ctx, &iam.CreateAccessKeyInput{UserName: aws.String(name)})
	if err != nil {
		return nil, fmt.Errorf("create access key for %s: %w", name, err)
	}
	return &MintResult{
		UserName:        name,
		PolicyARN:       policyARN,
		AccessKeyID:     aws.ToString(key.AccessKey.AccessKeyId),
		SecretAccessKey: aws.ToString(key.AccessKey.SecretAccessKey),
	}, nil
}

// ScuttleWriterIAM revokes a minted writer identity: deletes its access keys, detaches
// its policies, deletes the user, then deletes the cargoship-managed policy. It is
// idempotent — a missing user (already scuttled) is not an error. It only DELETES the
// policy cargoship created for this writer; any other attached policy is detached but
// left intact.
func ScuttleWriterIAM(ctx context.Context, api IAMWriterAPI, writerID string) error {
	if writerID == "" {
		return errors.New("writer id is required")
	}
	name := WriterIdentityName(writerID)

	keys, err := api.ListAccessKeys(ctx, &iam.ListAccessKeysInput{UserName: aws.String(name)})
	if err != nil {
		if isNoSuchEntity(err) {
			return nil // user already gone
		}
		return fmt.Errorf("list access keys for %s: %w", name, err)
	}
	for _, k := range keys.AccessKeyMetadata {
		if _, err := api.DeleteAccessKey(ctx, &iam.DeleteAccessKeyInput{
			UserName: aws.String(name), AccessKeyId: k.AccessKeyId,
		}); err != nil {
			return fmt.Errorf("delete access key %s: %w", aws.ToString(k.AccessKeyId), err)
		}
	}

	attached, err := api.ListAttachedUserPolicies(ctx, &iam.ListAttachedUserPoliciesInput{UserName: aws.String(name)})
	if err != nil {
		return fmt.Errorf("list attached policies for %s: %w", name, err)
	}
	var ownPolicyARN string
	for _, p := range attached.AttachedPolicies {
		arn := aws.ToString(p.PolicyArn)
		if _, err := api.DetachUserPolicy(ctx, &iam.DetachUserPolicyInput{
			UserName: aws.String(name), PolicyArn: p.PolicyArn,
		}); err != nil {
			return fmt.Errorf("detach policy %s: %w", arn, err)
		}
		// Only the policy cargoship created for this writer is a delete candidate.
		if aws.ToString(p.PolicyName) == name || strings.HasSuffix(arn, iamPath+name) {
			ownPolicyARN = arn
		}
	}

	if _, err := api.DeleteUser(ctx, &iam.DeleteUserInput{UserName: aws.String(name)}); err != nil {
		if !isNoSuchEntity(err) {
			return fmt.Errorf("delete user %s: %w", name, err)
		}
	}

	if ownPolicyARN != "" {
		if _, err := api.DeletePolicy(ctx, &iam.DeletePolicyInput{PolicyArn: aws.String(ownPolicyARN)}); err != nil {
			if !isNoSuchEntity(err) {
				return fmt.Errorf("delete policy %s: %w", ownPolicyARN, err)
			}
		}
	}
	return nil
}

func iamErrCode(err error) string {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode()
	}
	return ""
}

func isEntityAlreadyExists(err error) bool { return iamErrCode(err) == "EntityAlreadyExists" }
func isNoSuchEntity(err error) bool        { return iamErrCode(err) == "NoSuchEntity" }
