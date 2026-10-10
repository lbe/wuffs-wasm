#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

MANIFEST="testdata/bench/manifest.tsv"
WUFFS_COMMIT="$(sed -n '1s/^# wuffs_commit=//p' "$MANIFEST")"
if [[ -z $WUFFS_COMMIT ]]; then
  echo "missing wuffs_commit in ${MANIFEST} header" >&2
  exit 1
fi

while IFS=$'\t' read -r hash relpath basename; do
  [[ -z $hash || $hash == \#* ]] && continue
  dest="$ROOT/testdata/bench/$relpath"
  mkdir -p "$(dirname "$dest")"
  url="https://raw.githubusercontent.com/google/wuffs/${WUFFS_COMMIT}/test/data/${basename}"
  echo "fetch $relpath"
  curl -fsSL "$url" -o "$dest"
done <"$MANIFEST"

while IFS=$'\t' read -r hash relpath basename; do
  [[ -z $hash || $hash == \#* ]] && continue
  file="$ROOT/testdata/bench/$relpath"
  if ! echo "${hash}  ${file}" | sha256sum -c - >/dev/null; then
    echo "sha256 mismatch: $relpath" >&2
    exit 1
  fi
done <"$MANIFEST"

echo "bench fixtures OK (${MANIFEST})"
