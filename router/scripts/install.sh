#!/bin/sh
set -e

REPO="kirniy/aiway"
TMP_DIR="/tmp/aiway-manager-install"

fetch_text() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO- "$1"
    return
  fi
  echo "Need curl or wget" >&2
  exit 1
}

fetch_file() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$2" "$1"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
    return
  fi
  echo "Need curl or wget" >&2
  exit 1
}

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

detect_arch() {
  ARCH=$(opkg print-architecture 2>/dev/null | awk '/_kn/{print $2}' | sed 's/_kn.*//')
  [ -n "$ARCH" ] || { echo "Cannot detect Keenetic architecture" >&2; exit 1; }
}

fetch_version() {
  VERSION=$(fetch_text "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"v\([^"]*\)".*/\1/p' | sed -n '1p')
  [ -n "$VERSION" ] || { echo "Cannot detect latest release" >&2; exit 1; }
}

install_pkg() {
  PKG="aiway-manager_${VERSION}_${ARCH}-kn.ipk"
  URL="https://github.com/$REPO/releases/download/v${VERSION}/${PKG}"

  mkdir -p "$TMP_DIR"
  fetch_file "$URL" "$TMP_DIR/$PKG"
  opkg install "$TMP_DIR/$PKG"
}

detect_arch
fetch_version
install_pkg
