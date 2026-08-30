# Handover — restore caller-owned `Pix`

**Do not invent a new product.** Read `GOALS.md` and `API.md` first. `API.md`
is the **complete** public contract. This file is only the first correction
needed to match it: `DecodeRGBA` must write into the caller’s `Pix`.

The 17 Aug goal (restated 25 Aug): populate an existing `image.RGBA`; do not
allocate or replace `Pix`. Execution aliased `dst.Pix` to wasm to pass an
alloc-ceiling test. That is the bug.

Ignore any “Step 1/2/3” numbering from prior agents. Probe and `Reserve` exist.
`DecodeRGBA` does not meet `API.md`.

**This pass implements the `DecodeRGBA` Pix contract only.** Do not build the
rest of `API.md` (`DecodeNRGBA`, `DecodeGray`, animation, `Metadata`, package
`Probe`/`Decode`, `RegisterFormats`, extra format constants) unless the owner
asks.

---

## What is wrong (code)

`decoder.go` `DecodeRGBA`, after a successful guest decode:

- Sets `dst.Rect` / `dst.Stride` from the guest
- **Assigns** `dst.Pix = memBytes[lay.DstOff : lay.DstOff+pixLen]`
- Converts BGRA→RGBA in place on that wasm slice

The host size check for a non-zero `Rect` does not keep the caller’s `Pix`.
Tests and benches pass `image.NewRGBA(image.Rect(0,0,0,0))` and let decode
invent `Pix`. `TestIntegrationDecodeRGBA_AllocsPerRun` encodes the cheat
(`allocCeiling = 1` = the empty struct).

`convert.go` already writes into a **separate** `dst` slice. No new wasm
export. Do not `make generate` for this.

---

## What “done” looks like

```go
dst := image.NewRGBA(image.Rect(0, 0, int(meta.Width), int(meta.Height)))
pix := dst.Pix
_, err := d.DecodeRGBA(dst, src)
// err == nil
// dst.Pix is still pix (same backing array, not a wasm subslice)
// pixels are straight RGBA; golden CRC still matches
// Probe/Reserve after this does not change those bytes
```

Reuse the same `dst` for many decodes of the same size with **zero** Go heap
allocs from `DecodeRGBA` (`AllocsPerRun` with `NewRGBA` **outside** the loop,
after `Reserve`).

Guest still needs a wasm dst slot (`Reserve`). That is scratch. Host `Pix` is
the product buffer. Two buffers during decode is required with the current
shim (full frame in guest, then convert out).

---

## Immediate code change

In `DecodeRGBA` success path:

1. Do **not** assign `dst.Pix`.
2. Do **not** overwrite `Rect`/`Stride` except to match a decode that the
   caller already sized correctly (or leave them unchanged if they already
   match).
3. Convert: `convertBGRAToRGBA(dst.Pix, dst.Stride, wasmBGRA, width, height)`
   where `wasmBGRA` is the guest dst slot.
4. Reject bad host `dst` per `API.md`:
   - Empty `Rect`, nil/empty `Pix`, or `Dx`/`Dy` that do not equal decoded
     width/height → `ErrBadImage`.
   - `Stride < width*4` or `len(Pix) < Stride*Dy` when dimensions match →
     `*DstTooSmallError` (`errors.Is(err, ErrDstTooSmall)`).
   - Do **not** invent a wasm-backed `Pix` to “help.”

Add `ErrBadImage` and `ErrDstTooSmall` as specified in `API.md` if they are
missing. Keep `checkSrcCapacity`, guest `wuffs_decode_image`, and existing
mapping for unknown format / guest-slot too small (that is guest scratch, not
host `Pix`).

Do not “dual path” (empty Rect aliases, sized Rect copies). That is how the
cheat survives.

---

## Tests (this is most of the work)

Rewrite every `image.NewRGBA(empty Rect)` success path:

- `decoder_test.go`: `decodePreallocatedRGBA` and all call sites; Probe then
  decode test; golden CRC; WEBP; alloc test; concurrency tests.
- `decoder_bench_test.go`: size `dst` once outside the timed loop; `Reserve`
  guest slot from known or probed size.
- `scripts/gen_golden.go`: same.

**Must assert** (or the cheat can return):

- `Pix` pointer/len/cap identity (or `unsafe.SliceData`) before vs after
  `DecodeRGBA`.
- CRC / non-zero pixels still match fixtures.
- Second decode into the **same** `dst` does not allocate (`AllocsPerRun == 0`
  after warmup `Reserve`).
- After decode, `Probe` or `Reserve` does not change the caller’s pixel bytes.

Empty-`Rect` as a **success** path must go away. Tests that shrink the **guest**
dst slot and expect `*DstTooSmallError` can stay; they must not fill host `Pix`
from wasm. Tests for a wrong-sized host `dst` should expect `ErrBadImage`.

---

## Docs

- `README.md` usage: Probe → `NewRGBA` from `Meta` → `Reserve` guest →
  `DecodeRGBA`. Remove “Pix aliases wasm” and “one alloc = the shell.”
- `docs/DEVELOPMENT.md`: alloc story is 0 on the hot path with reused `dst`,
  not 1 for an empty struct.
- Do not treat `docs/package-capability-conversation.md` as the spec. The
  contract is `API.md`. Goals (your words) are `GOALS.md`.

---

## Do not do in this correction

- `DecodeNRGBA`, `DecodeGray`, `FrameCount`, `LoopCount`, `DecodeFrame`,
  `Metadata`
- Package-level `Probe` / `Decode` / `DecodeReader` / `RegisterFormats`
- Exporting the remaining format FourCC constants (needed for `API.md`, not
  for proving Pix ownership)
- Changing `wuffs_probe_image` / regenerating wasm unless a test proves a
  guest bug
- Re-litigating Plan 1 cycle count
- Numbering “steps”

---

## Remaining spec gap (not this pass)

`API.md` forbids `DecodeRGBA` / `Probe` from auto-growing guest memory;
the caller `Reserve`s. Today `DecodeRGBA` (and `Probe` for src) call
`Reserve` internally. That is a real gap against `API.md`. **Do not fix it in
this pass** unless the owner asks. Landing Pix does not require reverting
auto-grow.

---

## Proof command

```bash
make test
go test -run TestIntegrationDecodeRGBA_AllocsPerRun -v -count=1
```

Alloc test must fail until `dst` is reused and `DecodeRGBA` no longer
allocates `Pix`.
