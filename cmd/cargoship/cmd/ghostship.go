package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/cargoship/pkg/fleet"
	"github.com/scttfrdmn/cargoship/pkg/launch"
	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

// NewGhostshipCmd creates the 'ghostship' command group for running CargoShip as
// a fleet of unattended, writer-isolated backup agents (#604).
func NewGhostshipCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ghostship",
		Short: "Fleet mode: run CargoShip as unattended, writer-isolated backup agents",
		Long: `Ghostship groups the commands for running CargoShip as a fleet of unattended
backup agents that share one S3 bucket without colliding (writer isolation, #520).

Subcommands:
  run               Run an unattended, writer-isolated incremental backup on a schedule
  iam-policy        Emit the least-privilege IAM policy for one writer
  validate-config   Check a ghostship config (and optionally its scope) before deploy`,
	}
	cmd.AddCommand(
		newGhostshipRunCmd(),
		newGhostshipIAMPolicyCmd(),
		newGhostshipValidateConfigCmd(),
		newGhostshipConfigKeygenCmd(),
		newGhostshipSignConfigCmd(),
	)
	return cmd
}

func newGhostshipIAMPolicyCmd() *cobra.Command {
	var (
		writerID  string
		kmsKeyARN string
		output    string
	)
	cmd := &cobra.Command{
		Use:   "iam-policy S3_URL",
		Short: "Emit a least-privilege, delete-free IAM policy for a fleet writer",
		Long: `Emit the least-privilege AWS IAM policy for one ghostship writer backing up to
s3://BUCKET/PREFIX. Attach the emitted policy to the IAM user or role that your
writer runs as — a ghostship agent, or a cron'd 'cargoship sync --writer-id'.

The policy grants only what a writer needs: PutObject/GetObject/AbortMultipartUpload
scoped to writers/<id>/, plus ListBucket gated to that prefix. It is deliberately
delete-free and cannot reach another writer's data — a compromised writer can only
append to its own subtree, never delete or overwrite backups. With --kms-key-arn it
also grants kms:GenerateDataKey and kms:Decrypt on that one key (AWS requires both to
multipart-upload SSE-KMS objects).

Examples:
  cargoship ghostship iam-policy s3://my-bucket/backups --writer-id lab-nas-1
  cargoship ghostship iam-policy s3://my-bucket/backups --writer-id auto \
    --kms-key-arn arn:aws:kms:us-west-2:123456789012:key/abcd-1234 -o policy.json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bucket, prefix, err := parseS3URL(args[0])
			if err != nil {
				return fmt.Errorf("invalid S3 URL: %w", err)
			}
			resolvedWriterID, err := pipeline.ResolveWriterID(writerID)
			if err != nil {
				return fmt.Errorf("invalid --writer-id: %w", err)
			}
			eff := pipeline.WriterPrefix(prefix, resolvedWriterID)
			var ctrl string
			if resolvedWriterID != "" {
				ctrl = fleet.ControlPrefix(prefix, resolvedWriterID)
			}

			policy, err := fleet.WriterIAMPolicy(bucket, eff, ctrl, kmsKeyARN)
			if err != nil {
				return err
			}

			if output != "" {
				if err := os.WriteFile(output, []byte(policy+"\n"), 0o644); err != nil {
					return fmt.Errorf("write policy file: %w", err)
				}
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Wrote IAM policy to %s\n", output)
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), policy)
			}

			who := resolvedWriterID
			if who == "" {
				who = "(single-writer / base prefix)"
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
				"\nAttach this policy to the IAM user/role writer %q runs as. It is delete-free and scoped to writers/<id>/ — a compromised writer can only append to its own prefix.\n", who)
			return nil
		},
	}
	cmd.Flags().StringVar(&writerID, "writer-id", "", "Writer identity to scope the policy to (writers/<id>/). 'auto' derives a stable per-host id; empty scopes to the base prefix")
	cmd.Flags().StringVar(&kmsKeyARN, "kms-key-arn", "", "KMS key ARN for encrypted uploads; adds kms:GenerateDataKey and kms:Decrypt scoped to that key")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write the policy JSON to this file instead of stdout")
	return cmd
}

func newGhostshipValidateConfigCmd() *cobra.Command {
	var (
		baseline  string
		strict    bool
		publicKey string
		sigPath   string
	)
	cmd := &cobra.Command{
		Use:   "validate-config CONFIG_FILE",
		Short: "Validate a ghostship config, and optionally check it doesn't widen scope",
		Long: `Validate a ghostship config file before deploying it. Reports errors (an
invalid config) and warnings (dangerous-but-permitted combinations, e.g.
delete_after_archive with a broad matcher).

With --baseline, also reports how CONFIG_FILE widens what a writer reads or
deletes relative to the currently-deployed config (new/broadened watch paths,
recursive flips, added includes, removed excludes, newly-enabled source deletion)
— the check the fleet's config-over-S3 pull will enforce.

Read-only: no AWS calls, no network.

Examples:
  cargoship ghostship validate-config ghost_ship.yaml
  cargoship ghostship validate-config new.yaml --baseline deployed.yaml --strict`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, cfg, err := readGhostshipConfig(args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			// Signature verification (fail-closed): with --public-key, the config's
			// detached signature must verify against the trusted key, or this errors.
			if publicKey != "" {
				if err := verifyConfigSignature(raw, args[0], publicKey, sigPath); err != nil {
					return fmt.Errorf("signature: %w", err)
				}
				_, _ = fmt.Fprintln(out, "SIGNATURE: ok")
			}

			var errCount, warnCount int
			for _, is := range launch.ValidateConfig(cfg) {
				if is.Severity == launch.SeverityError {
					errCount++
				} else {
					warnCount++
				}
				_, _ = fmt.Fprintf(out, "%s: %s: %s\n", strings.ToUpper(string(is.Severity)), is.Field, is.Message)
			}

			var widenings []string
			if baseline != "" {
				base, err := loadGhostshipConfigFile(baseline)
				if err != nil {
					return fmt.Errorf("baseline: %w", err)
				}
				widenings = launch.WatchScopeWidenings(base, cfg)
				for _, wd := range widenings {
					_, _ = fmt.Fprintf(out, "WIDENS-SCOPE: %s\n", wd)
				}
			}

			if errCount == 0 && warnCount == 0 && len(widenings) == 0 {
				_, _ = fmt.Fprintln(out, "OK: config is valid")
			}
			if errCount > 0 {
				return fmt.Errorf("%d validation error(s)", errCount)
			}
			if strict && (warnCount > 0 || len(widenings) > 0) {
				return fmt.Errorf("--strict: %d warning(s), %d scope-widening(s)", warnCount, len(widenings))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&baseline, "baseline", "", "Path to the currently-deployed config; report how CONFIG_FILE widens watch scope vs it")
	cmd.Flags().BoolVar(&strict, "strict", false, "Treat warnings and scope-widenings as failures (non-zero exit)")
	cmd.Flags().StringVar(&publicKey, "public-key", "", "Verify the config's detached signature against this PEM ed25519 public key (fails if invalid)")
	cmd.Flags().StringVar(&sigPath, "signature", "", "Path to the signature sidecar (default CONFIG_FILE.sig)")
	return cmd
}

// verifyConfigSignature verifies raw config bytes against a trusted PEM public key,
// reading the detached signature from sigPath (default configPath + ".sig").
func verifyConfigSignature(raw []byte, configPath, publicKeyPath, sigPath string) error {
	pubPEM, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("read public key %s: %w", publicKeyPath, err)
	}
	pub, err := fleet.LoadConfigPublicKey(pubPEM)
	if err != nil {
		return err
	}
	if sigPath == "" {
		sigPath = configPath + ".sig"
	}
	sigData, err := os.ReadFile(sigPath)
	if err != nil {
		return fmt.Errorf("read signature %s: %w", sigPath, err)
	}
	sig, err := fleet.ParseConfigSignature(sigData)
	if err != nil {
		return err
	}
	return fleet.VerifyConfigBytes(pub, raw, sig)
}

func newGhostshipConfigKeygenCmd() *cobra.Command {
	var outDir string
	cmd := &cobra.Command{
		Use:   "config-keygen",
		Short: "Generate an ed25519 keypair for signing fleet configs (#614)",
		Long: `Generate an ed25519 signing keypair for the config-over-S3 trust path. The
operator keeps the private key and signs configs with 'ghostship sign-config'; only
the public key is baked into an agent at deploy, and the agent refuses any config that
doesn't verify against it.

Writes config-signing-private.pem (0600) and config-signing-public.pem; neither is
overwritten if it already exists. This keypair signs configs — it is separate from the
GPG keys 'cargoship create keys' makes for file encryption.

Example:
  cargoship ghostship config-keygen --out ./fleet-keys`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			privPEM, pubPEM, err := fleet.GenerateConfigKeypair()
			if err != nil {
				return err
			}
			dir := outDir
			if dir == "" {
				dir = "."
			}
			privPath := filepath.Join(dir, "config-signing-private.pem")
			pubPath := filepath.Join(dir, "config-signing-public.pem")
			if err := writeNewFile(privPath, privPEM, 0o600); err != nil {
				return err
			}
			if err := writeNewFile(pubPath, pubPEM, 0o644); err != nil {
				return err
			}
			pub, _ := fleet.LoadConfigPublicKey(pubPEM)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
				"Wrote %s (keep secret) and %s\nKey id: %s\nBake the public key into agents; sign configs with 'ghostship sign-config'.\n",
				privPath, pubPath, fleet.ConfigKeyID(pub))
			return nil
		},
	}
	cmd.Flags().StringVar(&outDir, "out", "", "Directory to write the keypair into (default current directory)")
	return cmd
}

func newGhostshipSignConfigCmd() *cobra.Command {
	var (
		keyPath string
		outPath string
	)
	cmd := &cobra.Command{
		Use:   "sign-config CONFIG_FILE",
		Short: "Sign a ghostship config with an ed25519 private key (#614)",
		Long: `Produce a detached signature over a ghostship config so agents can verify it
came from the operator. The config is validated first — a config with validation
errors is refused, so you never sign a broken config.

Writes a signature sidecar (default CONFIG_FILE.sig) that travels with the config.
Bump the config's 'version' field before signing so each rollout is identifiable in
writer heartbeats.

Example:
  cargoship ghostship sign-config ghost_ship.yaml --key config-signing-private.pem`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, cfg, err := readGhostshipConfig(args[0])
			if err != nil {
				return err
			}
			var errCount int
			for _, is := range launch.ValidateConfig(cfg) {
				if is.Severity == launch.SeverityError {
					errCount++
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "ERROR: %s: %s\n", is.Field, is.Message)
				}
			}
			if errCount > 0 {
				return fmt.Errorf("refusing to sign: %d validation error(s)", errCount)
			}
			privPEM, err := os.ReadFile(keyPath)
			if err != nil {
				return fmt.Errorf("read private key %s: %w", keyPath, err)
			}
			priv, err := fleet.LoadConfigPrivateKey(privPEM)
			if err != nil {
				return err
			}
			sig := fleet.SignConfigBytes(priv, raw)
			sigData, err := sig.Marshal()
			if err != nil {
				return err
			}
			if outPath == "" {
				outPath = args[0] + ".sig"
			}
			// #nosec G304,G703 -- operator-supplied signature output path on the control
			// machine (CLI arg); writing the sidecar there is the command's purpose.
			if err := os.WriteFile(outPath, append(sigData, '\n'), 0o644); err != nil {
				return fmt.Errorf("write signature %s: %w", outPath, err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Wrote signature %s (key id %s, config version %d)\n", outPath, sig.KeyID, cfg.Version)
			return nil
		},
	}
	cmd.Flags().StringVar(&keyPath, "key", "", "Path to the ed25519 private key PEM (required)")
	cmd.Flags().StringVarP(&outPath, "output", "o", "", "Write the signature to this path (default CONFIG_FILE.sig)")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}

// writeNewFile writes data to path with perm, failing if the file already exists
// (never clobbers an existing key). Mirrors pkg/gpg's key-file safety.
func writeNewFile(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	return f.Close()
}

// loadGhostshipConfigFile reads and YAML-decodes a ghostship config file.
func loadGhostshipConfigFile(path string) (*launch.GhostShipConfig, error) {
	_, cfg, err := readGhostshipConfig(path)
	return cfg, err
}

// readGhostshipConfig returns both the raw config bytes (needed for signature
// verification, which must hash the exact bytes on disk) and the parsed config.
func readGhostshipConfig(path string) ([]byte, *launch.GhostShipConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg, err := unmarshalGhostshipConfig(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%w (%s)", err, path)
	}
	return data, cfg, nil
}

// unmarshalGhostshipConfig YAML-decodes raw config bytes (from a file or an S3 pull).
func unmarshalGhostshipConfig(raw []byte) (*launch.GhostShipConfig, error) {
	var cfg launch.GhostShipConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
