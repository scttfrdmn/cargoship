# Encryption (KMS & GPG)

CargoShip encrypts data at two layers you control independently: the S3 objects
themselves (via AWS KMS) and, on top of that, the manifest (via KMS envelope
encryption). For portable, provider-independent encryption it can also use GPG
key pairs it generates for you.

How the encryption metadata is recorded in the archive — so a reader knows how to
decrypt — is documented in the [encryption format spec](/reference/format/encryption).
This guide is about turning it on.

## SSE-KMS for data chunks

Point CargoShip at a KMS key and every uploaded chunk is written with server-side
encryption (SSE-KMS):

```bash
cargoship upload ./my-data s3://my-bucket/archives/ \
  --kms-key-id alias/my-key
```

`--kms-key-id` accepts a key ID, ARN, or alias. S3 encrypts each object at rest
with that key; decryption on download is transparent to anyone with `kms:Decrypt`
permission on the key. This is the recommended baseline for sensitive data — the
key never leaves KMS and access is governed by IAM.

## Envelope-encrypt the manifest

The [manifest](/intro/concepts) is the index of what's in your upload — file
names, sizes, layout. SSE-KMS protects it at rest like any object, but you can
add a second layer that encrypts the manifest's contents with a KMS-derived data
key (envelope encryption) so it's unreadable without an explicit decrypt:

```bash
cargoship upload ./my-data s3://my-bucket/archives/ \
  --kms-key-id alias/my-key --encrypt-manifest
```

::: warning `--encrypt-manifest` requires `--kms-key-id`
Manifest envelope encryption is built on the same KMS key, so you must supply
`--kms-key-id`. Without it, `--encrypt-manifest` has nothing to derive a data key
from.
:::

Use this when even file *names and structure* are sensitive (e.g. clinical or
proprietary datasets), not just file contents.

## GPG key pairs

For encryption that doesn't depend on AWS KMS — portable archives, offline
recipients, or an existing GPG workflow — CargoShip can generate key pairs:

```bash
cargoship create keys --name "Data Archive" --email archive@example.com
```

| Flag | Meaning | Default |
|------|---------|---------|
| `--type` | Key type: `rsa` or `x25519` | `rsa` |
| `--bits` | RSA key length | `4096` |
| `--name` | Name attached to the key | — |
| `--email` | Email attached to the key | — |
| `--no-passphrase` | Generate an **unencrypted** private key | `false` |
| `-d, --destination` | Directory to write the key files (inherited) | current directory |

This writes a private/public key pair you can use to encrypt archives for a
specific recipient, independent of any cloud KMS. `x25519` produces smaller,
modern keys; `rsa` at 4096 bits maximizes compatibility.

### Passphrase protection

The private key is encrypted with a passphrase. You'll be prompted for it, or
you can set `CARGOSHIP_GPG_PASSPHRASE` for non-interactive use:

```bash
export CARGOSHIP_GPG_PASSPHRASE='...'
cargoship create keys --name "Data Archive" --email archive@example.com
```

There is deliberately no `--passphrase` flag — a flag value ends up in shell
history, in `ps` output, and in CI logs that echo command lines.

::: warning `--no-passphrase` writes plaintext key material
`--no-passphrase` generates an unencrypted private key. Anyone who can read the
file can decrypt every archive it protects. Use it only where the file itself is
already protected by something else, and never on a shared filesystem.

**Keys generated before v0.23.0 were always unencrypted**, and CargoShip wrote
them to a temporary directory by default. If you are holding a key pair from an
earlier version, treat the private key as unprotected: move it somewhere safe,
or generate and distribute a replacement.
:::

Existing `private.key` / `public.key` files are never overwritten — the command
fails instead, because a replaced private key cannot be recovered.

## KMS or GPG?

| | SSE-KMS | GPG |
|---|---------|-----|
| Key management | AWS KMS, IAM-governed | You manage the key files |
| Best for | AWS-native workflows, audited access | Portable / offline / cross-provider |
| Manifest layer | `--encrypt-manifest` | via recipient key |
| Setup | A KMS key + IAM | `cargoship create keys` |

Most AWS users want SSE-KMS. Reach for GPG when the archive must be decryptable
outside AWS or by a specific keyholder.

## Best practices

::: tip
- **Use SSE-KMS with a dedicated key** for sensitive data — access is auditable
  and revocable through IAM/KMS.
- **Add `--encrypt-manifest`** when file names or directory structure are
  themselves sensitive, not just contents.
- **Grant `kms:Decrypt` narrowly** — only the roles that must restore the data.
- **Prefer `x25519`** for new GPG keys; use `rsa 4096` only when a recipient needs
  broad compatibility.
- **Store private keys in a secrets manager**, never alongside the archive or in
  version control.
:::

## See also

- [Encryption metadata (format spec)](/reference/format/encryption) — how decryption info is recorded.
- [Uploading data](/guides/uploading) — where `--kms-key-id` and `--encrypt-manifest` fit in a run.
- [Security model](/project/security) — CargoShip's overall security posture.
- Reference: [Uploading & sync commands](/reference/commands/upload).
