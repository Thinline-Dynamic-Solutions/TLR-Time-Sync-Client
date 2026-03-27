#!/bin/bash
set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

VERSION=$(grep -E '^const Version' main.go 2>/dev/null | awk -F'"' '{print $2}')
if [ -z "$VERSION" ]; then
    VERSION="1.0.0"
fi

RELEASES_DIR="releases"
mkdir -p "$RELEASES_DIR"

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}TLR Time Sync Build Script${NC}"
echo -e "${GREEN}Version: ${VERSION}${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

build() {
    local GOOS=$1
    local GOARCH=$2
    local LABEL=$3

    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}Building: ${GOOS}/${GOARCH} (${LABEL})${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

    local EXT=""
    [ "$GOOS" = "windows" ] && EXT=".exe"

    local OUT="${RELEASES_DIR}/tlr-time-sync-${GOOS}-${GOARCH}-v${VERSION}${EXT}"

    GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build -ldflags="-s -w" -o "$OUT" .

    echo -e "${GREEN}  ✓ ${OUT}${NC}"
    echo ""
}

# Windows — primary target (most SDR-Trunk users)
build windows amd64  "64-bit"
build windows arm64  "ARM64"

# Linux
build linux   amd64  "64-bit"
build linux   arm64  "ARM64"
build linux   riscv64 "RISC-V 64"

# macOS
build darwin  amd64  "Intel"
build darwin  arm64  "Apple Silicon"

# Create macOS universal binary if lipo is available
if [[ "$OSTYPE" == "darwin"* ]] && command -v lipo &>/dev/null; then
    echo -e "${YELLOW}Creating macOS universal binary...${NC}"
    UNIVERSAL="${RELEASES_DIR}/tlr-time-sync-darwin-universal-v${VERSION}"
    lipo -create \
        "${RELEASES_DIR}/tlr-time-sync-darwin-amd64-v${VERSION}" \
        "${RELEASES_DIR}/tlr-time-sync-darwin-arm64-v${VERSION}" \
        -output "$UNIVERSAL"
    chmod +x "$UNIVERSAL"
    echo -e "${GREEN}  ✓ ${UNIVERSAL}${NC}"
    echo ""
fi

# FreeBSD
build freebsd amd64  "64-bit"
build freebsd arm64  "ARM64"

echo -e "${GREEN}════════════════════════════════════════${NC}"
echo -e "${GREEN}Build Complete!${NC}"
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo ""
echo "Artifacts in ${RELEASES_DIR}/:"
ls -1 "$RELEASES_DIR"/tlr-time-sync-* | while read f; do
    SIZE=$(du -sh "$f" | cut -f1)
    echo "  ✓ $(basename $f)  (${SIZE})"
done
echo ""
