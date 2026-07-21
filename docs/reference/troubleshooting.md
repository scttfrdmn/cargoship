# Troubleshooting

Common issues and how to resolve them. When in doubt, re-run with `--verbose`
(add `--trace` for even more detail) and validate your setup with
`cargoship config --validate-detailed`.

## First steps

```bash
cargoship --verbose <command>          # detailed logging
cargoship --verbose --trace <command>  # maximum detail
cargoship config --validate-detailed   # config + AWS connectivity + bucket access
cargoship config --show                # view resolved configuration
```

## Configuration

**Config file not found.** CargoShip searches `~/.cargoship.yaml`,
`~/.config/cargoship/.cargoship.yaml`, then `./.cargoship.yaml`. Generate one with
`cargoship setup` (wizard) or `cargoship config --generate`, or point at a file
with `--file /path/to/config.yaml`.

**Precedence.** Settings resolve highest-first: command-line flags →
environment variables (`CARGOSHIP_*`) → config file → built-in defaults. See
[Environment variables](/reference/environment-variables).

## AWS credentials & permissions

**Credentials not found.** Confirm your identity resolves:

```bash
aws sts get-caller-identity
```

If it fails, run `aws configure`, set `AWS_ACCESS_KEY_ID` /
`AWS_SECRET_ACCESS_KEY` / `AWS_REGION`, or use a profile — see
[AWS setup & credentials](/start/aws-setup).

**Permission denied.** Uploads need S3 `PutObject`, `GetObject`, `ListBucket`,
plus the multipart actions `AbortMultipartUpload`, `ListMultipartUploadParts`, and
`ListBucketMultipartUploads`. Encryption additionally needs the KMS permissions
described in the [Security model](/project/security).

**Bucket access denied.** Verify the bucket exists and its region matches your
config:

```bash
aws s3 ls s3://your-bucket-name
aws s3api get-bucket-location --bucket your-bucket-name
```

## Upload issues

**Orphaned multipart uploads** after a failure waste storage. List and abort them,
and set a lifecycle rule to auto-clean incomplete uploads:

```bash
aws s3api list-multipart-uploads --bucket your-bucket-name
```

**Interrupted uploads** resume automatically on re-run — see
[Resuming interrupted uploads](/guides/resuming).

## Performance

**High memory.** Memory scales with chunk size × workers. Lower `--shard-count`,
reduce chunk size in config, or cap it with `--memory-limit 4GB` (slower but avoids
OOM). Tuning `GOGC` (e.g. `GOGC=50`) makes garbage collection more aggressive.

**High CPU / slow uploads.** Use a faster compression algorithm or a lower
`--compression-level`; already-compressed content is skipped automatically.
[Benchmark](/guides/features/benchmarking) algorithms on your data, and profile a
run with `cargoship profile collect --cpu --memory --duration 60` then
`cargoship profile stats`.

## Common error messages

| Message | Cause & fix |
|---------|-------------|
| `failed to load AWS config` | Credentials missing/invalid — run `aws configure`, verify with `aws sts get-caller-identity`. |
| `failed to access bucket` | Bucket missing, wrong region, or no permission — check region and IAM. |
| `chunk size below S3 minimum` | S3 multipart minimum is 5 MB — raise the configured chunk size. |
| `invalid storage class` | Use a valid class: STANDARD, STANDARD_IA, ONEZONE_IA, INTELLIGENT_TIERING, GLACIER, GLACIER_IR, DEEP_ARCHIVE. |

## Getting help

Collect `cargoship --version`, `cargoship config --show` (redacted), and
`--verbose --trace` logs, then file an issue at
[GitHub Issues](https://github.com/scttfrdmn/cargoship/issues).

## See also

- [FAQ](/reference/faq).
- [AWS setup & credentials](/start/aws-setup).
- [Security model](/project/security).
