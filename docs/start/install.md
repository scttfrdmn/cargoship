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

The commands below pin the current release, `v0.38.2`. For a different version,
swap the tag and the version in the filename (the release assets embed the
version, e.g. `cargoship_0.38.2_linux_x86_64.tar.gz`).

::: code-group

```bash [Linux x86_64]
curl -sSL https://github.com/scttfrdmn/cargoship/releases/download/v0.38.2/cargoship_0.38.2_linux_x86_64.tar.gz | tar -xz
sudo mv cargoship /usr/local/bin/cargoship
```

```bash [Linux ARM64]
curl -sSL https://github.com/scttfrdmn/cargoship/releases/download/v0.38.2/cargoship_0.38.2_linux_arm64.tar.gz | tar -xz
sudo mv cargoship /usr/local/bin/cargoship
```

```bash [macOS (Apple Silicon)]
curl -sSL https://github.com/scttfrdmn/cargoship/releases/download/v0.38.2/cargoship_0.38.2_darwin_arm64.tar.gz | tar -xz
sudo mv cargoship /usr/local/bin/cargoship
```

```bash [macOS (Intel)]
curl -sSL https://github.com/scttfrdmn/cargoship/releases/download/v0.38.2/cargoship_0.38.2_darwin_x86_64.tar.gz | tar -xz
sudo mv cargoship /usr/local/bin/cargoship
```

```bash [Windows x86_64]
curl -sSL -o cargoship.tar.gz https://github.com/scttfrdmn/cargoship/releases/download/v0.38.2/cargoship_0.38.2_windows_x86_64.tar.gz
tar -xzf cargoship.tar.gz
# move cargoship.exe onto your PATH
```

:::

### macOS: signing and Gatekeeper

The macOS binaries are signed with an Apple Developer ID and notarized by Apple, so
they run from a download with no extra steps.

**Earlier releases were not.** They carried only an ad-hoc linker signature, and macOS
attaches a `com.apple.quarantine` attribute to anything a browser downloads.
Gatekeeper's response to a quarantined, un-notarized binary is not a warning dialog —
it **SIGKILLs the process**, which surfaces as a bare `zsh: killed` or `Killed: 9` with
no explanation. If `cargoship` appears to do nothing at all on an older release, that is
what happened. Either upgrade, or clear the attribute yourself:

```bash
xattr -p com.apple.quarantine /usr/local/bin/cargoship   # prints a value if quarantined
xattr -d com.apple.quarantine /usr/local/bin/cargoship   # remove it
```

**Verify the download before stripping quarantine** (next section). Removing that
attribute is exactly what you would do to run a tampered binary too, so do it only after
the cosign and checksum verification has passed — that check, not Gatekeeper, is what
establishes the binary came from this project's release workflow.

Homebrew and `go install` avoid this on any version: neither path sets the quarantine
attribute.

You can confirm any binary for yourself, which is more reliable than reasoning from a
version number:

```bash
codesign -dvvv $(which cargoship) 2>&1 | grep Authority
# Authority=Developer ID Application: ...

spctl -a -vvv -t install $(which cargoship)
# source=Notarized Developer ID
```

Note that `codesign --verify --strict` is *not* the check to use: it returns success for
an ad-hoc signed binary too, so it cannot tell signed from unsigned.

### Verify the download

Every release ships a signed `checksums.txt` (SHA-256) whose signature is bound
to the GitHub Actions build via a keyless cosign Sigstore bundle
(`checksums.txt.sigstore.json`).

```bash
# Fetch the checksums file and verify your archive against it
curl -sSLO https://github.com/scttfrdmn/cargoship/releases/download/v0.38.2/checksums.txt
sha256sum -c checksums.txt --ignore-missing

# Optionally verify the checksums file itself with cosign (keyless)
curl -sSLO https://github.com/scttfrdmn/cargoship/releases/download/v0.38.2/checksums.txt.sigstore.json
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
