#!/usr/bin/env bash
set -euo pipefail

# Output directory
DIST_DIR="./dist"
mkdir -p "${DIST_DIR}"

VERSION="1.0.0"
APP_NAME="mcp-gateway"
MOCK_NAME="mockserver"

PLATFORMS=(
    "darwin/arm64"
    "darwin/amd64"
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
)

echo "======================================================="
echo " Building ${APP_NAME} v${VERSION} for all target architectures"
echo "======================================================="

for PLATFORM in "${PLATFORMS[@]}"; do
    GOOS="${PLATFORM%/*}"
    GOARCH="${PLATFORM#*/}"
    
    EXT=""
    if [ "$GOOS" = "windows" ]; then
        EXT=".exe"
    fi

    GATEWAY_OUT="${DIST_DIR}/${APP_NAME}-${GOOS}-${GOARCH}${EXT}"
    MOCK_OUT="${DIST_DIR}/${MOCK_NAME}-${GOOS}-${GOARCH}${EXT}"

    echo "⚙️  Building for ${GOOS}/${GOARCH}..."

    # Build static Gateway binary
    CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o "${GATEWAY_OUT}" ./cmd/gateway

    # Build static Mockserver binary
    CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o "${MOCK_OUT}" ./cmd/mockserver

    echo "   ✅ Generated: ${GATEWAY_OUT}"
done

echo ""
echo "🎉 All distribution binaries successfully generated in ${DIST_DIR}/:"
ls -lh "${DIST_DIR}"
