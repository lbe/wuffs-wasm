#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WASM_DIR="$ROOT/wasm"
BUILD_DIR="$WASM_DIR/build"

PROFILE="${1:-perf}"
case "$PROFILE" in
perf) CLANG_OPT=(-O3 -flto) WASM_OPT_LEVEL=-O3 ;;
size) CLANG_OPT=(-Oz -flto) WASM_OPT_LEVEL=-Oz ;;
*)
  echo "usage: $0 [perf|size]" >&2
  exit 2
  ;;
esac

WASI_SDK="$("$WASM_DIR/wasi-sdk.sh")"

CC="$WASI_SDK/bin/clang"
SYSROOT="$WASI_SDK/share/wasi-sysroot"

mkdir -p "$BUILD_DIR"
OBJ="$BUILD_DIR/wuffs.o"
RAW="$BUILD_DIR/wuffs.raw.wasm"
OUT="$WASM_DIR/wuffs.wasm"

COMMON_CFLAGS=(
  --target=wasm32-wasip1
  --sysroot="$SYSROOT"
  -std=c17
  -Wall
  -Wextra
  -Wno-cast-function-type
  -ffunction-sections
  -fdata-sections
  -fno-exceptions
  -fno-rtti
  -I"$WASM_DIR"
)

LDFLAGS=(
  -mexec-model=reactor
  -Wl,--no-entry
  -Wl,--gc-sections
  -Wl,--import-memory
  -Wl,-z,stack-size=8388608
  -Wl,--initial-memory=33554432
  -Wl,--max-memory=268435456
)

echo "compiling wuffs wasm (${PROFILE})" >&2
"$CC" "${COMMON_CFLAGS[@]}" "${CLANG_OPT[@]}" \
  -c "$WASM_DIR/shim.c" -o "$OBJ"

echo "linking wuffs wasm (${PROFILE})" >&2
"$CC" "${COMMON_CFLAGS[@]}" "${CLANG_OPT[@]}" \
  "$OBJ" -o "$RAW" "${LDFLAGS[@]}"

if command -v wasm-opt >/dev/null 2>&1; then
  echo "running wasm-opt ${WASM_OPT_LEVEL}" >&2
  wasm-opt "${WASM_OPT_LEVEL}" --converge --strip-debug --strip-producers \
    "$RAW" -o "$OUT"
else
  cp "$RAW" "$OUT"
fi

ls -lh "$OUT" >&2
wasm-objdump -h "$OUT" 2>/dev/null | rg 'code|Data|memory' || true
