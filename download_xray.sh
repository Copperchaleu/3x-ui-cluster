#!/bin/bash

# Define binary directory
BIN_DIR="./bin"
mkdir -p "$BIN_DIR"

# Detect OS and Architecture
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

# Map architecture names to Xray naming convention
case "$ARCH" in
    x86_64)
        XRAY_ARCH="64"
        GO_ARCH="amd64"
        ;;
    aarch64|arm64)
        XRAY_ARCH="arm64-v8a"
        GO_ARCH="arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

echo "Detected OS: $OS, Arch: $ARCH (Xray: $XRAY_ARCH, Go: $GO_ARCH)"

# Download URL
DOWNLOAD_URL="https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-${XRAY_ARCH}.zip"
ZIP_FILE="xray.zip"

echo "Downloading Xray Core from $DOWNLOAD_URL..."
if command -v curl >/dev/null 2>&1; then
    curl -L -o "$ZIP_FILE" "$DOWNLOAD_URL"
elif command -v wget >/dev/null 2>&1; then
    wget -O "$ZIP_FILE" "$DOWNLOAD_URL"
else
    echo "Error: Neither curl nor wget found."
    exit 1
fi

if [ ! -f "$ZIP_FILE" ]; then
    echo "Download failed."
    exit 1
fi

echo "Extracting files to $BIN_DIR..."
# Extract xray executable, geoip.dat, and geosite.dat
unzip -o -j "$ZIP_FILE" xray geoip.dat geosite.dat -d "$BIN_DIR"

# Rename binary to match 3x-ui expectation: xray-<OS>-<ARCH>
TARGET_BIN_NAME="xray-${OS}-${GO_ARCH}"
echo "Renaming binary from 'xray' to '$TARGET_BIN_NAME'..."
mv "$BIN_DIR/xray" "$BIN_DIR/$TARGET_BIN_NAME"

# Set executable permissions
chmod +x "$BIN_DIR/$TARGET_BIN_NAME"

# Cleanup
rm "$ZIP_FILE"

echo "Done! Xray core installed to $BIN_DIR/$TARGET_BIN_NAME"
ls -l "$BIN_DIR"
