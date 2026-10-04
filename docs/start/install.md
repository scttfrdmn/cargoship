---
prev:
  text: Quick Start
  link: /start/quickstart
next:
  text: AWS setup & credentials
  link: /start/aws-setup
---

# Install

CargoShip ships as a single self-contained binary. Pick whichever method fits
your platform; all of them leave you with a `cargoship` command on your PATH.

## Homebrew (macOS / Linux)

The easiest option on macOS and Linux:

```bash
brew install scttfrdmn/tap/cargoship
```

## Go install

If you already have Go 1.26+:

```bash
go install github.com/scttfrdmn/cargoship/cmd/cargoship@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`.

## Scoop (Windows)

```bash
scoop bucket add scttfrdmn https://github.com/scttfrdmn/scoop-bucket
scoop install cargoship
```

## Pre-built binaries

Download the archive for your platform from the
[releases page](https://github.com/scttfrdmn/cargoship/releases/latest), unpack
it, and move the binary onto your PATH. Each archive contains a single
`cargoship` binary (plus `README.md`, `LICENSE`, and docs).

The commands below pin the current release, `v0.37.1`. For a different version,
swap the tag and the version in the filename (the release assets embed the
version, e.g. `cargoship_0.37.1_linux_x86_64.tar.gz`).

::: code-group

```bash [Linux x86_64]
curl -sSL https://github.com/scttfrdmn/cargoship/releases/download/v0.37.1/cargoship_0.37.1_linux_x86_64.tar.gz | tar -xz
sudo mv cargoship /usr/local/bin/cargoship
```

```bash [Linux ARM64]
curl -sSL https://github.com/scttfrdmn/cargoship/releases/download/v0.37.1/cargoship_0.37.1_linux_arm64.tar.gz | tar -xz
sudo mv cargoship /usr/local/bin/cargoship
```

```bash [macOS (Apple Silicon)]
curl -sSL https://github.com/scttfrdmn/cargoship/releases/download/v0.37.1/cargoship_0.37.1_darwin_arm64.tar.gz | tar -xz
sudo mv cargoship /usr/local/bin/cargoship
```

```bash [macOS (Intel)]
curl -sSL https://github.com/scttfrdmn/cargoship/releases/download/v0.37.1/cargoship_0.37.1_darwin_x86_64.tar.gz | tar -xz
sudo mv cargoship /usr/local/bin/cargoship
```

```bash [Windows x86_64]
curl -sSL -o cargoship.tar.gz https://github.com/scttfrdmn/cargoship/releases/download/v0.37.1/cargoship_0.37.1_windows_x86_64.tar.gz
tar -xzf cargoship.tar.gz
# move cargoship.exe onto your PATH
```

:::

### macOS: the downloaded binary is quarantined

::: warning Gatekeeper will kill a downloaded `cargoship` on first run
Released macOS binaries are **not yet notarized** by Apple
([#407](https://github.com/scttfrdmn/cargoship/issues/407)). macOS attaches a
`com.apple.quarantine` attribute to anything fetched from the internet, and for an
un-notarized binary Gatekeeper does not merely warn — it **SIGKILLs the process**, which
shows up as a bare `zsh: killed` or `Killed: 9` with no explanation.

This affects the tarballs above, not Homebrew (`brew install` binaries are not
quarantined) and not `go install` (you compiled it locally).

Confirm that is what happened, then clear it:

```bash
xattr -p com.apple.quarantine /usr/local/bin/cargoship   # prints a value if quarantined
xattr -d com.apple.quarantine /usr/local/bin/cargoship   # remove it
cargoship --version                                      # now runs
```

**Verify the download first** (next section). Stripping the quarantine attribute is
exactly what you would do to run a tampered binary too, so do it only after the cosign
and checksum verification below has passed — that check, not Gatekeeper, is what
establishes the binary came from this project's release workflow.

Prefer `brew install scttfrdmn/tap/cargoship` on macOS to avoid this entirely.
:::

### Verify the download

Every release ships a signed `checksums.txt` (SHA-256) whose signature is bound
to the GitHub Actions build via a keyless cosign Sigstore bundle
(`checksums.txt.sigstore.json`).

```bash
# Fetch the checksums file and verify your archive against it
curl -sSLO https://github.com/scttfrdmn/cargoship/releases/download/v0.37.1/checksums.txt
sha256sum -c checksums.txt --ignore-missing

# Optionally verify the checksums file itself with cosign (keyless)
curl -sSLO https://github.com/scttfrdmn/cargoship/releases/download/v0.37.1/checksums.txt.sigstore.json
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/scttfrdmn/cargoship' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

## Docker

Run CargoShip in a container without installing anything locally. Mount your data
and your AWS credentials:

```bash
docker run --rm \
  -v $(pwd):/data \
  -v ~/.aws:/root/.aws \
  scttfrdmn/cargoship:latest upload /data s3://my-bucket/archives/
```

## Verify

```bash
cargoship --version
```

## Next

- [AWS setup & credentials](/start/aws-setup) — the minimal IAM policy.
- [Interactive setup wizard](/guides/config/setup) — `cargoship setup`.
- [Your first upload](/start/first-upload).
