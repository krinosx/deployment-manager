#!/usr/bin/env bash
# Builds static deploy-check/deploy-tui binaries and packages them, the
# systemd unit templates, a config template, and install.sh into a single
# tarball that can be copied to any target machine (e.g. the game server)
# and installed there WITHOUT needing Go installed there.
#
# Must run on a machine with the Go SDK installed (the dev machine).
#
# Usage: ./scripts/build_package.sh [output-dir]
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${1:-$REPO_DIR/dist}"

if [[ -n "${GO_BIN:-}" ]]; then
	: # explicit override
elif command -v go >/dev/null 2>&1; then
	GO_BIN="$(command -v go)"
elif [[ -x /home/krinosx/go-sdk/go1.26.7/bin/go ]]; then
	GO_BIN=/home/krinosx/go-sdk/go1.26.7/bin/go
else
	echo "go toolchain not found; set GO_BIN=/path/to/go and re-run" >&2
	exit 1
fi

VERSION="$(cd "$REPO_DIR" && git rev-parse --short HEAD 2>/dev/null || echo dev)"
if [[ -n "$(cd "$REPO_DIR" && git status --porcelain 2>/dev/null)" ]]; then
	VERSION="${VERSION}-dirty"
fi

TARGET_GOARCH="$("$GO_BIN" env GOARCH)"
PKG_NAME="deploy-manager-${VERSION}"
STAGE_DIR="$(mktemp -d)"
PKG_DIR="$STAGE_DIR/$PKG_NAME"

echo "Building package $PKG_NAME (linux/$TARGET_GOARCH)"
mkdir -p "$PKG_DIR"/{bin,config,systemd}

echo "Building static binaries (CGO_ENABLED=0, so no glibc-version dependency on the target machine)..."
CGO_ENABLED=0 GOOS=linux GOARCH="$TARGET_GOARCH" "$GO_BIN" build -C "$REPO_DIR" -o "$PKG_DIR/bin/deploy-check" ./cmd/deploy-check
CGO_ENABLED=0 GOOS=linux GOARCH="$TARGET_GOARCH" "$GO_BIN" build -C "$REPO_DIR" -o "$PKG_DIR/bin/deploy-tui" ./cmd/deploy-tui

cp "$REPO_DIR/resources/config.json" "$PKG_DIR/config/config.json"
cp "$REPO_DIR/resources/deploy-check.service" "$PKG_DIR/systemd/deploy-check.service"
cp "$REPO_DIR/resources/deploy-check.timer" "$PKG_DIR/systemd/deploy-check.timer"
cp "$REPO_DIR/scripts/install.sh" "$PKG_DIR/install.sh"
chmod +x "$PKG_DIR/install.sh"

echo "$VERSION" >"$PKG_DIR/VERSION"

mkdir -p "$OUT_DIR"
TARBALL="$OUT_DIR/${PKG_NAME}.tar.gz"
tar -C "$STAGE_DIR" -czf "$TARBALL" "$PKG_NAME"
rm -rf "$STAGE_DIR"

echo
echo "Package created: $TARBALL"
echo "Copy it to the target machine, then:"
echo "  tar xzf $(basename "$TARBALL")"
echo "  cd $PKG_NAME"
echo "  sudo ./install.sh"
