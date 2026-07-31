# Costs & safety guarantees

CargoShip is built so you can predict what an archive will cost **before** you run
it, and so that destructive actions never happen by accident.

## What you pay for

CargoShip uploads to **your own** AWS account — there's no CargoShip service fee.
Your costs are ordinary S3 costs:

- **Storage** — per GB-month, varying by [storage class](/guides/cost/lifecycle).
  Compression reduces this; colder classes (Glacier, Deep Archive) cost far less
  to store but more to retrieve.
- **Requests** — PUT on upload, GET on restore. Sharding and chunking keep request
  counts sane by packing many files into fewer objects.
- **Retrieval** — Glacier/Deep Archive restores incur a per-GB retrieval charge
  and a thaw delay.

Estimate any of this up front:

```bash
cargoship estimate ./my-data --show-comparison
```

See [Estimating costs](/guides/cost/estimate), and set guardrails with
[budgets & volume quotas](/guides/cost/budgets).

## Safety guarantees

- **Nothing is deleted on upload.** Uploading never touches your source files.
- **Integrity is verifiable.** Every upload records per-chunk and per-file
  SHA-256 checksums. [`cargoship verify --deep`](/guides/verifying) re-downloads
  the stored bytes and re-hashes them to prove the archive still matches, and
  [restore verifies each file as it writes](/guides/restoring) so it never hands
  back corrupt data. Every release is validated against real S3 and ships a
  [verification report](/project/verification-reports). See the
  [Integrity model](/project/integrity) for the exact, bounded claim.
- **Destructive commands require confirmation.** They prompt before acting and
  support `--dry-run` to preview.

::: danger Destructive commands
Two commands remove data from S3 and cannot be undone:

- [`cargoship delete`](/reference/commands/destructive) removes one upload.
- [`cargoship scuttle`](/reference/commands/destructive) removes **all** CargoShip
  data under a bucket/prefix (a "nuclear" option with triple confirmation).

Always run them with `--dry-run` first. `--force` skips confirmation — reserve it
for automation you trust.
:::

## Opt-in features that touch external services

Some capabilities are off by default because they need credentials or extra
setup:

- **Alerts** (email/SMTP, Slack) require you to supply credentials and are
  [disabled until configured](/guides/cost/alerts).
- **Magika AI detection** needs a separate `pip install magika`; without it,
  CargoShip falls back to extension-based detection.
- **KMS encryption** requires a key you control.

## Next

- [Quick Start](/start/quickstart) — a safe, self-contained first run.
- [Estimating costs](/guides/cost/estimate) — model spend before uploading.
