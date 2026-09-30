# QNAP / Synology NAS deployment

Deploy a CargoShip [fleet writer](/enterprise/ghost-ship) on a NAS: an unattended,
**write-only** agent that backs up the NAS's own directories to S3 on a schedule, pulls
a **signed** config from S3, and cannot read back, decrypt, or delete anything — not
even its own data.

Written for QNAP Container Station; Synology Container Manager is the same picture with
its own volume paths. You drive setup from a **control machine** (your laptop) and the
NAS only ever runs the agent.

::: warning Read this before you point it at terabytes
Under the strict write-only identity an agent **cannot read its previous manifest**, so
every cycle is a **full** sync — it re-uploads everything, every interval. That is fine
for tens of GB and ruinous for a multi-TB NAS.

For a large NAS, emit the non-strict policy instead (drop `--write-only`):

```bash
cargoship ghostship iam-policy s3://my-bucket/nas --writer-id lab-nas-1 \
  --kms-key-arn arn:aws:kms:… > iam-policy.json
```

That grants the writer `GetObject` on **its own prefix only** (plus `kms:Decrypt` on the
one fleet key) so it can read its last manifest and upload just the delta. It is still
delete-free and still cannot touch another writer's data. A local manifest cache that
restores incremental sync *under* the strict policy is planned but not shipped.
:::

## Prerequisites

- **On the NAS:** Container Station (QNAP) or Container Manager (Synology), and SSH.
- **On your control machine:** `cargoship` installed ([install](/start/install)), AWS
  credentials that can create IAM identities (or your own IAM tooling), and an existing
  S3 bucket.
- The NAS needs **outbound HTTPS only**. No inbound port, no port forwarding.

::: tip Architecture is handled for you
The published image is a multi-arch manifest (`linux/amd64` + `linux/arm64`), so the same
reference works on older Intel NAS boxes and newer ARM ones. You no longer build or
transfer an image — that is what the old version of this guide did.

Container Station's docker binary is not on `PATH` for SSH sessions. On QNAP it is
typically `/share/ZFS530_DATA/.qpkg/container-station/bin/docker` — the exact volume name
varies, so find it with
`ls -d /share/*/.qpkg/container-station/bin/docker`. Below, `docker` means that path.
:::

## 1. Create the fleet signing key (control machine, once)

The operator signs configs; the NAS only ever holds the **public** half.

```bash
cargoship ghostship config-keygen --out ./fleet-keys
```

Keep `config-signing-private.pem` off the NAS. Anyone holding it can tell your fleet what
to back up.

## 2. Scaffold the writer bundle (control machine)

```bash
cargoship ghostship init s3://my-bucket/nas \
  --writer-id lab-nas-1 \
  --kms-key-arn arn:aws:kms:us-west-2:123456789012:key/abcd-1234 \
  --public-key ./fleet-keys/config-signing-public.pem \
  --region us-west-2 \
  --out ./lab-nas-1
```

You get `iam-policy.json`, a `config.yaml` skeleton, the baked public key, a
`compose.yaml` that already references the published image, and a README.

Add `--mint` if your credentials have `iam:Create*` and you want CargoShip to create the
IAM user, attach the policy, and write `aws-credentials` (mode 0600) into the bundle for
you. Otherwise attach `iam-policy.json` to a new IAM identity yourself and drop its key
into `./lab-nas-1/aws-credentials` in standard credentials-file format.

## 3. Fill in and sign the config (control machine)

Edit `./lab-nas-1/config.yaml` so `watch_paths` names the NAS paths to back up, **as the
container will see them**:

```yaml
id: lab-nas-1
writer_id: lab-nas-1
version: 1
s3_config:
  bucket: my-bucket
watch_paths:
  - path: /volume1/research-data
  - path: /volume1/Documents
scan_interval: 1h
```

Check it, then sign it:

```bash
cargoship ghostship validate-config ./lab-nas-1/config.yaml
cargoship ghostship sign-config ./lab-nas-1/config.yaml \
  --key ./fleet-keys/config-signing-private.pem
```

Upload the config and its signature to the control prefix:

```bash
aws s3 cp ./lab-nas-1/config.yaml     s3://my-bucket/nas/fleet/lab-nas-1/config.yaml
aws s3 cp ./lab-nas-1/config.yaml.sig s3://my-bucket/nas/fleet/lab-nas-1/config.yaml.sig
```

## 4. Add the data mounts

The emitted `compose.yaml` has a commented placeholder. Mount every `watch_paths`
directory **read-only, at the same path** the config names:

```yaml
    volumes:
      - ./config-signing-public.pem:/etc/cargoship/config-signing-public.pem:ro
      - ./aws-credentials:/home/cargoship/.aws/credentials:ro
      - cargoship-state:/home/cargoship/.cargoship
      - /volume1/research-data:/volume1/research-data:ro   # add these
      - /volume1/Documents:/volume1/Documents:ro
```

Read-only is deliberate: the agent only ever reads your data. Keep the
`cargoship-state` volume — it holds resume state, and losing it costs a re-scan.

## 5. Deploy (on the NAS)

```bash
ssh admin@nas.local "mkdir -p ~/cargoship"
scp -r ./lab-nas-1/* admin@nas.local:~/cargoship/
ssh admin@nas.local "cd ~/cargoship && \
  /share/ZFS530_DATA/.qpkg/container-station/bin/docker compose up -d"
```

The agent pulls its config, **verifies the signature** against the baked public key, and
starts backing up on the interval. An unsigned, tampered, or wrong-key config is refused
and nothing is uploaded. `restart: unless-stopped` brings it back after a NAS reboot.

## 6. Watch it from the control machine

You do not need to SSH in to see whether the fleet is healthy — every cycle writes a
heartbeat to S3:

```bash
cargoship fleet status  s3://my-bucket/nas          # age, health, config version
cargoship fleet monitor s3://my-bucket/nas --once   # alert on writers gone stale
cargoship dashboard     s3://my-bucket/nas          # TUI; press 6 for the Fleet tab
```

Both are read-only and need no agent credentials. On the NAS itself, `docker logs -f
cargoship-ghostship-1` is the local channel — the agent binds **no** port, so there is
no health endpoint to curl.

## 7. Rehearse a restore before you trust it

A backup you have not restored is a hypothesis. Do this once, from the control machine,
with a break-glass identity (bucket read + `kms:Decrypt` — *not* the writer's key):

```bash
cargoship fleet status s3://my-bucket/nas --json   # find the writer's upload id
cargoship restore s3://my-bucket/writers/lab-nas-1/uploads/<upload-id> ./restore-test --all
```

Compare a few files against the NAS. Recovery never needs the original box, its writer
id, or any local state.

## 8. Protect the history

The agent cannot delete, but a stolen *control* credential could. Enable S3 Versioning +
Object Lock on the bucket and verify it:

```bash
cargoship fleet lock-status s3://my-bucket/nas
```

See the retention and immutability section of [ghost-ship](/enterprise/ghost-ship) for
the rest, including why pruning runs from the control machine and why losing the fleet
CMK loses the data.

## Troubleshooting

| Symptom | Cause & fix |
|---------|-------------|
| `docker: command not found` over SSH | Container Station's docker is not on `PATH`; use the full path (`ls -d /share/*/.qpkg/container-station/bin/docker`). |
| `exec format error` | An image built for the wrong arch. The published image is multi-arch — pull it rather than building locally, and don't pin an `-amd64`/`-arm64` suffixed tag by hand. |
| `exec: cargoship: not found` | You are running a locally built image that has no binary in it. Pull `ghcr.io/scttfrdmn/cargoship:<version>`. |
| S3 301 `PermanentRedirect` | Region mismatch. The emitted compose passes `--region`; make it the bucket's region. |
| Credentials mounted but "no credentials" | Mounted at `/root/.aws`? The image runs as uid 65532 whose home is `/home/cargoship` — mount `/home/cargoship/.aws/credentials`. |
| `Permission denied` reading `aws-credentials` | `chmod 600 ~/cargoship/aws-credentials` on the NAS. |
| Config refused: signature | The uploaded `config.yaml` no longer matches `config.yaml.sig`. Re-sign after **every** edit and upload both. |
| Config refused: "widens scope" | You added a watch path (or enabled source deletion). That needs `allow_scope_expansion: true` in the signed config — deliberately, so a config pull cannot silently broaden what the NAS reads. |
| Every cycle re-uploads everything | Expected under the strict write-only policy — see the warning at the top. Use the non-strict policy for large datasets. |
| `fleet status` shows nothing | No heartbeat yet (give it one interval), or you pointed it at the wrong base prefix — use the same `s3://bucket/base` you passed to `init`. |
| Writer shows `config_error` but stays healthy | Working as designed: a bad config refresh keeps the **last-good** config running. Fix and re-upload; the next cycle adopts it. |
| Looking for a metrics or health endpoint | There isn't one. The agent is outbound-only; `fleet status` and `docker logs` are the status channels. |

## Security notes

- The agent runs as **non-root** (uid 65532) and mounts your data **read-only**.
- Its IAM identity is **write-only and delete-free** — no `s3:DeleteObject`, no access to
  other writers, and (with `--write-only`) no `kms:Decrypt`. A compromised NAS cannot
  destroy or read back your backups.
- Credentials arrive as a **mounted file**, never environment variables, which would
  otherwise be visible in `docker inspect` and NAS UI panels.
- No inbound port is opened or needed.
- Decommission a writer with
  `cargoship ghostship scuttle lab-nas-1` (add `--purge-data s3://my-bucket/nas --yes` to
  delete its backups too).

## See also

- [ghost-ship](/enterprise/ghost-ship) — the fleet agent in full.
- [Fleet tutorial](/enterprise/fleet-tutorial) — the same flow without NAS specifics.
- [Deployment guide](/enterprise/deployment).
