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

    local STAGING=$(mktemp -d)
    trap "rm -rf '$STAGING'" RETURN

    if [ "$GOOS" = "windows" ]; then
        local BIN="$STAGING/tlr-time-sync.exe"
        local ARCHIVE="${RELEASES_DIR}/tlr-time-sync-${GOOS}-${GOARCH}-v${VERSION}.zip"

        GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN" .
        cp tlr-time-sync.ini "$STAGING/"
        cp README.md "$STAGING/"

        (cd "$STAGING" && zip -r - .) > "$ARCHIVE"
    else
        local BIN="$STAGING/tlr-time-sync"
        local ARCHIVE="${RELEASES_DIR}/tlr-time-sync-${GOOS}-${GOARCH}-v${VERSION}.tar.gz"

        GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN" .
        chmod +x "$BIN"
        cp tlr-time-sync.ini "$STAGING/"
        cp README.md "$STAGING/"

        tar -czf "$ARCHIVE" -C "$STAGING" .
    fi

    echo -e "${GREEN}  ✓ $(basename $ARCHIVE)${NC}"
    echo ""
}

# Windows — primary target (most SDR-Trunk users)
build windows amd64  "64-bit"
build windows arm64  "ARM64"

# Linux
build linux   amd64   "64-bit"
build linux   arm64   "ARM64"
build linux   riscv64 "RISC-V 64"

# macOS
build darwin  amd64  "Intel"
build darwin  arm64  "Apple Silicon"

# macOS universal binary — combine amd64 + arm64 then repackage
if [[ "$OSTYPE" == "darwin"* ]] && command -v lipo &>/dev/null; then
    echo -e "${YELLOW}Creating macOS universal binary...${NC}"

    STAGING_AMD=$(mktemp -d)
    STAGING_ARM=$(mktemp -d)
    STAGING_UNI=$(mktemp -d)
    trap "rm -rf '$STAGING_AMD' '$STAGING_ARM' '$STAGING_UNI'" EXIT

    # Extract the two archives to get the binaries
    tar -xzf "${RELEASES_DIR}/tlr-time-sync-darwin-amd64-v${VERSION}.tar.gz" -C "$STAGING_AMD"
    tar -xzf "${RELEASES_DIR}/tlr-time-sync-darwin-arm64-v${VERSION}.tar.gz" -C "$STAGING_ARM"

    lipo -create "$STAGING_AMD/tlr-time-sync" "$STAGING_ARM/tlr-time-sync" -output "$STAGING_UNI/tlr-time-sync"
    chmod +x "$STAGING_UNI/tlr-time-sync"
    cp tlr-time-sync.ini "$STAGING_UNI/"
    cp README.md "$STAGING_UNI/"

    UNIVERSAL="${RELEASES_DIR}/tlr-time-sync-darwin-universal-v${VERSION}.tar.gz"
    tar -czf "$UNIVERSAL" -C "$STAGING_UNI" .
    echo -e "${GREEN}  ✓ $(basename $UNIVERSAL)${NC}"
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
ls -1 "$RELEASES_DIR"/tlr-time-sync-*-v${VERSION}.* 2>/dev/null | while read f; do
    SIZE=$(du -sh "$f" | cut -f1)
    echo "  ✓ $(basename $f)  (${SIZE})"
done
echo ""
