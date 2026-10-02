---
title: Beta tester brief
description: What to try, what is known-rough, and how to report findings — written for someone evaluating CargoShip for the first time.
---

# Beta tester brief

This page is for someone evaluating CargoShip who did not write it. It states what is
worth exercising, what is **already known to be rough** so you don't spend time
rediscovering it, and what a useful report looks like.

If you are deploying fleet mode on a NAS, read this first and then the
[NAS guide](/enterprise/qnap).

## What is solid enough to lean on

These are covered by integration tests and a per-release run against **real AWS S3**,
published as a [dated verification report](/project/verification-reports) attached to every
release:

- **Upload → verify → restore round-trips are byte-identical**, across both the `direct`
  (small-file) and `chunked` (`tar.zst`) paths, over an adversarial corpus (empty files,
  large files, incompressible and highly-compressible content, deep nesting, unicode /
  spaces / dotfile names).
- **The archive format is open and portable** — `tar.zst` plus a JSON manifest, readable
  with `tar`, `zstd` and `jq` without CargoShip installed. This is the property to check
  if you care about long-term recoverability.
- **Releases are signed** (cosign keyless) and ship an SPDX SBOM. See
  [verifying a release](https://github.com/scttfrdmn/cargoship/blob/main/SECURITY.md#verifying-a-release).

## What we most want tested

Roughly in order of how much we would learn:

1. **Restore, not just upload.** A backup you have not restored is a hypothesis. Restore a
   single file with `--file`, and a whole dataset with `--all`. This is the path where
   real-world surprises have historically shown up.
2. **Fleet mode on real hardware** ([NAS guide](/enterprise/qnap)) — a real NAS, real
   directory layout, left running for several cycles. Specifically: does it stay healthy
   across a reboot, and does `cargoship fleet status` tell you the truth about it?
3. **Incremental sync over time.** Let a writer run for days with files being added,
   changed, moved and deleted, then restore and compare. Check `delta_source` in
   `cargoship fleet status` — see "known-rough" below.
4. **Your actual data shape.** Our corpus is adversarial but synthetic. Millions of tiny
   files, or a handful of multi-TB files, will stress things our tests do not.
5. **Cost estimates against your real bill.** `cargoship estimate` models spend before
   upload; we would like to know how close it lands.

## Known-rough — please don't spend time here

Reporting these again costs you time and tells us nothing new.

| Area | Status |
|---|---|
| **macOS downloads are SIGKILLed** | Binaries are not notarized ([#407](https://github.com/scttfrdmn/cargoship/issues/407)). Gatekeeper kills them with a bare `Killed: 9`. Use Homebrew, or clear the quarantine attribute — see [installation](/start/install). |
| **`--use-checksum` does nothing** | The flag is accepted but change detection is always size + mtime ([#678](https://github.com/scttfrdmn/cargoship/issues/678)). Do not rely on it to catch a same-size, same-mtime edit. |
| **`dataset prune` refuses encrypted manifests** | It needs to read the chain to decide what is safe to delete. It errors clearly rather than deleting the wrong thing, but those datasets accumulate. |
| **Multi-region is library-only** | `pkg/multiregion` is **not** wired into `cargoship upload`. Use `--region` plus sharding. See [maturity](/project/maturity). |
| **Legacy Docker images** | `docker/Dockerfile.{ghost-ship,qnap,astrapi}` build a dormant daemon and are not the fleet agent. Use the published `ghcr.io/scttfrdmn/cargoship` image. |
| **Fleet config surface may change** | Fleet mode is **Beta**: the config schema and `fleet`/`ghostship` commands can change between minor releases. |

## Specifically for write-only fleet agents

A strict write-only identity cannot read its own previous manifest, so CargoShip keeps a
**local manifest cache** and diffs against that. Two things to watch, because they are new:

- **`delta_source` should be `cache`** (or `s3`) on steady-state cycles. A writer reporting
  `none` every cycle is re-uploading everything — usually because the `cargoship-state`
  volume is missing from the compose file, so the cache is recreated empty on each restart.
- **One full cycle a month is expected.** The cache is trusted for 30 days, because it
  cannot detect that objects it vouches for were deleted or lifecycled. Expiry forces a
  full re-establish on purpose.

If you suspect the cache is wrong, `--force` ignores it entirely and rebuilds from scratch.
We would very much like to hear if you ever need that.

## What a useful report contains

File issues at
[github.com/scttfrdmn/cargoship/issues](https://github.com/scttfrdmn/cargoship/issues).

Most valuable, in order:

1. **`cargoship --version`** (the full line — it includes the commit).
2. **The exact command**, with secrets removed.
3. **What you expected and what happened.** For a data-integrity concern, the *specific*
   path that differed matters more than a summary count.
4. **Platform**: OS, architecture, and for fleet mode the NAS model and container runtime.
5. **Scale**: file count and total size, at least to an order of magnitude.

For anything that looks like **data loss, a wrong restore, or a byte mismatch**, say so
plainly in the title — that class gets priority over everything else, and we would rather
chase a false alarm than miss a real one.

Please report security issues **privately** via
[GitHub Security Advisories](https://github.com/scttfrdmn/cargoship/security/advisories/new),
not as a public issue.

## What we will not pretend

There is no support SLA, no recovery-time objective, and this is pre-1.0 software whose
beta components may change shape between minor releases. The thing we hold most firmly is
the [archive-compatibility guarantee](/project/maturity#guarantees): archives written by one
release stay readable by later ones within the documented format versions.
