# Deployment guide

Running CargoShip in production — CI/CD, EC2, cron, or a scheduled batch host.
This covers the IAM policy, system sizing, credential setup, workload-based
tuning, and a go-live checklist. For autonomous archival on NAS hardware, see
[ghost-ship](/enterprise/ghost-ship).

## IAM permissions

Grant the minimum S3 actions plus the multipart actions CargoShip relies on:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject", "s3:GetObject", "s3:DeleteObject",
        "s3:ListBucket", "s3:GetBucketLocation",
        "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts",
        "s3:ListBucketMultipartUploads"
      ],
      "Resource": [
        "arn:aws:s3:::your-bucket-name",
        "arn:aws:s3:::your-bucket-name/*"
      ]
    }
  ]
}
```

Add `kms:GenerateDataKey`, `kms:Decrypt`, and `kms:DescribeKey` for encryption
(see [Security model](/project/security)), `cloudwatch:PutMetricData` for metrics,
and the S3 lifecycle actions if you manage policies with
[`cargoship lifecycle`](/guides/cost/lifecycle).

## System sizing

| Tier | CPU | RAM | Network |
|------|-----|-----|---------|
| Minimum | 2 cores | 4 GB | 10 Mbps |
| Recommended | 8+ cores | 16 GB | 1 Gbps |
| High performance | 16+ cores | 32 GB+ | 10 Gbps |

CargoShip is statically linked with minimal dependencies. Memory scales with chunk
size × workers, so size RAM to your chosen concurrency. Allow HTTPS (443) egress to
S3 endpoints.

## Credentials

CargoShip uses the standard AWS credential chain — no CargoShip-specific setup.

```bash
# CI/CD: environment variables
export AWS_REGION=us-west-2
export AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=...

# Local: named profile
export AWS_PROFILE=production AWS_REGION=us-west-2

# EC2: use an instance profile — nothing to configure
```

For persistent defaults, generate a config file with
[`cargoship setup`](/guides/config/setup) or `cargoship config --generate` and
place it at `~/.cargoship.yaml`. See
[Config files & precedence](/guides/config/files).

## Workload-based tuning

Leave [shard count on auto](/guides/features/sharding) unless you're benchmarking.
The main levers are compression level and network-matched concurrency.

- **Many small files** — group aggressively, high compression:
  `--compression-level 9`.
- **Mixed dataset** — omit `--compression-level` entirely. Content-aware
  selection picks per chunk, which a single pinned level cannot; tag `--project`
  for cost tracking.
- **Few large / already-compressed files** — pin a fast level:
  `--compression-level 1`.

Match parallelism to your uplink: residential broadband wants a small shard count,
a datacenter link can drive many. See [Performance tuning](/guides/features/optimization).

### Storage class by access pattern

| Class | Best for |
|-------|----------|
| STANDARD | Active data, frequent access |
| STANDARD_IA | Infrequent access (monthly) |
| GLACIER_IR / GLACIER | Long-term archive, rare access |
| DEEP_ARCHIVE | Compliance / cold archive (slow, cheap) |

Assign per chunk by file age with [tier-aware storage](/guides/features/tiering).

## Observability

Enable Prometheus metrics and OpenTelemetry tracing per run:

```bash
cargoship upload ./data s3://my-bucket/archives/ \
  --prometheus-addr :9090 \
  --tracing --tracing-exporter otlp --tracing-endpoint http://localhost:4318
```

See [Observability & tracing](/guides/features/observability) for the exporters and
metric names.

## Go-live checklist

- [ ] IAM policy applied and verified (`aws sts get-caller-identity`).
- [ ] Bucket exists in the region CargoShip is configured for.
- [ ] `cargoship config --validate-detailed` passes.
- [ ] A test upload round-trips (`upload` → `verify` → `restore`).
- [ ] Budget and alerts configured if spend needs bounding
      ([Budgets](/guides/cost/budgets)).
- [ ] Encryption enabled if data is sensitive (`--kms-key-id --encrypt-manifest`).
- [ ] Monitoring wired up (Prometheus / CloudWatch) for scheduled runs.

## See also

- [Install](/start/install) · [AWS setup & credentials](/start/aws-setup).
- [Security model](/project/security).
- [Troubleshooting](/reference/troubleshooting).
- [Distributed / Enterprise overview](/enterprise/).
