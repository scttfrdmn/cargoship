# Verification reports

The [Integrity model](/project/integrity) describes *how* CargoShip makes its
byte-identity claim checkable. This page is the other half: the **dated,
per-release evidence** that the claim actually held for each published version.

Trust in a storage tool is not a one-time proof — it is the same routine,
adversarial verification run again and again, in the open. Every CargoShip
release is validated against **real AWS S3** before it ships, and the result is
published as a dated report attached to the release.

## What each report contains

For a given version, the report records:

- **Files and bytes round-tripped byte-identical** — a hostile corpus (empty
  files, large files, incompressible and highly-compressible content, deep
  nesting, unicode / spaces / dotfile names) uploaded through the real pipeline
  and restored through the real restore path, then compared by SHA-256.
- **Storage paths exercised** — both `direct` (small files) and `chunked`
  (`tar.zst` objects) must pass; the report names which ran.
- **Integration suites** — the credential-gated `//go:build integration` suites
  that ran against real S3, with pass/fail counts.
- **The date, version, and commit** the evidence was produced from.

Crucially, the report is generated from the **exact test run that gated the
release** — not a separate, later run — so the published numbers cannot drift
from what actually passed.

## Published reports

Every report below is linked directly — you should not have to dig through
release assets to find the evidence.

| Version | Date | Files | Bytes | Paths | Suites | Result | Report |
|---------|------|-------|-------|-------|--------|--------|--------|
| v0.23.0 | 2026-08-04 | 20 | 61.01 MB | direct, chunked | 43 / 0 | ✅ Passed | [`v0.23.0-2026-08-04.md`](https://github.com/scttfrdmn/cargoship/releases/download/v0.23.0/v0.23.0-2026-08-04.md) |
| v0.22.0 | 2026-08-04 | 20 | 61.01 MB | direct, chunked | 43 / 0 | ✅ Passed | [`v0.22.0-2026-08-04.md`](https://github.com/scttfrdmn/cargoship/releases/download/v0.22.0/v0.22.0-2026-08-04.md) |
| v0.21.0 | 2026-08-03 | 20 | 61.01 MB | direct, chunked | 43 / 0 | ✅ Passed | [`v0.21.0-2026-08-03.md`](https://github.com/scttfrdmn/cargoship/releases/download/v0.21.0/v0.21.0-2026-08-03.md) |
| v0.20.0 | 2026-08-03 | 20 | 61.01 MB | direct, chunked | 43 / 0 | ✅ Passed | [`v0.20.0-2026-08-03.md`](https://github.com/scttfrdmn/cargoship/releases/download/v0.20.0/v0.20.0-2026-08-03.md) |
| v0.19.0 | 2026-08-03 | 20 | 61.01 MB | direct, chunked | 46 / 0 | ✅ Passed | [`v0.19.0-2026-08-03.md`](https://github.com/scttfrdmn/cargoship/releases/download/v0.19.0/v0.19.0-2026-08-03.md) |
| v0.18.0 | 2026-08-02 | 20 | 61.01 MB | direct, chunked | 45 / 0 | ✅ Passed | [`v0.18.0-2026-08-02.md`](https://github.com/scttfrdmn/cargoship/releases/download/v0.18.0/v0.18.0-2026-08-02.md) |
| v0.17.1 | 2026-08-02 | 20 | 61.01 MB | direct, chunked | 45 / 0 | ✅ Passed | [`v0.17.1-2026-08-02.md`](https://github.com/scttfrdmn/cargoship/releases/download/v0.17.1/v0.17.1-2026-08-02.md) |
| v0.17.0 | 2026-08-02 | 20 | 61.01 MB | direct, chunked | 45 / 0 | ✅ Passed | [`v0.17.0-2026-08-02.md`](https://github.com/scttfrdmn/cargoship/releases/download/v0.17.0/v0.17.0-2026-08-02.md) |
| v0.16.1 | 2026-08-02 | 20 | 61.01 MB | direct, chunked | 45 / 0 | ✅ Passed | [`v0.16.1-2026-08-02.md`](https://github.com/scttfrdmn/cargoship/releases/download/v0.16.1/v0.16.1-2026-08-02.md) |
| v0.16.0 | 2026-07-31 | 20 | 61.01 MB | direct, chunked | 45 / 0 | ✅ Passed | [`v0.16.0-2026-07-31.md`](https://github.com/scttfrdmn/cargoship/releases/download/v0.16.0/v0.16.0-2026-07-31.md) |

*Files* = round-tripped byte-identical; *Suites* = integration suites
passed / failed against real S3. Reports begin at v0.16.0, the release that
introduced the lane; earlier versions have no published report.

A row reading **⏳ Run in progress** means the tag was just pushed and the
real-AWS lane has not finished. The row is added with the release commit rather
than after the fact, so a release can never be published without its evidence
being accounted for — but the numbers are deliberately left blank until the run
produces them, because they come from that run and cannot be known in advance.
If a row stays pending, the run has not passed and the release should be
treated as unverified.

The corpus is fixed, so the file and byte counts are expected to be identical
across releases — what each row attests is that *that build* preserved them. A
change in those numbers means the corpus changed, not that more was proven.

The *suite* count can move: it counts Go packages run under the `integration`
tag, so adding or deleting a package moves it. It rose at v0.19.0 (45 → 46) when
`internal/audit`, the structural tar-loop guard from #321, was added — more
surface covered, not more of the byte-identity claim proven. It fell at v0.20.0
(46 → 43) when `pkg/controller`, `cmd/controller` and `cmd/cargoship-launch` were
deleted with the distributed subsystem ([#340](https://github.com/scttfrdmn/cargoship/issues/340))
— less code to cover, not less coverage. It held at 43 across v0.21.0 even though
[#347](https://github.com/scttfrdmn/cargoship/issues/347) deleted 57 functions,
which is the expected behaviour: the count tracks *packages*, and `pkg/launch`
survived. A drop that does *not* correspond to a deletion would mean a suite
stopped running, which is a regression.

- Reports are also **attached to each
  [GitHub Release](https://github.com/scttfrdmn/cargoship/releases)** as
  `vX.Y.Z-YYYY-MM-DD.md`.
- The weekly canary produces one too (as a workflow artifact) to catch
  environmental drift between releases, even with no code change.

## What a report does and does not establish

- **Does** show that, for that exact build, upload → restore preserved bytes
  across both storage paths, verified against real S3, over an adversarial
  corpus, with an independently readable and schema-valid manifest.
- **Does not** show a proof for all possible inputs, protect against
  source-side corruption that predates the upload, or make cost/performance
  guarantees. Those are honestly bounded, not proven — see the
  [Integrity model](/project/integrity) for the full set of non-goals.

## Reproduce it yourself

You never have to trust the published report. With AWS credentials and a test
bucket, run the same invariant the release lane runs:

```bash
CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 \
CARGOSHIP_ENABLE_AWS_INTEGRATION_TESTS=true \
CARGOSHIP_TEST_BUCKET=<your-bucket> AWS_REGION=us-east-1 \
  go test -tags integration -run TestRoundTripProperty -v ./pkg/pipeline/
```

## See also

- [Integrity model](/project/integrity) — the mechanisms behind the claim
- [`verify` command reference](/reference/commands/inspect)
- [Archive & Manifest Format Spec](/reference/format/)
