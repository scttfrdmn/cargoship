# Security model

CargoShip encrypts data at rest using AWS Key Management Service (KMS). It offers
two independent, composable layers — encrypt data chunks, encrypt the manifest, or
both for end-to-end protection. This page covers the model and setup; for the
day-to-day flags see [Encryption (KMS & GPG)](/guides/features/encryption).

## Two encryption layers

### Data chunk encryption (SSE-KMS)

Chunks (`tar.zst` objects) are encrypted server-side with SSE-KMS. It's enabled
automatically when you pass `--kms-key-id`, has no client-side performance
overhead (AWS handles it), and is logged to CloudTrail.

### Manifest encryption (envelope encryption)

The manifest holds file metadata (paths, sizes, timestamps). With
`--encrypt-manifest`, CargoShip encrypts it client-side with AES-256-GCM before
upload, using a data key wrapped by your KMS CMK — so the manifest is never stored
unencrypted and integrity is authenticated.

```bash
cargoship upload ./data s3://bucket/prefix \
  --kms-key-id alias/cargoship-encryption \
  --encrypt-manifest
```

Downloads and inspection (`download`, `list`, `info`) detect and decrypt encrypted
manifests automatically, given the right KMS permissions.

## KMS setup

Create a symmetric encrypt/decrypt key (an alias is recommended so you can rotate
without changing commands):

```bash
aws kms create-key --description "CargoShip data encryption key" --key-usage ENCRYPT_DECRYPT
aws kms create-alias --alias-name alias/cargoship-encryption --target-key-id KEY_ID
```

Grant least-privilege IAM permissions:

- **Upload:** `kms:GenerateDataKey`, `kms:DescribeKey`
- **Download:** `kms:Decrypt`, `kms:DescribeKey`

You can additionally enforce encryption on the bucket with an S3 bucket policy that
denies `s3:PutObject` unless `s3:x-amz-server-side-encryption` is `aws:kms`.

## Best practices

::: tip
- Use **key aliases** and separate keys per environment (prod / staging).
- Enable **automatic key rotation**: `aws kms enable-key-rotation --key-id KEY_ID`.
- **Always `--encrypt-manifest` in production** — metadata is sensitive.
- Follow **least privilege**: uploaders get `GenerateDataKey`, downloaders get `Decrypt`.
- Enable **CloudTrail** for a KMS audit trail.
:::

## Compliance

AWS KMS uses FIPS 140-2 validated HSMs and is HIPAA-eligible, PCI DSS, and SOC 2
compliant; all KMS operations are logged to CloudTrail. KMS keys are regional —
SSE-KMS requires the bucket and key in the same region, while client-side manifest
envelope encryption allows cross-region uploads.

## Reporting a vulnerability

Report security issues privately via
[GitHub Security Advisories](https://github.com/scttfrdmn/cargoship/security/advisories/new).

## See also

- [Encryption (KMS & GPG)](/guides/features/encryption).
- [Encryption metadata format](/reference/format/encryption).
- [Costs & safety guarantees](/intro/costs-and-safety).
