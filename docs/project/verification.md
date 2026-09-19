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
| Writer isolation (#520): upload → restore byte-exact under `writers/<id>/` | Verified | `` `test:TestWriterIsolationRoundTrip` `` (emulator + real S3), `` `report` `` |
| Single-file random access via the 2.1 frame index (one ranged GET) | Verified | `` `test:TestFrameIndexRandomAccess` ``, `` `test:TestSelectiveExtractorFrameRestore` ``, `` `make:torture` `` |
| Data-level integrity check (`verify --deep`: chunk + file + frame) | Verified | `` `cmd:verify` ``, `` `test:TestDeepVerifyFrameIndex` `` |
| Per-frame content integrity — a tampered ranged fetch is rejected before decode | Verified | `` `test:TestFrameChecksumCatchesTamperedRange` ``, `` `test:TestArchiverStage_Process_Frames` `` |
| Open, portable format (2.2) readable with stdlib + zstd only | Verified | `` `test:TestIndependentReader` ``, `` `test:TestSchemaMatchesStructs` ``, `` `test:TestVersionFixturesParseAndValidate` `` |
| True compressed size recorded in the manifest | Verified | `` `test:TestHashingReadCloser_HashesStreamedBytes` ``, `` `test:TestJob_ArchiveCompressedSize` `` |
| Content-Encoding honored for caller-pre-encoded objects | Verified | `` `test:TestTransporterContentEncoding` `` |
| KMS envelope encryption of the manifest | Verified | `` `test:TestEncryptDecryptLargeManifest` `` |
| No advertised feature is theater; roadmap/simulated code stays off the CLI | Verified | `` `script:scripts/ci/check-no-theater.sh` ``, `` `script:scripts/ci/check-no-dead-imports.sh` `` |

Releases are signed (cosign keyless) with SBOMs and a published per-release
real-AWS verification report — see [Security](/project/security) and
[verification reports](/project/verification-reports).

## Dataset versioning (Beta)

Introduced in v0.31.0 (manifest format 2.2). Marked **Beta**: the model and its
failure-safety are tested, but it is newer than core upload/restore and the
garbage collector has documented first-cut limitations (keep-last-N only;
chunked, unencrypted-manifest datasets), so it has not yet had the same
production soak time.

| Capability | Status | Evidence |
|---|---|---|
| Dataset identity + version ordering (chain-derived, legacy-tolerant) | Beta | `` `test:TestDatasetIDOf` ``, `` `test:TestNextVersion` ``, `` `test:TestSetDatasetInfo` `` |
| Chain resolution: bounded, cycles fail loudly, missing ancestors error | Beta | `` `test:TestResolveEffective_IncrementalChain` ``, `` `test:TestResolveEffective_Cycle` ``, `` `test:TestResolveEffective_MissingAncestor` `` |
| Newest-wins merge + deletion tombstones | Beta | `` `test:TestResolveEffective_ThreeVersionsNewestWins` ``, `` `test:TestResolveEffective_DeleteTombstone` ``, `` `test:TestResolveEffective_DeleteThenReAdd` `` |
| Historical restore by version/date (`restore --version/--as-of`) | Beta | `` `cmd:restore` ``, `` `test:TestSelectDatasetVersion` ``, `` `test:TestResolveSelector` ``, `` `make:torture` `` |
| Dataset diff between two versions | Beta | `` `cmd:dataset` ``, `` `test:TestDiffFiles` `` |
| Chain-aware delete (won't strand a dependent version) | Verified | `` `cmd:delete` ``, `` `test:TestDependentUploads` `` |
| Reference-aware GC (`dataset prune`): mark-sweep by S3Key + verified compaction | Beta | `` `cmd:dataset` ``, `` `test:TestPlanKeepLast` ``, `` `make:torture` `` |

## Fleet mode (ghostship)

Shipped in v0.32.0 (see [ghost-ship](/enterprise/ghost-ship)). Each agent runs the same
verified incremental-sync + restore engine; the fleet-specific trust claims below are
gated by unit tests, the emulator E2E suite, and the per-release real-AWS round-trip.

| Capability | Status | Evidence |
|---|---|---|
| Writer isolation: own-prefix, delete-free **write-only** IAM (no Delete, no Decrypt) | Verified | `` `test:TestWriterIAMPolicy` ``, real-S3 writer-scoped round-trip (report marker `VERIFICATION_WRITER_ISO`) |
| **Fleet data paths byte-exact on real S3** — concurrent multi-writer isolation, real sync cycles (full → no-change → incremental) under a writer prefix with chain-resolved restore, repeated write-only full syncs, and heartbeat/data coexistence | Verified | `` `make:torture` `` `TestTorture/ghostship_*` — byte-identity by **relative path**, run on the emulator and manually against real S3 ([#654](https://github.com/scttfrdmn/cargoship/issues/654)) |
| Signed config-over-S3: verify + validate + keep-last-good + no silent scope-widening | Verified | `` `test:TestConfigSign_VerifyRoundTrip` ``, `` `test:TestPullSignedConfig_TamperedFails` ``, `` `test:TestGhostshipRun_ConfigPull_KeepLastGood` ``, `` `test:TestGhostshipRun_ConfigPull_RefusesSilentScopeWidening` `` |
| Heartbeat + control-side `fleet status`/`monitor` (stale-writer alert) | Verified | `` `cmd:fleet` ``, `` `test:TestListWriterStatuses` ``, `` `test:TestFleetStatus_ListsWriter` ``, `` `test:TestFleetMonitor_DetectsStale` `` |
| Immutability posture audit (`fleet lock-status`: versioning / Object Lock / abort-MPU) | Verified | `` `cmd:fleet` ``, `` `test:TestAuditBucketImmutability_NoBackstop` ``, `` `test:TestFleetLockStatus_Runs` `` |
| Disaster recovery (`restore --all`, independent of the writer's identity) | Verified | `` `cmd:restore` ``, `` `test:TestGhostshipRun_DisasterRecovery` ``, real-S3 |
| Live IAM minting / scuttle (`init --mint`, `scuttle`) | Verified | `` `test:TestMintWriter_HappyPath` ``, `` `test:TestScuttleWriterIAM_FullTeardown` ``, plus a manual real-IAM smoke (the live path is not CI-testable): mint → **12/12 permission assertions** → real cycle with the minted key → break-glass restore byte-identical → fail-closed re-mint → scuttle + idempotent re-scuttle. See [#655](https://github.com/scttfrdmn/cargoship/issues/655). |
| **Write-only agent guarantee proven against real IAM** — the minted identity can append to its own prefix and read its own config, and nothing else | Verified | Real-IAM smoke [#655](https://github.com/scttfrdmn/cargoship/issues/655): `PutObject` own prefix ✅; **denied** GetObject-own-data, DeleteObject, bucket-wide ListBucket, cross-writer PutObject, DeleteBucket, and PutObject on its own config prefix. |

## Throughput & scale (priority 2: performance)

| Capability | Status | Evidence |
|---|---|---|
| Multi-prefix sharding (parallel prefixes) | Verified | `` `make:torture` ``, `` `test:TestAdaptiveShardCalculator_CalculateOptimalShardCount_Integration` `` |
| Content-aware compression + Magika detection | Verified | `` `test:TestArchiverStage_AnalyzeChunkContentTypesWithMagika` `` |
| Real congestion control (BBR/CUBIC) moves bytes intact | Verified | `` `make:torture` `` |
| Resumable uploads (skip already-uploaded chunks) | Verified | `` `cmd:resume` ``, `` `test:TestNewResumePipelineConfig_EnablesResume` ``, `` `make:torture` `` |
| Internal throughput benchmarks | Verified | `` `cmd:benchmark` `` |
| **Fastest mover for many-small-file workloads** — faster than `aws s3 cp`, `s5cmd`, `rclone` | Verified | Measured 2026-09-11 (5000-file corpus, LA→`us-west-2`): CargoShip **9.7 MB/s** vs s5cmd 2.2 / rclone 0.1 / tar 3.5 — packing avoids per-object latency. On **few-large** files s5cmd is faster (56 vs 47), so this is **not** a blanket claim. See [benchmarks](/reference/benchmarks). `` `script:benchmarks/cargohold/benchmarks.go` ``, `` `script:pkg/s3metrics/s3metrics.go` `` |

## Cost efficiency (priority 3, tied to performance)

| Capability | Status | Evidence |
|---|---|---|
| Cost estimation before upload | Verified | `` `cmd:estimate` ``, `` `test:TestAnalyzeBurnRate_IncreasingPattern` `` |
| Budgets & volume quotas | Verified | `` `cmd:budget` ``, `` `test:TestAlertCooldownPeriod` `` |
| Packing many files into archives → far fewer S3 requests | Verified | `` `make:torture` `` (the pipeline packs; round-trip proves it) |
| **Lowest S3 cost for many-small-file workloads** — fewest requests + bytes → dollars | Verified | Measured 2026-09-11 (5000-file corpus): **7 requests → $0.00003** vs s5cmd 5000 → $0.025 and rclone 15001 → $0.029 (≈700× fewer requests; competitor counts from CloudWatch, priced by `pkg/s3cost`). On few-large workloads request counts are negligible for all, so cost is competitive, not ahead; storage is equal on incompressible data. See [benchmarks](/reference/benchmarks). `` `script:pkg/s3cost/cost.go` ``, `` `script:benchmarks/cargohold/cloudwatch.go` `` |

## Experimental / not shipping

| Capability | Status | Evidence |
|---|---|---|
| Multi-region orchestration (`pkg/multiregion`) | Experimental | Off the CLI by design; `` `script:scripts/ci/check-no-dead-imports.sh` `` enforces it stays off until genuinely wired. See [the roadmap](/project/roadmap). |
| Removed / deferred capabilities (central agent+controller, web UI, the legacy per-file archival daemon, …) | Roadmap | [the roadmap](/project/roadmap) |

---

*Maintainers:* when you add a user-facing capability, add a row here with a real
evidence token, or CI (`check-capabilities.sh`) will not fail — but the promise
this page makes will be weaker. When you claim "fastest" or "most cost-efficient,"
it must move from **Aspirational** to **Verified** with a comparative-benchmark
citation, never before.
