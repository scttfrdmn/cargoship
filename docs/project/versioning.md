# API stability & versioning

CargoShip follows [Semantic Versioning 2.0.0](https://semver.org/):
`MAJOR.MINOR.PATCH`. This page explains what that means for the CLI and for
projects that import CargoShip as a Go library.

## Semantic versioning

- **MAJOR** (`v0.x → v1.0`) — breaking API changes: removing or renaming public
  functions, changing signatures or return types, removing struct fields, or
  changing documented behavior. Requires consumer code changes; a migration guide
  is provided.
- **MINOR** (`v0.4 → v0.5`) — backward-compatible additions: new features, new
  methods, new struct fields with zero-value defaults, new packages, and
  deprecations (marking, not removing).
- **PATCH** (`v0.4.4 → v0.4.5`) — bug fixes, security patches, performance
  improvements, and documentation. Safe to upgrade immediately.

## Pre-1.0 status

CargoShip is currently `v0.x`. In this phase, `v0.MINOR` bumps *may* include
documented breaking changes, while `v0.PATCH` is always backward compatible.
Despite the `v0.x` label, the **stable** library APIs are treated as
production-ready and are not broken casually.

## Stability levels for the Go library

Public APIs are classified into three levels:

- **Stable** — `pkg/aws/s3` (the `Transporter`, `Archive`, `UploadResult` types)
  and `pkg/aws/config` (`S3Config`, `StorageClass` constants). Backward compatible
  within the major version; new methods and fields may be added, but existing ones
  are not removed or changed.
- **Beta** — cost-control config and `pkg/multiregion` coordination. Production-
  ready but evolving; changes come with a deprecation notice (at least 2 minor
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
