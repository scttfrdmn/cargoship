#!/bin/bash

# Setup Git Hooks for CargoShip
# This script configures the git hooks directory and installs the pre-commit hook

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

# Check if we're in a git repository
if [ ! -d ".git" ]; then
    echo "Error: This script must be run from the root of a git repository"
    exit 1
fi

# Configure git to use our hooks directory
log_info "Configuring git hooks directory..."
git config core.hooksPath .githooks

# Make hooks executable
log_info "Making hooks executable..."
chmod +x .githooks/*

# Verify the setup
log_info "Verifying hook setup..."
if [ -x ".githooks/pre-commit" ]; then
    log_success "Pre-commit hook is installed and executable"
else
    echo "Error: Pre-commit hook installation failed"
    exit 1
fi

echo
log_success "🎉 Git hooks successfully configured!"
echo
echo "The following hooks are now active:"
echo "  • pre-commit: Runs linting, tests, and quality checks"
echo
echo "To bypass hooks temporarily (not recommended), use:"
echo "  git commit --no-verify"
echo
echo "To test the pre-commit hook manually:"
echo "  .githooks/pre-commit"