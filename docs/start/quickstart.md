---
next:
  text: Install
  link: /start/install
---

# Quick Start

Zero to a verified upload in a few minutes. This is the happy path — each step
links to a page with the full detail.

## 1. Install

::: code-group

```bash [Homebrew]
brew install scttfrdmn/tap/cargoship
```

```bash [Go]
go install github.com/scttfrdmn/cargoship/cmd/cargoship@latest
```

:::

Verify it's on your PATH:

```bash
cargoship --version
```

More options on the [Install](/start/install) page.

## 2. Point it at AWS

CargoShip uses your standard AWS credentials (same as the AWS CLI). If you can run
`aws s3 ls`, you're ready. If not:

```bash
aws configure   # or set AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_REGION
```

See [AWS setup & credentials](/start/aws-setup) for the minimal IAM policy.

## 3. Upload a directory

```bash
cargoship upload ./my-data s3://my-bucket/archives/
```

CargoShip scans the directory, compresses and shards it, uploads in parallel, and
prints an **upload ID** (e.g. `20260721-a1b2c3`) when it finishes. That ID is how
you refer to the upload later.

::: tip
Not sure what it'll cost first? Run `cargoship estimate ./my-data` before
uploading.
:::

## 4. Confirm it's really there

```bash
cargoship info s3://my-bucket/archives/uploads/20260721-a1b2c3
cargoship verify s3://my-bucket/archives/uploads/20260721-a1b2c3
```

`info` shows the manifest summary; `verify` checks the archive against its
recorded checksums.

## 5. Restore a file back

```bash
cargoship restore s3://my-bucket/archives/uploads/20260721-a1b2c3 ./restored \
  --file path/inside/dataset.txt
```

If that file comes back byte-for-byte, you've proven the full round trip.

## Next

- [Your first upload](/start/first-upload) — the same flow explained line by line.
- [Verify & restore it](/start/verify-and-restore) — integrity and retrieval in depth.
- Pick a [use-case tutorial](/tutorials/) if you have a specific kind of data.
