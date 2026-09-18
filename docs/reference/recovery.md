# Recovery & operations runbook

What to do when something goes wrong mid-upload or mid-restore. Each scenario is
safe to work through — CargoShip never modifies your source files, and its S3
operations are designed to be re-runnable.

## Retries and idempotency

- **Automatic retries.** Transient S3 errors are retried automatically — 3
  attempts per chunk by default (configurable via the `aws.max_retries` config
  key). Each retry is traced when tracing is enabled.
- **Chunk-level idempotency.** Object keys are deterministic
  (`…/uploads/<id>/shard-N/chunk-M.tar.zst`), so re-running an upload overwrites
  the same keys rather than duplicating data. Re-uploading is safe.
- **Resume, don't restart.** An interrupted `cargoship upload` leaves a state
  file in `~/.cargoship/state/`; re-running the same command resumes from it.
  The streaming variant additionally offers `--skip-existing` (HeadObject check
  to skip chunks already in S3).

## Scenarios

### Interrupted upload (Ctrl-C, network drop, crash)

Nothing is corrupted — you have some chunks in S3 and a saved state file. Just
re-run the same command:

```bash
cargoship upload ./data s3://my-bucket/prefix
# → detects saved state, resumes the incomplete upload
```

To start over instead, `--force-restart`. To inspect or tidy saved states:
`cargoship resume list` and `cargoship resume clean --older-than 168h`. See
[Resuming interrupted uploads](/guides/resuming).

### Orphaned S3 multipart uploads

A hard crash can leave incomplete S3 multipart uploads that quietly accrue
storage cost. List and abort them (also a good candidate for an S3 lifecycle rule
that auto-aborts after N days):

```bash
aws s3api list-multipart-uploads --bucket my-bucket
aws s3api abort-multipart-upload --bucket my-bucket --key KEY --upload-id UPLOAD_ID
```

### Missing or unreadable manifest

The manifest is the index for an upload. If `info`/`list`/`restore` can't read it:

- Confirm the exact upload path: `aws s3 ls s3://my-bucket/prefix/uploads/`.
- The manifest may be gzip (`manifest.json.gz`) or KMS-encrypted
  (`manifest.encrypted.json.gz`). If encrypted, ensure your identity still has
  `kms:Decrypt` on the key.
- No manifest at all usually means the upload never completed — re-run the upload
  (it resumes and rewrites the manifest on completion).
- Worst case, your data is still recoverable without the manifest: the chunks are
  plain `tar.zst` objects you can download and unpack with standard tools.

### A chunk fails verification / looks damaged

```bash
cargoship verify s3://my-bucket/prefix/uploads/<id> --verbose
```

`verify` checks the archive against the manifest's recorded checksums and exits
non-zero on any mismatch. If a specific chunk is bad, re-upload — chunk keys are
deterministic, so re-running overwrites the damaged object in place. Use
`--quick` for a fast metadata-only check.

### Expired or rotated credentials mid-run

You'll see `AccessDenied` / expired-token errors. Refresh credentials (renew SSO,
new session token, etc.), confirm with `aws s3 ls s3://my-bucket/`, then re-run
the upload — it resumes from saved state and only uploads what's missing.

### KMS permissions changed after upload

If a key policy or your role changed, `restore`/`list` on a KMS-encrypted manifest
(or SSE-KMS chunks) will fail with a KMS `AccessDenied`. Restore
`kms:Decrypt` (and `kms:GenerateDataKey` for new uploads) on the key used at
upload time — the key ID is recorded in the manifest's encryption metadata. See
[Encryption metadata](/reference/format/encryption).

### Partial or failed restore

`restore` downloads only the chunks holding your selected files, so a partial
failure just means some files didn't land. Re-run with the same selection
(`--file` / `--hash` / `--dvc-stage`); already-written files are simply
rewritten. For Glacier/Deep Archive, if the thaw hasn't finished, retry with
`--wait`, or check pending jobs with `cargoship restore jobs list|check`. See
[Restoring files](/guides/restoring).

### Recovering a ghostship fleet writer

A [fleet](/enterprise/ghost-ship) writer's backups do not depend on the box that wrote
them — recovery needs only bucket read + the decryption key, never the writer's
identity or any local state.

- **Whole-writer recovery.** `cargoship restore s3://BUCKET/writers/<id>/uploads/<upload-id> ./out --all`
  reconstructs every file in the upload onto any machine. Restore is self-describing (the
  manifest carries the chunk layout + encrypted data-key references), so a dead box needs no
  replacement identity.
- **Use a break-glass role, not the agent.** The restore identity needs `s3:GetObject` +
  `s3:ListBucket` on the writer's prefix and `kms:Decrypt` on the fleet CMK — the read-only
  inverse of the write-only agent policy. The agent itself can't decrypt or read back.
- **Config recovery is automatic.** In pull mode an agent keeps running its last-good signed
  config if a refresh fails; push a fixed, re-signed config and the next cycle adopts it.
- **Confirm the immutability backstop** with `cargoship fleet lock-status s3://BUCKET/BASE`
  (versioning + Object Lock + abort-incomplete-MPU) before you rely on point-in-time recovery.
- **CMK is the SPOF.** With immutable history and no scheduled deletion, losing the fleet CMK
  loses the data — back up / multi-Region the key material. See [security](/project/security).

## See also

- [ghost-ship](/enterprise/ghost-ship) · [Fleet tutorial](/enterprise/fleet-tutorial)
- [Resuming interrupted uploads](/guides/resuming)
- [Verifying integrity](/guides/verifying)
- [Restoring files](/guides/restoring)
- [Troubleshooting](/reference/troubleshooting) · [FAQ](/reference/faq)
