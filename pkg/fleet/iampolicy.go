// Package fleet holds provisioning and control-plane helpers for running
// CargoShip as a fleet of writer-isolated agents ("ghostships", #604). It builds
// on the writer-isolation primitives in pkg/pipeline (#520).
package fleet

import (
	"encoding/json"
	"fmt"
	"strings"
)

// iamPolicyVersion is the fixed AWS IAM policy language version.
const iamPolicyVersion = "2012-10-17"

// policyDoc / statement are minimal emit-only IAM policy types. The parse-side
// structs in pkg/aws/access are unexported and drop Action/Resource, so they
// cannot be reused to marshal a usable policy.
type policyDoc struct {
	Version   string      `json:"Version"`
	Statement []statement `json:"Statement"`
}

type statement struct {
	Sid       string              `json:"Sid"`
	Effect    string              `json:"Effect"`
	Action    []string            `json:"Action"`
	Resource  string              `json:"Resource"`
	Condition map[string]condKeys `json:"Condition,omitempty"`
}

// condKeys is one IAM condition operator's key→values map (e.g.
// {"s3:prefix": ["p/writers/w1/*"]}).
type condKeys map[string][]string

// WriterIAMPolicy returns an indented JSON IAM policy granting a single fleet
// writer the least privilege it needs to back up to its own subtree of an S3
// bucket, and nothing more (#520/#613).
//
// effectivePrefix is the writer-scoped key prefix (build it with
// pipeline.WriterPrefix); an empty prefix scopes to the whole bucket. bucket is
// required. kmsKeyARN is optional: when set, the policy adds KMS permissions for
// the given customer master key.
//
// The policy is deliberately delete-free and cross-writer-free — that absence,
// together with the own-prefix scoping, is the durable ransomware guarantee: a
// compromised writer can only append to its own prefix, never delete or overwrite
// another writer's data. Note it is NOT decrypt-free when a KMS key is supplied:
// AWS requires kms:Decrypt (alongside kms:GenerateDataKey) to multipart-upload
// SSE-KMS-encrypted objects, so both are granted on that one key.
func WriterIAMPolicy(bucket, effectivePrefix, kmsKeyARN string) (string, error) {
	if bucket == "" {
		return "", fmt.Errorf("bucket is required")
	}

	objectResource := "arn:aws:s3:::" + bucket + "/*"
	var listCond map[string]condKeys
	if p := strings.Trim(effectivePrefix, "/"); p != "" {
		objectResource = "arn:aws:s3:::" + bucket + "/" + p + "/*"
		listCond = map[string]condKeys{
			"StringLike": {"s3:prefix": {p + "/*"}},
		}
	}

	doc := policyDoc{
		Version: iamPolicyVersion,
		Statement: []statement{
			{
				// PutObject authorizes create/upload-part/complete-multipart;
				// GetObject covers HeadObject (skip-existing) and reading this
				// writer's own manifest chain for incremental sync;
				// AbortMultipartUpload lets it clean up its own stale uploads.
				Sid:      "WriterObjectAccess",
				Effect:   "Allow",
				Action:   []string{"s3:PutObject", "s3:GetObject", "s3:AbortMultipartUpload"},
				Resource: objectResource,
			},
			{
				// ListBucket is a bucket-ARN action, gated to the writer's subtree
				// so it can enumerate only its own uploads (sync base detection).
				Sid:       "WriterListOwnPrefix",
				Effect:    "Allow",
				Action:    []string{"s3:ListBucket"},
				Resource:  "arn:aws:s3:::" + bucket,
				Condition: listCond,
			},
		},
	}

	if kmsKeyARN != "" {
		doc.Statement = append(doc.Statement, statement{
			// GenerateDataKey for manifest envelope encryption; Decrypt is required
			// by AWS for SSE-KMS multipart uploads of data chunks. Scoped to the one
			// fleet key — the writer still cannot read any other key's data.
			Sid:      "WriterKMS",
			Effect:   "Allow",
			Action:   []string{"kms:GenerateDataKey", "kms:Decrypt"},
			Resource: kmsKeyARN,
		})
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal IAM policy: %w", err)
	}
	return string(out), nil
}
