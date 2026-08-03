# Changelog

All notable changes to CargoShip will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Removed

- **BREAKING: the distributed controller subsystem is gone** — `cargoship
  controller`, `cargoship webui`, the `cargoship-launch` agent binary, the
  `cargoship-launch`/`controller`/`launch-server` commands, `pkg/controller`, and
  `pkg/launch/central_controller.go`. An external security audit found
  unauthenticated remote command execution in `cmd/launch-server` and an
  authentication bypass in `pkg/controller`; the subsystem was removed rather than
  hardened. (#340)
  - **It shipped.** The audit judged `pkg/controller` dormant because
    `cmd/controller` used `pkg/launch` instead — but the released CLI imported
    `pkg/controller` directly, from `NewControllerCmd()` and `webuiCmd` in
    `root.go`. `controller` and `webui` both appeared in `cargoship --help`, so
    the auth bypass was reachable from the release artifact. `cmd/launch-server`,
    which carried the RCE, was never built by anything and was in no release.
  - **Hardening it would have secured nothing.** It was unfinished scaffolding:
    eight empty handler bodies, four "Implementation would" comments, connection
    handlers that logged and returned. There was no defined behavior to secure.
  - Three defects the audit did not report died with it: the agent auth token was
    logged in cleartext at two sites (against this repo's own standing rule), the
    bearer-token comparison was not constant-time, and an empty configured token
    disabled auth for every REST route — a fail-open default, latent only because
    both entry points rejected a blank token.
  - **This is the fifth abandoned duplicate** in this repo, after #308, #311, #316
    and #325, and by far the most expensive. `pkg/launch` has been added to the
    `deadcode` filter; `pkg/pipeline`'s absence from that filter is exactly why
    #325 stayed invisible.
  - A **live defect** the audit did not mention, fixed here too: the `controller`
    execution context (`pkg/context`) is applied by the shipped CLI at every
    startup, and would have filtered the command set down to a command that no
    longer exists. A stale cached context now self-heals with a warning.
  - Two direct dependencies go with it: `github.com/golang-jwt/jwt/v5` and
    `github.com/gorilla/mux` were used by nothing else, so this is a real
    supply-chain reduction as well as an attack-surface one.
  - **Ghost ships are unaffected** — they were built to archive autonomously and
    never required a controller. `controller_url`, `auth_token` and `tls_config`
    in a ghost-ship config configured only that connection and now have no effect,
    as do the previously documented `CARGOSHIP_AGENT_*`,
    `CARGOSHIP_CONTROLLER_URL`, `CARGOSHIP_WATCH_PATHS` and
    `CARGOSHIP_DESTINATION` variables, which were read only by `cargoship-launch`.

## [0.19.0] - 2026-08-03

**Assurance depth.** An external review scored the project 8.9/10 and asked for
depth rather than breadth: prove the promises already made instead of adding new
ones. The work was meant to be verification — qualify an overstated maturity
label, pin each recent defect with a fixture, and check that archives written by
*older releases* still read. The last of those found four live data defects in
`restore`, which is why this is a release and not a test-only chore.

The mechanism is the point. Every other test in the repo writes and reads with
the same build, which proves self-consistency — not the backward readability
archival users depend on. Reading archives produced by real v0.14.0–v0.18.0
binaries broke that symmetry, and the asymmetry is what exposed the bugs: a
prefixed chunked archive that could not be restored at all, a copied archive
whose `verify --deep` certified bytes it never read, `restore` exiting 0 when
every file failed, and a truncated entry landing on disk as a short file with a
success exit code.

All four had passing tests beforehand. Each passed **vacuously**, for a different
reason — a fixture that set no prefix, an e2e that restored by basename, a
partially-tolerant API whose `err == nil` was read as success, and an `io.Copy`
that cannot distinguish a truncated tar entry from EOF. The lesson carried
forward from #308/#311/#325 holds: a test not watched failing against the broken
code is not known to test anything.

Two are severe for a backup tool. A green exit code on a total restore failure is
what deletes someone's only copy, since restore is overwhelmingly scripted; and
an integrity check that reports a copy as intact while reading the original is
the one thing such a check must never do.

No archive-format change. Archives written by every release from v0.14.0 onward
restore identically — now with a committed corpus that says so on every CI run.

### Added

- **Cross-version archive readability tests** — the format spec promises manifest
  v2.0 with v1.0 still readable, and the maturity page tells users their archives
  do not become unreadable on upgrade. Every other test in the repo writes and
  reads with the same build, which proves self-consistency, not the backward
  readability archival users actually depend on. `tests/e2e/crossversion_test.go`
  now serves committed archive fixtures back from the emulator and restores them
  with current code, asserting byte-identity. (#322)
  - Fixtures are captured from **actual released binaries** (v0.14.0 through
    v0.18.0, both the direct and chunked layouts) via
    `scripts/gen-archive-fixtures.sh`, not reconstructed by the current writer. A
    reconstruction shares today's bugs and would pass vacuously; generating from
    the real binary is the only reason this test can catch a format regression.
  - Each fixture records the file paths **verbatim from the old binary's own
    manifest** rather than deriving them. Releases through v0.18.0 record an
    absolute source path, so the recorded string looks odd committed to a repo —
    but `restore --file` matches against it, and deriving it would bake today's
    assumption about path shape into the test whose purpose is to notice when
    that shape changes.
  - v0.14.0/v0.15.0 chunked archives are marked writer-corrupt: those releases
    shipped #275 (unflushed final zstd frame), so the last file in every chunk is
    genuinely truncated in the stored object and no reader could return it. They
    are kept and tested under a different, stronger assertion — that current code
    refuses to hand back a file which looks complete and is not. This independent
    rediscovery of #275 from real binaries is what surfaced #337 and #336 below.

### Fixed

- **A copied, replicated, or renamed archive read from its original location.**
  Every restore and verify path resolved objects against the bucket recorded
  *inside* the manifest at upload time, never the bucket the manifest was just
  read out of — so `cargoship restore s3://archive-copy/…` loaded the manifest
  from `archive-copy` and then fetched every chunk from the original bucket,
  silently, having been handed a bucket it ignored. The 404 is the benign case:
  the dangerous one is a `verify --deep` that reports the copy as intact while
  never touching a byte of it, which is the one thing an integrity check must not
  do. `SelectiveExtractor` and `DeepVerifier` now take a `SetBucket` override, and
  every command wires the bucket it parsed from the user's URL. Omitting the
  override keeps the manifest's own bucket, so embedders are unaffected. `browse`
  also picked up the #336 exit-code fix, which it had missed. (#335)
- **Restoring any prefixed chunked archive by exact path failed outright** with
  `glacier pre-flight check failed: ... HeadObject ... 404`. Manifests record
  `S3Key` relative to `manifest.Prefix`, and the download path resolved it
  through `ResolveObjectKey` while the Glacier pre-flight `HeadObject` used the
  raw value — so the pre-flight 404'd on an object that was present and
  accessible. Both paths now resolve through one helper. Two independent reasons
  the existing tests missed this: the unit fixture set no `Prefix` at all, and
  the e2e quickstart restores by basename, which made the pre-flight check
  vacuous rather than passing. `ChunkKeysForPaths` also resolved targets with
  exact match while `BatchRestore` uses basename/suffix fallback (#228), so the
  two disagreed about which chunks a restore would even touch. (#334)
- **`restore` exited 0 when every requested file failed.** `BatchRestore` is
  deliberately partially tolerant — it counts a per-file failure and continues
  rather than aborting — so `err == nil` even on total failure, and the command
  printed `✅ Restore complete!` above `Files failed: 2`. This is the worst
  failure mode the tool has: restore is overwhelmingly scripted
  (`cargoship restore … && rm -rf ./local`), so a green exit code on an empty
  restore is what deletes someone's only copy. The exit code now agrees with the
  numbers printed beside it, and a partial restore is an error too. (#336)
- **A truncated archive restored as a silently-incomplete file with a success
  exit code.** The unverified extract path used `io.Copy`, for which a tar entry
  cut short mid-stream is indistinguishable from EOF: it returns nil having
  written short. It now uses `io.CopyN` bounded by the manifest's declared size,
  reports the shortfall, and removes the partial file — an absent file is a
  correct, loud outcome; a short one that looks complete is not. This is the path
  taken when no per-file checksum was recorded, which is precisely the case for
  archives written before #270 — and v0.14.0/v0.15.0, which carry the #275
  truncation bug, are exactly those. The archives most likely to be truncated
  took the one path that could not notice. (#337)

### Removed

- **The dead shard subsystem** — `ShardCoordinator`, `ShardPipeline`,
  `ShardProgressRenderer`, and `CalculateIntelligentShardCount` — 1,504 lines of
  non-test code with no production caller, plus the 3,353 lines of tests and two
  benchmark files that exercised only it. This was the fourth abandoned duplicate
  of the same shape as #308 and #311, and the reason v0.18.0's flags looked
  implemented but weren't: it was the only code in the repo that honored a
  compression level, so the flags pointed at an implementation nothing reached.
  With `--compression-level` now handled by the live archiver (#316), the
  subsystem held no remaining capability. (#325)
  - `ShardProgressRenderer` was assessed for salvage rather than assumed dead,
    since it is the per-shard TUI `--interactive` would need. It is not
    salvageable as-is: it takes a `*ShardCoordinator`, reads a per-shard stats
    breakdown the live `Progress` struct does not have, and calls
    `Pause()`/`Resume()`/`IsPaused()`, which exist nowhere outside the deleted
    code. Driving it from the live pipeline means building per-shard progress and
    pause/resume first — a feature, not a salvage. `--interactive` stays hidden
    until that exists.
  - `pkg/pipeline`'s coverage floor drops 57 → 56 in `.coverage-baseline`. The
    deleted code carried more test lines than implementation lines, so removing
    it lowers the package ratio without making anything less tested; the reason
    is recorded in the baseline file. The weekly dead-code audit over
    `pkg/pipeline` goes from 132 findings to 87 — the 45 this subsystem
    contributed. The remainder is a pre-existing backlog of unreachable
    accessors on live types, which is a separate question.
- **`scripts/integration-test.sh`** — 332 lines built entirely around LocalStack
  and port 4566, with no caller anywhere and last touched before the migration to
  the in-process Substrate emulator. It documented a test setup that no longer
  exists. This completes the LocalStack cleanup begun in v0.18.0, which removed
  the stale references from the live pre-commit gates. (#325)

### Changed

- **"Stable" on the maturity page is now defined rather than implied.** It read
  as "battle-tested in production for years," which is not what it meant and not
  what the record supports — v0.16.1 and v0.17.0 fixed a last-file truncation and
  a path traversal in paths labelled stable. It now states what it does mean: the
  interface and format are intended for continued use and are protected by
  release-gating verification against real S3, not that a commercial SLA or long
  production history exists. Beta and experimental are defined too. The published
  verification reports are also listed in one table with direct asset URLs and
  linked from the two places the byte-identity claim is actually made, so a reader
  wanting proof no longer has to open a release and scroll through its assets. A
  new check in `scripts/ci/check-doc-versions.sh` fails if a release lands without
  a row. Also corrected an overstatement found while doing this: `integrity.md`
  said the real-AWS lane runs on merge to main. It does not, and deliberately so —
  it needs real credentials and mutates a shared bucket, so it runs weekly, on
  release tags, and on dispatch. (#320)

### Testing & CI

- **A committed fixture per v0.16/v0.17 defect, each verified against the
  pre-fix code.** All nine were audited; seven were already pinned behaviorally
  and were confirmed to fail by actually reverting the fix, not assumed. The two
  that were not covered now are. Nil-slice manifests had lived only inside a fuzz
  differential with no named test — the new one asserts the **raw JSON** carries
  arrays, which matters because a round trip through `Manifest` hides the
  violation (`null` and `[]` both unmarshal to an empty-reading slice). (#321)
  - #311 gets a *structural* guard rather than a behavioral one, because it was a
    duplicate extraction loop: no test of the shared extractor could have caught
    it, since the vulnerable code never called the shared extractor. What made it
    invisible was that a new tar loop could appear without the containment
    question being re-asked. `internal/audit/tarloop_test.go` inventories every
    non-test `tar.NewReader` site with the reason each is safe and fails on an
    unlisted one — verified against the pre-fix tree, where it flags the exact
    file that carried the traversal. A second test rejects stale entries so the
    allowlist cannot overstate its coverage.
- **The `performance` and `benchmark` test suites did not compile**, and had not
  for an unknown length of time. No CI lane set either tag, and absence of a
  signal reads exactly like success. `performance` called helpers defined only
  behind `//go:build integration` — two tags that are never set together, so the
  caller could never see the callee; those helpers moved to a file tagged
  `integration || performance`. `benchmark` called `Options()` on an `interface{}`
  field that has no methods. Both are repaired, and
  `scripts/ci/check-build-tags.sh` now runs `go vet` for every custom tag as a CI
  job. The tag list is **derived** from `//go:build` lines rather than hardcoded,
  so a new tag is covered the moment it appears — a hardcoded list would rot the
  same way these files did. It immediately found a third unbuilt tag the issue had
  not mentioned (`benchmarks`, plural, distinct from `benchmark`). These suites
  need real credentials to *run*, so the check only compiles them; compiling is
  the part that was regressing. This is the build-tag analogue of the
  #308/#311/#325 duplicate-drift pattern: code nothing compiles stops tracking
  reality. (#329)

## [0.18.0] - 2026-08-02

**CLI contract honesty.** An external review flagged that `cargoship upload`
advertised behavioral controls that never reached the pipeline. Verifying it
found the scope wider than reported — five flags across two commands, plus a
config-file path that rejected its own default.

The stakes are what make this a release rather than a cleanup: a rejected flag
is an inconvenience, but an accepted flag that silently does nothing means a
user who ran `--compression-level 19` for cold archival believed they got it and
did not. The published guides taught these flags, so following the
documentation produced silent non-compliance.

The root cause was not forgetfulness. The flags were wired to a **fourth
abandoned duplicate implementation** — a complete second sharded-upload
subsystem in `pkg/pipeline` with no production caller, which was the only code
in the repo that honored a compression level. Same mechanism as #308 and #311:
a second copy stops being called, stops being reviewed, and silently misses
every fix the live copy receives. The dead-code audit that would have caught it
did not cover `pkg/pipeline`; it does now. The subsystem's own removal is
tracked in #325.

Every test added here was verified to fail against the pre-fix code, by
stashing the fix and watching it go red. Asserting coverage without that step
is how the previous duplicates stayed invisible: the old tests checked that
`--shard-strategy` *defaulted to* `hash`, which passed whether or not the flag
did anything.

No archive-format change; existing archives restore identically.

### Fixed

- **`upload` and `sync` accepted five flags that had no effect.** The CLI parsed
  them, printed some of them back, and then dropped them before the pipeline ran.
  A user who passed `--compression-level 19` for a cold archive got the automatic
  level instead, with nothing to indicate otherwise — following the published
  guides produced silent non-compliance. All five now do what they say. (#316)
  - `--compression-level` is an **override**: an explicitly passed value pins
    every chunk to that level and turns off content-aware per-chunk selection
    (#105/#30). Omitting the flag keeps the automatic behavior — the flag's
    default is *not* forwarded, because doing so would have pinned every existing
    user's uploads to level 3. Levels outside the pre-built encoder tiers (1/3/6/9)
    now get their own encoder rather than silently falling back to 3. Note that
    Go's zstd exposes four internal levels, so 1–22 collapses into four bands;
    the effective setting is printed at upload start.
  - `--shard-strategy` now implements all of its advertised values. `hash` hashes
    the chunk's file identities, `size` picks the least-loaded shard by bytes,
    `type` groups by predominant content type, and `directory` groups by common
    path prefix. The default is now `round-robin`, which is what the archiver has
    always actually done — the flag previously *defaulted* to `hash` while
    behaving as round-robin. Existing archives are unaffected: each chunk's shard
    is recorded in the manifest, so restore never recomputes assignment.
  - `--direct-upload-threshold-mb` and `--direct-upload-workers` are now read.
  - `--interactive` is hidden. Its per-shard TUI exists only in an unreachable
    subsystem, so it stays defined (scripts passing it won't break) but is no
    longer advertised. See #325.
- **A single shard decision.** The S3 key was built from `job.ID % shardCount`
  while `selectOutput` separately computed `job.Chunk.ID % shardCount`; the two
  could disagree. The shard is now chosen once and reused for the key,
  `job.ShardID`, and the output-channel pick. (#316)
- **`cargoship config --validate` rejected `round-robin`** — the actual default
  shard strategy — because `pkg/config` carried its own hardcoded strategy list.
  A cross-package test now pins the two lists together. (#316)
- **Traces were labeled `v0.6.2` regardless of the running binary.** The tracing
  service version now comes from `internal/version`, the single canonical source
  (#260), and defaults to it when unset rather than emitting an unlabeled
  attribute. (#318)
- **The ROADMAP body claimed the current release was v0.13.2** and described
  already-shipped work as upcoming. Version numbers are now stated once at the
  top of the file and omitted from the body, so the body cannot drift. (#319)
- **The shard-count default was documented four different ways** — "8 shards",
  "10 shards", "4–32 adaptive", and an undocumented fallback of 8. Every mention
  now reads: CargoShip automatically selects between 4 and 32 shards when
  `--shard-count` is 0 (the default), falling back to 8 if automatic selection
  fails. (#324)

### Changed

- The archive-layout spec no longer implies readers can recompute a chunk's
  shard. The strategy is an upload-time choice that is not recorded in the
  manifest, so readers must take each chunk's shard from the manifest — a reader
  assuming `chunk.ID % shard_count` addresses the wrong object for any archive
  written with a non-default strategy. (#316)
- The weekly dead-code audit now covers `pkg/pipeline`. The flags above were
  inert because the only code honoring `--compression-level` lived in an
  unreachable fourth duplicate implementation, and `pkg/pipeline` was outside the
  audit's filter — the same duplicate-drift mechanism as #308 and #311. (#316)

## [0.17.1] - 2026-08-02

A docs-infrastructure patch. No changes to the tool itself — same binaries,
same archive format, same behavior as v0.17.0.

Verifying the v0.17.0 release turned up two reasons cargoship.app was showing
the wrong version, both of which needed a tag to fix.

### Fixed

- **The docs site advertised v0.13.2.** The VitePress theme hardcoded the
  release number behind an "update this when cutting a release" comment.
  Nothing checked it, so it went stale by four releases;
  `scripts/ci/check-doc-versions.sh` only greps markdown, so a constant in a
  `.ts` file was outside the guarded set. It now reads
  `internal/version/version.txt` — the single canonical source (#260) the
  release process already bumps — so there is no second copy to forget. (#313)
- **The "these docs track `main`" banner rendered on the released tree too**,
  where it was misinformation rather than merely stale. It is now gated to the
  `/dev/` tree.
- **Release tags could not deploy the docs site at all.** The `github-pages`
  environment's deployment branch policy allow-listed only `main`, so every
  tag-triggered run was rejected before its first step ("Tag ... is not allowed
  to deploy to github-pages due to environment protection rules"). This
  silently failed on v0.16.0, v0.16.1 and v0.17.0. The policy now includes
  `v*`, and the reason is documented in `docs.yml` next to the `environment:`
  key, since nothing in the workflow file reveals a repo-settings dependency.
- **The root docs tree could publish one release behind.** A release pushes the
  version-bump commit to `main` and the tag seconds later, so the main-push
  run's checkout can predate the tag and resolve "latest" to the previous
  release — and, landing second in the `pages` concurrency group, overwrite the
  tag run's output. The resolve step now fetches tags first, making the result
  independent of which run wins the race.

## [0.17.0] - 2026-08-02

A security release. Asking "do we need more fuzz coverage?" led to a survey of
the parse/path surface, and that survey — not a new fuzz target — found **two
more copies of the #282 path traversal**. One was dead code, deleted (#308). The
other was **live**, on the default `cargoship download` path for every chunked
archive (#311), where it had also silently missed verify-on-restore and the
dataset-relative restore layout.

Both had the same cause: a duplicate implementation of an operation that stopped
being called, and so stopped receiving fixes the live copy got. So this release
also adds the two scanners that catch that class — `gosec` (which is what found
#311) and a weekly dead-code audit — and a flag-parity test that fails if
`download` and `restore` diverge again.

Removing `manifest.Extractor` drops exported API, which is why this is a minor
rather than a patch. Every pinned GitHub Action also moved off the deprecated
Node 20 runtime.

### Fixed
- **Path traversal in `cargoship download` on chunked archives (#311).**
  `download` carried its own tar-extraction loop that joined the untrusted tar
  `header.Name` onto the output directory with no containment check, so a
  crafted archive could write outside the destination. This was a third copy of
  the #282 traversal — the first was fixed in `SelectiveExtractor`, the second
  deleted with the dead `manifest.Extractor` (#308) — and the only live one.
  `download` now routes both storage layouts through `SelectiveExtractor`, and
  the duplicate loop is gone.
- **`download` did not verify checksums or preserve restore layout (#311).**
  Because it never went through the shared extractor, `download` also missed
  verify-on-restore (#283) — it would write corrupt bytes where `restore`
  refuses to — and the dataset-relative layout (#287), so the two commands
  disagreed on where files landed. They now behave identically and expose the
  same `--no-verify` and `--flatten` controls.

### Added
- **Restore preserves modification times (#311).** Both `restore` and
  `download` now stamp each restored file with the mtime recorded in the
  manifest, across direct and chunked storage. Previously only `download`'s
  now-removed loop did this, and only for chunked archives.
- **`gosec` runs in CI (#311).** A static-analysis pass over the Go source, in
  the Security workflow alongside govulncheck, gitleaks, Trivy, and Semgrep.
  Its first run is what found the traversal above.
- **Weekly dead-code audit.** A non-blocking scheduled `deadcode` report over
  the trust packages, so unreachable code — which is where the #308 and #311
  traversals survived unfixed — surfaces on its own.

### Removed
- **The superseded `manifest.Extractor` API (#308).** `pkg/manifest/extract.go`
  held the original selective-extraction implementation from #93. It had no
  non-test caller — every restore path (`restore`, `download`, `browse`,
  `shell`, the TUI) reaches `SelectiveExtractor` — and it carried an
  unfixed copy of the #282 path traversal: `getOutputPath` joined an untrusted
  tar `header.Name` onto the output directory with no containment check.
  Deleted rather than patched, so there is one extraction path and one
  sanitizer instead of two that can drift apart. Removed: `Extractor`,
  `NewExtractor`, `Extract`, `EstimateExtractCost`, `ListExtractableFiles`,
  `ExtractRequest`, `ExtractResult`. `S3Downloader` is unchanged and moved to
  `pkg/manifest/s3.go`.

### Changed
- **Every pinned GitHub Action is on the Node 24 runtime (#305, #307).** Node 20
  is deprecated on GitHub-hosted runners. All 15 pins were bumped, still
  SHA-pinned with the exact patch version recorded in a trailing comment. Two
  changes were more than a version bump: `codecov-action` v7 dropped the
  singular `file:` input for `files:` — left alone, coverage upload becomes a
  silent no-op — and `upload-pages-artifact`, though composite, wrapped a
  Node 20 action internally.

## [0.16.1] - 2026-08-02

A correctness patch. Extending fuzzing to the trust/integrity surface (#302)
turned up four ways `verify --deep` and `restore` could resolve a chunk's S3
object key to the *wrong* object, plus a manifest that violated its own
published schema. All were in code with passing table tests — the kind of edge
case invariant fuzzing finds and examples don't.

### Fixed
- **Object keys containing `://` were destroyed (#302).** `ResolveObjectKey`
  matched `://` anywhere in a key, so a legitimately-named object (S3 permits
  `:` in keys) collapsed to the bare prefix and could not be verified or
  restored. Only a scheme in true scheme position is now treated as a URL.
- **A trailing slash on the prefix, or a leading slash on the key, addressed a
  different object (#302).** These produced `prefix//key`, which in S3 is a
  distinct object from `prefix/key` — so verification looked for something that
  was never written. `s3://bucket/archives/` is a shape users type.
- **Key resolution was not idempotent (#302).** A resolved key whose first path
  segment happened to equal the bucket name had that segment stripped on a
  second resolve pass, silently dropping a path component.
- **A URL-shaped key with a leading slash bypassed URL detection entirely
  (#302),** reaching `GetObject` as a URL rather than a key.
- **Manifests with an empty `files`, `chunks`, or `shards` collection violated
  the published schema (#302).** Go marshals a nil slice as `null`, but the
  schema declares all three as required arrays, so an upload producing no chunks
  wrote a non-conforming manifest.

### Added
- **Fuzzing extended to the trust surface (#302).** Three invariant-based fuzz
  targets: the restore path sanitizer (an accepted path always lands inside the
  destination — a security boundary), the S3 key resolver (never a URL, always
  prefix-scoped, idempotent), and a differential check that every manifest
  CargoShip writes validates against its own published schema. All nine crashing
  inputs found are committed as permanent regression corpus. A new weekly
  deep-fuzz workflow runs every target ~40× longer than the per-PR lane.

## [0.16.0] - 2026-07-31

The **Trust & Verifiability** release (#270). CargoShip's core promise is that
what you restore is byte-identical to what you uploaded. This release makes that
claim continuously and independently checkable — a data-level verify, an open
and drift-checked format, an adversarial round-trip run against real S3 every
release, and a published dated verification report — rather than an assurance
you have to take on faith. Along the way this work caught and fixed a data-loss
bug and a path-traversal bug (see Fixed).

### Added
- **Data-level `verify --deep` (#271).** `verify` previously only checked that a
  manifest was internally coherent; it never re-read stored bytes. `--deep` now
  re-downloads each chunk and recomputes its SHA-256 against the manifest, and
  extracts and hashes every file, so checksum verification is a first-class
  PASS/FAIL. Every upload records SHA-256 at two levels: per chunk (the exact
  stored `.tar.zst` bytes) and per file. Per-file checksums are on by default;
  opt out with `upload --no-file-checksums`.
- **Verify-on-restore (#283).** Restore now verifies each file's SHA-256 as it
  writes and refuses to emit corrupt bytes — it fails the file instead of
  silently returning whatever S3 handed back. On by default; `restore
  --no-verify` opts out. Covers both direct and chunked storage paths.
- **Open, machine-checkable archive format (#274).** A JSON Schema for the
  manifest (`pkg/manifest/schema.json`, embedded), a struct↔schema drift guard,
  a dependency-free draft-07 validator that checks the *real* uploaded manifest,
  version-compat fixtures, and an independent-reader test that parses archives
  using only the standard library + zstd — proving the format is not locked to
  CargoShip's own code.
- **Whole-pipeline round-trip property test (#281).** A deliberately hostile
  corpus (empty/large files, incompressible and highly-compressible content,
  deep nesting, unicode / spaces / dotfile names) is uploaded through the real
  pipeline and restored through the real restore path, then compared
  byte-for-byte by SHA-256 — across both direct and chunked storage.
- **Real-AWS integration lane (#279, #290, #291).** The credential-gated
  integration suite now runs against real S3 in a dedicated `cargoship-dev`
  account via GitHub OIDC (no long-lived keys), on a weekly canary, on every
  release tag, and on demand. This is the standing, continuous evidence behind
  the integrity claim.
- **Published per-release verification report (#299).** Each release attaches a
  dated report (`vX.Y.Z-YYYY-MM-DD.md`) to its GitHub Release, stating how many
  files and bytes round-tripped byte-identical across both storage paths and
  which integration suites passed on real S3. It is generated from the exact
  release-gating test run, so the published numbers cannot drift from what
  actually passed. See the new [Integrity model](https://cargoship.app/project/integrity)
  and [Verification reports](https://cargoship.app/project/verification-reports)
  pages.
- **Dataset-relative restore layout + `--flatten` (#287).** Restores now
  reconstruct paths relative to the upload root by default; `--flatten` writes
  basenames into the destination for targeted restores.
- **Staging performance trend & anomaly analysis (#140).** Trend and anomaly
  detection over the upload-outcome corpus introduced in v0.15.0.

### Changed
- Restore writes a consistent, escape-safe layout across direct and chunked
  modes (#282).

### Fixed
- **Data-loss in compressed chunks (#275).** The archiver closed its pipe in the
  wrong order, so the final zstd frame was not flushed and the last file in
  every compressed chunk was silently truncated. The chunk-level checksum did
  not catch it; the new per-file verify did. Fixed the close order and added a
  regression test.
- **Path traversal on restore (#282).** A manifest entry with an absolute path
  or `..` components could escape the restore destination directory. Both write
  paths now sanitize entry paths and verify containment.
- **Chunked restore of full-URL S3 keys (#281, #273).** Chunked restore passed a
  stored full-URL key to `GetObject` verbatim and failed every file; manifest
  S3 keys are now kept portable (prefix-relative), with a defensive normalizer
  for older manifests.
- CI: `go.sum` tidied in the library-usage example modules for govulncheck (#285).

## [0.15.0] - 2026-07-23

### Added
- **Per-upload outcome history — the measurement corpus (#261).** An opt-in,
  durable, append-only record of each completed upload joins its inputs
  (dataset size, file count, file-type mix, chunk/shard counts, compression
  algorithm/level, storage class, region) to its outcomes (actual compression
  ratio, throughput, duration, error count, cost). Off by default; enable with
  `CARGOSHIP_UPLOAD_HISTORY` or `cost_control.upload_history_location`. Metadata
  only — no file content, names, or paths. Inspect it with `cargoship cost
  history` (table or `--json`). This is the substrate later optimization work
  learns from.
- **Persistent staging compression-ratio history (#262).** The staging
  compression predictor's online learner now survives restarts (opt-in via
  `CARGOSHIP_COMPRESSION_HISTORY`), so predictions converge across runs instead
  of starting cold each process. Decay-window pruning and the per-content-type
  cap are honored on load.

### Changed
- **Real exponential & moving-average cost forecasts (#263).** `cargoship cost
  forecast` previously advertised four models but only linear was implemented —
  the other two silently fell back to linear, which also made the "ensemble"
  a linear forecast in disguise. Exponential now uses Holt's double exponential
  smoothing (level + trend); moving average uses an exponentially-weighted
  7-day window; the ensemble genuinely blends the three distinct components.

### Fixed
- CI: regenerated the CLI reference for the #246 budget flags (`--global`,
  `--store`) and made the fuzz lane deterministic (execution-count budget
  instead of a wall-clock deadline that could spuriously time out) (#266).

## [0.14.0] - 2026-07-23

### Added
- **Durable, shareable budgets (#246).** Project spend/volume is now recorded on
  every upload and persisted, so `cargoship budget status` reflects real spend
  across restarts. Budgets can be stored in S3 (`budget --store s3://bucket/prefix`)
  with optimistic-concurrency (ETag/If-Match) writes so a laptop, CI, and
  teammates converge without lost updates. Adds an org/team-wide budget ceiling
  (`budget set --global`) enforced across all projects on top of per-project caps.
- Live AWS Price List API integration for S3 storage & request pricing, replacing the hardcoded-only fallback path (#235)
- First-class VitePress documentation site at cargoship.app, replacing the mkdocs tree (#216)
- Versioned docs: `latest` (root) + `dev` (/dev) trees with a version switcher (#231)
- Benchmark methodology page plus reproducible provenance recorded by the benchmark runner (#230)
- Tutorial: millions of small files & the S3 request-cost problem (#236)

### Fixed
- **S3 request pricing is now storage-class-aware everywhere (#237, #252).**
  Consolidated three drifting fallback price tables into one canonical source
  (`pkg/aws/pricingfallback`), fixing a second live instance of the 10× PUT-price
  bug and correcting archival PUT costs (Glacier/Deep Archive) in both the
  fallback and live Price List API paths.
- Corrected the S3 PUT request fallback price: $0.0005 → $0.005 per 1,000 requests (10× error) (#233)
- Restore & verify now work for direct-upload archives (the documented default path was broken) (#228, #229)
- Project budgets now persist across CLI invocations (#241, superseded by #246)
- `dvc` list_files no longer calls a broken `cargoship list` contract (#219)
- Recorded first-packet send time in the BBR prober (#232)

### Testing & CI
- Integration suite now runs in CI against the in-process Substrate emulator (credential-free); repaired rotted integration tests (#240)
- End-to-end coverage for `estimate`/`cost`/`list`/`download`/`sync`/`dvc`, which surfaced and fixed two bugs (#242)
- Comprehensive test-harness build-out (#238): shared fixture builder + golden files (#251), fuzz targets for the manifest & chunking formats (#249), a scheduled real-AWS integration lane (#248), and a monotonic per-package coverage ratchet enforced in CI (#250)
- Pilot mutation testing (gremlins) on the cost/manifest/chunking packages to surface weak assertions (#256)
- Corrected the large-file memory assertion in `TestIntegration_LargeFiles`: the reported multi-GB usage was an emulator measurement artifact, not a product regression — real-S3 memory is a flat ~12 MB for a 5 GB file (#239, #245)
- Fixed data races in the pipeline progress counter (#234) and the `TestWaitForRestore` mock client (#220)
- CI speedup: dropped the redundant Test job and run integration with `-short` (#243)

## [0.13.2] - 2026-07-07

### CI/CD & Security
- Repaired CI, which had failed on every job since February: integration tests imported the Substrate module root instead of the `github.com/scttfrdmn/substrate/emulator` package, breaking `go mod tidy` and leaving `go.sum` incomplete. Imports now target the emulator package; the local-path `replace` directive was dropped and `substrate` pinned to `v0.71.0`.
- Added `.github/workflows/security.yml`: govulncheck (pinned v1.3.0), gitleaks, Trivy (filesystem + config), and Semgrep, all SHA-pinned. Removed the duplicate govulncheck-only job from `test.yml`.
- SHA-pinned every action across all active workflows; routed `${{ inputs }}` through `env` in `performance.yml` to close a shell-injection vector.
- Added Dependabot cooldown and enabled Dependabot alerts/automated security fixes.
- Added `.gitleaks.toml` and `.semgrepignore` to allowlist documentation/test/example false positives.

### Fixed
- Populate manifest `S3Key`/`ShardID`/`ChunkID` in pipeline direct-upload mode (#205)
- Guard against a nil-pointer panic in `upload.go` when `pipe.Run` returns a nil result on the error path

### Security Hardening
- Bumped `go-git/v5` v5.16.5 → v5.19.1 (resolves 5 CVEs including an RCE, plus GO-2026-5496)
- Bumped `aws-sdk-go-v2/service/s3` v1.82.0 → v1.97.3 (resolves GO-2026-5764 in the eventstream protocol)
- Bumped transitive Python dependencies (cryptography, orjson, urllib3, dulwich) in `dvc-cargoship` via uv `constraint-dependencies`; dropped EOL Python 3.9
- WebSocket upgraders in `pkg/controller` and `pkg/launch` now enforce same-origin checks (CWE-352); dashboard derives `ws`/`wss` from the page scheme
- `Dockerfile.controller` runs as a non-root user; compose services set `no-new-privileges`

## [0.13.1] - 2026-03-18

### Test Infrastructure
- Migrated all remaining integration test packages from LocalStack to in-process Substrate emulator (`github.com/scttfrdmn/substrate`): `cmd/cargoship/cmd`, `pkg/pipeline`, `pkg/manifest`, `pkg/s3optimization`, `tests/integration/dvc`
- All integration tests now run without Docker or external AWS services by default; real-AWS path retained via `CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1`
- Removed LocalStack endpoint hard-codes (`localhost:4566`) from all test files
- Fixed nil-pointer dereferences in manifest integration tests (`aws.ToString` for optional response fields)

## [0.13.0] - 2026-02-19

### DVC Pipeline Auto-Discovery
- `BuildFileStageIndex` in `pkg/manifest`: parses `dvc.yaml` and returns a map of output path → stage name; directory outputs stored with trailing "/" for prefix matching
- `AnnotateFilesWithDVCStages` in `pkg/manifest`: walks `[]FileEntry` and sets `DVCMetadata.Stage` for every file that matches a stage output; graceful no-op when `dvc.yaml` is absent
- `cargoship upload --dvc-auto`: auto-discovers DVC stages from `dvc.yaml` and annotates each `FileEntry` with its stage name; re-uploads manifest to S3 so stage-aware commands work correctly
- `cargoship dvc stages <S3_URL>`: prints stage → file-count table from manifest DVCMetadata
- `cargoship dvc status <LOCAL_PATH> <S3_URL>`: compares local files against manifest by content hash; reports `unchanged`, `modified`, or `missing` per file
- `cargoship dvc export <S3_URL> [OUTPUT_DIR]`: downloads manifest and generates `.dvc` sidecar files via existing `GenerateDVCFiles`

## [0.12.0] - 2026-02-19

### Archive Filesystem Shell (Issue #203)
- `cargoship shell s3://bucket/prefix` — interactive filesystem shell for CargoShip archives
  - Navigate archive structure: `ls`, `cd`, `pwd` — no extraction required
  - Inspect files: `stat` (size, hash, chunk, DVC stage, git commit), `cat`, `head`
  - Search: `find <glob>` searches full path and basename
  - DVC-aware: `stage list` shows all pipeline stages; `stage <name>` lists stage files
  - On-demand extraction: `get <file> [dest]` restores a single file to local disk
  - Falls back to generic CargoShip REPL when called with no arguments
- New `pkg/archivefs` package: virtual filesystem tree built from manifest paths

### Documentation (Issue #204)
- CLI_REFERENCE.md: added Data Retrieval Commands section (restore, restore jobs, browse);
  expanded cargoship shell entry to cover archive filesystem mode; version bumped to v0.11.0
- USER_GUIDE.md: added Retrieving Archived Data section covering shell browsing, selective
  restore, TUI browse, Glacier async workflow, and restore job management
- mkdocs.yml: added Data Retrieval nav entry

## [0.11.0] - 2026-02-19

### Enhanced Data Retrieval (Issues #200–#202)
- S3 Glacier/Deep Archive pre-flight check and restore orchestration (Issue #200)
  - `GlacierRestorer` with `CheckAndRestore` — HeadObject checks, RestoreObject requests, Expedited/Standard/Bulk tiers
  - `WaitForRestore` — polls until all chunks accessible, with progress callback
  - `EstimateRetrievalCost` — approximate USD fees by storage class and tier
  - New `ChunkKeysForPaths`, `ChunkKeysForDVCStage`, `ChunkKeysForCommit`, `AllChunkKeys` helpers on `SelectiveExtractor`
- Quota-aware restore with `--max-restore-cost` flag (Issue #201)
- `cargoship restore` new flags: `--tier`, `--wait`, `--dry-run`, `--max-restore-cost`, `--restore-days`
- `cargoship browse` new flags: `--tier`, `--wait`, `--max-restore-cost`, `--restore-days`
- Restore job management for async Glacier restores (Issue #202)
  - `pkg/restore` — Job persistence layer (`~/.cargoship/restore-jobs/`, XDG_DATA_HOME aware)
  - `cargoship restore jobs list` — tabular view of all queued/in-progress/completed jobs
  - `cargoship restore jobs check [job-id]` — poll S3 and mark jobs ready when chunks accessible
  - `cargoship restore jobs download <job-id>` — download files once Glacier restore completes
  - `cargoship restore jobs clean [--older-than 24h]` — remove old completed/failed jobs
  - When Glacier restore is pending without `--wait`, job is auto-saved with instructions

## [0.10.0] - 2026-02-19

### DVC Integration (Issues #171–#192)
- Core DVC remote interface and Python package (`dvc-cargoship`) (Issue #181)
- DVC `.dvc` file generation for tracked datasets (Issue #180)
- MD5 content hashing with persistent hash cache (Issue #177)
- Git metadata extraction for manifest v2.0 (Issue #178)
- IncrementalScanner with MD5-based change detection (Issue #179)
- Performance helpers for DVC remote — batching and parallel restore (Issue #182)
- Budget integration for DVC operations (Issue #183)
- PyPI publication and integration tests for `dvc-cargoship` (Issue #184)
- DVC pipeline metadata extraction (Issue #185)
- DVC stage and git commit cost tracking (Issue #186)
- Federal compliance report generation for NSF/NIH grants (Issue #187)
- Hash-based manifest query API with DVC-aware lookups (Issue #188)
- Selective chunk extraction and batch restore with LRU cache (Issue #189)
- Interactive TUI file browser for selective restore (Issue #190)
- End-to-end DVC integration test suite (Issue #191)
- DVC performance benchmark suite (Issue #192)

### Security
- Remove `InsecureSkipVerify` from `TLSConfig` struct; restrict to `CARGOSHIP_TLS_INSECURE` env var (Issue #195)
- Add `--` separator in Magika CLI invocation to prevent flag injection (Issue #196)
- Mark `SMTPPassword`, `SlackWebhookURL`, `WebhookURL` with `json:"-"` to prevent credential leakage (Issue #197)
- Validate WebSocket `Origin` header against server `Host` (Issue #198)
- Validate symlink targets in tar extraction stay within output directory (Issue #198)
- Profile output directories use mode `0700` (Issue #199)
- Add `Timeout` to `http.Client` in geo locator (Issue #199)
- Filter job environment variables through security denylist in launch server (Issue #199)

## [0.9.1] - 2026-01-02

### Fixed
- Cost package test flakiness and inconsistent period filtering

## [0.9.0] - 2026-01-02

### Added
- **File Deduplication** — Cross-upload deduplication via content hashing, complete implementation (Issue #108)
  - Pipeline integration, dedup index, manifest export, CLI `--enable-dedup` flag
- **Shard Balance Analysis** — `cargoship balance` command, complete implementation (Issue #109)
  - Analysis, planning, chunk download/extraction, and full execution pipeline
- **Tier-Aware Chunking** — Groups files by storage tier for optimal cost savings (Issue #164)
  - `--tier-strategy tier-aware` flag; cost warning prompts with `--yes` bypass
- **Tier Cost Limits** — `--tier-max` flag prevents automatic selection of restrictive tiers (Issue #168)
- **Archive Tier TCO Modeling** — Total cost of ownership analysis for Glacier/Deep Archive (Issue #169)
- **Cost Benchmarking & Transparency** — ASCII cost comparison charts, chunking cost breakdown (Issue #165)
- **Direct Upload Mode** — Fast path bypassing archiving/compression for small datasets; 3.7× improvement (Issue #166)
  - `--direct-upload`, `--force-direct-upload`, `--direct-upload-threshold-mb` flags
- **S3 Cost Analyzer** — `cargoship analyze` command for existing bucket cost analysis (Issue #170)
  - S3-compatible storage provider support
- **Interactive TUI** — Pause/resume controls and live worker counts (Issue #112)

### Fixed
- Race conditions across multiregion, pipeline, staging, and adaptive controller packages
- Deadlock in `RealTimeLoadBalancer` and congestion control
- CloudWatch publisher timer race condition (Issue #15)
- AWS Open Data bucket configurations for benchmark suite (Issue #166)
- GitHub Actions: updated to Go 1.24, artifact actions v4

## [0.7.3] - 2025-12-17

### Added
- **AWS KMS Encryption** — SSE-KMS for data chunks + envelope encryption for manifests (Issue #163)
  - `--kms-key-id` and `--encrypt-manifest` flags; decrypt support in download/list/info
- **Magika AI File Type Detection** — Optional AI-powered compression type selection (Issue #30)
  - Lazy detection with extension pre-filter; ~1000 files/sec throughput
- **Distributed Tracing** — OpenTelemetry tracing across all pipeline stages (Issue #155)
  - stdout, Jaeger, and OTLP exporters; `--tracing`, `--tracing-exporter`, `--tracing-endpoint` flags
- **Prometheus Metrics** — `--prometheus-addr` flag; per-upload counters and throughput gauges (Issue #155)
- **Adaptive Shard Count** — Auto-tunes 4–32 shards from file count, data size, CPU/memory (Issue #106)
- **Resume Interrupted Uploads** — Local state persistence + auto-detection + `cargoship resume` command (Issue #119)
- **Automatic Storage Tier Selection** — `--auto-tier` selects STANDARD/GLACIER/DEEP_ARCHIVE by file age (Issue #32)
- **Zero-Copy I/O** — Linux splice: BufferedPipe pooling, upload buffer pooling (Issue #153)
- **HTTP/2 and TCP Network Tuning** — 3× throughput improvement on high-latency links (Issue #154)
- **Content-Aware Compression** — Multi-level encoder pools; code/text/binary/media profiles (Issue #105)
- **Adaptive Worker Scaling** — Worker count scales with workload size (Issue #58)
- **Performance Optimizations** — mmap LRU cache, lock-free manifest updates, parallel scanner batching, HTTP connection pooling (Issue #34)
- **CloudWatch Metrics** — CargoHold pipeline operation metrics (Issue #111)
- **`cargoship migrate`** — Archive conversion command (Issue #100)
- **CargoHold config file** — Config file support for CargoHold options (Issue #101)
- **GoReleaser** — Automated multi-platform release builds (Issue #160)
- **Incremental Sync** — Only upload changed files using content hashing (Issue #148)
- **Manifest Enhancements** — Fast in-memory indexing, validation, compression (Issues #88, #91, #92)
- **CargoHold** — Archive format with selective extraction, manifest query API (Issues #89, #90, #93)
- **`upload` command** — Primary upload command with CargoHold sharding support (Issue #95)
- **`download` command** — S3 URL support and auto-compression detection (Issue #96)
- **`verify` command** — Dataset integrity verification (Issue #99)

### Changed
- Staging package refactored: removed Simple* stubs, merged compression types (Issue #16)
- Default S3 upload workers increased from 4 to 8 (Issue #64)
- TLS certificate loading for controller (Issue #141)
- Session key generation for load balancer affinity (Issue #139)

### Fixed
- Critical context propagation bug in pipeline stages (Issue #155)
- BBR packet tracking timing tolerance (Issue #152)
- Monitoring interval configurability for flaky test (Issue #151)
- Goroutine leaks in staging package via `Shutdown()` methods (Issue #142)
- AWS SDK HTTP connection leaks (Issue #65)
- PrefixRouter deadlock on context cancellation
- Platform build issues: mmap, NUMA, SYS_GETCPU cross-compilation
- CloudWatch publisher race condition (Issue #15)

### Issues Closed
- #15, #16, #30, #32, #34, #58, #64, #65, #88–#96, #98–#101, #104–#106, #108–#109
- #111, #114, #119, #138–#142, #148, #151–#155, #157–#160, #163

## [0.6.2] - 2025-12-15

### Added
- Advanced S3 transporters integrated into Pipeline CLI: staging, adaptive, optimized, and basic modes
  - `--transporter` flag: `basic`, `staging`, `adaptive`, `optimized`, `none`
  - `--optimization`, `--congestion-control`, `--disable-staging` flags

### Fixed
- OptimizedTransporter Content-Length bug — switched to S3 manager uploader (Issue #162)

## [0.6.1] - 2025-12-14

### Added
- **Performance Profiling Infrastructure** — Phases 1–4: continuous profiling, regression detection, CI/CD integration (Issue #33)
- **S3 URL support for `info` command** (Issue #98)
- **Upload Failure Cleanup** — Automatic cleanup of partial S3 multipart uploads (Issue #158)
- **Resume Failed Uploads** — Initial resume infrastructure (Issue #157)
- **Manifest Enhancements** — Thread-safe builder, validation, compression, query API (Issues #88–#93)
- **`upload` command** — CargoHold sharding, `--shard-count`, `--shard-strategy` (Issue #95)
- **`download` command** — S3 URL + auto-compression detection (Issue #96)
- **`verify` command** — Dataset integrity verification (Issue #99)
- **`migrate` command** — Archive conversion (Issue #100)
- **CargoHold config** — Config file support (Issue #101)
- **CloudWatch metrics** — Pipeline operation metrics (Issue #111)
- Budget alert notification system (Issue #133)
- HTTP/2 and TCP network tuning (Issue #154)
- Zero-copy I/O optimizations (Issue #153)

### Fixed
- Goroutine leaks in staging package via `Shutdown()` methods (Issue #142)
- AWS SDK HTTP connection leaks (Issue #65)
- BBR packet tracking timing tolerance (Issue #152)
- Monitoring interval configurability (Issue #151)
- Platform build issues: mmap, NUMA cross-compilation

## [0.6.0] - 2025-12-09

### Added
- **Budget & Cost Management System** - Comprehensive enterprise cost tracking and forecasting
  - Dual budget controls: Cost budgets (USD) AND volume quotas (GB) enforced independently
  - Grant period management: 1-3 year budget periods with rollover support
  - Threshold alerts: Warning at 80%, critical at 100%
  - Budget enforcement: Operations blocked if limits would be exceeded
- **Project-Based Cost Tracking** - Each manifest upload ID becomes a project for granular analysis
  - Time period filtering (day/week/month/year/custom date ranges)
  - Multi-dimensional breakdowns (region, storage class, project)
  - Cost summaries with total costs, savings, file counts, data volumes
- **ML-Powered Forecasting** - Budget forecasting and burn rate analysis
  - 4 forecasting models: linear, exponential, moving_average, ensemble
  - Confidence intervals: 90%, 95%, 99% prediction bounds
  - Burn rate analysis: Historical trends, acceleration, volatility tracking
  - Budget exhaustion predictions with exact dates and probability estimates
- **Multi-Channel Alert Notifications** - Production-ready alert system
  - Email (SMTP): TLS 1.2+ encrypted, multiple recipients, Gmail/Office 365/AWS SES support
  - Slack webhooks: Rich message formatting with color-coded attachments
  - Custom webhooks: JSON payload with complete alert metadata
  - CloudWatch integration: Native AWS metrics and alarms
  - 6 alert types: cost_threshold, volume_threshold, cost_over_budget, volume_over_quota, budget_projection, volume_projection
  - 3 severity levels: info, warning, critical
- **Comprehensive CLI Commands** - 20+ subcommands across 3 command groups
  - Budget management: `budget status`, `budget set`, `budget list`, `budget remove`
  - Cost tracking: `cost summary`, `cost projects`, `cost project`, `cost forecast`, `cost burnrate`, `cost exhaustion`
  - Alert configuration: `alerts configure`, `alerts test`, `alerts enable/disable`
- **Comprehensive Documentation** - 2,970+ lines of user and developer docs
  - User guide: `docs/BUDGET_USER_GUIDE.md` (870 lines)
  - API reference: `docs/BUDGET_API.md` (1,100+ lines)
  - Alert setup: `docs/ALERTS_CONFIGURATION.md` (1,000+ lines)

### Changed
- Enhanced cost estimation system for S3 operations with regional pricing support

### Fixed
- Timezone issue in week period calculation (Issue #150)

### Removed
- **Rclone Integration** - CargoShip transitioned to S3-native architecture
  - `--cloud-destination` CLI flag removed (use direct S3 commands instead)
  - `cargoship rclone` command removed (use [rclone](https://rclone.org/) directly for non-S3 providers)
  - Cloud transporter plugin removed (S3-only focus)
  - Rclone configuration sections removed from config files
  - **Migration**: Use `cargoship create upload` with `--bucket` and `--prefix` flags for S3 uploads
  - **Note**: Non-S3 cloud providers are no longer supported; users needing GCS, Azure Blob, etc. should use rclone as a standalone tool

### Technical Details
- **Production Code**: ~140KB across 6 core files
- **Test Code**: ~102KB with 67 tests passing
- **Test Coverage**: pkg/aws/cost 72.5%
- **Quality**: Zero linting issues, zero security vulnerabilities

### Issues Closed
- #3: Define Grant and Project types
- #4: Implement cost estimator for S3 operations
- #5: Implement budget CLI commands
- #6: Implement budget alert system
- #39: Remove rclone integration code (Phase 2)
- #40: Update dependencies (Phase 3)
- #41: Remove configuration and documentation (Phase 4)
- #136: Implement alert notification system (duplicate of #6)
- #147: Budget & Cost Management System (Phase 1-6)
- #148: Incremental sync with manifest-based delta detection
- #149: Project-based cost tracking
- #150: Timezone issue in week period calculation

## [0.5.1] - 2025-11-08

### Added
- **Integration Testing Framework** - Comprehensive testing with real AWS S3 validation
  - 19 new integration tests validating end-to-end workflows
  - Real AWS S3 validation (not just LocalStack simulation)
  - Automatic bucket lifecycle management with proper cleanup
- **Performance Benchmark Suite** - 5 comprehensive benchmarks
  - Compression speed testing (gzip, zstd, bzip2 throughput)
  - S3 throughput validation (upload/download with 10MB-100MB files)
  - Memory efficiency testing (100MB-1GB files)
  - Deduplication overhead analysis
  - End-to-end workflow benchmarking (50 files)
- **Failure Scenario Tests** - 7 production reliability tests
  - S3 bucket not found error handling
  - Corrupted archive detection
  - Invalid permissions handling
  - Network timeout and retry logic
  - Partial upload cleanup validation
  - Concurrent upload race condition testing (10 concurrent)
  - Disk space monitoring and handling
- **Large-Scale Scenario Tests** - 5 comprehensive edge case tests
  - Large directory tree: 10,000 files in 11.38s (11,383 files/sec)
  - Deep nesting: 25 directory levels, 330-char paths
  - Long paths: 484-494 character path validation
  - Special characters: Unicode, emoji, punctuation (7 files)
  - Mixed file sizes: 184 files (1KB-50MB), 855 MB/s compression

### Performance Metrics
- **Compression**: zstd 527.30 MB/s (10.7x faster than gzip 49.14 MB/s)
- **S3 Upload**: 10MB → 32.20 MB/s, 100MB → 23.07 MB/s
- **S3 Download**: 10MB → 64.15 MB/s, 100MB → 89.68 MB/s
- **Memory Efficiency**: 4.21 MB peak for 100MB file (4.21% ratio)
- **Large-Scale**: 10,000 files in 9.26s (133x faster than 20min target)

### Impact Summary
- 19 new integration tests with real AWS S3
- All 7 failure scenarios validated for production readiness
- Comprehensive benchmarks proving 10x+ improvements
- Successfully handles 10,000 files, 25-level nesting, 494-char paths
- Zero linting issues, all tests passing with real AWS validation

## [0.4.1] - 2025-07-27

### Changed
- **Documentation Accuracy**: Corrected ML claims to accurately reflect proven algorithm implementations
- **Performance Transparency**: Updated benchmarks to emphasize BBR congestion control and CUBIC algorithms
- **Test Coverage**: Standardized 95% test coverage reporting across all files
- **Enterprise Messaging**: Aligned all documentation for consistent professional positioning
- **GitHub Pages**: Enhanced site highlighting production-proven network algorithms

### Added
- **Algorithm Transparency**: Clear descriptions of BBR (Google), CUBIC (Linux kernel), and signal processing methods
- **Technical Honesty**: Accurate representation of deterministic algorithms vs future ML capabilities
- **Realistic Roadmap**: Updated roadmap with honest ML implementation timeline (v0.6.0 - September 2026)

### Fixed
- **ML Overclaims**: Removed misleading references to "AI-driven" optimization where deterministic algorithms are used
- **Documentation Inconsistency**: Unified messaging across README, docs, and GitHub Pages
- **Link References**: Corrected documentation cross-references and outdated URLs

### Transparency Note
This release prioritizes honest representation of CargoShip's capabilities. The 4.6x performance improvements are achieved through Google's production-tested BBR algorithm and Linux kernel's CUBIC implementation, not machine learning. Future ML capabilities are planned for v0.6.0 with proper data collection and model training infrastructure.

## [0.4.0] - 2025-07-27

### Added
- **BBR Congestion Control**: Complete implementation of Google's BBR algorithm with bandwidth probing and state machine management
- **CUBIC TCP Algorithm**: Advanced congestion control with cubic function-based window growth and Hystart support
- **RTT Estimation System**: Sophisticated round-trip time analysis with multiple algorithms (Exponential, Kalman, Jacobson-Karels, Adaptive, Ensemble)
- **Loss Detection & Recovery**: Multi-method packet loss detection (timeout, duplicate ACK, SACK, ECN) with comprehensive recovery strategies
- **Bandwidth-Delay Product**: Dynamic BDP calculation with optimization algorithms and adaptive buffer sizing
- **Advanced Network Adaptation**: Real-time parameter optimization with ML integration and predictive algorithms
- **Comprehensive Test Suite**: 95+ test functions across all flow control components with 100% pass rate

### Changed
- **Upload Performance**: Improved from 3x to 4.6x faster uploads with advanced network optimization
- **Memory Efficiency**: Optimized data structures with bounded collections and automatic cleanup
- **Network Intelligence**: Enhanced network condition monitoring and adaptive parameter adjustment
- **Enterprise Features**: Strengthened enterprise-grade observability and monitoring capabilities

### Technical Details
- **Lines of Code**: 8,386+ lines of production-ready network optimization algorithms
- **Components**: 5 major algorithmic components (BBR, CUBIC, RTT, Loss Detection, BDP)
- **Files Created**: 10 new files (5 implementation + 5 comprehensive test files)
- **Static Analysis**: Zero violations with clean compilation across all components
- **Thread Safety**: Full concurrent access patterns with proper locking mechanisms

### Performance Improvements
- **BBR Algorithm**: Optimal bandwidth utilization with sophisticated probing
- **CUBIC Control**: Enhanced congestion window management with TCP-friendly fallback
- **RTT Analysis**: Multi-algorithm estimation with confidence scoring and accuracy tracking
- **Loss Recovery**: Fast, timeout, and congestion-based recovery with adaptive thresholds
- **BDP Optimization**: Dynamic buffer sizing with network condition awareness

## [0.3.2] - 2025-07-13

### Added
- **Multi-Region Stability**: Complete region selection strategy testing with advanced failover scenarios
- **Performance Benchmarking**: Comprehensive throughput, latency, and scalability testing framework
- **Real-World Simulation**: Network partition, data center outage, and load spike testing

### Changed
- **Region Selection**: Enhanced algorithms for round-robin, weighted, latency-based, geographic, and priority-based selection
- **Failover Logic**: Improved cross-region retry scenarios and timeout handling
- **Test Coverage**: Expanded multiregion package testing with realistic failure patterns

## [0.3.1] - 2025-06-28

### Added
- **JWT Authentication**: Complete JWT-based authentication with RSA and HMAC signing support
- **Role-Based Access Control**: Agent, admin, and readonly roles with comprehensive permission management
- **TUI/GUI Interface**: Full Terminal and Graphical User Interface supporting all CargoShip operations
- **Security Framework**: Integrated gosec vulnerability scanning and security best practices
- **LocalStack Integration**: Complete AWS testing framework with LocalStack S3 simulation

### Fixed
- **Resource Management**: Resolved goroutine leaks and improved resource cleanup
- **Memory Usage**: Optimized memory allocation patterns and reduced resource consumption

### Security
- **Vulnerability Scanning**: Integrated gosec for continuous security analysis
- **Access Control**: Implemented comprehensive role-based permission system
- **Secure Authentication**: JWT tokens with configurable signing algorithms

## [0.3.0] - 2024-07-11

### Added
- Multi-region pipeline distribution with intelligent failover
- Advanced failover optimization with circuit breakers and predictive monitoring
- Comprehensive performance benchmarking command
- Production-grade security scanning pipeline
- Complete code signing infrastructure with GPG key management
- Extensive user documentation and security guides

### Changed
- Improved multiregion package coverage from 80% to 85.9%
- Enhanced GPG package test coverage to 88.8%
- Optimized rclone package performance and reliability

### Fixed
- Multiregion coordinator initialization and background services
- Test failures in coordinator validation and health checks
- Memory leaks in connection pooling and health monitoring

## [0.2.0] - 2024-07-10

### Added
- Predictive chunk staging with content analysis
- Network adaptation for optimal transfer performance
- Enhanced staging system with memory-efficient buffering
- Comprehensive test suite with 85%+ coverage
- Advanced compression algorithms (zstd, lz4)
- Multi-threaded upload optimization

### Changed
- Improved staging package coverage from 71.1% to 81.8%
- Enhanced compression package reliability and performance
- Optimized memory usage in chunk staging operations

### Fixed
- Buffer overflow issues in staging operations
- Race conditions in concurrent upload scenarios
- Memory leaks in compression and staging pipelines

## [0.1.0] - 2024-07-09

### Added
- Core AWS S3 integration with native SDK support
- Intelligent cost optimization and storage class selection
- Basic multi-region support and coordination
- Comprehensive AWS configuration and credential management
- Cost estimation and budget tracking
- CloudWatch metrics integration
- Basic CLI interface with survey, estimate, and ship commands

### Changed
- Migrated from rclone to native AWS SDK for improved performance
- Implemented intelligent storage class selection algorithms
- Added comprehensive error handling and retry logic

### Fixed
- S3 multipart upload reliability issues
- AWS credential handling and region selection
- Cost calculation accuracy for different storage classes

---

## Version Schema

CargoShip follows [Semantic Versioning 2.0.0](https://semver.org/):

- **MAJOR** version when you make incompatible API changes
- **MINOR** version when you add functionality in a backwards compatible manner  
- **PATCH** version when you make backwards compatible bug fixes

### Pre-1.0.0 Development

During pre-1.0.0 development:
- **0.MINOR.PATCH** where MINOR may include breaking changes
- **0.x.0** releases may contain significant new features
- **0.x.y** releases contain bug fixes and small improvements

### Release Types

- **Alpha** (`0.x.0-alpha.1`): Early development, unstable API
- **Beta** (`0.x.0-beta.1`): Feature complete, API stabilizing
- **Release Candidate** (`0.x.0-rc.1`): Production ready candidate
- **Stable** (`0.x.0`): Production ready release