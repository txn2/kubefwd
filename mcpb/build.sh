#!/bin/bash
# Build MCPB bundles for Claude Desktop
# Usage: ./mcpb/build.sh [version] [--use-dist]
#
# This script creates platform-specific .mcpb bundles for:
# - macOS (darwin) amd64 and arm64
# - Windows amd64
#
# Options:
#   --use-dist  Use binaries from goreleaser's dist/ folder instead of building
#
# Prerequisites:
# - Go toolchain (if building from source)
# - goreleaser dist/ output (if using --use-dist)

set -e

VERSION="${1:-dev}"
USE_DIST=false

# Check for --use-dist flag
for arg in "$@"; do
    if [ "$arg" = "--use-dist" ]; then
        USE_DIST=true
    fi
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_ROOT/dist/mcpb"
MANIFEST_TEMPLATE="$SCRIPT_DIR/manifest.json"

echo "Building MCPB bundles for kubefwd v${VERSION}"
if [ "$USE_DIST" = true ]; then
    echo "Using pre-built binaries from dist/"
fi

# Clean and create build directory
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# Platforms to build for (Claude Desktop supports macOS and Windows)
# Format: GOOS:GOARCH:goreleaser_dir_suffix
# Note: goreleaser adds architecture version suffixes (v1 for amd64, v8.0 for arm64)
PLATFORMS=(
    "darwin:amd64:darwin_amd64_v1"
    "darwin:arm64:darwin_arm64_v8.0"
    "windows:amd64:windows_amd64_v1"
)

for platform in "${PLATFORMS[@]}"; do
    IFS=':' read -r GOOS GOARCH DIST_SUFFIX <<< "$platform"

    PLATFORM_NAME="${GOOS}-${GOARCH}"
    BUNDLE_DIR="$BUILD_DIR/kubefwd-${PLATFORM_NAME}"

    echo ""
    echo "=== Building for ${PLATFORM_NAME} ==="

    # Create bundle directory
    mkdir -p "$BUNDLE_DIR"

    # Determine binary name
    BINARY_NAME="kubefwd"
    if [ "$GOOS" = "windows" ]; then
        BINARY_NAME="kubefwd.exe"
    fi

    if [ "$USE_DIST" = true ]; then
        # Use goreleaser's pre-built binary
        DIST_BINARY="$PROJECT_ROOT/dist/kubefwd_${DIST_SUFFIX}/${BINARY_NAME}"
        if [ ! -f "$DIST_BINARY" ]; then
            echo "ERROR: Binary not found at $DIST_BINARY"
            echo "Make sure goreleaser has run first, or omit --use-dist to build from source"
            exit 1
        fi
        echo "Copying ${BINARY_NAME} from dist/..."
        cp "$DIST_BINARY" "$BUNDLE_DIR/$BINARY_NAME"
    else
        # Build from source
        echo "Compiling ${BINARY_NAME}..."
        CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build \
            -trimpath \
            -tags=netgo \
            -ldflags="-s -w -X main.Version=${VERSION}" \
            -o "$BUNDLE_DIR/$BINARY_NAME" \
            "$PROJECT_ROOT/cmd/kubefwd/kubefwd.go"
    fi

    # Copy and update manifest
    echo "Creating manifest.json..."
    sed "s/\"version\": \"0.0.0\"/\"version\": \"${VERSION}\"/" "$MANIFEST_TEMPLATE" > "$BUNDLE_DIR/manifest.json"

    # Create .mcpb bundle
    MCPB_FILE="$BUILD_DIR/kubefwd-${VERSION}-${PLATFORM_NAME}.mcpb"

    echo "Packaging ${MCPB_FILE}..."
    if command -v mcpb &> /dev/null; then
        # Use official mcpb CLI if available
        mcpb pack "$BUNDLE_DIR" "$MCPB_FILE"
    else
        # Fallback to zip (mcpb files are just zip archives)
        echo "Note: mcpb CLI not found, using zip fallback"
        (cd "$BUNDLE_DIR" && zip -r "$MCPB_FILE" .)
    fi

    echo "Created: $MCPB_FILE"
done

echo ""
echo "=== Build complete ==="
echo "MCPB bundles created in: $BUILD_DIR"
ls -la "$BUILD_DIR"/*.mcpb 2>/dev/null || echo "No .mcpb files found"
