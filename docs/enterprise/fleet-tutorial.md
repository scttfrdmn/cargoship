# Tutorial: stand up a ghostship fleet

This is an end-to-end, task-oriented walkthrough for running CargoShip as a **fleet of
unattended, write-only backup agents** ("ghostships"): one control machine you operate
from, and any number of boxes (NAS, lab servers, workstations) that each back up their
own data to a shared S3 bucket without being able to read, decrypt, or delete anyone
else's. For the concepts and reference detail behind each step, see
[ghost-ship](/enterprise/ghost-ship).

By the end you will have: a signing key, one deployed writer backing up on a schedule
from a signed config it pulls from S3, fleet-wide visibility, and a tested recovery
path.

## Model in one picture

- **Control machine** (you): holds the config-signing **private** key and a
  delete-capable identity for retention. Signs configs, watches the fleet, prunes.
- **Each writer**: holds only a **write-only** IAM identity and the signing **public**
  key. Pulls its signed config from S3, backs up its own `writers/<id>/` prefix, and
  can do nothing else.
- **The bucket**: one bucket, two prefixes per writer — `fleet/<id>/` (operator-written
  config, agent-read-only) and `writers/<id>/` (agent-written data).

Trust rests on absence: the writer's policy has no `s3:DeleteObject`, no `kms:Decrypt`,
and no access to other writers — so a compromised writer can only append to its own
subtree.

## 0. Prerequisites

- CargoShip installed on the control machine and on each writer box ([install](/start/install)).
- An S3 bucket for the fleet, and (recommended) a dedicated KMS key.
- On the control machine, AWS credentials that can create the writer identities (or
  your own IAM tooling).

## 1. Create the fleet signing key (once)

The operator signs every config; agents verify against the public half.

```bash
cargoship ghostship config-keygen --out ./fleet-keys
# writes ./fleet-keys/config-signing-private.pem (keep secret) and .public.pem
```

Keep the private key on the control machine (a secrets manager is ideal). The public
key is safe to distribute — it only lets an agent *verify*.

## 2. Scaffold a writer bundle

`init` generates everything one writer needs, and makes **no AWS calls**:

```bash
cargoship ghostship init s3://backups/nas \
  --writer-id lab-nas-1 \
  --kms-key-arn arn:aws:kms:us-west-2:123456789012:key/abcd-1234 \
  --public-key ./fleet-keys/config-signing-public.pem \
  --out ./lab-nas-1
```

The `./lab-nas-1/` bundle contains:

| File | Purpose |
| --- | --- |
| `iam-policy.json` | The **write-only** IAM policy for this writer. |
| `config.yaml` | Config skeleton — fill in `watch_paths`, then sign + upload. |
| `config-signing-public.pem` | The public key the agent verifies against. |
| `compose.yaml` | Runs the agent in signed config-over-S3 (pull) mode; credentials mount as a **file**, never env. |
| `README.md` | These steps, specialized for this writer. |

## 3. Create the writer identity

Attach `iam-policy.json` to a new IAM user (or role) — with your own tooling, Terraform,
or the console. That policy grants exactly: `s3:PutObject` on `writers/lab-nas-1/*`,
`s3:ListBucket` scoped to it, read-only `s3:GetObject` on `fleet/lab-nas-1/*`, and
`kms:GenerateDataKey` on the fleet key. Nothing else.

Put the resulting access key + secret in `./lab-nas-1/aws-credentials` (standard AWS
credentials-file format). `compose.yaml` mounts it read-only into the container.

::: tip Let CargoShip mint it (turnkey)
If your control-machine credentials have `iam:Create*`, re-run init with `--mint` and
CargoShip provisions the identity for you — it creates the IAM user, attaches the
write-only policy, creates an access key, writes `./lab-nas-1/aws-credentials`, and runs
the #529 access preflight:

```bash
cargoship ghostship init s3://backups/nas --writer-id lab-nas-1 \
  --kms-key-arn arn:… --public-key ./fleet-keys/config-signing-public.pem \
  --out ./lab-nas-1 --mint
```

Decommission the writer later with `cargoship ghostship scuttle lab-nas-1` (deletes the
key, policy, and user; add `--purge-data s3://backups/nas --yes` to also delete its
backups). Without `--mint`, init makes no AWS calls — you attach the policy yourself.
:::

::: tip Write-only trade-off
Without `GetObject` on its data, the agent can't re-read its previous manifest, so
incremental sync degrades to a full re-scan each cycle. If you prefer incremental (at
the cost of granting the writer read-back + `kms:Decrypt` on its own prefix), emit the
non-strict policy with `cargoship ghostship iam-policy s3://backups/nas --writer-id
lab-nas-1 --kms-key-arn …` (no `--write-only`).
:::

## 4. Fill in, validate, and sign the config

Edit `./lab-nas-1/config.yaml` — set `watch_paths` to what this box backs up:

```yaml
id: lab-nas-1
writer_id: lab-nas-1
version: 1
s3_config:
  bucket: backups
watch_paths:
  - path: /volume1/Documents
  - path: /volume1/Research
scan_interval: 1h
```

Validate before you sign (no AWS calls):

```bash
cargoship ghostship validate-config ./lab-nas-1/config.yaml
```

Sign it (this refuses to sign a config with validation errors):

```bash
cargoship ghostship sign-config ./lab-nas-1/config.yaml \
  --key ./fleet-keys/config-signing-private.pem
# writes ./lab-nas-1/config.yaml.sig
```

## 5. Upload the signed config to the control prefix

```bash
aws s3 cp ./lab-nas-1/config.yaml     s3://backups/nas/fleet/lab-nas-1/config.yaml
aws s3 cp ./lab-nas-1/config.yaml.sig s3://backups/nas/fleet/lab-nas-1/config.yaml.sig
```

## 6. Deploy the writer

On the box, with the bundle (and its `aws-credentials`) present:

```bash
docker compose -f ./lab-nas-1/compose.yaml up -d
```

The agent pulls `fleet/lab-nas-1/config.yaml`, **verifies the signature** against the
baked public key, validates it, and starts backing up on the interval. An unsigned,
wrong-key, or tampered config is refused and nothing is backed up. Every cycle it
re-pulls and re-verifies; if a refresh fails it **keeps running the last-good config**
and reports the problem in its heartbeat.

Prefer a plain config file (no S3 pull)? `cargoship ghostship run --config
./lab-nas-1/config.yaml` works too — you lose signature verification and hot-reload.

## 7. Watch the fleet

From the control machine (read-only — needs only bucket read/list):

```bash
cargoship fleet status  s3://backups/nas            # every writer: age, health, config version
cargoship fleet monitor s3://backups/nas --once     # alert on writers gone stale
cargoship dashboard     s3://backups/nas            # TUI; press 6 for the 🚢 Fleet tab
```

`fleet monitor` fires a stale-writer alert (and evaluates budgets) through the channels
you configure with `cargoship alerts`; run it on a schedule from the control machine.
Each writer reports the **config version** it is running, so a bad rollout is visible
fleet-wide.

## 8. Confirm the immutability backstop

The agent can't delete, but a stolen *control* credential could. Protect history with
S3 Versioning + Object Lock, and verify it (read-only):

```bash
cargoship fleet lock-status s3://backups/nas
```

It reports versioning, Object Lock (mode + retention), and the abort-incomplete-MPU
lifecycle rule, with remediation for anything missing. See the retention + immutability
section of [ghost-ship](/enterprise/ghost-ship).

## 9. Add more writers

Repeat steps 2–6 with a new `--writer-id` (e.g. `lab-nas-2`). One signing key covers the
whole fleet; each writer is isolated under its own prefix. `fleet status` lists them all.

## 10. Change a config safely

Edit, **bump `version`**, re-sign, and re-upload — the running agent adopts it on the
next cycle and reports the new version. Two guardrails:

- **Preview scope changes** before deploy:
  ```bash
  cargoship ghostship validate-config new.yaml --baseline deployed.yaml
  ```
  `WIDENS-SCOPE:` lines show anything that would broaden what the writer reads or deletes.
- **Widening is refused by default.** Adding a watch path, enabling `delete_after_archive`,
  or broadening a matcher requires `allow_scope_expansion: true` in the (signed) config;
  otherwise the agent keeps the last-good config and flags `config_error`. Set that flag
  deliberately on the rollout that widens.

## 11. Recover a writer's data

Recovery is independent of the box that wrote it — you need only bucket read + the
decryption key. Full runbook in [ghost-ship](/enterprise/ghost-ship); in short:

```bash
cargoship restore s3://backups/writers/lab-nas-1/uploads/<upload-id> ./restored --all
```

Restore uses a **break-glass** role (bucket `GetObject`/`ListBucket` + `kms:Decrypt`),
never the write-only agent identity. See [verify and restore](/start/verify-and-restore)
for verification.

## Retention (control-side only)

The agent is delete-free, so pruning old versions runs from the control machine:

```bash
cargoship dataset prune --dataset-id <id> --keep-last 30 --region us-west-2
```

`dataset prune` needs delete permission the writer deliberately lacks — keep it to an
operator role.

## Where to go next

- [ghost-ship](/enterprise/ghost-ship) — concepts + reference for every step here.
- [Deployment guide](/enterprise/deployment) and [QNAP / NAS deployment](/enterprise/qnap).
- [Recovery & operations runbook](/reference/recovery).
