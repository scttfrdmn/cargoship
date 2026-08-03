# Project maturity & compatibility

CargoShip is production-oriented software, but it is still in the `v0.x` series
and maintained by a small open-source team. This page states plainly what is
stable, what is still evolving, and what guarantees you can (and can't) rely on —
so you can judge it against your own risk tolerance. For the semantic-versioning
rules behind these levels, see [API stability & versioning](/project/versioning).

## What the levels mean

**Stable** means the interface and archive format are intended for continued use
and are protected by release-gating verification against real S3. It does not
imply a commercial support SLA or a long production history.

That distinction is deliberate. Stable paths *have* had consequential defects
found in them — v0.16.1 and v0.17.0 fixed last-file truncation in compressed
chunks and a path traversal on the default `download` path. What the label
promises is not that no bug will be found; it is that the path is covered by the
release gate, that a fix ships, and that the format keeps its compatibility
guarantee. Every release's verification evidence is published — see
[verification reports](/project/verification-reports#published-reports).

**Beta** means functional and used, but the command surface, configuration, or
wire format may change between minor releases (documented per release).
**Experimental** means a capability exists in the library but is not wired into
the CLI, or has not been exercised enough to rely on.

## Component maturity

| Capability | Status | What that means |
|-----------|--------|-----------------|
| **Archive & manifest format** | **Stable** | Documented in the [format spec](/reference/format/). Manifest is `v2.0`; `v1.0` remains readable. Backward-readable across the versions the spec states — your archives don't become unreadable on upgrade. |
| **Core upload / verify / restore** | **Stable** | The canonical `cargoship upload`, `verify`, `download`, and `restore` paths are covered by integration + regression tests. |
| **Cost, budget & lifecycle** | **Stable** | Estimation, budgets/quotas, and lifecycle commands are established. |
| **Sharding & compression** | **Stable** | Multi-prefix sharding and content-aware zstd are core to the pipeline. |
| **Encryption (SSE / KMS-envelope manifest)** | **Stable** | See the [security model](/project/security). |
| **DVC integration** | **Beta** | The Go `dvc` commands and Python plugin work; command surface and metadata may change between minor releases (changes are documented per release). |
| **Magika AI file detection** | **Beta / opt-in** | Requires `pip install magika`; degrades gracefully when absent. |
| **Multi-region** | **Experimental** | A library capability in `pkg/multiregion`, **not** wired into `cargoship upload` today — see the [multi-region note](/guides/features/multi-region). Use `--region` + sharding for in-region parallelism. |
| **Distributed agents / controller / ghost-ship** | **Beta** | Functional; configuration and wire formats may change. See [Distributed / Enterprise](/enterprise/). |
| **Public Go API** | **Mixed** | Stability is per-package — some packages are stable, others beta. See [API stability](/project/versioning#stability-levels-for-the-go-library). |

## Guarantees

- **Archive compatibility** — archives written by a given release stay readable by
  later releases within the format-version guarantees in the spec. This is the
  guarantee that matters most for archival software, and it's the one we hold
  most firmly.
- **Semantic versioning** — `v0.PATCH` is always safe to upgrade;
  `v0.MINOR` *may* carry documented breaking changes while pre-1.0. See
  [versioning](/project/versioning).
- **Security** — the current minor release receives security fixes; see the
  [Security Policy](https://github.com/scttfrdmn/cargoship/blob/main/SECURITY.md).

## What is *not* promised

- **No support SLA.** Issues and vulnerability reports are handled best-effort by
  a small team — there is no contractual response or resolution time.
- **No recovery-time objective** for the distributed/agent components.
- **Pre-1.0 API churn.** Beta and experimental components may change shape between
  minor releases; pin a version if you need stability.

## Using it in production today

Reasonable if you: pin a specific release, rely on the **stable** components, keep
`verify` in your workflow (prove the round trip), and treat beta/experimental
features as opt-in. The archive format's portability — plain `tar.zst` objects
plus a documented JSON manifest — means your data isn't hostage to the tool even
if you stop using it.

Judge the claim on the evidence rather than the label:
[per-release verification reports](/project/verification-reports#published-reports)
record what round-tripped byte-identical on real S3 for each published version,
and the [integrity model](/project/integrity) explains how to reproduce it
yourself.
