# FAQ

## What format does CargoShip write?

Open, documented, and portable ones. An upload is a set of `tar.zst` objects
(tar archives compressed with Zstandard) plus a JSON manifest that indexes every
file. Nothing is proprietary — the [archive layout](/reference/format/archive-layout)
and [manifest schema](/reference/format/manifest) are fully specified.

## Can I extract my data without CargoShip?

Yes. Because chunks are ordinary `tar.zst` objects, you can download one from S3
and unpack it with standard tools (`zstd -d`, then `tar -xf`) — no CargoShip
binary required. The manifest tells you which chunk holds which file. CargoShip
just makes selective, checksum-verified extraction convenient.

## Is CargoShip a service or a subscription?

Neither. It's a self-contained Go command-line tool (Apache 2.0 licensed) that
runs on your machine and talks directly to your own S3 bucket using your own AWS
credentials. There is no CargoShip server, account, or hosted component in the
data path.

## Does CargoShip delete or modify my source files?

No. CargoShip only reads your source directory — it never modifies or deletes
local files during an upload. Destructive S3-side operations (like `cargoship
delete` or `scuttle`) are separate, explicit commands; see
[Destructive operations](/reference/commands/destructive) and
[Costs & safety guarantees](/intro/costs-and-safety).

## What AWS credentials does it use?

The standard AWS credential chain — the same one the AWS CLI uses (environment
variables, `AWS_PROFILE`, `~/.aws/*`, or an IAM role). If `aws s3 ls` works,
CargoShip works. CargoShip stores no credentials of its own. See
[AWS setup](/start/aws-setup).

## What S3 permissions do I need?

Read/write on the target bucket, including the multipart actions large uploads
use. The [minimal IAM policy](/start/aws-setup#minimal-iam-policy) lists exactly
what to grant (and which extra permissions Glacier restores and KMS need).

## What happens if an upload is interrupted?

Nothing is corrupted — an incomplete upload just leaves some chunks in S3 and a
saved state file in `~/.cargoship/state/`. Re-run the same command and CargoShip
detects the state and resumes; `--force-restart` ignores it and starts fresh. See
[Resuming interrupted uploads](/guides/resuming) and the
[recovery runbook](/reference/recovery).

## Is it safe to retry a failed upload?

Yes. Uploads are idempotent at the chunk level — re-running re-uploads only what's
missing, and object keys are deterministic so a retry overwrites the same key
rather than duplicating data. Transient S3 errors are retried automatically
(3 attempts by default). Details in [Recovery](/reference/recovery#retries-and-idempotency).

## How do I clean up a partial or unwanted upload?

`cargoship delete s3://…/uploads/<id> --dry-run` to preview, then without
`--dry-run` to remove that one upload. Orphaned S3 multipart uploads from a hard
crash can be listed and aborted with `aws s3api list-multipart-uploads` /
`abort-multipart-upload`. See [Clean up](/start/cleanup).

## My data is in Glacier / Deep Archive — how do I get it back?

`cargoship restore` handles the thaw: `--tier` picks the retrieval speed
(`expedited` 1–5 min, `standard` 3–5 h, `bulk` 5–12 h), `--wait` blocks until it's
ready, and `--dry-run` / `--max-restore-cost` let you see or cap the retrieval
cost first. See [Restoring files](/guides/restoring#glacier).

## Will archives written today still be readable later?

Yes — the archive/manifest format is versioned and backward-readable within the
guarantees in the [format spec](/reference/format/) and
[maturity page](/project/maturity). And since chunks are plain `tar.zst`, you can
always fall back to standard tools.

## How much memory does it use?

Bounded — roughly `chunk_size × workers`, not the size of your dataset, because
data streams to S3 without staging to disk. A multi-terabyte upload runs in a few
GB. See [How it works](/intro/how-it-works).

## How do I keep costs predictable?

Run `cargoship estimate` before uploading, tag uploads with `--project` and set
[budgets and quotas](/guides/cost/budgets), and choose a
[storage class](/guides/cost/lifecycle) that matches your access pattern. Cold
classes store cheaply but cost more (and take longer) to retrieve.

## See also

- [Concepts & terminology](/intro/concepts).
- [How it works](/intro/how-it-works).
- [Troubleshooting](/reference/troubleshooting).
- [Glossary](/reference/glossary).
