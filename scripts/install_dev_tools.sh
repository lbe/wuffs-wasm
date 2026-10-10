#!/usr/bin/env bash
# Install CLI tools used by wuffs-go (make format-check, make lint, make generate).
# Does not install Go itself. Pins match .github/workflows/ci.yml where noted.
#
# Usage:
#   ./scripts/install_dev_tools.sh
#   INSTALL_DIR=~/.local/bin ./scripts/install_dev_tools.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TREE_VERSION=2.5.0
DPRINT_VERSION=0.55.2
GOLANGCI_VERSION=v2.13.1

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
GOBIN="${GOBIN:-$(go env GOPATH 2>/dev/null)/bin}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

install_bin() {
  local src="$1"
  local name="$2"
  local dest="$INSTALL_DIR/$name"
  if [[ -w $INSTALL_DIR ]]; then
    install -m 755 "$src" "$dest"
  else
    sudo install -m 755 "$src" "$dest"
  fi
  echo "installed $dest"
}

map_platform() {
  local os arch
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
  Linux)
    OS=linux
    ;;
  Darwin)
    OS=darwin
    ;;
  *)
    echo "unsupported OS: $os (need Linux or macOS)" >&2
    exit 1
    ;;
  esac
  case "$arch" in
  x86_64 | amd64)
    ARCH=amd64
    DPRINT_TRIPLE=x86_64
    ;;
  arm64 | aarch64)
    ARCH=arm64
    DPRINT_TRIPLE=aarch64
    ;;
  *)
    echo "unsupported CPU: $arch" >&2
    exit 1
    ;;
  esac
}

map_platform

need_cmd go
need_cmd curl
need_cmd tar

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$INSTALL_DIR" "$GOBIN"

# treefmt (orchestrates gofmt, goimports, dprint, shfmt via treefmt.toml)
TREE_TAR="treefmt_${TREE_VERSION}_${OS}_${ARCH}.tar.gz"
TREE_URL="https://github.com/numtide/treefmt/releases/download/v${TREE_VERSION}/${TREE_TAR}"
echo "fetching $TREE_URL"
curl -fsSL "$TREE_URL" -o "$TMP/treefmt.tar.gz"
tar -xzf "$TMP/treefmt.tar.gz" -C "$TMP"
install_bin "$TMP/treefmt" treefmt

# dprint (markdown)
case "$OS" in
linux)
  DPRINT_ZIP="dprint-${DPRINT_TRIPLE}-unknown-linux-gnu.zip"
  ;;
darwin)
  DPRINT_ZIP="dprint-${DPRINT_TRIPLE}-apple-darwin.zip"
  ;;
esac
DPRINT_URL="https://github.com/dprint/dprint/releases/download/${DPRINT_VERSION}/${DPRINT_ZIP}"
echo "fetching $DPRINT_URL"
need_cmd unzip
curl -fsSL "$DPRINT_URL" -o "$TMP/dprint.zip"
unzip -q -o "$TMP/dprint.zip" -d "$TMP/dprint"
install_bin "$TMP/dprint/dprint" dprint

# Go-installed tools (versions pinned where CI pins them)
export PATH="$GOBIN:$PATH"
echo "go install -> $GOBIN"
go install golang.org/x/tools/cmd/goimports@latest
go install mvdan.cc/sh/v3/cmd/shfmt@latest
go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_VERSION}"
go install github.com/ncruces/wasm2go/cmd/wasm2go@latest

echo ""
echo "Done. Ensure these are on PATH:"
echo "  export PATH=\"$INSTALL_DIR:\$HOME/go/bin:\$PATH\""
echo ""
echo "Verify from repo root:"
echo "  make format-check"
echo "  make lint"
echo "  command -v wasm2go treefmt dprint goimports shfmt golangci-lint"
