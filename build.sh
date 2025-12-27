#!/bin/bash
# Build script for Reolink plugin

set -e

echo "Building Reolink plugin for all platforms..."

# Build for current platform
echo "Building for current platform..."
go build -o reolink-plugin .

# Build for Linux AMD64 (most common Docker)
echo "Building for Linux AMD64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o reolink-plugin-linux-amd64 .

# Build for Linux ARM64 (Apple Silicon Docker, Raspberry Pi)
echo "Building for Linux ARM64..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o reolink-plugin-linux-arm64 .

echo "Build complete!"
ls -la reolink-plugin*

echo ""
echo "For Docker deployment, copy the appropriate binary:"
echo "  - AMD64 Docker: cp reolink-plugin-linux-amd64 reolink-plugin"
echo "  - ARM64 Docker: cp reolink-plugin-linux-arm64 reolink-plugin"
