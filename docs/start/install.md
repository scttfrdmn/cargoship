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
