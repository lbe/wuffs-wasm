#!/usr/bin/env bash
# Ensures the pinned wasi-sdk is present under tools/ and prints its path.
# wasm/build.sh compiles the guest with it and make lint runs its clang-tidy,
# so the build and lint always use the same LLVM release.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WASI_VERSION="33.0"
WASI_ARCH="x86_64"
WASI_SDK="$ROOT/tools/wasi-sdk-${WASI_VERSION}-${WASI_ARCH}-linux"
WASI_TAR="wasi-sdk-${WASI_VERSION}-${WASI_ARCH}-linux.tar.gz"
WASI_URL="https://github.com/WebAssembly/wasi-sdk/releases/download/wasi-sdk-33/${WASI_TAR}"

if [[ ! -x "$WASI_SDK/bin/clang" ]]; then
  mkdir -p "$ROOT/tools"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  echo "downloading ${WASI_URL}" >&2
  curl -fsSL "$WASI_URL" -o "$tmp/$WASI_TAR"
  tar -C "$ROOT/tools" -xf "$tmp/$WASI_TAR"
fi

echo "$WASI_SDK"
