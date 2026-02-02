#!/bin/bash
set -e

# Ralph installer
# Usage: curl -fsSL https://raw.githubusercontent.com/scripness/ralph/main/install.sh | bash

REPO="scripness/ralph"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Detect OS and architecture
detect_platform() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"

    case "$ARCH" in
        x86_64|amd64)
            ARCH="x64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            echo "Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac

    case "$OS" in
        linux|darwin)
            ;;
        *)
            echo "Unsupported OS: $OS"
            exit 1
            ;;
    esac

    PLATFORM="${OS}-${ARCH}"
}

# Get latest release version
get_latest_version() {
    curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/'
}

# Download and install
install_ralph() {
    detect_platform
    
    echo "Detecting platform: $PLATFORM"
    
    VERSION=$(get_latest_version)
    if [ -z "$VERSION" ]; then
        echo "Failed to get latest version"
        exit 1
    fi
    
    echo "Latest version: v$VERSION"
    
    ASSET_NAME="ralph-${PLATFORM}"
    DOWNLOAD_URL="https://github.com/$REPO/releases/download/v$VERSION/$ASSET_NAME"
    
    echo "Downloading $ASSET_NAME..."
    
    # Create install directory
    mkdir -p "$INSTALL_DIR"
    
    # Download binary
    TMP_FILE=$(mktemp)
    if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_FILE"; then
        echo "Failed to download ralph"
        rm -f "$TMP_FILE"
        exit 1
    fi
    
    # Install binary
    chmod +x "$TMP_FILE"
    mv "$TMP_FILE" "$INSTALL_DIR/ralph"
    
    echo ""
    echo "✓ Ralph v$VERSION installed to $INSTALL_DIR/ralph"
    
    # Check if install dir is in PATH
    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        echo ""
        echo "Add to your PATH by running:"
        echo ""
        echo "  echo 'export PATH=\"\$PATH:$INSTALL_DIR\"' >> ~/.bashrc"
        echo "  source ~/.bashrc"
        echo ""
        echo "Or for zsh:"
        echo ""
        echo "  echo 'export PATH=\"\$PATH:$INSTALL_DIR\"' >> ~/.zshrc"
        echo "  source ~/.zshrc"
    fi
    
    echo ""
    echo "Run 'ralph --help' to get started."
}

install_ralph
