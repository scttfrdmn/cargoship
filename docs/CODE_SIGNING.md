# CargoShip Code Signing Guide

## Overview

This guide covers code signing strategies for CargoShip, including GPG keys for release signing, commit signing, and emerging alternatives like Sigstore.

## Current State of Code Signing

### Traditional Code Signing
- **Windows**: Authenticode with certificates from CAs like DigiCert, Sectigo
- **macOS**: Apple Developer certificates with notarization
- **Linux**: GPG signatures for packages and releases

### ACME for Code Signing
**Current Status (2024)**: ACME protocol support for code signing is **limited and experimental**:

- **ACME Protocol**: Primarily designed for TLS certificates
- **Code Signing Extensions**: Some CAs exploring ACME for code signing, but not standardized
- **Industry Adoption**: Very limited production support
- **Recommendation**: Use traditional CA-issued certificates for production code signing

### Modern Alternatives
- **Sigstore/Cosign**: Keyless signing for containers and artifacts
- **GitHub Artifact Attestations**: Built-in signing for GitHub Actions
- **SLSA (Supply-chain Levels for Software Artifacts)**: Framework for supply chain security

## Recommended Code Signing Strategy for CargoShip

### 1. GPG Key Setup for Project

Create a dedicated GPG key for CargoShip releases:

```bash
# Generate project-specific GPG key
gpg --full-generate-key

# Key parameters:
# - Type: RSA and RSA (default)
# - Key size: 4096 bits
# - Expiration: 2 years
# - Real name: CargoShip Release Key
# - Email: releases@cargoship.dev
# - Comment: Official CargoShip release signing key
```

**Complete setup script:**
```bash
#!/bin/bash
# setup-signing-key.sh

echo "Setting up CargoShip code signing..."

# Generate signing key with batch parameters
cat > cargoship-key-params <<EOF
%echo Generating CargoShip signing key
Key-Type: RSA
Key-Length: 4096
Subkey-Type: RSA
Subkey-Length: 4096
Name-Real: CargoShip Release Key
Name-Comment: Official CargoShip release signing key
Name-Email: releases@cargoship.dev
Expire-Date: 2y
Passphrase: $(openssl rand -base64 32)
%commit
%echo Done
EOF

# Generate the key
gpg --batch --generate-key cargoship-key-params

# Get the key ID
KEY_ID=$(gpg --list-secret-keys --keyid-format LONG releases@cargoship.dev | grep sec | awk '{print $2}' | cut -d'/' -f2)

echo "Generated key ID: $KEY_ID"

# Export public key
gpg --armor --export $KEY_ID > cargoship-public-key.asc

# Export key fingerprint
gpg --fingerprint $KEY_ID > cargoship-key-fingerprint.txt

echo "Public key exported to: cargoship-public-key.asc"
echo "Key fingerprint saved to: cargoship-key-fingerprint.txt"

# Clean up
rm cargoship-key-params

echo "Setup complete!"
echo "Key ID: $KEY_ID"
echo "Next steps:"
echo "1. Securely backup the private key"
echo "2. Upload public key to keyservers"
echo "3. Configure GitHub Actions secrets"
```

### 2. Multi-Platform Code Signing

#### Windows Code Signing
```bash
# Use osslsigncode for Windows binaries
osslsigncode sign \
  -certs windows-cert.p12 \
  -key windows-key.pem \
  -n "CargoShip" \
  -i https://github.com/scttfrdmn/cargoship \
  -t http://timestamp.digicert.com \
  -in cargoship.exe \
  -out cargoship-signed.exe
```

#### macOS Code Signing
```bash
# Sign macOS binary
codesign \
  --sign "Developer ID Application: Your Name" \
  --options runtime \
  --timestamp \
  cargoship-darwin

# Create notarized package
xcrun notarytool submit cargoship-darwin.zip \
  --apple-id your-apple-id@example.com \
  --password app-specific-password \
  --team-id YOUR_TEAM_ID
```

#### Linux/Universal GPG Signing
```bash
# Sign release archives
gpg --detach-sign --armor cargoship-linux-amd64.tar.gz
gpg --detach-sign --armor cargoship-darwin-amd64.tar.gz
gpg --detach-sign --armor cargoship-windows-amd64.zip

# Create checksums and sign them
sha256sum cargoship-* > checksums.txt
gpg --clearsign checksums.txt
```

### 3. Container Image Signing with Sigstore

```bash
# Install cosign
go install github.com/sigstore/cosign/v2/cmd/cosign@latest

# Sign container image (keyless)
cosign sign --yes cargoship:latest

# Sign with custom key
cosign generate-key-pair
cosign sign --key cosign.key cargoship:latest

# Verify signature
cosign verify cargoship:latest
```

### 4. GitHub Actions Integration

Create `.github/workflows/release.yml`:

```yaml
name: Release and Sign

on:
  release:
    types: [published]

permissions:
  contents: write
  packages: write
  id-token: write # for Sigstore

jobs:
  build-and-sign:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'
    
    - name: Import GPG key
      env:
        GPG_PRIVATE_KEY: ${{ secrets.GPG_PRIVATE_KEY }}
        GPG_PASSPHRASE: ${{ secrets.GPG_PASSPHRASE }}
      run: |
        echo "$GPG_PRIVATE_KEY" | gpg --batch --import
        echo "allow-loopback-pinentry" >> ~/.gnupg/gpg-agent.conf
        echo "pinentry-mode loopback" >> ~/.gnupg/gpg.conf
    
    - name: Build binaries
      run: |
        make build-all
    
    - name: Sign binaries
      env:
        GPG_PASSPHRASE: ${{ secrets.GPG_PASSPHRASE }}
      run: |
        for binary in dist/*; do
          gpg --batch --yes --passphrase "$GPG_PASSPHRASE" \
              --pinentry-mode loopback \
              --detach-sign --armor "$binary"
        done
    
    - name: Create checksums
      run: |
        cd dist
        sha256sum * > checksums.txt
        gpg --batch --yes --passphrase "$GPG_PASSPHRASE" \
            --pinentry-mode loopback \
            --clearsign checksums.txt
    
    - name: Install Cosign
      uses: sigstore/cosign-installer@v3
    
    - name: Sign container image
      run: |
        echo "${{ secrets.GITHUB_TOKEN }}" | docker login ghcr.io -u ${{ github.actor }} --password-stdin
        docker build -t ghcr.io/${{ github.repository }}:${{ github.ref_name }} .
        docker push ghcr.io/${{ github.repository }}:${{ github.ref_name }}
        cosign sign --yes ghcr.io/${{ github.repository }}:${{ github.ref_name }}
    
    - name: Upload release assets
      uses: softprops/action-gh-release@v1
      with:
        files: |
          dist/*
          dist/checksums.txt.asc
```

### 5. Key Management Best Practices

#### Secure Key Storage
```bash
# Create secure backup
gpg --export-secret-keys --armor releases@cargoship.dev > cargoship-private-key.asc

# Split key into shares (using Shamir's Secret Sharing)
ssss-split -t 3 -n 5 cargoship-private-key.asc
```

#### Key Rotation Policy
```bash
#!/bin/bash
# rotate-signing-key.sh

OLD_KEY_ID="$1"
NEW_KEY_EMAIL="releases-$(date +%Y)@cargoship.dev"

echo "Generating new signing key for $(date +%Y)..."

# Generate new key
gpg --batch --generate-key <<EOF
%echo Generating new CargoShip signing key
Key-Type: RSA
Key-Length: 4096
Subkey-Type: RSA
Subkey-Length: 4096
Name-Real: CargoShip Release Key $(date +%Y)
Name-Comment: CargoShip release signing key for $(date +%Y)
Name-Email: $NEW_KEY_EMAIL
Expire-Date: 2y
%commit
%echo Done
EOF

NEW_KEY_ID=$(gpg --list-secret-keys --keyid-format LONG $NEW_KEY_EMAIL | grep sec | awk '{print $2}' | cut -d'/' -f2)

echo "New key ID: $NEW_KEY_ID"

# Sign new key with old key (key transition)
if [ -n "$OLD_KEY_ID" ]; then
    gpg --default-key $OLD_KEY_ID --sign-key $NEW_KEY_ID
    echo "New key signed with old key for transition"
fi

# Export new public key
gpg --armor --export $NEW_KEY_ID > cargoship-public-key-$(date +%Y).asc

echo "Key rotation complete!"
echo "Update GitHub secrets with new key"
echo "Upload public key to keyservers"
echo "Update documentation with new fingerprint"
```

### 6. Verification Documentation

Create verification instructions for users:

```markdown
# Verifying CargoShip Releases

## GPG Verification

1. Import the CargoShip public key:
```bash
curl -L https://github.com/scttfrdmn/cargoship/releases/download/latest/cargoship-public-key.asc | gpg --import
```

2. Verify the key fingerprint:
```bash
gpg --fingerprint releases@cargoship.dev
```
Expected fingerprint: `1234 5678 9ABC DEF0 1234 5678 9ABC DEF0 1234 5678`

3. Download and verify a release:
```bash
curl -L -O https://github.com/scttfrdmn/cargoship/releases/download/v1.0.0/cargoship-linux-amd64.tar.gz
curl -L -O https://github.com/scttfrdmn/cargoship/releases/download/v1.0.0/cargoship-linux-amd64.tar.gz.asc

gpg --verify cargoship-linux-amd64.tar.gz.asc cargoship-linux-amd64.tar.gz
```

## Container Image Verification

Verify signed container images with Cosign:

```bash
# Install cosign
go install github.com/sigstore/cosign/v2/cmd/cosign@latest

# Verify image signature
cosign verify ghcr.io/scttfrdmn/cargoship:latest

# Verify with public key
cosign verify --key cargoship-cosign-public.key ghcr.io/scttfrdmn/cargoship:latest
```

## Checksum Verification

Always verify checksums after downloading:

```bash
# Download checksums
curl -L -O https://github.com/scttfrdmn/cargoship/releases/download/v1.0.0/checksums.txt.asc

# Verify checksums signature
gpg --verify checksums.txt.asc

# Verify file checksum
sha256sum -c checksums.txt.asc
```
```

### 7. Advanced Signing Options

#### SLSA Provenance
```yaml
# .github/workflows/slsa.yml
name: SLSA Provenance
on:
  release:
    types: [published]

permissions:
  id-token: write
  contents: read
  actions: read

jobs:
  build:
    outputs:
      digests: ${{ steps.hash.outputs.digests }}
    runs-on: ubuntu-latest
    steps:
    - name: Checkout
      uses: actions/checkout@v4
    
    - name: Setup Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'
    
    - name: Build
      run: |
        make build-all
    
    - name: Generate digests
      id: hash
      run: |
        cd dist && echo "digests=$(sha256sum * | base64 -w0)" >> "$GITHUB_OUTPUT"
    
    - name: Upload artifacts
      uses: actions/upload-artifact@v3
      with:
        name: binaries
        path: dist/

  provenance:
    needs: [build]
    permissions:
      actions: read
      id-token: write
      contents: write
    uses: slsa-framework/slsa-github-generator/.github/workflows/generator_generic_slsa3.yml@v1.7.0
    with:
      base64-subjects: "${{ needs.build.outputs.digests }}"
```

#### Reproducible Builds
```bash
# Build script for reproducible builds
#!/bin/bash
# build-reproducible.sh

export SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64

go build \
  -trimpath \
  -buildvcs=false \
  -ldflags="-w -s -X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$SOURCE_DATE_EPOCH" \
  -o cargoship-$GOOS-$GOARCH \
  ./cmd/cargoship
```

## Implementation Checklist

- [ ] Generate project-specific GPG key
- [ ] Set up GitHub Actions secrets
- [ ] Configure multi-platform signing
- [ ] Implement Sigstore/Cosign for containers
- [ ] Create verification documentation
- [ ] Set up key rotation procedures
- [ ] Configure SLSA provenance (optional)
- [ ] Test signing and verification process
- [ ] Document security contact information
- [ ] Plan for key compromise response

## Security Considerations

1. **Private Key Protection**:
   - Store private keys in hardware security modules (HSMs)
   - Use strong passphrases
   - Implement key splitting for backup

2. **Key Rotation**:
   - Rotate keys annually or after any compromise
   - Maintain old keys for verification of existing releases
   - Plan smooth transition process

3. **Verification**:
   - Always verify signatures before trusting releases
   - Check key fingerprints from multiple sources
   - Monitor for certificate transparency logs

4. **Supply Chain Security**:
   - Sign all release artifacts
   - Use SLSA provenance for build attestation
   - Implement reproducible builds where possible

---

**Note**: This guide will be updated as ACME code signing support matures and new signing technologies become available.