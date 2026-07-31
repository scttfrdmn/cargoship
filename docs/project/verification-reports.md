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

## Where to find them

- **Attached to each [GitHub Release](https://github.com/scttfrdmn/cargoship/releases)**
  as `vX.Y.Z-YYYY-MM-DD.md`.
- Also produced by the weekly canary run (as a workflow artifact) to catch
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
