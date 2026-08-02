# CargoShip Roadmap

**Last Updated**: July 2026
**Current Version**: v0.17.1 (Released 2026-08-02)

This is a forward-looking roadmap: it answers "what's next," not "what happened."
For released-version history, see [CHANGELOG.md](CHANGELOG.md).

Dates and scope are targets, not commitments, and may shift with community
feedback and development progress.

---

## Current release — v0.13.2

CI repair and security hardening: multi-scanner security workflow, dependency
CVE fixes, and the new documentation site. See the [changelog](CHANGELOG.md) for
details.

## Next — v0.14.0 (Planning)

Theme: **enterprise budget foundations**. Candidate items (planned/proposed, not
yet committed to a date):

- Multi-grant portfolio management — track multiple concurrent grants/budgets.
- Hierarchical budget controls — department → lab → project → user inheritance.
- Advanced burn-rate analytics — predictive spending with seasonality.
- Budget approval workflows — multi-stage approval for large transfers.

Exact scope will be finalized during planning; some items may land across
several minor releases.

## Near-term (following minor releases)

High-level themes, in rough priority order:

- **Institutional workflows** — cost-center integration and chargeback;
  research/federal compliance reporting (OSTP 2025, NSF/NIH).
- **Transfer integrations** — Globus endpoint integration for institutional
  data movement.
- **Maturity work** — graduate beta components (DVC integration, distributed
  agents) toward stable; expand test coverage and docs.

## Longer-term — path to v1.0.0

v1.0.0 is about stability and maturity, not a feature dump:

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
