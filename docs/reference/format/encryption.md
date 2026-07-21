# Encryption

CargoShip supports two independent layers of encryption, both keyed by AWS KMS:

1. **Data encryption** — S3 server-side encryption (SSE-KMS) applied to the chunk
   objects. The chunk bytes themselves are standard `.tar.zst`; S3 handles the
   at-rest encryption transparently.
2. **Manifest encryption** — KMS **envelope** encryption applied to the manifest
   JSON, producing a `manifest.encrypted.json[.gz]` wrapper.

Both are optional and configured independently. The manifest records which are
active in its `encryption` block.

## `EncryptionMetadata`

The `encryption` field of the [manifest](/reference/format/manifest), when
present:

```go
type EncryptionMetadata struct {
	// Enabled indicates if encryption is active
	Enabled bool `json:"enabled"`

	// Data encryption (S3 server-side encryption)
	DataKMSKeyID string `json:"data_kms_key_id,omitempty"` // KMS key ID/ARN for data chunks

	// Manifest encryption (envelope encryption)
	ManifestEncrypted bool   `json:"manifest_encrypted"`            // Whether manifest itself is encrypted
	ManifestKMSKeyID  string `json:"manifest_kms_key_id,omitempty"` // KMS key ID/ARN for manifest
	Algorithm         string `json:"algorithm,omitempty"`           // e.g. "AES-256-GCM"
	EncryptedDEK      string `json:"encrypted_dek,omitempty"`       // Base64-encoded encrypted data encryption key
}
```

| Field | Meaning |
|-------|---------|
| `enabled` | True when data (chunk) encryption is active — set when a data KMS key is configured. |
| `data_kms_key_id` | KMS key ID/ARN used for S3 SSE-KMS on the chunk objects. |
| `manifest_encrypted` | True when the manifest is stored as an envelope-encrypted wrapper. |
| `manifest_kms_key_id` | KMS key ID/ARN used to wrap the manifest's data key. |
| `algorithm` | Symmetric algorithm for the manifest payload — `AES-256-GCM`. |
| `encrypted_dek` | Base64 of the KMS-encrypted data encryption key (mirrors the wrapper's `encrypted_dek`). |

::: info The two layers are orthogonal
You can encrypt data without encrypting the manifest, or vice versa. `enabled`
governs data/chunk encryption; `manifest_encrypted` governs the manifest wrapper.
:::

## Data encryption (chunks)

When `data_kms_key_id` is set, chunk objects are written with S3 server-side
encryption using that KMS key. The object layout is unchanged — a chunk is still
a `.tar.zst` (or `.tar`) object — and decryption is handled by S3 on `GetObject`
for any caller with `kms:Decrypt` on the key. No CargoShip-specific unwrapping is
needed to read the bytes.

## Manifest encryption (envelope)

When `manifest_encrypted` is true, the manifest is not stored as plaintext JSON.
Instead CargoShip writes an **`EncryptedManifest`** wrapper to
`manifest.encrypted.json` (or `.gz`):

```go
type EncryptedManifest struct {
	// Algorithm used for encryption
	Algorithm string `json:"algorithm"`

	// KMS key ID used to encrypt the data key
	KMSKeyID string `json:"kms_key_id"`

	// Base64-encoded encrypted data encryption key (DEK)
	EncryptedDEK string `json:"encrypted_dek"`

	// Base64-encoded initialization vector (IV/nonce) for AES-GCM
	IV string `json:"iv"`

	// Base64-encoded encrypted manifest data
	EncryptedData string `json:"encrypted_data"`
}
```

### Envelope encryption flow (write)

1. Call KMS `GenerateDataKey` with `KeySpec: AES_256` against `manifest_kms_key_id`.
   This returns both a **plaintext** 32-byte DEK and a **KMS-encrypted** copy of
   it (the ciphertext blob).
2. Encrypt the (optionally gzip-compressed) manifest JSON with **AES-256-GCM**
   using the plaintext DEK and a fresh random IV (nonce). GCM provides
   authenticated encryption — confidentiality **and** integrity.
3. Discard the plaintext DEK. Store the KMS-encrypted DEK, the IV, and the
   ciphertext (all base64-encoded) in the `EncryptedManifest` wrapper.
4. Serialize the wrapper to JSON and upload it as `manifest.encrypted.json[.gz]`.

The wrapper's `Algorithm` and `EncryptedDEK` are also copied back into the
manifest's `EncryptionMetadata` for reference.

### Decryption flow (read)

To recover the manifest JSON:

1. Fetch `manifest.encrypted.json.gz` (fall back to `manifest.encrypted.json`),
   and gunzip if compressed.
2. Parse the `EncryptedManifest` JSON and verify `Algorithm == "AES-256-GCM"`.
3. Base64-decode `encrypted_dek`, `iv`, and `encrypted_data`.
4. Call KMS `Decrypt` on the encrypted DEK to recover the plaintext 32-byte key.
5. AES-256-GCM `Open` the ciphertext with the DEK and IV. A GCM authentication
   failure means the data was tampered with or the wrong key was used — treat it
   as a hard error, not a soft fallback.
6. The result is the plaintext manifest JSON (still gzip-compressed if it was
   compressed before encryption — gunzip again if so).

::: warning Ordering: compress-then-encrypt
When both apply, the manifest is gzip-compressed **before** encryption, so the
GCM ciphertext wraps compressed bytes. On read, decrypt first, then gunzip.
:::

### Discovery order for a reader

A reader that supports encryption should probe keys in this order, falling back
on miss:

1. `manifest.encrypted.json.gz` — encrypted + gzip
2. `manifest.encrypted.json` — encrypted, plain
3. `manifest.json.gz` — plaintext + gzip
4. `manifest.json` — plaintext, plain

This mirrors CargoShip's own `DownloadFromS3WithDecryption`, which tries the
encrypted variants first and falls back to the plaintext download.

## Wrapper example

```json
{
  "algorithm": "AES-256-GCM",
  "kms_key_id": "arn:aws:kms:us-west-2:123456789012:key/abcd-1234",
  "encrypted_dek": "AQIDAHh...base64...",
  "iv": "n0Nc3Byt3s...base64...",
  "encrypted_data": "…base64 ciphertext of the (gzipped) manifest JSON…"
}
```

## Reader requirements

- You need `kms:Decrypt` on the manifest key to read an encrypted manifest, and
  on the data key to read SSE-KMS chunks.
- Enforce `Algorithm == "AES-256-GCM"`; reject unknown algorithms.
- Use the authenticated-decryption result — GCM verification is your integrity
  check for the manifest payload.
- The Go implementation lives in `pkg/encryption` (`EncryptManifest` /
  `DecryptManifest`, `EncryptedManifest`); see
  [Reading archives](/reference/format/library-api) for using it.
