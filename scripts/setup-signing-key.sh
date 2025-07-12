#!/bin/bash
# CargoShip GPG Key Setup Script
# This script generates a project-specific GPG key for code signing

set -euo pipefail

# Configuration
PROJECT_NAME="CargoShip"
KEY_EMAIL="releases@cargoship.dev"
KEY_COMMENT="Official CargoShip release signing key"
KEY_SIZE=4096
KEY_EXPIRY="2y"
OUTPUT_DIR="./keys"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check dependencies
check_dependencies() {
    log_info "Checking dependencies..."
    
    if ! command -v gpg &> /dev/null; then
        log_error "GPG is not installed. Please install GnuPG."
        exit 1
    fi
    
    if ! command -v openssl &> /dev/null; then
        log_error "OpenSSL is not installed. Please install OpenSSL."
        exit 1
    fi
    
    log_success "All dependencies are available"
}

# Check if key already exists
check_existing_key() {
    log_info "Checking for existing CargoShip signing key..."
    
    if gpg --list-secret-keys "$KEY_EMAIL" &> /dev/null; then
        log_warning "A signing key for $KEY_EMAIL already exists!"
        echo
        gpg --list-secret-keys --keyid-format LONG "$KEY_EMAIL"
        echo
        
        read -p "Do you want to continue and create a new key? This will not delete the existing key. (y/N): " -r
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Aborted by user"
            exit 0
        fi
    fi
}

# Generate secure passphrase
generate_passphrase() {
    log_info "Generating secure passphrase..."
    
    # Generate a strong passphrase
    PASSPHRASE=$(openssl rand -base64 32 | tr -d "=+/" | cut -c1-25)
    
    # Add some memorable words for human-friendliness
    WORDS=("Alpha" "Bravo" "Charlie" "Delta" "Echo" "Foxtrot" "Golf" "Hotel" "India" "Juliet")
    WORD1=${WORDS[$RANDOM % ${#WORDS[@]}]}
    WORD2=${WORDS[$RANDOM % ${#WORDS[@]}]}
    
    PASSPHRASE="${WORD1}${WORD2}${PASSPHRASE}"
    
    log_success "Secure passphrase generated"
}

# Create key generation parameters
create_key_params() {
    log_info "Creating key generation parameters..."
    
    mkdir -p "$OUTPUT_DIR"
    
    cat > "$OUTPUT_DIR/cargoship-key-params" <<EOF
%echo Generating CargoShip signing key
Key-Type: RSA
Key-Length: $KEY_SIZE
Subkey-Type: RSA
Subkey-Length: $KEY_SIZE
Name-Real: $PROJECT_NAME Release Key
Name-Comment: $KEY_COMMENT
Name-Email: $KEY_EMAIL
Expire-Date: $KEY_EXPIRY
Passphrase: $PASSPHRASE
%commit
%echo Done
EOF
    
    log_success "Key parameters created"
}

# Generate the GPG key
generate_key() {
    log_info "Generating GPG key (this may take a few minutes)..."
    
    # Increase entropy for better key generation
    if command -v haveged &> /dev/null; then
        log_info "Starting haveged for better entropy..."
        sudo haveged -w 1024 &
        HAVEGED_PID=$!
    fi
    
    # Generate the key
    gpg --batch --generate-key "$OUTPUT_DIR/cargoship-key-params"
    
    # Stop haveged if we started it
    if [ -n "${HAVEGED_PID:-}" ]; then
        sudo kill $HAVEGED_PID 2>/dev/null || true
    fi
    
    log_success "GPG key generated successfully"
}

# Export key information
export_key_info() {
    log_info "Exporting key information..."
    
    # Get the key ID
    KEY_ID=$(gpg --list-secret-keys --keyid-format LONG "$KEY_EMAIL" | grep sec | awk '{print $2}' | cut -d'/' -f2 | head -1)
    
    if [ -z "$KEY_ID" ]; then
        log_error "Failed to get key ID"
        exit 1
    fi
    
    log_info "Key ID: $KEY_ID"
    
    # Export public key
    gpg --armor --export "$KEY_ID" > "$OUTPUT_DIR/cargoship-public-key.asc"
    log_success "Public key exported to $OUTPUT_DIR/cargoship-public-key.asc"
    
    # Export key fingerprint
    gpg --fingerprint "$KEY_ID" > "$OUTPUT_DIR/cargoship-key-fingerprint.txt"
    log_success "Key fingerprint saved to $OUTPUT_DIR/cargoship-key-fingerprint.txt"
    
    # Create key info file
    cat > "$OUTPUT_DIR/key-info.txt" <<EOF
CargoShip Release Signing Key Information
========================================

Key ID: $KEY_ID
Email: $KEY_EMAIL
Generated: $(date)
Expires: $(gpg --list-keys --keyid-format LONG "$KEY_EMAIL" | grep "expires:" | head -1 | sed 's/.*expires: //')

Fingerprint:
$(gpg --fingerprint "$KEY_ID" | grep "Key fingerprint" | sed 's/.*= //')

Public Key File: cargoship-public-key.asc
Private Key: Stored in GPG keyring (use 'gpg --list-secret-keys')

Passphrase: $PASSPHRASE
WARNING: Store this passphrase securely! It cannot be recovered if lost.

Usage:
- Sign releases: gpg --detach-sign --armor --default-key $KEY_ID <file>
- Verify: gpg --verify <file>.asc <file>

Next Steps:
1. Backup the private key securely
2. Upload public key to keyservers
3. Configure GitHub Actions secrets
4. Update documentation with fingerprint
EOF
    
    log_success "Key information saved to $OUTPUT_DIR/key-info.txt"
}

# Backup private key
backup_private_key() {
    log_info "Creating encrypted backup of private key..."
    
    # Export private key
    gpg --armor --export-secret-keys "$KEY_ID" > "$OUTPUT_DIR/cargoship-private-key.asc"
    
    # Encrypt the private key backup with a different passphrase
    BACKUP_PASSPHRASE=$(openssl rand -base64 32)
    echo "$BACKUP_PASSPHRASE" | gpg --batch --yes --passphrase-fd 0 --symmetric --cipher-algo AES256 --output "$OUTPUT_DIR/cargoship-private-key.asc.gpg" "$OUTPUT_DIR/cargoship-private-key.asc"
    
    # Remove unencrypted private key
    rm "$OUTPUT_DIR/cargoship-private-key.asc"
    
    # Save backup info
    cat >> "$OUTPUT_DIR/key-info.txt" <<EOF

Backup Information:
==================
Encrypted Private Key: cargoship-private-key.asc.gpg
Backup Passphrase: $BACKUP_PASSPHRASE

To restore:
gpg --batch --quiet --passphrase "$BACKUP_PASSPHRASE" --decrypt cargoship-private-key.asc.gpg | gpg --import
EOF
    
    log_success "Encrypted backup created"
}

# Upload to keyservers
upload_to_keyservers() {
    read -p "Do you want to upload the public key to keyservers? (y/N): " -r
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        log_info "Uploading public key to keyservers..."
        
        # List of keyservers to try
        KEYSERVERS=(
            "hkps://keys.openpgp.org"
            "hkps://keyserver.ubuntu.com"
            "hkps://pgp.mit.edu"
        )
        
        for server in "${KEYSERVERS[@]}"; do
            log_info "Uploading to $server..."
            if gpg --keyserver "$server" --send-keys "$KEY_ID"; then
                log_success "Successfully uploaded to $server"
            else
                log_warning "Failed to upload to $server"
            fi
        done
    fi
}

# Display GitHub Actions setup instructions
show_github_setup() {
    log_info "GitHub Actions Setup Instructions:"
    
    cat <<EOF

To configure GitHub Actions for automatic signing:

1. Go to your repository settings
2. Navigate to Secrets and variables > Actions
3. Add the following secrets:

   GPG_PRIVATE_KEY:
   $(gpg --armor --export-secret-keys "$KEY_ID" | base64 -w 0)

   GPG_PASSPHRASE:
   $PASSPHRASE

   GPG_KEY_ID:
   $KEY_ID

4. Update your workflow files to use these secrets for signing

Example workflow step:
```yaml
- name: Import GPG key
  env:
    GPG_PRIVATE_KEY: \${{ secrets.GPG_PRIVATE_KEY }}
    GPG_PASSPHRASE: \${{ secrets.GPG_PASSPHRASE }}
  run: |
    echo "\$GPG_PRIVATE_KEY" | base64 -d | gpg --batch --import
    echo "allow-loopback-pinentry" >> ~/.gnupg/gpg-agent.conf
    echo "pinentry-mode loopback" >> ~/.gnupg/gpg.conf
```

EOF
}

# Cleanup
cleanup() {
    log_info "Cleaning up temporary files..."
    rm -f "$OUTPUT_DIR/cargoship-key-params"
    log_success "Cleanup complete"
}

# Security reminders
show_security_reminders() {
    log_warning "Security Reminders:"
    cat <<EOF

🔐 IMPORTANT SECURITY NOTES:

1. PASSPHRASE SECURITY:
   - Store the passphrase in a secure password manager
   - Never commit the passphrase to version control
   - Consider using a hardware security module (HSM) for production

2. PRIVATE KEY SECURITY:
   - The private key is stored in your GPG keyring
   - Create secure backups and store them offline
   - Consider key splitting for high-security environments

3. ACCESS CONTROL:
   - Limit access to the private key to authorized personnel only
   - Use separate keys for different environments (dev/staging/prod)
   - Implement key rotation policies (recommended: annually)

4. VERIFICATION:
   - Always verify signatures before trusting releases
   - Publish key fingerprints through multiple channels
   - Monitor for unauthorized key usage

5. INCIDENT RESPONSE:
   - Have a plan for key compromise scenarios
   - Know how to revoke and rotate keys quickly
   - Keep contact information for security team updated

EOF
}

# Main execution
main() {
    echo
    log_info "🔐 CargoShip GPG Key Setup"
    echo "================================"
    echo
    
    check_dependencies
    check_existing_key
    generate_passphrase
    create_key_params
    generate_key
    export_key_info
    backup_private_key
    upload_to_keyservers
    cleanup
    
    echo
    log_success "🎉 GPG key setup completed successfully!"
    echo
    
    log_info "Key details:"
    echo "  Email: $KEY_EMAIL"
    echo "  Key ID: $KEY_ID"
    echo "  Files: $OUTPUT_DIR/"
    echo
    
    show_github_setup
    show_security_reminders
    
    log_info "Next steps:"
    echo "1. Securely store the passphrase and backup"
    echo "2. Configure GitHub Actions secrets"
    echo "3. Update project documentation"
    echo "4. Test signing and verification"
    echo
}

# Run main function
main "$@"