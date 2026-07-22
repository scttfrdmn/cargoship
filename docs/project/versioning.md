# API stability & versioning

CargoShip follows [Semantic Versioning 2.0.0](https://semver.org/):
`MAJOR.MINOR.PATCH`. This page explains what that means for the CLI and for
projects that import CargoShip as a Go library.

CargoShip follows [Semantic Versioning 2.0.0](https://semver.org/), but the exact
compatibility rules differ before and after `v1.0` — as SemVer itself specifies
for the `0.x` series. **CargoShip is currently `v0.x`, so the pre-1.0 policy
below is the one in force today.**

## Current policy (pre-1.0)

- **`0.MINOR.0`** (`v0.13 → v0.14`) — *may* contain **documented breaking
  changes** alongside new features. This is standard for the `0.x` series. Any
  break is called out in the release notes with a migration path.
- **`0.MINOR.PATCH`** (`v0.13.1 → v0.13.2`) — bug fixes, security patches,
  performance, and docs. Always backward compatible; safe to upgrade immediately.

Even so, the packages marked **Stable** below are not broken casually — they get
the [deprecation policy](#deprecation-policy) treatment rather than abrupt
changes. Pin a version if you need certainty across minor bumps.

## Policy after 1.0

Once CargoShip reaches `v1.0`, standard SemVer applies:

- **MAJOR** (`v1 → v2`) — breaking changes; migration guide provided.
- **MINOR** (`v1.1 → v1.2`) — backward-compatible additions only.
- **PATCH** (`v1.1.1 → v1.1.2`) — backward-compatible fixes.

## Stability levels for the Go library

Public APIs are classified into three levels:

- **Stable** — `pkg/aws/s3` (the `Transporter`, `Archive`, `UploadResult` types)
  and `pkg/aws/config` (`S3Config`, `StorageClass` constants). Backward compatible
  within the major version; new methods and fields may be added, but existing ones
  are not removed or changed.
- **Beta** — cost-control config and `pkg/multiregion` coordination. Usable but
  still evolving; changes come with a deprecation notice (at least 2 minor
  versions).
- **Experimental** — `pkg/staging`, `pkg/s3optimization` (predictive prefetching).
  May change in any version without notice; marked with
  `// Experimental` comments.

## Deprecation policy

Breaking a stable API is a multi-step process: mark it `// Deprecated` with a
replacement, keep both available for at least 2 minor versions (or 6 months) with
a documented migration path, then remove it only in a major release.

## Dependency pinning

For production, pin an exact version and test before upgrading rather than tracking
`latest`:

```
require github.com/scttfrdmn/cargoship v0.13.2
```

Patch upgrades are always safe; test minor upgrades in development first; plan for
major upgrades using the migration guide.

## See also

- [Manifest schema (v2.0)](/reference/format/manifest) — the on-disk format version.
- [Reading archives (Go library)](/reference/format/library-api).
- [CHANGELOG](https://github.com/scttfrdmn/cargoship/blob/main/CHANGELOG.md).
