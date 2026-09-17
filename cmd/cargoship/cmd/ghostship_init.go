package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/fleet"
	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

func newGhostshipInitCmd() *cobra.Command {
	var (
		writerID  string
		kmsKeyARN string
		publicKey string
		outDir    string
		image     string
		region    string
	)
	cmd := &cobra.Command{
		Use:   "init S3_URL",
		Short: "Scaffold a deployable, write-only fleet-writer bundle (#613)",
		Long: `Generate everything needed to deploy one ghostship writer against
s3://BUCKET/BASE: a strict write-only IAM policy, a config skeleton, a compose file
that wires credentials via a mounted file (not env), and the operator's config-signing
public key baked in.

This emits artifacts only — it does not call AWS. Attach the emitted policy to a new
IAM identity yourself (or use your own tooling), then fill in and sign the config. The
writer runs in signed config-over-S3 (pull) mode.

The emitted IAM policy is write-only and delete-free: PutObject on the writer's own
prefix, ListBucket scoped to it, read-only on its config, and kms:GenerateDataKey (no
Decrypt). A compromised writer can neither read back, decrypt, nor delete any data.

Example:
  cargoship ghostship config-keygen --out ./keys
  cargoship ghostship init s3://backups/nas --writer-id lab-nas-1 \
    --kms-key-arn arn:aws:kms:us-west-2:123456789012:key/abcd \
    --public-key ./keys/config-signing-public.pem --out ./lab-nas-1`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bucket, base, err := parseS3URL(args[0])
			if err != nil {
				return fmt.Errorf("invalid S3 URL: %w", err)
			}
			id, err := pipeline.ResolveWriterID(writerID)
			if err != nil {
				return fmt.Errorf("invalid --writer-id: %w", err)
			}
			if id == "" {
				return fmt.Errorf("--writer-id is required (a bundle is per-writer); pass one or 'auto'")
			}

			// Validate the public key up front so the bundle can't ship a broken key.
			pubPEM, err := os.ReadFile(publicKey)
			if err != nil {
				return fmt.Errorf("read public key %s: %w", publicKey, err)
			}
			if _, err := fleet.LoadConfigPublicKey(pubPEM); err != nil {
				return fmt.Errorf("public key %s: %w", publicKey, err)
			}

			// Data lands at the bucket-root writers/<id>/ prefix (the sync engine ignores
			// the URL's base for data); the signed config lives under <base>/fleet/<id>/.
			dataPrefix := pipeline.WriterPrefix("", id)
			controlPrefix := fleet.ControlPrefix(base, id)
			policy, err := fleet.WriterIAMPolicy(fleet.WriterPolicyOptions{
				Bucket: bucket, DataPrefix: dataPrefix, ControlPrefix: controlPrefix,
				KMSKeyARN: kmsKeyARN, WriteOnly: true,
			})
			if err != nil {
				return err
			}

			dir := outDir
			if dir == "" {
				dir = id
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create bundle dir %s: %w", dir, err)
			}

			files := map[string][]byte{
				"iam-policy.json":           []byte(policy + "\n"),
				"config.yaml":               []byte(initConfigYAML(id, bucket, base)),
				"config-signing-public.pem": pubPEM,
				"compose.yaml":              []byte(initComposeYAML(id, bucket, base, image, region)),
				"README.md":                 []byte(initReadme(id, bucket, base)),
			}
			for name, data := range files {
				if err := writeNewFile(filepath.Join(dir, name), data, 0o644); err != nil {
					return err
				}
			}

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
				"Wrote a write-only writer bundle for %q to %s/\nNext: see %s/README.md (attach the policy, then fill in + sign config.yaml).\n",
				id, dir, dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&writerID, "writer-id", "", "Writer identity for this bundle (writers/<id>/). 'auto' derives a stable per-host id")
	cmd.Flags().StringVar(&kmsKeyARN, "kms-key-arn", "", "Fleet KMS key ARN; adds kms:GenerateDataKey (write-only, no Decrypt) scoped to that key")
	cmd.Flags().StringVar(&publicKey, "public-key", "", "Path to the config-signing public key (from 'ghostship config-keygen') to bake into the bundle (required)")
	cmd.Flags().StringVar(&outDir, "out", "", "Directory to write the bundle into (default ./<writer-id>)")
	cmd.Flags().StringVar(&image, "image", "cargoship:latest", "Container image to run in the emitted compose file")
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region for the emitted compose file")
	_ = cmd.MarkFlagRequired("public-key")
	return cmd
}

func initConfigYAML(id, bucket, base string) string {
	return fmt.Sprintf(`# CargoShip ghostship config for writer %q.
# 1. Set watch_paths below to the directories this box backs up.
# 2. Bump 'version' on every change.
# 3. Sign it:   cargoship ghostship sign-config config.yaml --key <your-private-key.pem>
# 4. Upload it: aws s3 cp config.yaml     s3://%s/%s/fleet/%s/config.yaml
#               aws s3 cp config.yaml.sig s3://%s/%s/fleet/%s/config.yaml.sig
id: %s
writer_id: %s
version: 1
s3_config:
  bucket: %s
watch_paths:
  - path: /data   # TODO: set the directory to back up
scan_interval: 1h
`, id, bucket, base, id, bucket, base, id, id, id, bucket)
}

func initComposeYAML(id, bucket, base, image, region string) string {
	return fmt.Sprintf(`# Deploy one write-only ghostship writer. Credentials are provided via a MOUNTED
# FILE (./aws-credentials -> ~/.aws/credentials), never environment variables.
services:
  ghostship:
    image: %s
    command:
      - ghostship
      - run
      - --config-url
      - s3://%s/%s
      - --public-key
      - /etc/cargoship/config-signing-public.pem
      - --writer-id
      - %s
      - --region
      - %s
      - --interval
      - 1h
    volumes:
      - ./config-signing-public.pem:/etc/cargoship/config-signing-public.pem:ro
      - ./aws-credentials:/root/.aws/credentials:ro
      - cargoship-state:/root/.cargoship
    restart: unless-stopped
volumes:
  cargoship-state:
`, image, bucket, base, id, region)
}

func initReadme(id, bucket, base string) string {
	return fmt.Sprintf("# ghostship writer bundle: %s\n\n"+
		"This bundle deploys one **write-only, delete-free** CargoShip backup agent.\n\n"+
		"## Files\n"+
		"- `iam-policy.json` — least-privilege, write-only IAM policy for this writer.\n"+
		"- `config.yaml` — config skeleton; fill in `watch_paths`, then sign + upload.\n"+
		"- `config-signing-public.pem` — the operator public key the agent verifies against.\n"+
		"- `compose.yaml` — runs the agent in signed config-over-S3 (pull) mode.\n\n"+
		"## Steps\n"+
		"1. **Create the identity** and attach `iam-policy.json` to it (IAM user or role).\n"+
		"2. Put its access key + secret in `./aws-credentials` (standard AWS credentials\n"+
		"   file format). It is mounted read-only into the container — never passed as env.\n"+
		"3. **Fill in** `config.yaml` `watch_paths`, then **sign** it with your private key:\n"+
		"   `cargoship ghostship sign-config config.yaml --key <private-key.pem>`\n"+
		"4. **Upload** the config + signature to the control prefix:\n"+
		"   - `aws s3 cp config.yaml     s3://%s/%s/fleet/%s/config.yaml`\n"+
		"   - `aws s3 cp config.yaml.sig s3://%s/%s/fleet/%s/config.yaml.sig`\n"+
		"5. **Deploy**: `docker compose up -d`.\n\n"+
		"The agent pulls + verifies its config every cycle (keep-last-good), writes only to\n"+
		"its own `writers/%s/` prefix, and cannot read back, decrypt, or delete any data.\n",
		id, bucket, base, id, bucket, base, id, id)
}
