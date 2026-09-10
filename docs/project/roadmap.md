# Roadmap — removed & deferred capabilities

These are capabilities **not currently in CargoShip**. Some were *removed*
because they were non-functional or simulated (see the trust remediation
below); others are *deferred* or *experimental*. **Nothing here is a shipping
feature or a commitment** — it is a maintainer-facing candidate list for future
work, kept deliberately separate from what CargoShip actually does today.

For what CargoShip *does* today, see the
[maturity page](/project/maturity) (stable vs. beta vs. experimental) and the
[command reference](/reference/) and [format spec](/reference/format/).

## Why this list exists

A recent "trust remediation" pass audited the codebase for capabilities that
were *advertised or present as code* but were **simulated, fabricated, or dead**
— surfaces that reported fake metrics, ran no real work, or coordinated with
components that had already been deleted. Those were removed rather than left in
place to mislead. This page records what was taken out (and why) so that if any
of it is worth building *for real* later, the starting point and the pitfalls
are written down. It also captures a few genuinely deferred and experimental
items.

Status legend:

- **removed-as-theater** — was present but non-functional/simulated; deleted.
- **removed-legacy-command** — a SuitcaseCTL-heritage command removed during the
  fork's cleanup.
- **deferred** — a real capability intentionally left for later (may still exist
  dormant in-tree).
- **experimental** — code exists in-tree but is **not** wired into the CLI.

## Status at a glance

| Capability | Group | Status | Present in tree? |
|-----------|-------|--------|------------------|
| Distributed agent + controller (fleet) | Fleet / remote | removed-as-theater | No |
| Web UI dashboard | Fleet / remote | removed-as-theater | No |
| Autonomous archival / directory watching (ghost-ship) | Automation | deferred | Yes (dormant) |
| Interactive setup wizard (`wizard`) | Automation / legacy | removed-legacy-command | No |
| `travelagent` server | Automation / legacy | removed-legacy-command | No |
| `schema` command | Legacy tooling | removed-legacy-command | No |
| Real-time monitoring / metrics dashboards | Observability | removed-as-theater | No |
| Predictive prefetch / ML optimization | Optimization | removed-as-theater | No |
| Multi-region orchestration | Scale | experimental | Yes (unwired) |
| Advanced dashboard views | TUI | deferred | Partly (base rebuilt) |

---

## Fleet / remote operation

### Distributed agent + controller (fleet management)

**What it would do.** Run CargoShip as remote agents on many machines, each
reporting to a central controller that coordinates and monitors uploads across a
fleet — a way to drive and observe large multi-host transfers from one place.

**Status:** removed-as-theater.

**Why it's not here now.** The distributed-agent surface was unfinished and
carried real, exploitable defects — an unauthenticated remote-code-execution
path and an auth-bypass. It was removed as part of the v0.20.0 attack-surface
reduction (#340): the concrete deletion of `cmd/controller`, `cmd/launch-server`,
and `pkg/controller` landed in #343 (commit `97ca253`), and the parallel
`agent` execution surface that duplicated the ghost-ship implementation was
deleted afterward in #350 (commit `ec70d57`). Because it removed a genuine
network attack surface (JWT auth + websocket control channel), it is a net
security improvement that it is gone.

**Sketch of a real implementation.** A credible version would treat auth and
network exposure as the *first* design problem, not an afterthought: mutually
authenticated transport (mTLS) between agent and controller, short-lived scoped
credentials rather than a long-lived shared token, no inbound control channel to
the agent (agents poll/pull work instead of accepting pushed commands), and a
controller with a minimal, audited API surface. The upload work itself would
reuse today's real pipeline; the new code is purely the coordination/observation
plane, and it must not reintroduce a listener that can be reached before it is
authenticated.

### Web UI

**What it would do.** A browser dashboard for driving and watching uploads.

**Status:** removed-as-theater.

**Why it's not here now.** The `webui` surface was tied to the deleted
controller and had no independent function; it was removed with the rest of the
distributed surface in v0.20.0 (#340).

**Sketch of a real implementation.** A web UI only makes sense on top of a real,
authenticated control/observation API (see the fleet item above). It would be a
read-mostly view over that API — no direct filesystem or credential access from
the browser — and would inherit the same auth model. Until there is a real API
to render, there is nothing to build a UI against.

---

## Automation

### Autonomous archival / directory watching (ghost-ship)

**What it would do.** Watch configured directories on a host (e.g. a NAS), match
new/changed files against archival rules (include/exclude patterns, minimum age,
storage class), and upload matches to S3 on its own on a polling interval — no
inbound connections, nothing else required to be running.

**Status:** deferred — **not deleted.** `cmd/ghost-ship/main.go` and the
`pkg/launch` core (`GhostShip`, `WatchPath`, `FileWatcher`, `JobState`) remain
in-tree but dormant. The package doc already records that the central controller
it once talked to was removed in #340, and the duplicate agent implementation in
#347.

**Why it's not here now.** It is functional-but-dormant rather than a promoted,
release-gated command. It has no recovery-time objective and its config/wire
formats are not frozen. Its earlier framing was entangled with the (now removed)
distributed controller, so it needs a deliberate pass before being presented as a
first-class feature.

**Sketch of a real implementation.** Keep it strictly standalone (its current
no-inbound-connection design is the right one), pin down the watch-rule config
schema, decide on persistence of job state across restarts, and gate it behind
the same real-AWS round-trip verification the core upload path uses before
calling it supported. See the deferred discussion in the ghost-ship notes.

### Interactive setup wizard (`wizard` command)

**What it would do.** A console wizard for guided first-run creation — the
"easy button" for users who wanted a single interactive command instead of
learning the full flag surface. The removed command (aliases `wiz`,
`easybutton`) walked the user through building an inventory and then created the
archive.

**Status:** removed-legacy-command.

**Why it's not here now.** It was SuitcaseCTL-heritage code removed in the
Porter/rclone/suitcase legacy cleanup (#120, commit `3c26ea0`). It depended on
the old `porter`/`inventory` model and the `travelagent` status channel, none of
which survive in the current pipeline.

**Sketch of a real implementation.** A modern wizard would be a thin interactive
front-end that assembles today's `upload` flags/config and then calls the real
upload path — no separate data model. It is low-risk to add but is genuinely new
UX work, not a restoration of the old command.

### `travelagent` server

**What it would do.** The removed command (`travelagent CREDENTIAL_FILE`, marked
"NOT FOR PRODUCTION USE") started a small HTTP server that received status
updates from CargoShip runs. It read a YAML credentials file containing an admin
token and a set of static "transfers," and other commands (e.g. the wizard) sent
it `StatusUpdate` messages as an upload progressed.

**Status:** removed-legacy-command.

**Why it's not here now.** SuitcaseCTL-heritage code removed in #120 (commit
`3c26ea0`) along with `pkg/travelagent`. It was an unauthenticated-by-default,
explicitly non-production status sink tied to the old porter model — exactly the
kind of ad-hoc network surface later hardening work set out to remove.

**Sketch of a real implementation.** If progress reporting to an external
endpoint is wanted again, it belongs as an *outbound* reporter (CargoShip POSTs
status to a user-configured endpoint with a real credential), not as a bundled
server that listens. Any bundled receiver would need the auth model described in
the fleet item above.

---

## Legacy tooling

### `schema` command

**What it would do.** Emit the JSON schema for the manifest/inventory definition
from the CLI. The removed command was hidden and reflected a JSON schema from the
old `inventory.Inventory` struct, printing it to stdout.

**Status:** removed-legacy-command (removed in #120, commit `3c26ea0`).

**Why it's not here now.** It described the old inventory type, which no longer
exists in the current form. The manifest schema is now maintained as an embedded,
drift-tested artifact at `pkg/manifest/schema.json` rather than reflected at
runtime.

**Sketch of a real implementation.** A real `schema` command would be trivial and
low-risk: print (or write) the already-embedded `pkg/manifest/schema.json`,
optionally with a `--version` selector as the manifest format evolves. It would
re-expose an artifact that already exists and is already tested for drift, rather
than reflecting a live struct.

---

## Observability

### Real-time monitoring / metrics dashboards

**What it would do.** Live throughput/latency/error dashboards and
Prometheus-style metrics for an in-flight upload.

**Status:** removed-as-theater (#429).

**Why it's not here now.** The `pkg/monitoring` surface produced *simulated*
metrics — numbers not derived from the real upload — so it misrepresented what
was happening. It was deleted rather than left to report fiction.

**What real observability exists today.** CargoShip does expose opt-in, real
listeners: `--prometheus-addr` (serve real process/pipeline metrics) and
`--pprof` (Go profiling endpoint). These are off by default and are the honest
building blocks any future dashboard should read from.

**Sketch of a real implementation.** Emit real counters/gauges from the pipeline
stages (bytes in/out, chunks, retries, S3 status codes, per-shard concurrency)
through the existing metrics registry, expose them via `--prometheus-addr`, and
let an external dashboard (Grafana, etc.) render them. The tool should surface
real numbers and let established tooling do the visualization, rather than ship a
bespoke dashboard fed by invented data.

---

## Optimization

### Predictive prefetch / ML-driven optimization

**What it would do.** Adaptively tune upload parameters and prefetch data using
learned/ML models to improve throughput.

**Status:** removed-as-theater (#431).

**Why it's not here now.** The s3opt prefetch + ML layer was fabricated — it
advertised adaptive/ML behavior and improvement figures that did not come from
any real model or measurement. It was removed in #431.

**What real adaptivity exists today.** The upload path does real, measured
adaptation: automatic shard-count selection from the workload, content-aware
compression, and — as of the trust remediation — real congestion control
(a BBR-fed token-bucket pacer wired into the upload path, #424/#433) that reacts
to actual S3 `503 SlowDown` responses.

**Sketch of a real implementation.** Any future "smart" tuning must be grounded
in measured signals and validated against the real-AWS round-trip suite before it
is claimed. Prefetch specifically needs a real workload model and a demonstrated,
reproducible win over the current streaming pipeline; absent that evidence it
should stay out. The lesson from the removed code is that optimization claims
require published measurement, not asserted percentages.

---

## Scale

### Multi-region orchestration

**What it would do.** Region-aware load balancing, health checking, and failover
across multiple AWS regions.

**Status:** experimental — the one item on this page that is still present as
code.

**Why it's not here now (as a feature).** `pkg/multiregion` provides building
blocks (coordinator, advanced load balancer, geo routing, failover), but it is
**not wired into `cargoship upload`**: the CLI uploads to a single `--region`,
and the coordinator/health/failover paths are simulated and reached only by
internal tooling. A `check-no-dead-imports` guard deliberately keeps the package
off the CLI until it is genuinely wired, so it cannot be mistaken for a shipping
feature. It is kept as an honest roadmap library rather than deleted. See the
[multi-region note](/guides/features/multi-region).

**Sketch of a real implementation.** Replace the simulated health/failover paths
with real S3/endpoint health signals, define the semantics of a multi-region
upload (replication vs. partitioning, and how the manifest records region
placement), wire it into the upload path behind an explicit flag, and prove it on
real cross-region S3 with the verification suite before promoting it past
experimental. For in-region parallelism today, use `--region` plus sharding.

---

## TUI

### Advanced dashboard views

**What it would do.** Extend the terminal dashboard beyond its current
real-data views. The `dashboard` command was rebuilt (#435) as a focused,
read-only, real-data view with three tabs (Overview / Costs / Uploads) sourced
from the local cost ledger and resume state — no fabricated data. Candidate
additional views:

- a **completed-uploads inventory** built from S3 manifests (list what has
  actually been uploaded to a bucket, read from the manifests themselves); and
- an on-demand **bucket `analyze` view** (summarize/inspect a bucket's archives
  on demand).

**Status:** deferred (follow-ups to #435). The base dashboard is real and
shipping; these specific additional views are not built.

**Why it's not here now.** The rebuild deliberately scoped to data CargoShip can
read locally without fabrication. The S3-manifest inventory and bucket-analyze
views require reading and summarizing remote manifests, which is additional real
work left for later.

**Sketch of a real implementation.** Reuse the existing manifest-reading path
(the same one behind `info`) to list/summarize archives under a bucket/prefix,
render the result read-only in the TUI, and keep the honest empty/unavailable
states the rebuilt dashboard already uses when there is nothing to show.
