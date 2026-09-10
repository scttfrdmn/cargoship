# Capability Verification

Trust is CargoShip's first priority: the tool must do exactly what it says. This
page is the machine-checked ledger of that promise — every user-facing capability
mapped to the **evidence** that proves it, with an honest **status**. It is not
marketing; a claim with no backing evidence does not belong here as "Verified."

**Status legend**

- **Verified** — proven by a test, guard, or the per-release real-AWS
  verification report cited in the Evidence column. CI enforces that every cited
  artifact actually exists (`scripts/ci/check-capabilities.sh`).
- **Experimental** — present in the tree but not wired into the shipping CLI
  (a guard keeps it off).
- **Roadmap** — not built; see [the roadmap](/project/roadmap).
- **Aspirational** — a stated goal we have **not yet measured**. Per the trust
  priority, we do not advertise it as fact until evidence exists.

**Evidence tokens** (checked by CI): `` `test:Name` `` a Go test that must exist;
`` `make:target` `` a Makefile target; `` `script:path` `` a repo script;
`` `cmd:name` `` a CLI command; `` `report` `` the per-release real-AWS
[verification report](/project/verification-reports).

## Integrity & correctness (priority 1: trust)

| Capability | Status | Evidence |
|---|---|---|
| Byte-exact round-trip: upload → restore preserves bytes | Verified | `` `make:torture` ``, `` `report` ``, `` `test:TestArchiverStage_Process_NoTruncation` `` |
| Single-file random access via the 2.1 frame index (one ranged GET) | Verified | `` `test:TestFrameIndexRandomAccess` ``, `` `test:TestSelectiveExtractorFrameRestore` ``, `` `make:torture` `` |
| Data-level integrity check (`verify --deep`: chunk + file + frame) | Verified | `` `cmd:verify` ``, `` `test:TestDeepVerifyFrameIndex` `` |
| Per-frame content integrity — a tampered ranged fetch is rejected before decode | Verified | `` `test:TestFrameChecksumCatchesTamperedRange` ``, `` `test:TestArchiverStage_Process_Frames` `` |
| Open, portable format (2.1) readable with stdlib + zstd only | Verified | `` `test:TestIndependentReader` ``, `` `test:TestSchemaMatchesStructs` ``, `` `test:TestVersionFixturesParseAndValidate` `` |
| True compressed size recorded in the manifest | Verified | `` `test:TestHashingReadCloser_HashesStreamedBytes` ``, `` `test:TestJob_ArchiveCompressedSize` `` |
| Content-Encoding honored for caller-pre-encoded objects | Verified | `` `test:TestTransporterContentEncoding` `` |
| KMS envelope encryption of the manifest | Verified | `` `test:TestEncryptDecryptLargeManifest` `` |
| No advertised feature is theater; roadmap/simulated code stays off the CLI | Verified | `` `script:scripts/ci/check-no-theater.sh` ``, `` `script:scripts/ci/check-no-dead-imports.sh` `` |

Releases are signed (cosign keyless) with SBOMs and a published per-release
real-AWS verification report — see [Security](/project/security) and
[verification reports](/project/verification-reports).

## Throughput & scale (priority 2: performance)

| Capability | Status | Evidence |
|---|---|---|
| Multi-prefix sharding (parallel prefixes) | Verified | `` `make:torture` ``, `` `test:TestAdaptiveShardCalculator_CalculateOptimalShardCount_Integration` `` |
| Content-aware compression + Magika detection | Verified | `` `test:TestArchiverStage_AnalyzeChunkContentTypesWithMagika` `` |
| Real congestion control (BBR/CUBIC) moves bytes intact | Verified | `` `make:torture` `` |
| Resumable uploads (skip already-uploaded chunks) | Verified | `` `cmd:resume` ``, `` `test:TestNewResumePipelineConfig_EnablesResume` ``, `` `make:torture` `` |
| Internal throughput benchmarks | Verified | `` `cmd:benchmark` `` |
| **Fastest mover in class** — faster than `aws s3 cp`, `s5cmd`, `rclone` for the same data | **Aspirational** | **Not yet measured.** No head-to-head benchmark exists. Tracked as a v0.25.0 workstream (a comparative harness); until it lands, CargoShip does **not** claim to be the fastest. |

## Cost efficiency (priority 3, tied to performance)

| Capability | Status | Evidence |
|---|---|---|
| Cost estimation before upload | Verified | `` `cmd:estimate` ``, `` `test:TestAnalyzeBurnRate_IncreasingPattern` `` |
| Budgets & volume quotas | Verified | `` `cmd:budget` ``, `` `test:TestAlertCooldownPeriod` `` |
| Packing many files into archives → far fewer S3 requests | Verified | `` `make:torture` `` (the pipeline packs; round-trip proves it) |
| **Most cost-efficient mover in class** — lowest total S3 cost to move + store the same data | **Aspirational** | **Not yet measured.** Needs the comparative harness to also count requests / bytes transferred / bytes stored → dollars. Until then, not claimed. |

## Experimental / not shipping

| Capability | Status | Evidence |
|---|---|---|
| Multi-region orchestration (`pkg/multiregion`) | Experimental | Off the CLI by design; `` `script:scripts/ci/check-no-dead-imports.sh` `` enforces it stays off until genuinely wired. See [the roadmap](/project/roadmap). |
| Removed / deferred capabilities (agent+controller, web UI, autonomous archival, …) | Roadmap | [the roadmap](/project/roadmap) |

---

*Maintainers:* when you add a user-facing capability, add a row here with a real
evidence token, or CI (`check-capabilities.sh`) will not fail — but the promise
this page makes will be weaker. When you claim "fastest" or "most cost-efficient,"
it must move from **Aspirational** to **Verified** with a comparative-benchmark
citation, never before.
