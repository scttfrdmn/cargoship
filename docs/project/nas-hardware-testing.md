---
title: NAS hardware test runbook
description: The manual, periodic validation of fleet mode on real Synology and QNAP hardware — what it proves that CI cannot, and how to run it.
---

# NAS hardware test runbook

CI has no NAS and cannot reboot anything. This runbook covers the checks that therefore
cannot be automated, in the same spirit as the other manual-periodic lanes (real-AWS
torture, real-IAM mint smoke): **a repeatable procedure with explicit assertions, run
before a release that touches fleet mode — not a gate.**

Calling it manual is deliberate. A check that looks automated but is not is worse than one
honestly labelled, because the green checkmark is doing no work.

## What this proves that CI cannot

| Claim | Why it needs hardware |
|---|---|
| **The manifest cache survives a reboot** | The write-only incremental sync shipped in v0.35.0 depends on the `cargoship-state` volume genuinely persisting. If it does not, the agent silently reverts to re-uploading everything — which looks like "slow", not "broken". No CI runner can power-cycle a NAS. |
| **Real share furniture round-trips** | Synology scatters `@eaDir` thumbnail directories and a `#recycle` bin through every share. Nothing in the synthetic corpus looks like that. |
| **Two real writers, one bucket** | Two devices with two write-only IAM identities exercise [writer isolation](/enterprise/ghost-ship) against *real* IAM, which the release lane only ever does with one writer. |
| **Container Station / Container Manager** | Non-standard docker paths, volume semantics, and the exec-format trap. |
| **Real filesystem semantics** | ACLs, extended attributes, sparse files, hardlinks — the generator produces all of these; the synthetic corpus produces none. |

## Safety rules

These are not optional. The devices hold real data and the bucket costs real money.

- **Mount data read-only.** Every `watch_paths` entry is bind-mounted `:ro`. CargoShip only
  ever reads your data, and the mount should enforce that rather than trusting it.
- **Use a throwaway bucket**, and delete it afterwards. Verify it is gone.
- **One write-only IAM identity per device**, minted for the test and
  [scuttled](/enterprise/ghost-ship) after. Never a long-lived admin key on a NAS.
- **Start with the generated corpus, not a real share.** ~16 MB finds everything a
  multi-TB share would, in minutes instead of days, for cents instead of real spend.
- **Never commit the minted credentials** or the signing private key.

## 1. Build the corpus (control machine)

```bash
scripts/make-nas-corpus.sh build /tmp/nas-corpus
```

26 files, ~16 MB, deliberately hostile: `@eaDir` thumbnails, `#recycle`, `.DS_Store`,
unicode and emoji names, a leading-dash name, an empty file, an extensionless file, 8
levels of nesting, a hardlinked pair, a relative and a **broken** symlink, incompressible
and highly-compressible 4 MB files, an already-gzipped file, a sparse 8 MB file, a file
dated 2000 and one dated **tomorrow**, plus read-only and executable modes.

Copy it to a **dedicated test directory** on the NAS — not a real share:

```bash
ssh you@nas "mkdir -p /volume1/docker/cargoship-nastest/corpus"
tar --no-mac-metadata -cf - -C /tmp/nas-corpus . \
  | ssh you@nas "tar -xf - -C /volume1/docker/cargoship-nastest/corpus"
```

::: warning Two transfer traps, both of which silently change the corpus
**macOS `tar` pollutes it.** `bsdtar` stores Apple metadata by default, which lands on the
NAS as AppleDouble `._*` sidecars — 47 extra files beside 26 real ones in our run. The
restore then contains files the source does not, and the verifier correctly reports them as
`UNEXPECTED`. Use `--no-mac-metadata` (or `COPYFILE_DISABLE=1`), and check the file count on
the NAS matches the source before starting.

**`rsync` may not use your SSH key.** In our run `ssh` key auth worked while `rsync` fell
through to password auth and transferred nothing. `tar` over `ssh` uses the connection you
already proved, so prefer it.
:::

Confirm the hostile shapes survived the copy — `ext4` keeps all of them, but the transfer
is where they get lost:

```bash
ssh you@nas "cd /volume1/docker/cargoship-nastest/corpus && \
  echo files=\$(find . -type f | wc -l) symlinks=\$(find . -type l | wc -l) && \
  echo eaDir=\$(find . -path '*@eaDir*' -type f | wc -l) && \
  stat -c 'hardlink nlink=%h' links/hardlink-a.txt"
```

## 2. Provision and deploy

Follow the [NAS guide](/enterprise/qnap) — it is the same procedure a tester follows, which
is the point: running it here is also a test *of the guide*. In short:

```bash
cargoship ghostship config-keygen --out ./fleet-keys
cargoship ghostship init s3://THROWAWAY-BUCKET/nas --writer-id syno-1 \
  --public-key ./fleet-keys/config-signing-public.pem --mint --out ./syno-1
# edit ./syno-1/config.yaml so watch_paths names /volume1/cargoship-test
cargoship ghostship sign-config ./syno-1/config.yaml --key ./fleet-keys/config-signing-private.pem
# upload config + .sig to s3://THROWAWAY-BUCKET/nas/fleet/syno-1/
# add the read-only data mount to compose.yaml, scp the bundle, docker compose up -d
```

## 3. Assertions, in order

Each step has one thing to check. Record the answer even when it passes — the point of a
periodic lane is the trend, not a single green run.

**Cycle 1 — first sync**

```bash
cargoship fleet status s3://THROWAWAY-BUCKET/nas --json
```

- [ ] the writer appears, healthy, with a recent heartbeat age
- [ ] `delta_source` is `none` (nothing to diff against yet — correct on a first cycle)
- [ ] `sync_type` is `full`

**Cycle 2 — no changes**

- [ ] the cycle reports no changes and creates **no** new upload
- [ ] `delta_source` is `cache` (**not** `s3`: a write-only identity cannot read its own
      manifest, so reading `s3` here would mean the IAM policy is wider than intended)

**Cycle 3 — a real change**

Add one file to the share, wait for the next interval.

- [ ] only the new file is uploaded (`files=1`, not the whole corpus)
- [ ] `sync_type` is `incremental`, `delta_source` is `cache`

**Reboot — the v0.35.0 claim**

Power-cycle the NAS. After it comes back and one interval elapses:

- [ ] the container restarted on its own (`restart: unless-stopped`)
- [ ] `delta_source` is **still `cache`** — if it is `none`, the state volume did not
      persist and the agent is re-uploading everything. This is the single most valuable
      assertion in this runbook.

**Restore and verify (control machine, break-glass identity)**

Use a read-capable identity — *not* the writer's key.

```bash
cargoship restore s3://THROWAWAY-BUCKET/writers/syno-1/uploads/<upload-id> /tmp/nas-restore --all
scripts/make-nas-corpus.sh verify /tmp/nas-corpus /tmp/nas-restore
```

- [ ] `PASS — every file byte-identical at its original relative path`

The verifier compares **by relative path**, never by basename: right bytes at the wrong
path is a failure, and basename comparison cannot see it ([#486](https://github.com/scttfrdmn/cargoship/issues/486)).

## Results that are expected, not bugs

Three shapes in the corpus legitimately do not round-trip byte-for-byte, and the verifier
is built not to false-fail on them:

- **Symlinks are skipped.** The scanner ignores them unless `FollowSymlinks` is set, and
  enforces that again when opening files. So `symlink-rel.txt` and `symlink-broken.txt`
  should be absent from the restore. `find -type f` excludes symlinks, so the verifier
  never looks for them.
- **The sparse file restores fully allocated.** Its *bytes* are identical; only its block
  allocation differs. Bytes are the contract, so the verifier checks content, not `du`.
  (It may also be non-sparse at the source, depending on the filesystem — exFAT and HFS+
  will not create one.)
- **The hardlinked pair may restore as two independent files.** Both paths hold the
  correct content either way, which is what the contract requires.

Anything else — a missing `@eaDir` thumbnail, a mangled unicode name, the future-dated file
re-uploading every cycle — is a real finding. File it with the device model, DSM/QTS
version, and the `fleet status --json` output.

## 4. Tear down

```bash
cargoship ghostship scuttle syno-1 --purge-data s3://THROWAWAY-BUCKET/nas --yes
aws s3 rb s3://THROWAWAY-BUCKET --force      # then confirm it is gone
rm -rf ./syno-1 ./fleet-keys /tmp/nas-corpus /tmp/nas-restore
```

- [ ] the bucket is gone
- [ ] no `cargoship-writer-*` IAM users remain under the `/cargoship/` path
- [ ] the minted credentials and the signing private key are off the disk

## See also

- [NAS deployment guide](/enterprise/qnap) — the procedure this runbook exercises
- [Beta tester brief](/project/beta-testing) — what an outside tester should try
- [Verification reports](/project/verification-reports) — the automated per-release evidence
