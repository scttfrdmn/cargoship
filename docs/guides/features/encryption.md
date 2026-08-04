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
:::

Existing `private.key` / `public.key` files are never overwritten — the command
fails instead, because a replaced private key cannot be recovered.

### Rotating a key generated before v0.23.0 {#rotating-pre-v0230-keys}

**Every private key CargoShip generated before v0.23.0 is unencrypted**, and by
default it was written into a temporary directory (`$TMPDIR`) rather than a
location you chose. Two consequences worth being blunt about:

- The passphrase option existed but was never applied, so supplying one did not
  protect the key. There was no way to get a protected key out of those versions.
- On multi-user systems `$TMPDIR` is often a shared filesystem, so the file may
  have been readable by others, and may also have been deleted from under you by
  a cleanup job.

No upgrade can fix a file already on disk. Rotation is a manual step.

#### What rotation does and does not achieve

Archives are encrypted to the **public** key; the private key only decrypts. So:

| | Effect of rotating |
|---|---|
| Archives you upload *after* rotating | Protected by the new key. |
| Archives already uploaded with the old key | **Still decryptable with the old private key.** A new key cannot retroactively protect them. |

If an old private key may have been exposed, treat every archive encrypted to it
as exposed too. Rotating limits future damage; it does not undo past exposure.
To actually protect that existing data, re-encrypt it to the new key (download,
then re-upload with the new `--public-key`) and delete the old objects — or, if
the data is still sensitive enough to warrant it, treat it as disclosed.

#### Steps

1. **Find the old keys.** They are likely still in your temp directory:

   ```bash
   ls -la "${TMPDIR:-/tmp}"/gpg-keys*
   ```

2. **Generate a replacement**, into a directory you choose, with a passphrase:

   ```bash
   mkdir -p ~/keys/cargoship && cd ~/keys/cargoship
   cargoship create keys --name "Data Archive" --email archive@example.com
   ```

3. **Confirm the new private key is actually encrypted.** Ask `gpg` what is in
   the packets:

   ```bash
   gpg --list-packets private.key | grep -i protection
   ```

   A protected key reports something like `iter+salt S2K, algo: 9, SHA1
   protection` and `skey[2]: [v4 protected]`. **Empty output means the key is
   not encrypted.**

   ::: warning Do not grep the key file for "ENCRYPTED"
   An armored PGP private key has no plaintext marker saying whether it is
   protected — the `-----BEGIN PGP PRIVATE KEY BLOCK-----` header is identical
   either way. Searching the file for words like `ENCRYPTED` or `Passphrase`
   returns no match even for a correctly protected key, so it will tell you a
   safe key is unsafe. Only a tool that parses the packets can answer this.
   :::

4. **Distribute the new public key** to whoever uploads on your behalf, and
   update any `--public-key` arguments.

5. **Re-encrypt anything that still matters**, per the table above.

6. **Destroy the old private keys** once you are certain nothing you still need
   is encrypted only to them. On macOS and Linux:

   ```bash
   rm -rf "${TMPDIR:-/tmp}"/gpg-keys*
   ```

   Deleting a file does not reliably erase it from an SSD. If the key protected
   data you would report as breached, use full-disk-encryption key destruction or
   physical media destruction rather than `rm`.

::: tip Verify before you delete
Do step 6 last. If an archive you still need is encrypted only to the old key,
destroying that key destroys your access to the archive.
:::

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
