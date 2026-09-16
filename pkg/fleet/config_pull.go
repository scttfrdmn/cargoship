package fleet

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	// controlSegment is the S3 prefix segment for the fleet control plane — where a
	// writer's signed config lives (#614). It is deliberately distinct from the
	// writers/<id>/ DATA prefix so the two can carry different IAM (a config object is
	// operator-written and agent-read-only; data is agent-written).
	controlSegment = "fleet"
	// ConfigObjectName / configSigObjectName are the config object and its detached
	// signature sidecar under a writer's control prefix.
	ConfigObjectName    = "config.yaml"
	configSigObjectName = ConfigObjectName + ".sig"
	// maxConfigBytes bounds an untrusted config/signature read.
	maxConfigBytes = 1 << 20 // 1 MiB
)

// ControlPrefix returns the control-plane prefix for a writer's signed config
// (<base>/fleet/<id>), the operator-written, agent-read-only sibling of the
// writers/<id>/ data prefix. writerID is required (a writer pulls its own config).
func ControlPrefix(base, writerID string) string {
	return path.Join(base, controlSegment, writerID)
}

// ConfigKey / ConfigSigKey are the config object and signature keys under a control prefix.
func ConfigKey(controlPrefix string) string    { return path.Join(controlPrefix, ConfigObjectName) }
func ConfigSigKey(controlPrefix string) string { return path.Join(controlPrefix, configSigObjectName) }

// PullSignedConfig fetches the config object and its detached signature from a
// writer's control prefix and verifies the signature against pub. It fails closed: a
// missing config, missing signature, or an invalid/wrong-key/tampered signature all
// return an error and the verified bytes are never produced. Returns the raw config
// bytes (the caller parses + validates them).
func PullSignedConfig(ctx context.Context, api s3ReadAPI, bucket, controlPrefix string, pub ed25519.PublicKey) ([]byte, error) {
	raw, err := getObjectBounded(ctx, api, bucket, ConfigKey(controlPrefix))
	if err != nil {
		return nil, fmt.Errorf("pull config: %w", err)
	}
	sigRaw, err := getObjectBounded(ctx, api, bucket, ConfigSigKey(controlPrefix))
	if err != nil {
		return nil, fmt.Errorf("pull config signature: %w", err)
	}
	sig, err := ParseConfigSignature(sigRaw)
	if err != nil {
		return nil, err
	}
	if err := VerifyConfigBytes(pub, raw, sig); err != nil {
		return nil, fmt.Errorf("verify config: %w", err)
	}
	return raw, nil
}

// getObjectBounded GETs an object and reads at most maxConfigBytes of its body.
func getObjectBounded(ctx context.Context, api s3ReadAPI, bucket, key string) ([]byte, error) {
	obj, err := api.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", key, err)
	}
	defer func() { _ = obj.Body.Close() }()
	return io.ReadAll(io.LimitReader(obj.Body, maxConfigBytes))
}
