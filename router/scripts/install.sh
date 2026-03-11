#!/bin/sh
set -e

REPO="kirniy/aiway"
TMP_DIR="/tmp/aiway-manager-install"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

detect_arch() {
  ARCH=$(opkg print-architecture 2>/dev/null | awk '/_kn/{print $2}' | sed 's/_kn.*//')
  [ -n "$ARCH" ] || { echo "Cannot detect Keenetic architecture" >&2; exit 1; }
}

fetch_version() {
  VERSION=$(curl -sI "https://github.com/$REPO/releases/latest" | sed -n 's/^[Ll]ocation:.*\/v\([^ \t\r]*\).*/\1/p' | tr -d '\r\n')
  [ -n "$VERSION" ] || { echo "Cannot detect latest release" >&2; exit 1; }
}

install_pkg() {
  PKG="aiway-manager_${VERSION}_${ARCH}-kn.ipk"
  URL="https://github.com/$REPO/releases/download/v${VERSION}/${PKG}"

  mkdir -p "$TMP_DIR"
  curl -fL -o "$TMP_DIR/$PKG" "$URL"
  opkg install "$TMP_DIR/$PKG"
}

detect_arch
fetch_version
install_pkg
