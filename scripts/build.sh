#!/bin/bash
# Build script for GusSync
# Builds binaries for multiple platforms

set -e

VERSION="${1:-dev}"
BINARY_NAME="gussync"
BUILD_DIR="dist"

echo "Building GusSync version: $VERSION"

# Clean build directory
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# Build for Linux amd64
echo "Building Linux amd64..."
GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$VERSION" -o "$BUILD_DIR/${BINARY_NAME}-linux-amd64" .

# Build for Linux arm64
echo "Building Linux arm64..."
GOOS=linux GOARCH=arm64 go build -ldflags "-X main.version=$VERSION" -o "$BUILD_DIR/${BINARY_NAME}-linux-arm64" .

# Build for Windows amd64
echo "Building Windows amd64..."
GOOS=windows GOARCH=amd64 go build -ldflags "-X main.version=$VERSION" -o "$BUILD_DIR/${BINARY_NAME}-windows-amd64.exe" .

# Build for Windows 386
echo "Building Windows 386..."
GOOS=windows GOARCH=386 go build -ldflags "-X main.version=$VERSION" -o "$BUILD_DIR/${BINARY_NAME}-windows-386.exe" .

echo "Build complete! Binaries in $BUILD_DIR:"
ls -lh "$BUILD_DIR"


