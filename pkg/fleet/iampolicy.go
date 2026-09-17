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

// WriterPolicyOptions configures the emitted writer IAM policy (#520/#613/#614).
type WriterPolicyOptions struct {
	// Bucket is required.
	Bucket string
	// DataPrefix is the writer-scoped data key prefix (build it with
	// pipeline.WriterPrefix); empty scopes objects to the whole bucket.
	DataPrefix string
	// ControlPrefix is the writer's config control prefix (build it with
	// fleet.ControlPrefix); when set the policy adds READ-ONLY GetObject there so the
	// agent can pull its signed config (#614). Empty omits the config-read grant.
	ControlPrefix string
	// KMSKeyARN is the optional fleet customer master key.
	KMSKeyARN string
	// WriteOnly emits the strict write-only agent policy (#613): PutObject-only on the
	// data prefix (no GetObject on data, no AbortMultipartUpload) and, with a KMS key,
	// kms:GenerateDataKey WITHOUT kms:Decrypt. This is the genuine write-only footprint —
	// a compromised writer can neither read back nor delete any data. Trade-offs: it
	// assumes CargoShip's application-level (envelope) encryption rather than bucket
	// SSE-KMS (which needs Decrypt for multipart), and incremental sync degrades to a
	// full re-scan each cycle because the previous manifest can't be re-read from S3
	// (a local manifest cache restores incremental — future work). When false, the
	// policy is delete-free but grants GetObject on the writer's own prefix (chain
	// re-read) and kms:Decrypt (SSE-KMS multipart).
	WriteOnly bool
}

// WriterIAMPolicy returns an indented JSON IAM policy granting a single fleet writer
// the least privilege it needs to back up to its own subtree of an S3 bucket, and
// nothing more (#520/#613/#614).
//
// The policy is always deliberately delete-free and cross-writer-free — that absence,
// together with the own-prefix scoping, is the durable ransomware guarantee: a
// compromised writer can only append to its own prefix, never delete or overwrite
// another writer's data. See WriterPolicyOptions.WriteOnly for the strict variant.
func WriterIAMPolicy(o WriterPolicyOptions) (string, error) {
	if o.Bucket == "" {
		return "", fmt.Errorf("bucket is required")
	}

	objectResource := "arn:aws:s3:::" + o.Bucket + "/*"
	var listCond map[string]condKeys
	if p := strings.Trim(o.DataPrefix, "/"); p != "" {
		objectResource = "arn:aws:s3:::" + o.Bucket + "/" + p + "/*"
		listCond = map[string]condKeys{
			"StringLike": {"s3:prefix": {p + "/*"}},
		}
	}

	// PutObject authorizes create/upload-part/complete-multipart. The non-write-only
	// variant also grants GetObject (HeadObject skip-existing + reading this writer's
	// own manifest chain for incremental sync) and AbortMultipartUpload (clean up its
	// own stale uploads).
	objectActions := []string{"s3:PutObject"}
	if !o.WriteOnly {
		objectActions = []string{"s3:PutObject", "s3:GetObject", "s3:AbortMultipartUpload"}
	}

	doc := policyDoc{
		Version: iamPolicyVersion,
		Statement: []statement{
			{
				Sid:      "WriterObjectAccess",
				Effect:   "Allow",
				Action:   objectActions,
				Resource: objectResource,
			},
			{
				// ListBucket is a bucket-ARN action, gated to the writer's subtree
				// so it can enumerate only its own uploads (sync base detection).
				Sid:       "WriterListOwnPrefix",
				Effect:    "Allow",
				Action:    []string{"s3:ListBucket"},
				Resource:  "arn:aws:s3:::" + o.Bucket,
				Condition: listCond,
			},
		},
	}

	if cp := strings.Trim(o.ControlPrefix, "/"); cp != "" {
		doc.Statement = append(doc.Statement, statement{
			// Read-only access to the writer's signed config (#614). GetObject only —
			// the config is operator-written; the agent never writes the control prefix.
			Sid:      "WriterConfigRead",
			Effect:   "Allow",
			Action:   []string{"s3:GetObject"},
			Resource: "arn:aws:s3:::" + o.Bucket + "/" + cp + "/*",
		})
	}

	if o.KMSKeyARN != "" {
		// GenerateDataKey for manifest/data envelope encryption. The non-write-only
		// variant also grants Decrypt, which AWS requires to multipart-upload SSE-KMS
		// objects. Scoped to the one fleet key.
		kmsActions := []string{"kms:GenerateDataKey"}
		if !o.WriteOnly {
			kmsActions = []string{"kms:GenerateDataKey", "kms:Decrypt"}
		}
		doc.Statement = append(doc.Statement, statement{
			Sid:      "WriterKMS",
			Effect:   "Allow",
			Action:   kmsActions,
			Resource: o.KMSKeyARN,
		})
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal IAM policy: %w", err)
	}
	return string(out), nil
}
