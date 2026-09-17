# ghost-ship

A ghost ship is an autonomous CargoShip agent deployed to a remote NAS (QNAP or
Synology) that continuously monitors local files and archives them directly to S3
— no data round-trip through a central host. It runs fully independently: no
central coordinator is involved, and archival continues on its own schedule.

## How it works

Ghost ships apply configurable **archival rules** to watched paths: match files by
pattern and minimum age, pick a storage class, optionally encrypt, and (optionally)
delete after a successful archive to free local space. Files go straight from the
NAS to S3.

```yaml
# ghost_ship.yaml (excerpt)
id: "production-ghost-ship"
name: "Production Ghost Ship"

s3_config:
  bucket: "cargoship-production"
  region: "us-west-2"          # must match the bucket's region
  concurrency: 20

watch_paths:
  - path: "/volume1/research-data"
    include_patterns: ["*.fastq.gz", "*.bam", "*.vcf.gz"]
    exclude_patterns: ["*.tmp", "*.lock"]
    min_age: "1h"
    storage_class: "STANDARD"
    recursive: true

max_concurrent_jobs: 4
scan_interval: "5m"
```

## Deployment

Ghost ships are distributed as platform-specific container images and run via
Container Station (QNAP) or Container Manager (Synology). A typical run mounts the
data directory read-only, the config, and AWS credentials read-only:

```bash
docker run -d \
  --name cargoship-ghost-container \
  --platform linux/amd64 \
  -e AWS_PROFILE=aws \
  -e AWS_DEFAULT_REGION=us-west-2 \
  -v /path/to/.aws:/home/cargoship/.aws:ro \
  -v /path/to/ghost_ship.yaml:/etc/cargoship/ghost_ship.yaml:ro \
  -v /volume1/research-data:/volume1/research-data:ro \
  --restart unless-stopped \
  cargoship-ghost:latest
```

::: warning Region must match
Set `AWS_DEFAULT_REGION` to the bucket's region explicitly. The AWS SDK prioritizes
the environment variable over the config file, and a mismatch causes S3 301
`PermanentRedirect` errors. QNAP/Synology are typically x86_64 — build/run images
with `--platform linux/amd64` to avoid exec-format errors.
:::

## Signed config over S3 (pull mode)

Instead of shipping a config file to every box, the fleet can pull a **signed** config
from S3. The operator signs the config with an ed25519 key; each agent bakes only the
**public** key and refuses any config that doesn't verify — so a tampered or
attacker-substituted config can't tell a box what to back up.

```bash
# once, on the control machine:
cargoship ghostship config-keygen --out ./fleet-keys      # private + public PEM
cargoship ghostship sign-config box.yaml --key ./fleet-keys/config-signing-private.pem

# upload the config + its signature to the control prefix:
aws s3 cp box.yaml     s3://backups/nas/fleet/lab-nas-1/config.yaml
aws s3 cp box.yaml.sig s3://backups/nas/fleet/lab-nas-1/config.yaml.sig

# on each agent (bake only the public key):
cargoship ghostship run --config-url s3://backups/nas \
  --public-key /etc/cargoship/config-signing-public.pem --writer-id lab-nas-1 --once
```

The config lives under a **control prefix** `fleet/<id>/`, separate from the
`writers/<id>/` data prefix, and the writer's IAM grants only read there
(`cargoship ghostship iam-policy` emits it). Bump the config's `version` before signing
— each writer reports the version it is running in its heartbeat (`fleet status`), so a
bad rollout is visible fleet-wide. Verification is fail-closed: an unsigned, wrong-key,
or tampered config is refused and nothing is backed up.

**Keep-last-good.** When running (not `--once`), the agent re-pulls and re-verifies the
config every cycle and swaps only if it is a valid signed config. If a refresh fails —
S3 unreachable, unsigned, bad signature, or invalid — it keeps running the **last-good**
config, keeps backing up, and reports `config_error` in its heartbeat (visible in
`fleet status`) while still reporting the version it is actually running. Push a fixed,
re-signed config and the next cycle adopts it automatically.

**No silent scope-widening.** A signature proves the operator authored a config, but a
pulled config that *widens* what the writer reads or deletes — adds/broadens a watch
path, flips `recursive` on, adds includes, removes excludes, or enables
`delete_after_archive` — is refused unless it also sets `allow_scope_expansion: true`
(itself inside the signed bytes). Without that explicit opt-in the daemon keeps the
last-good config and reports `config_error`. Set `allow_scope_expansion: true` on the
rollout that legitimately widens scope; you can validate this before deploy with
`cargoship ghostship validate-config new.yaml --baseline deployed.yaml` (it prints
`WIDENS-SCOPE:` lines).

## Security

- Runs as a non-root user; data and AWS credentials are mounted read-only.
- Outbound HTTPS to S3 only; a ghost ship opens no listening port.
- Config is pulled read-only and signature-verified (see above); the writer cannot write
  its own control prefix.
- Optional server-side encryption for archived files.

## Monitoring

Each backup cycle writes a heartbeat to `writers/<id>/status.json`. Read the fleet from
the control side (read-only — no agent identity needed):

```bash
cargoship fleet status  s3://my-bucket/backups            # last check-in, health, per writer
cargoship fleet monitor s3://my-bucket/backups --once     # alert on writers gone stale
```

`fleet monitor` fires a stale-writer alert (and evaluates budgets) through the channels you
configure with `cargoship alerts`; run it on a schedule from the control machine. Local
container health is still just:

```bash
docker ps | grep cargoship-ghost
docker logs cargoship-ghost-container --tail 20
```

## Retention, lifecycle & immutability

The agent is **write-only and delete-free** — its IAM policy grants no `s3:DeleteObject`, so
a compromised agent can only append to its own prefix, never destroy history. The flip side:
**all deletion happens on the control machine**, where the delete-capable credential lives.

### Control-side retention

Prune old dataset versions from the control machine, never the agent:

```bash
cargoship dataset prune --dataset-id <id> --keep-last 30 --region <r>
```

`dataset prune` needs `s3:DeleteObjects` — exactly the permission the agent withholds — so it
belongs to an operator role, not the fleet. (It does not yet support encrypted-manifest
datasets; those are retained until that lands.)

### Lifecycle hygiene (the agent never cleans up)

The agent's partial/rollback cleanup is intentionally disabled (it can't delete). Add an S3
lifecycle rule so failed multipart uploads don't accrue cost forever:

- `AbortIncompleteMultipartUpload` after e.g. 7 days.

You can apply lifecycle rules with `cargoship lifecycle` or the AWS console/CLI.

### Immutability backstop (versioning + Object Lock)

Enable **S3 Versioning + Object Lock (governance mode)** on the fleet bucket so that even a
*stolen delete-capable control credential* can't destroy history within the retention window.
Object Lock requires versioning and is set at (or, on a versioned bucket, via
`PutObjectLockConfiguration` per the AWS docs) bucket setup — it is deliberately **not** agent
code. Governance mode can be bypassed only by a principal holding
`s3:BypassGovernanceRetention`; compliance mode cannot be bypassed by anyone, including root.

Verify the backstop is actually in place (read-only, changes nothing):

```bash
cargoship fleet lock-status s3://my-bucket/backups
```

It audits versioning, Object Lock (mode + default retention), and the abort-incomplete-MPU
lifecycle rule, and prints remediation for anything missing.

### CMK custody is the long-term recovery SPOF

Use a **dedicated fleet CMK**: the agent gets `kms:GenerateDataKey` (+ `kms:Decrypt`, which
AWS requires for SSE-KMS multipart uploads) on that one key; a separate restore/break-glass
role holds `kms:Decrypt`. Be honest about the trade: with no scheduled deletion and immutable
history, **losing the CMK means losing the data**. Treat the key as the crown jewel — enable
key rotation, back up / multi-Region the key material, and guard its key policy accordingly.

## Disaster recovery: restore a writer to a new box

A ghostship's backups do not depend on the box that wrote them. If a NAS/server dies,
recover its **entire** backup onto any machine with read access to the bucket (and, if the
data is encrypted, `kms:Decrypt` on the key) — you do **not** need the old box, its
writer id, or any local state.

1. **Find the upload** — list the writer's uploads:
   ```bash
   cargoship dashboard s3://my-bucket/backups
   # or inspect the prefix directly:
   #   s3://my-bucket/backups/writers/<writer-id>/uploads/<upload-id>/
   ```
2. **Provision read / break-glass access** — a restore identity needs only
   `s3:GetObject` + `s3:ListBucket` on the writer's prefix (plus `kms:Decrypt` on the CMK
   if encrypted): the read-only inverse of the write-only policy that
   `cargoship ghostship iam-policy` emits.
3. **Restore the whole upload** to a fresh directory:
   ```bash
   cargoship restore s3://my-bucket/backups/writers/<writer-id>/uploads/<upload-id> ./restored --all
   ```
   `--all` reconstructs every file in the upload. Restore is self-describing — the manifest
   carries the chunk layout and (encrypted) data-key references; no writer identity or agent
   state is consulted.
4. **Verify** (optional):
   ```bash
   cargoship verify s3://my-bucket/backups/writers/<writer-id>/uploads/<upload-id>
   ```

This recovery path is exercised end-to-end in the emulator (`TestGhostshipRun_DisasterRecovery`)
and against real S3 (writer-scoped round-trip), and is surfaced in the per-release
[verification report](/project/verification-reports).

## See also

- [Distributed / Enterprise overview](/enterprise/).
- [QNAP / NAS deployment](/enterprise/qnap) — step-by-step QNAP guide.
