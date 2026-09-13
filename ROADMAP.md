# CargoShip Roadmap

**Last Updated**: August 2026
**Current Version**: v0.28.0 (Released 2026-09-13)

This is a forward-looking roadmap: it answers "what's next," not "what happened."
For released-version history, see [CHANGELOG.md](CHANGELOG.md).

Dates and scope are targets, not commitments, and may shift with community
feedback and development progress.

## Guiding goal

CargoShip aims to be **the fastest and most efficient — in both S3 operations
and dollars — way to move data into and out of S3**, without compromising the
first priority: trust (byte-exact, verifiable round-trips). Every performance and
cost feature is measured against `aws s3 cp` / `s5cmd` / `rclone` and only
claimed where the evidence supports it — see the
[capability matrix](https://cargoship.app/project/verification). Ordering:
(1) trust, (2) speed, (3) cost efficiency.

---

## Next (in planning)

Version numbers are deliberately omitted below (#319): this file describes
*themes and ordering*, and pinning a number to each one meant the body drifted
out of date on every release. The version currently released is stated once, at
the top of this file; what shipped in it is in the [changelog](CHANGELOG.md).

Theme: **enterprise budget depth**, building on the durable/shareable budgets
already shipped. Candidate items (proposed, not committed to a date):

- Multi-grant portfolio management — track multiple concurrent grants/budgets.
- Hierarchical budget controls — department → lab → project → user inheritance.
- Advanced burn-rate analytics — predictive spending with seasonality.
- Budget approval workflows — multi-stage approval for large transfers.

Exact scope is finalized during planning; some items may land across several
minor releases.

## Near-term (following minor releases)

High-level themes, in rough priority order:

- **Institutional workflows** — cost-center integration and chargeback;
  research/federal compliance reporting (OSTP 2025, NSF/NIH).
- **Transfer integrations** — Globus endpoint integration for institutional
  data movement.
- **Maturity work** — graduate beta components (DVC integration, distributed
  agents) toward stable; expand test coverage and docs.

## Longer-term — path to the first stable release

The first stable (1.x) release is about stability and maturity, not a feature
dump:

- **API stabilization** — settle the public Go API package-by-package (today
  it's mixed per `docs/project/maturity.md`) and commit to semver guarantees.
- **Graduate beta → stable** — DVC commands/plugin, distributed agents, and
  Magika detection move from beta to stable once wire formats and command
  surfaces settle.
- **Experimental → supported** — decide the fate of multi-region (library-only
  today, not wired into `cargoship upload`).
- **Uphold the archive-compatibility guarantee** — archives stay readable across
  releases within the documented format versions.

Aspirational, and gated on real-world validation rather than a fixed date.

## Exploratory — NOT committed

Ideas under consideration only. These are not promises and may never ship:

- Multi-cloud support (Azure, GCP) alongside AWS.
- True ML / predictive optimization — learned parameter tuning and network
  modeling (current optimization is deterministic).
- Serverless / auto-scaling agents and container-orchestrated deployment.
- Next-gen protocols (HTTP/3) and a REST API surface.

---

**Note**: This roadmap is subject to change. All dates are targets and may be
adjusted based on development progress, quality requirements, and priorities.
