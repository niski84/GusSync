#!/bin/bash
# Create GitHub release script
# Creates a release with attached binaries

set -e

VERSION="$1"
REPO="niski84/GusSync"
BUILD_DIR="dist"

if [ -z "$VERSION" ]; then
    echo "Error: Version required"
    echo "Usage: $0 <version>"
    exit 1
fi

# Remove 'v' prefix if present
VERSION_TAG="${VERSION#v}"
FULL_TAG="v${VERSION_TAG}"

echo "Creating release $FULL_TAG for $REPO"

# Check if dist directory exists and has binaries
if [ ! -d "$BUILD_DIR" ] || [ -z "$(ls -A $BUILD_DIR)" ]; then
    echo "Error: Build directory is empty. Run build.sh first."
    exit 1
fi

# Create release using GitHub CLI or API
if command -v gh &> /dev/null; then
    echo "Using GitHub CLI to create release..."
    
    # Create release notes
    NOTES="GusSync $VERSION_TAG Release
    
Binaries for Linux (amd64, arm64) and Windows (amd64, 386)."

    # Create release
    gh release create "$FULL_TAG" \
        --title "GusSync $VERSION_TAG" \
        --notes "$NOTES" \
        "$BUILD_DIR"/*
    
    echo "Release created: https://github.com/$REPO/releases/tag/$FULL_TAG"
else
    echo "GitHub CLI not found. Install 'gh' CLI or use GitHub API directly."
    echo "Release tag: $FULL_TAG"
    echo "Binaries in: $BUILD_DIR"
    exit 1
fi

