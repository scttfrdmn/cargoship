# Integrity model

This page states what CargoShip promises about the integrity and recoverability
of your data — and, just as importantly, what it does **not**. It is written
like a threat model: descriptive, bounded, and backed by mechanisms you can
inspect and verify yourself, not by assurances you have to take on faith.

::: info Why "integrity model", not "guarantees"
No tool can promise your bytes are eternally safe — that depends on AWS, the
network, and hardware nobody controls. What CargoShip *can* do is make a
**narrow, checkable claim** and give you the tools to falsify it at any time.
That's the model below. For the separate question of *who can read* your data,
see the [Security model](/project/security).
:::

## The core claim

> **Every file CargoShip uploads is recorded with enough information to detect,
> on restore, any deviation from what was uploaded — and you can verify this
> yourself, with or without CargoShip.**

This is the same trust model that established backup tools (`restic`, `borg`)
rely on: integrity is not asserted once, it is **continuously and independently
checkable**. Three things make the claim real.

## 1. Integrity is checkable — `verify --deep`

Every upload records a SHA-256 checksum at two levels:

- **Per chunk** — the exact bytes of each stored `.tar.zst` object.
- **Per file** — the content of each individual file inside the archive (on by
  default; see `--no-file-checksums`).

The algorithm is recorded in the manifest (`checksum_algorithm`), so a reader
always knows how to recompute.

[`cargoship verify`](/reference/commands/inspect) validates the manifest's
internal consistency. **`cargoship verify --deep`** goes further: it
re-downloads the stored objects, recomputes both levels of checksum, and
compares them to the manifest. It exits non-zero on **any** corrupted, missing,
or unverifiable file or chunk.

```bash
# Fast: manifest structure only.
cargoship verify s3://bucket/prefix/uploads/<upload-id>

# Deep: re-download and re-checksum the actual stored data.
cargoship verify s3://bucket/prefix/uploads/<upload-id> --deep
```

Deep verify is the mechanism behind the core claim. It is deliberately strict:
a manifest that *can't* be verified (no checksums recorded) fails in deep mode
rather than passing silently — the whole point is to confirm data, and a
manifest without checksums can't.

::: tip What "unverifiable" means
Deep verify treats a chunk or file with no recorded checksum as a **failure**,
not a pass. Archives written before checksum capture existed (or with
`--no-file-checksums`) are reported honestly as unverifiable rather than
waved through.
:::

### Verify on restore, too

`verify --deep` is an explicit audit, but you don't have to remember to run it
to be protected on the path that matters most. **`cargoship restore` verifies
each file's checksum as it writes**, and **refuses to write a file whose stored
bytes don't match the manifest** — it counts that file as failed rather than
handing you corrupt data. Files with no recorded checksum restore as before.
Pass `--no-verify` to skip the check when throughput matters more than catching
corruption.

## 2. The format is open — you don't need CargoShip to recover

Integrity you can only check with the tool that wrote the data is weak
integrity. CargoShip's on-disk format is **open, documented, and versioned**:

- Chunks are `tar` archives wrapped in a single `zstd` frame — extract with
  standard `tar` and `zstd`.
- The manifest is JSON (optionally gzipped) with a
  [published schema](/reference/format/manifest).
- The full wire format is specified in the
  [Archive & Manifest Format Spec](/reference/format/).

This is enforced, not just claimed:

- A **machine-checkable JSON Schema** ships in the repo
  (`pkg/manifest/schema.json`) and is **drift-checked in CI** against the Go
  structs, so the schema, the code, and the docs cannot silently diverge.
- An **independent-reader test** extracts a manifest and a file using only
  standard-library tooling (`gzip`, `json`, `tar`) plus `zstd` — none of
  CargoShip's own code — proving the spec is sufficient for a third party to
  recover data in any language.
- **Version fixtures** (`2.0`, `1.0`) are checked in and must keep parsing and
  validating, so a format-version bump can't silently orphan older archives.

If CargoShip disappeared tomorrow, your data would still come back with tools
that ship on every Unix system.

## 3. The claim is tested continuously

The integrity mechanisms are exercised on every change, not just at release:

- Emulator-based round-trip tests (upload → download → verify) run on every PR,
  credential-free.
- A real-AWS integration lane runs the same round-trip against a real S3 bucket
  on **every release tag** — so a published version is always validated against
  real S3, and the result is published as a
  [verification report](/project/verification-reports). A weekly canary runs the
  same lane to catch environmental drift between releases. It deliberately does
  *not* run per-PR: it needs real credentials and mutates a shared bucket, and
  the emulator lane above already guards every change.
- The deep-verify path is tested against **deliberately corrupted** objects —
  the test suite confirms that flipping a byte in storage makes `verify --deep`
  fail loudly. Detecting corruption is the guarantee; the tests prove it detects.
- The format and verification code is **fuzzed** — see below.

### Fuzzing the trust surface

Hand-written tests cover the cases someone thought of. Fuzzing covers the ones
nobody did, which is exactly where integrity bugs hide: a manifest is external
input, and the code that parses it, resolves S3 keys from it, and turns its
recorded paths into local filenames is all reachable from data CargoShip did not
author.

So those functions are fuzzed against **invariants**, not just for crashes:

| Target | Invariant |
| --- | --- |
| Restore path sanitizer | An accepted path always lands **inside** the destination directory. A violation is a [path traversal](/project/security), not a wrong answer. |
| S3 key resolver | A resolved key is never a URL, is always prefix-scoped, and resolving twice changes nothing. |
| Schema validator | Any manifest CargoShip writes validates against its own [published schema](/reference/format/manifest). |
| Manifest (de)serializers | Arbitrary bytes never panic; a parsed manifest re-serializes byte-identically. |

Every pull request runs a short burst against each target; a weekly lane runs
them roughly 40× longer. Every crashing input ever found is committed to the
repository as permanent regression corpus, so a fixed bug is re-tested on every
run forever. This has already found real defects that tests missed — including a
resolver that destroyed any object key containing `://` in a filename, and
manifests being written in a shape the published schema rejected.

## What CargoShip does **not** promise

Being explicit about the boundaries is part of the model:

- **Not durability.** CargoShip does not protect against AWS losing an object,
  a bucket being deleted, or a region outage. That is S3's durability domain
  (and your bucket policy, versioning, and replication choices). CargoShip lets
  you **detect** loss on restore; it does not prevent it. Enable S3 Versioning
  and appropriate replication for durability.
- **Not tamper-proofing against an attacker with write access.** The checksums
  detect accidental corruption and bitrot. An adversary who can rewrite *both*
  a chunk and the manifest it's checksummed in could produce a self-consistent
  forgery. For confidentiality and authenticated metadata, use
  [manifest encryption](/project/security); for immutability, use S3 Object
  Lock.
- **Not verification of data CargoShip never saw.** Deep verify confirms stored
  bytes match what was recorded *at upload time*. It cannot vouch for a file
  that was already corrupt on disk before upload — checksum what matters at the
  source if that is a concern.
- **Standalone auditing is not automatic.** Restore verifies as it writes (see
  above), so the recovery path is guarded by default. But the proactive
  `verify --deep` audit — re-downloading data to catch bitrot *before* you need
  a restore — costs GET requests and transfer and is a command you run (or
  schedule), not a background process. Run it periodically for long-term
  archives.

## Verifying for yourself

You never have to trust this page. To confirm integrity end to end:

```bash
# 1. Deep-verify a completed upload against its manifest.
cargoship verify s3://bucket/prefix/uploads/<upload-id> --deep

# 2. Or bypass CargoShip entirely — pull a chunk and extract with standard tools.
aws s3 cp s3://bucket/prefix/uploads/<upload-id>/shard-0/chunk-0.tar.zst .
zstd -d chunk-0.tar.zst
tar xf chunk-0.tar

# 3. Recompute a file's checksum and compare it to the manifest's record.
sha256sum path/to/restored/file
```

If step 3's digest matches the `checksum` recorded for that file in the
manifest, the file is byte-identical to what was uploaded. That check depends on
nothing but `sha256sum` and a JSON parser — which is the whole point.

And you don't have to take the claim on faith for a given release either. The
same round trip is run against real S3 before each version ships, and the result
is published: for **v0.18.0**, 20 files / 61.01 MB round-tripped byte-identical
across both the direct and chunked paths, with 0 byte-identity failures and 45/45
integration suites passing —
[read the report](https://github.com/scttfrdmn/cargoship/releases/download/v0.18.0/v0.18.0-2026-08-02.md)
or see [all published reports](/project/verification-reports#published-reports).

## See also

- [Verification reports](/project/verification-reports) — dated, per-release
  evidence this claim held on real S3
- [`verify` command reference](/reference/commands/inspect)
- [Archive & Manifest Format Spec](/reference/format/)
- [Manifest schema](/reference/format/manifest)
- [Security model](/project/security) — confidentiality and encryption
- [Recovery & operations runbook](/reference/recovery)
