#!/bin/bash

# CargoShip Release Build Script
# This script builds CargoShip binaries for all supported platforms

set -e

# Get version from git tag or use dev
VERSION=${1:-"dev"}
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS="-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}"

echo "🚢 Building CargoShip binaries"
echo "Version: ${VERSION}"
echo "Commit: ${COMMIT}"
echo "Date: ${DATE}"
echo ""

# Create dist directory
mkdir -p dist

# Build matrix
declare -a BUILDS=(
    "linux,amd64"
    "linux,arm64"
    "darwin,amd64"
    "darwin,arm64"
    "windows,amd64"
)

for build in "${BUILDS[@]}"; do
    IFS=',' read -r GOOS GOARCH <<< "$build"
    
    # Set output filename
    OUTPUT="dist/cargoship-${GOOS}-${GOARCH}"
    if [ "$GOOS" = "windows" ]; then
        OUTPUT="${OUTPUT}.exe"
    fi
    
    echo "🔨 Building ${GOOS}/${GOARCH}..."
    
    # Build binary
    CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
        -ldflags "${LDFLAGS}" \
        -o "${OUTPUT}" \
        ./cmd/cargoship
    
    # Create archive
    cd dist
    if [ "$GOOS" = "windows" ]; then
        zip "cargoship-${GOOS}-${GOARCH}.zip" "cargoship-${GOOS}-${GOARCH}.exe"
        rm "cargoship-${GOOS}-${GOARCH}.exe"
    else
        tar -czf "cargoship-${GOOS}-${GOARCH}.tar.gz" "cargoship-${GOOS}-${GOARCH}"
        rm "cargoship-${GOOS}-${GOARCH}"
    fi
    cd ..
    
    echo "✅ Created archive for ${GOOS}/${GOARCH}"
done

echo ""
echo "📦 Generating checksums..."
cd dist
sha256sum * > checksums.txt
echo "✅ Checksums generated"
cd ..

echo ""
echo "🎉 Build complete! Archives created in dist/ directory:"
ls -la dist/

echo ""
echo "To test a binary:"
echo "  # Extract and test (example for current platform)"
if [[ "$OSTYPE" == "darwin"* ]]; then
    if [[ $(uname -m) == "arm64" ]]; then
        echo "  tar -xzf dist/cargoship-darwin-arm64.tar.gz"
        echo "  ./cargoship-darwin-arm64 --version"
    else
        echo "  tar -xzf dist/cargoship-darwin-amd64.tar.gz" 
        echo "  ./cargoship-darwin-amd64 --version"
    fi
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "  tar -xzf dist/cargoship-linux-amd64.tar.gz"
    echo "  ./cargoship-linux-amd64 --version"
else
    echo "  # Extract appropriate archive for your platform"
fi

echo ""
echo "To create a GitHub release:"
echo "  git tag v1.0.0"
echo "  git push origin v1.0.0"
echo "  # GitHub Actions will automatically create the release"