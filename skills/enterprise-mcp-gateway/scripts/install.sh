#!/usr/bin/env bash
# Enterprise MCP Gateway installation helper
set -euo pipefail

REPO="BenjaminJ/enterprise-mcp-gateway"
DEST_DIR="${1:-$HOME/.local/bin}"
BIN_NAME="mcp-gateway"

mkdir -p "$DEST_DIR"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Error: Unsupported architecture: $ARCH" >&2 && exit 1 ;;
esac

echo "Detected OS: $OS, Architecture: $ARCH"

# 1. If running inside git repo with Go installed, build from source
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

if [ -f "$REPO_ROOT/cmd/gateway/main.go" ] && command -v go >/dev/null 2>&1; then
  echo "Building $BIN_NAME from local repository source..."
  (cd "$REPO_ROOT" && go build -o "$DEST_DIR/$BIN_NAME" ./cmd/gateway)
  echo "✅ Successfully built and installed: $DEST_DIR/$BIN_NAME"
  exit 0
fi

# 2. Otherwise download release asset from GitHub
TARGET_BIN="${BIN_NAME}-${OS}-${ARCH}"
if [ "$OS" = "windows" ]; then
  TARGET_BIN="${TARGET_BIN}.exe"
  BIN_NAME="${BIN_NAME}.exe"
fi

RELEASE_URL="https://github.com/${REPO}/releases/latest/download/${TARGET_BIN}"

echo "Fetching release binary from GitHub: $RELEASE_URL"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$RELEASE_URL" -o "$DEST_DIR/$BIN_NAME"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$DEST_DIR/$BIN_NAME" "$RELEASE_URL"
else
  echo "Error: curl or wget is required to download binary." >&2
  exit 1
fi

chmod +x "$DEST_DIR/$BIN_NAME"
echo "✅ Successfully installed: $DEST_DIR/$BIN_NAME"
