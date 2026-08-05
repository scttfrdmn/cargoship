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
it, and move the binary onto your PATH.

::: code-group

```bash [Linux x86_64]
curl -sSL https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-linux-amd64.tar.gz | tar -xz
sudo mv cargoship-linux-amd64 /usr/local/bin/cargoship
```

```bash [Linux ARM64]
curl -sSL https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-linux-arm64.tar.gz | tar -xz
sudo mv cargoship-linux-arm64 /usr/local/bin/cargoship
```

```bash [macOS (Apple Silicon)]
curl -sSL https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-darwin-arm64.tar.gz | tar -xz
sudo mv cargoship-darwin-arm64 /usr/local/bin/cargoship
```

```bash [macOS (Intel)]
curl -sSL https://github.com/scttfrdmn/cargoship/releases/latest/download/cargoship-darwin-amd64.tar.gz | tar -xz
sudo mv cargoship-darwin-amd64 /usr/local/bin/cargoship
```

:::

## macOS: signing and Gatekeeper

From **v0.24.0** the macOS binaries are signed with an Apple Developer ID and
notarized by Apple, so they run from a download with no extra steps.

**Earlier versions were not.** They carried only an ad-hoc signature, and macOS
attaches a `com.apple.quarantine` attribute to anything a browser downloads.
Gatekeeper's response to a quarantined, un-notarized binary is not a warning
dialog — it **kills the process outright**, with no output. If you are on
v0.23.0 or older and `cargoship` appears to do nothing at all, that is what
happened. Either upgrade, or clear the attribute yourself:

```bash
xattr -d com.apple.quarantine /usr/local/bin/cargoship
```

Installing via Homebrew or `go install` avoids this on any version: neither path
sets the quarantine attribute.

You can confirm a binary is signed and notarized:

```bash
codesign -dvvv $(which cargoship) 2>&1 | grep Authority
# Authority=Developer ID Application: ...

spctl -a -vvv -t install $(which cargoship)
# source=Notarized Developer ID
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
