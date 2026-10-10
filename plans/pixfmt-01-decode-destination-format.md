# PIXFMT-01 — Guest destination pixel format and host row copy

**Execution protocol:** `plans/pi-runbook.md`

**Execution branch:** `pixfmt-01-decode-destination-format`

**Progress file:** `tmp/pi_progress.md`

**Commit authorized:** Yes — one reviewed commit per committing task

**Roadmap:** None — do **not** edit `plans/api-roadmap.md`.

**Purpose:** Fix `DecodeRGBA` / `DecodeFrame` output semantics (alpha-premultiplied
`image.RGBA`), decode `DecodeNRGBA` as straight `RGBA_NONPREMUL` in the guest,
and replace host BGRA shuffle/unpremultiply with stride-aware row `copy` for
RGBA and NRGBA. Keep `DecodeGray` on guest `BGRA_PREMUL` with the existing gray
conversion kernel.

**Background:** `/tmp/thoughts.md` and `/tmp/cpu-harvesters-wuffs-prof-summary.md`
(profile evidence). SIMD/asm on `convertBGRAToRGBARegion` is out of scope.

---

## Sources of truth

1. `AGENTS.md`
2. The user's latest instruction
3. `plans/pi-runbook.md`
4. This plan
5. `GOALS.md`
6. `API.md` — contract must match Go `image` semantics after this plan
7. `HANDOVER.md` — caller-owned `Pix`; guest is scratch only

Go references:

- `image.RGBA` / `color.RGBA`: alpha-premultiplied channel values in `Pix`.
- `image.NRGBA`: straight (non-premultiplied) RGBA bytes in `Pix`.

---

## Product invariants (unchanged)

- No CGO. Caller owns `Pix` / `Stride` / `Rect`. Zero heap allocs per successful
  decode after `Reserve` on existing allocation tests.
- Do not hand-edit `internal/wuffswasm/wuffs.go`.
- Guest/bindings change only in tasks that run `make generate`.
- `Probe` still reports `Stride == Width*4` and does not decode pixels.
- `DecodeGray` still uses four-byte guest scratch; `Meta.Stride` remains `Width*4`.

---

## Target behavior (fixed)

| Host API | Guest `pixel_config` (Wuffs) | Host transfer |
| -------- | ------------------------------ | ------------- |
| `DecodeRGBA`, `DecodeFrame` | `WUFFS_BASE__PIXEL_FORMAT__RGBA_PREMUL` | `copy` each row from scratch sub-rectangle into `dst.Pix` |
| `DecodeNRGBA` | `WUFFS_BASE__PIXEL_FORMAT__RGBA_NONPREMUL` | `copy` each full-canvas row into `dst.Pix` |
| `DecodeGray` | `WUFFS_BASE__PIXEL_FORMAT__BGRA_PREMUL` | existing `convertBGRAToGray` (unpremul only inside gray kernel) |

Remove `convertBGRAToRGBARegion`, `convertBGRAToStraight`, and
`convertBGRAToNRGBA` from the RGBA/NRGBA decode paths. Delete dead helpers when
no references remain.

**Oracle rule for tests (fixed):**

- **DecodeRGBA / `DecodeFrame`:** `png.Decode` → `draw.Draw` into
  `image.NewRGBA` with `draw.Src`; compare `Pix` to `DecodeRGBA` output (tight
  stride).
- **DecodeNRGBA:** `png.Decode` into `*image.NRGBA` via
  `img, err := png.Decode(...)` then `nrgba, ok := img.(*image.NRGBA)`; if
  `!ok`, STOP and notify the user (fixture must be NRGBA-native). Compare `Pix`
  to `DecodeNRGBA` output.

---

## Guest ABI (fixed)

Add `int32_t dst_pixfmt` as the **last** parameter on:

- `wuffs_decode_image`
- `wuffs_decode_frame`

Numeric codes (defined in `wasm/shim.c`; mirrored as `const` in `decoder.go`):

| Code | Constant name (Go + C) | Wuffs format |
| ---- | ---------------------- | ------------ |
| `1` | `guestDstPixfmtRGBAPremul` | `WUFFS_BASE__PIXEL_FORMAT__RGBA_PREMUL` |
| `2` | `guestDstPixfmtRGBANonPremul` | `WUFFS_BASE__PIXEL_FORMAT__RGBA_NONPREMUL` |
| `3` | `guestDstPixfmtBGRAPremul` | `WUFFS_BASE__PIXEL_FORMAT__BGRA_PREMUL` |

Any other value → `WUFFS_WASM_ERR_BAD_ARG` on both meta slots (`decode_frame`)
or meta (`decode_image`).

Refactor `decode_image` and `decode_frame` to call one shared helper that maps
`dst_pixfmt` to `wuffs_base__make_pixel_format` and applies `pixel_config__set`.

`probe_image` stays on `BGRA_PREMUL` override for dimensions only.

### `wasm/wuffs_config.h` (fixed)

Replace the single allow with:

```c
#define WUFFS_CONFIG__DST_PIXEL_FORMAT__ENABLE_ALLOWLIST
#define WUFFS_CONFIG__DST_PIXEL_FORMAT__ALLOW_RGBA_PREMUL
#define WUFFS_CONFIG__DST_PIXEL_FORMAT__ALLOW_RGBA_NONPREMUL
#define WUFFS_CONFIG__DST_PIXEL_FORMAT__ALLOW_BGRA_PREMUL
```

---

## Host copy helpers (fixed)

Add to `convert.go`:

- `copyScratchToRGBARegion(dst []byte, dstStride int, src []byte, canvasWidth int, bounds image.Rectangle)`
- `copyScratchToNRGBA(dst []byte, dstStride int, src []byte, width, height int)`

Each row: `copy(dstRow, srcRow)` where row length is `(bounds.Dx())*4` for
region, `width*4` for NRGBA. No channel swap when guest emits RGBA layouts.

Add `TestUnpremultiplyGrayTable` in `convert_test.go`: exhaustive `a` in
`1..255`, `c` in `0..255`, compare `unpremultiply(c,a)` to `(c*255)/a` (keep
`unpremultiply` for gray path only).

---

## Executor STOP gates (mandatory)

**Global rules:** `plans/pi-runbook.md` standing constraint 13; **Plan executor STOP (zero deviation)** in the same runbook.

Executors **must not** edit this plan file. On any trigger below (or any global rule): **STOP**, message the user (task id, evidence), **no commit**, **no next task** until written user direction.

| # | Trigger |
| - | ------- |
| 1 | Hand-edit `internal/wuffswasm/wuffs.go` |
| 2 | `make generate` on a task that does not authorize it |
| 3 | Skip or weaken a failing correctness test to force green |
| 4 | Change outside the current task allowed-file list |
| 5 | Wuffs build fails: `RGBA_PREMUL` / `RGBA_NONPREMUL` not in allowlist or unknown at link time |
| 6 | `git add` under `testdata/bench/m/` or `testdata/bench/l/` |
| 7 | Copy fixtures from `../wuffs/` or any local checkout |
| 8 | `png.Decode` of `testdata/alpha-pixels.png` is not `*image.NRGBA` |
| 9 | Task 6 profile step: `make bench-fixtures` not run and L-tier PNG missing |

---

## Preflight

Branch hygiene only.

### Worker instructions

1. If `git status --short` is non-empty → STOP.
2. If current branch is `main` → `git switch -c pixfmt-01-decode-destination-format`.
3. If current branch is `pixfmt-01-decode-destination-format` → continue.
4. If current branch is any other name → STOP.
5. Create `tmp/pi_progress.md` with `BASELINE_HEAD=$(git rev-parse HEAD)`.

### Allowed files

- `tmp/pi_progress.md`
- `tmp/pixfmt-01-decode-destination-format/` (logs — do not stage)

### Verify

```bash
git branch --show-current
git status --short
git diff --name-only
test -f plans/pixfmt-01-decode-destination-format.md
```

Expected branch: `pixfmt-01-decode-destination-format`. Expected `git status --short`: empty.

Commit authorized: **no**.

---

## Task 1 — Guest shim, host copy path, RGBA premul correctness

**Prerequisite:** Preflight complete; `plans/pixfmt-01-decode-destination-format.md`
committed and present on the branch.

Task 1 must not commit until RGBA premul integration tests pass (same task).

### Allowed files

- `wasm/shim.c`
- `wasm/wuffs_config.h`
- `wasm/wuffs.wasm` (generated)
- `internal/wuffswasm/wuffs.go` (generated only via `make generate`)
- `decoder.go`
- `convert.go`
- `convert_test.go`
- `decode_rgba_premul_integration_test.go` (new, `package wuffs_test`)
- `docs/DEVELOPMENT.md` (guest export signature paragraph only)

### Worker instructions

1. Implement **Guest ABI** and **Target behavior** in `wasm/shim.c` and
   `wasm/wuffs_config.h`.
2. Run `make generate` from repo root. Do not edit `internal/wuffswasm/wuffs.go`
   by hand.
3. Add Go guest pixfmt constants in `decoder.go` and pass them from:
   - `decodeImage` → `Xwuffs_decode_image(..., dstPixfmt)`
   - `DecodeFrame` → `Xwuffs_decode_frame(..., dstPixfmt)` with RGBA premul code
   - `decodeImageForDestination` / `DecodeNRGBA` with non-premul code
   - `DecodeGray` path with BGRA premul code `3`
4. Replace RGBA/NRGBA conversion calls with **Host copy helpers**.
5. Update `convert_test.go`: remove cases that assert BGRA→straight for NRGBA;
   add unit tests for row `copy` with padded stride on synthetic RGBA scratch bytes.
6. Add `TestUnpremultiplyGrayTable` per plan.
7. Add `TestIntegrationDecodeRGBA_AlphaPixelsMatchesStdlibPremul` and
   `TestIntegrationDecodeRGBA_DrawCompositesTranslucentPixel` per **Oracle rule**
   using `testdata/alpha-pixels.png`.
8. Update `docs/DEVELOPMENT.md` export list to show the new `dst_pixfmt` argument
   on `wuffs_decode_image` and `wuffs_decode_frame`.
9. Run `make format`.

### Verify

```bash
make generate
go build ./...
rg 'func \(m \*Module\) Xwuffs_decode_image\(v0, v1, v2, v3, v4, v5 int32\)' internal/wuffswasm/wuffs.go
rg 'func \(m \*Module\) Xwuffs_decode_frame\(v0, v1, v2, v3, v4, v5, v6, v7 int32\)' internal/wuffswasm/wuffs.go
go test -run='^TestConvert|^TestUnpremultiplyGrayTable$' -count=1 .
go test -run='^TestIntegrationDecodeRGBA_AlphaPixelsMatchesStdlibPremul$|^TestIntegrationDecodeRGBA_DrawCompositesTranslucentPixel$' -count=1 .
make format-check
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`.

Commit authorized: **yes**.

---

## Task 2 — NRGBA stdlib oracles and WebP alpha spot check

**Prerequisite:** Task 1 committed.

### Allowed files

- `decode_nrgba_integration_test.go`
- `decoder_nrgba_integration_test.go`
- `decode_rgba_premul_integration_test.go` (WebP test only)

### Worker instructions

1. Update NRGBA integration tests to compare `DecodeNRGBA` and package
   `DecodeNRGBA` against **Oracle rule** NRGBA bytes on `alpha-pixels.png`
   (replace hand-picked byte expectations that assumed BGRA premul + host
   unpremul).
2. Add `TestIntegrationDecodeRGBA_WEBP_AlphaMatchesStdlibPremul` in
   `decode_rgba_premul_integration_test.go`: same oracle pattern as PNG on
   `testdata/bricks-color.lossless.webp` (160×120).
3. Keep existing dimension, allocation, and error-matrix tests; do not delete
   coverage.

### Verify

```bash
go test -run='^TestIntegrationDecodeNRGBA' -count=1 .
go test -run='^TestIntegrationDecodeRGBA_WEBP_AlphaMatchesStdlibPremul$' -count=1 .
make format-check
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`.

Commit authorized: **yes**.

---

## Task 3 — Golden CRCs, format manifests, animation characterization

**Prerequisite:** Task 2 committed.

`DecodeRGBA` / `DecodeFrame` now emit **premultiplied** `Pix`; every pinned
CRC that assumed straight RGBA must be recomputed from library decode output.

### Allowed files

- `decoder_test.go`
- `scripts/gen_golden.go` (comments and regeneration only)
- `testdata/bricks-color.golden.crc32`
- `decode_frame_integration_test.go`
- `decode_nie_integration_test.go`
- `decode_hnsm_integration_test.go`
- `decode_etc2_integration_test.go`
- `decode_th_integration_test.go`
- `animation_nie_integration_test.go`
- `animation_gif_integration_test.go`
- `testdata/README`
- `testdata/nie.golden.manifest`
- `testdata/hnsm.golden.manifest`
- `testdata/etc2.golden.manifest`
- `testdata/th.golden.manifest`
- `testdata/nie.animation.golden.manifest`
- `testdata/gif.animation.golden.manifest`

### Worker instructions

1. Run `go run scripts/gen_golden.go`. Stage `testdata/bricks-color.golden.crc32`
   when `git diff --name-only` lists that file after the run.
2. Update golden-manifest **comment** lines to state premultiplied `image.RGBA`
   `Pix` CRCs and guest `RGBA_PREMUL` + host row `copy`.
3. Recompute and update every **numeric CRC line** in the allowed `*.golden.manifest`
   files using `DecodeRGBA` or `DecodeFrame` on the vendored fixture named in
   each manifest row (same geometry as today). Update
   `nieFrameZeroCanonicalCRC` in `animation_nie_integration_test.go` when the
   nie-bn4 frame-zero CRC changes.
4. Update NIE/HNSM/ETC2/TH inline oracle builders so guest-scratch simulation
   uses `RGBA_PREMUL` byte order (R,G,B,A); remove BGRA swap and host unpremul
   from those builders. Characterization tests must still pass using the updated
   manifest CRCs from step 3.
5. Align `decode_frame_integration_test.go` comments and assertions with premul
   `Pix` per Task 1 **Oracle rule**.

### Verify

```bash
go run scripts/gen_golden.go
go test -run='^TestIntegrationDecodeRGBA_PNGGolden$' -count=1 .
go test -run='^TestIntegrationDecodeFrame' -count=1 .
go test -run='^TestIntegrationNIEDecodeCharacterization$|^TestIntegrationHNSMDecodeCharacterization$|^TestIntegrationETC2DecodeCharacterization$|^TestIntegrationTHDecodeCharacterization$' -count=1 .
go test -run='^TestIntegrationNIEAnimationCharacterization$|^TestIntegrationGIFAnimationCharacterization$' -count=1 .
make format-check
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`.

Commit authorized: **yes**.

---

## Task 4 — Public contract documentation

**Prerequisite:** Task 3 committed.

### Allowed files

- `API.md`
- `README.md` (decode semantics paragraph only if present)
- `decoder.go` (godoc on `DecodeRGBA`, `DecodeNRGBA`, `DecodeFrame` only)
- `doc.go` (package comment decode semantics only)
- `HANDOVER.md` (guest scratch layout sentences only)

### Worker instructions

1. `API.md`: `DecodeRGBA` / `DecodeFrame` output is **alpha-premultiplied**
   `image.RGBA` bytes. `DecodeNRGBA` output is **straight** NRGBA. Guest scratch
   format per path matches **Target behavior** table; host transfer is row `copy`
   for RGBA/NRGBA.
2. Remove every sentence claiming `DecodeRGBA` or `DecodeFrame` emits straight
   (non-premultiplied) color in `Pix`.
3. Align `decoder.go` and `doc.go` comments with `API.md`.
4. `HANDOVER.md`: note RGBA premul + guest format selection (no API shape change).

### Verify

```bash
make format-check
go build ./...
! rg -n 'straight \(non-premultiplied\).*DecodeRGBA|DecodeRGBA.*straight \(non-premultiplied\)' API.md decoder.go doc.go
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`.

Commit authorized: **yes**.

---

## Task 5 — Allocation guards

**Prerequisite:** Task 4 committed.

### Allowed files

- `decoder_test.go` (`TestIntegrationDecodeRGBA_AllocsPerRun` only)
- `decoder_nrgba_integration_test.go` (`TestIntegrationDecodeNRGBANoAllocs` only)
- `decode_gray_integration_test.go` (`TestIntegrationDecodeGrayNoAllocs` only)

### Worker instructions

1. Re-run allocation tests; fix regressions only in listed tests or production
   decode path (no new APIs).
2. Warm-up decode once before `testing.AllocsPerRun` when Task 1 changed guest
   entry signatures; target remains **0** allocs/op on success.

### Verify

```bash
go test -run='^TestIntegrationDecodeRGBA_AllocsPerRun$|^TestIntegrationDecodeNRGBANoAllocs$|^TestIntegrationDecodeGrayNoAllocs$' -count=1 -v .
make format-check
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`.

Commit authorized: **yes**.

---

## Task 6 — Final verification and profile note

**Prerequisite:** Task 5 committed.

### Worker instructions

1. Run every command in **Verify**; save combined output to
   `tmp/pixfmt-01-decode-destination-format/final-gates.log`.
2. Run `make bench-fixtures` when `testdata/bench/l/harvesters.png` is absent.
3. Run local profile (not in Verify; no pass/fail gate):
   `go test -cpuprofile=tmp/cpu-harvesters-wuffs.prof -bench=BenchmarkDecodeRGBA_PNG_Harvesters -benchtime=3x -run=^$ .`
   Write one line to `tmp/pixfmt-01-decode-destination-format/profile-note.txt`
   comparing `copyScratchToRGBARegion` / `convertBGRAToRGBARegion` flat CPU
   share to `/tmp/cpu-harvesters-wuffs-prof-summary.md` baseline (state "N/A" when
   baseline file missing).
4. Commit authorized: **no**.

### Verify

```bash
make format-check
make lint
make test
make test-race
go build ./...
git status --porcelain --untracked-files=all
```

FORBIDDEN before commit: N/A (no commit).

---

## Completion report

1. `git rev-parse HEAD` on `pixfmt-01-decode-destination-format`.
2. Confirmation that `DecodeRGBA` alpha fixture matches stdlib premul oracle.
3. `final-gates.log` path.
4. Whether `bricks-color.golden.crc32` changed (from Task 3 commit).
5. `profile-note.txt` path from Task 6.

---

## Execution prompt (for Cursor Agent)

```text
Execute plans/pixfmt-01-decode-destination-format.md in order:
Preflight → Task 1 → Task 2 → Task 3 → Task 4 → Task 5 → Task 6.
Follow plans/pi-runbook.md. Progress: tmp/pi_progress.md.
Task 1 must run make generate; never hand-edit internal/wuffswasm/wuffs.go.
Do not edit plans/api-roadmap.md. Do not push unless I instruct.
Begin with Preflight only on a clean main; ensure this plan file is committed before Task 1.
```
