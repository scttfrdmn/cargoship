# Security Policy

## Supported Versions

CargoShip is in the `0.x` series and ships fixes on the latest release line.
Security updates are provided for the current minor release; older lines are not
backported.

| Version | Supported |
| ------- | --------- |
| 0.13.x  | :white_check_mark: |
| < 0.13  | :x: |

Always run the [latest release](https://github.com/scttfrdmn/cargoship/releases/latest).

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Report privately via GitHub Security Advisories:
**[Report a vulnerability](https://github.com/scttfrdmn/cargoship/security/advisories/new)**

Include what you can: a description, reproduction steps, the affected
version/commit, and impact. As a small open-source project we triage reports on a
best-effort basis — there is no contractual response-time SLA. We aim to
acknowledge reports promptly, agree on a disclosure timeline with the reporter,
and credit reporters who wish to be named.

## How the project is secured

Security scanning runs in CI (`.github/workflows/security.yml`) on every push,
pull request, and weekly:

- **govulncheck** — Go vulnerability database scanning (pinned to a working
  version), across the root module and every nested `go.mod`.
- **gitleaks** — full-history secret scanning.
- **Trivy** — filesystem vulnerability + secret scanning and IaC misconfiguration
  scanning.
- **Semgrep** — static analysis (`--config=auto`), failing the build on findings.

All GitHub Actions are pinned to commit SHAs. On the repository itself,
**Dependabot** alerts and security updates, **secret scanning**, and **push
protection** are enabled. The pre-commit hook additionally runs `govulncheck`
with a zero-known-vulnerability policy, so dependency CVEs are caught before they
land.

## Security model

CargoShip's data-protection model has two distinct layers — see the
[Security model](https://cargoship.app/project/security) and the
[Encryption metadata spec](https://cargoship.app/reference/format/encryption) for
detail:

- **Archive chunks** are encrypted server-side by S3. With `--kms-key-id`,
  objects use SSE-KMS; otherwise standard server-side encryption applies.
- **The manifest** can additionally be **client-side envelope-encrypted**
  (`--encrypt-manifest`): a KMS-generated data key encrypts the manifest with
  AES-256-GCM, and the KMS-wrapped data key is stored alongside it.
- **GPG** key generation is available (`cargoship create keys`) for workflows that
  sign or encrypt with OpenPGP, but it is not the primary data-protection
  mechanism.

Operational controls: uploads use the standard AWS credential chain (CargoShip
stores no credentials of its own). As of v0.20.0 CargoShip has **no
network-facing authentication surface**: the distributed controller, `cargoship
webui`, the `cargoship-launch` agent, and the ghost-ship controller client were
all removed ([#340](https://github.com/scttfrdmn/cargoship/issues/340)), so
nothing accepts remote commands and nothing dials a non-AWS endpoint. Two
listeners remain, both **off by default and opt-in per invocation**: the pprof
endpoint (`--pprof`, defaulting to `localhost:6060` via `--pprof-addr`) and the
Prometheus metrics endpoint (`--prometheus-addr`, unset). Neither is
authenticated, so bind them to a loopback address only. Grant only the S3 (and optional KMS) permissions
the workflow needs — see [AWS setup](https://cargoship.app/start/aws-setup).

## User best practices

- Run the latest release; let Dependabot keep dependencies current.
- Scope IAM to the exact bucket/prefix you archive to; add KMS permissions only
  if you use `--kms-key-id`.
- Use `--kms-key-id` and `--encrypt-manifest` for sensitive datasets.
- Prefer the credential chain (profiles, roles, SSO) over long-lived access keys.
- For ghost ships on a NAS, scope the credentials that agent uses to the prefix it
  archives to; it needs no inbound network access.
