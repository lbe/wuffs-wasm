# Package capability conversation (23 Aug 2026)

This is a record of the design conversation that defined the intended package
shape and call order. It is **not** an implementation spec and was not in the
repo until this file.

Source: Cursor session transcript, Sunday 23 August 2026. Tool calls and
skill dumps are omitted. Wording is the conversation as sent, not a rewrite.

On 25 August 2026 it was confirmed that the later “Step 1 / Step 3” numbering
existed only in this chat. What the repo implemented for Probe is
`(*Decoder).Probe` plus tests; `DecodeRGBA` still points `dst.Pix` at wasm
linear memory.

---

## User — 6:08 PM

This is too damn fragmented of an approach for me. That is my fault. Not yours.
Pause for a minute while I think

## Assistant

Understood. Take your time — I'll be here when you're ready.

---

## User — 6:11 PM

I need to understand what the full package capability will be. What types will
be support, their methods, functions ... What is the best way to go about this?
I don't really want to read through all of wuffs myself. Take a look through.
Is there what you believe to be an obvious solution here? Do not try to build
the design in one go. Do your investigation and then give me a high level
response back and we can work it from there.

## Assistant

There is an obvious way to think about this, and it is **not** “read Wuffs, then
list every C type.”

### The mistake to avoid

Wuffs does **not** give you a PNG type, a JPEG type, a GIF type. It gives
**one** `image_decoder` interface. Every format implements the same methods:

`decode_image_config` → `decode_frame_config` → `decode_frame` (repeat for
animation)

Formats are **inputs**, not types. If we design around “Plan 2 = QOI, then JPEG,
then GIF,” we will keep fragmenting the package.

That is already how this repo is built: one `Decoder`, one `DecodeRGBA`. The
wasm guest already compiles all Wuffs image codecs. What is missing is
**Go-side completeness**, not more format-specific types.

### What Wuffs actually is (capability envelope)

Wuffs-the-library is larger than this package:

| Domain                | In Wuffs                                                                         | In this package today                      |
| --------------------- | -------------------------------------------------------------------------------- | ------------------------------------------ |
| Image decode          | PNG, JPEG, GIF, WebP, BMP, TGA, QOI, Netpbm, WBMP, NIE, ETC2, ThumbHash, Handsum | Image-only; PNG + lossless WebP tested     |
| Compression           | gzip, zlib, deflate, bzip2, lzma, xz, …                                          | Out of scope (wired only as PNG/WebP deps) |
| Hashers / JSON / CBOR | Yes                                                                              | Out of scope                               |

For **this** Go module, the honest product is: **image decode, via Wuffs, in
Go**. Compression/hash/JSON would be a different package.

### The obvious Go mapping

Wuffs itself already split this problem:

- Low-level C API: full power, allocation-free, coroutines, quirks, metadata,
  animation.
- High-level C++ helper: `wuffs_aux::DecodeImage` — one call, one result. That
  is their “just decode the picture” API.

Go already has the same split:

- `image.Decode` / `image.DecodeConfig` — what almost every Go user wants.
- A reusable `Decoder` — for people who care about allocs, reuse, and large
  images (what you have now).

So the full package capability is **not** a zoo of types. It is a small Go
surface that sits on that one Wuffs decoder:

1. **Still-image decode into `image.Image`** (stdlib-shaped). That is the public
   product.
2. **Reusable `Decoder`** for reserve / retry / alloc control. That is the
   performance product. You already have this.
3. **Optional later, same type, more methods:** probe/config without pixels,
   animation frames, metadata (EXIF/ICC). Those are extra methods, not extra
   packages.

You do **not** need a type per format. You do **not** need to expose Wuffs’
`pixel_buffer`, `io_buffer`, quirks, or coroutines unless you later want a
“power user” subpackage.

### What “complete” would mean, at high level

If we froze the product as a Go image library:

- **In:** all Wuffs image formats, verified, through one decode path.
- **Types:** `Decoder`, `Meta`/`Config`, a handful of errors. Maybe `Frame`
  later for GIF/WebP animation.
- **Functions:** decode (and likely `DecodeConfig` / probe). Register with
  `image.RegisterFormat` so `image.Decode` works.
- **Output:** `image.RGBA` is a fine v1 default. Other `image.Image` kinds
  (`NRGBA`, gray, paletted) are optional, not a new architecture.
- **Out:** AVIF/TIFF/HEIC/JXL/SVG/PSD (Wuffs does not decode them). Multi-frame
  as a first-class animation player can wait. Metadata can wait. Compression
  codecs as a public API can wait.

That is the whole capability picture. Formats are a **test matrix**, not an API
matrix.

### Best way to proceed (process, not design)

Do **not** start by enumerating Wuffs C methods. Start by locking three
decisions, in this order:

1. **Product boundary** — this module is image decode only. Yes/no.
2. **User-facing shape** — stdlib-shaped (`Decode` / `DecodeConfig`) on top of
   the existing `Decoder`, vs keep `DecodeRGBA` as the only public API.
3. **v1 vs later** — still images + RGBA + no metadata is a complete v1;
   animation and metadata are the same `Decoder`, later methods.

Once those three are decided, types and methods fall out in a page, not a plan
with 13 cycles per format.

I have not designed the API. When you are ready, we can take those three
questions one at a time.

---

## User — 6:21 PM

1. Yes
2. I want to be able to decode to a pre-allocated variable. How does that
   affect this decision?
3. I would like to spell out the shape of the remaining methods, types if
   needed, though there definition can wait until later.

## Assistant

Pre-allocation does **not** kill a stdlib-shaped API. It **ranks** the APIs: the
decoder you already have is the real product; `image.Decode` is an optional
wrapper that allocates for people who do not care.

### How pre-allocation affects decision 2

Go’s `image.Decode` always allocates. There is no stdlib “decode into this
buffer.” That is the gap this package is for.

So the contract is:

| Path                                          | Who allocates                             | Role                                                      |
| --------------------------------------------- | ----------------------------------------- | --------------------------------------------------------- |
| `(*Decoder).DecodeRGBA(dst, src)`             | Caller (`dst`) + wasm slots via `Reserve` | **Primary.** This is the package.                         |
| Convenience `Decode` / `image.RegisterFormat` | Package                                   | Optional later. Implemented _on top of_ the primary path. |

What pre-allocation implies:

- Keep `[]byte` in, `*image.RGBA` out on the hot path. Wasm has to copy `src`
  into guest memory anyway; `io.Reader` would just add a buffer before that
  copy.
- A **probe / config** method becomes more important, not less. You need
  width/height (and format) _before_ you size `dst` and `Reserve`. Today that is
  approximated by empty `Rect` + `DstTooSmallError`. That works; it is a retry
  protocol, not a first-class probe.
- `image.RegisterFormat` can exist later as a thin allocating adapter. It must
  not become the core type system.

**Decision 2, restated:** primary API = reusable `Decoder` + pre-allocated
`dst`. Stdlib-shaped functions are convenience, not the architecture.

### Shape of the package (names can change; this is the inventory)

#### Already there (keep as the core)

```
func New() *Decoder

func (d *Decoder) DecodeRGBA(dst *image.RGBA, src []byte) (*Meta, error)
func (d *Decoder) Reserve(dstBytes, srcBytes int) error
func (d *Decoder) Version() string
func (d *Decoder) VersionNum() int32

type Meta struct { Width, Height, Stride, BytesWritten, ... }

var ErrUnknownFormat, ErrSrcTooLarge, ErrDecode error
type DstTooSmallError struct { MinBytes, Width, Height, Stride uint32 }
```

`SlotLayout`, `MemoryLayout`, `ReadMeta` are runtime internals. They should not
grow into the public product surface.

#### Remaining methods / types (define later; this is the shape)

**1. Probe (still-image v1, or immediately after current `DecodeRGBA`)**

Needed so callers can allocate `dst` without guessing or relying on
`DstTooSmallError`.

```
func (d *Decoder) Config(src []byte) (Config, error)
```

`Config` is `image.Config` **or** a small package type with width, height, and
format name. No pixels. No animation. Same `Decoder`, same `src`.

Package-level convenience, if you want it:

```
func DecodeConfig(src []byte) (Config, error)  // New + Config; allocates a Decoder
```

**2. Allocating convenience (optional; does not replace pre-alloc)**

```
func Decode(src []byte) (*image.RGBA, *Meta, error)
```

Internally: `New`, `Config` or retry, allocate `image.RGBA`, `DecodeRGBA`. For
users who do not pre-size. Not the hot path.

Stdlib adapter, if you ever want `image.Decode` to find this package:

```
func DecodeReader(r io.Reader) (image.Image, string, error)  // image.Decode signature
```

That one reads into a `[]byte` then calls the primary path. It **cannot** be
zero-alloc. That is fine; it is a different job.

**3. Animation (later, same `Decoder`)**

No per-format types. One extra result type, two methods:

```
type Frame struct {
    // Index, Delay, Bounds (and whatever Wuffs frame_config actually gives)
}

func (d *Decoder) FrameCount(src []byte) (int, error)
func (d *Decoder) DecodeFrame(dst *image.RGBA, src []byte, index int) (*Frame, error)
```

`DecodeRGBA` stays “frame 0.” GIF/WebP looping (`num_animation_loops`) can be a
field on `Config` or `Frame` when you get there. Random access maps to Wuffs
`restart_frame`; sequential `NextFrame` is an alternative spelling of the same
thing — pick one when you implement, not now.

**4. Metadata (later, same `Decoder`)**

```
type Metadata struct {
    // EXIF, ICC, XMP, gamma, chromaticities — raw blobs + a few parsed numbers
}

func (d *Decoder) Metadata(src []byte) (*Metadata, error)
```

Wuffs skips this unless you opt in. One method, one struct. Not a type per
FourCC.

**5. Other pixel types (later, same `Decoder`)**

Only if you outgrow “everything is `image.RGBA`”:

```
func (d *Decoder) Decode(dst image.Image, src []byte) (*Meta, error)
```

or a second method `DecodeNRGBA`. That is an output-format extension, not a
format matrix. v1 can stay `DecodeRGBA` only.

#### What this is _not_

- No `PNGDecoder` / `JPEGDecoder`.
- No public Wuffs `pixel_buffer` / `io_buffer` / quirk API unless you later want
  an `internal` or `advanced` escape hatch.
- No compression/hash types in this module.

#### How the pieces relate

```
src []byte
    │
    ├─ Config / DecodeConfig     → size dst + Reserve
    ├─ DecodeRGBA                → pixels into caller’s image.RGBA   [core, now]
    ├─ Decode / DecodeReader     → same, but package allocates       [optional]
    ├─ DecodeFrame / FrameCount  → animation                         [later]
    └─ Metadata                  → EXIF/ICC/etc.                     [later]
```

If you want a next step: we can pin **only** the probe (`Config`) signature
against the current `DstTooSmallError` retry path — that is the one remaining
hole in the pre-alloc story. Everything else can stay names-on-a-list until you
need it.

---

## User — 8:57 PM

I think we should include Probe in the plan. It is logically the first thing to
call.

## Assistant

**Yes. Probe is in, and it is the first call on the happy path.**

That is the pre-alloc sequence, not a convenience extra:

```
Probe(src)  →  size dst + Reserve  →  DecodeRGBA(dst, src)
```

`DecodeRGBA` with an empty `Rect` and `DstTooSmallError` retry stays as a
fallback, not the intended API. Callers who already know dimensions can skip
Probe; everyone else starts there.

### What Probe is

Same `Decoder`, same `[]byte` `src`, **no pixel write**.

It is Wuffs `decode_image_config`: format + overall width/height (and enough to
size the destination). It is not frame pixels, not animation iteration, not
EXIF.

Return value can stay close to `Meta` (width, height, stride, and a format
identifier). A separate `Config` type is optional; I would not add one unless
`Meta` would get overloaded. Names later — the job is: **enough to allocate
`dst` and call `Reserve`.**

```
func (d *Decoder) Probe(src []byte) (*Meta, error)
```

Package-level `Probe(src)` can wait; the method on `Decoder` is the one that
matters (reuse, wasm instance).

### Guest implication (so the plan is honest)

Today the wasm export is decode-to-pixels only. Dimensions on undersize is a
side effect of that. A real Probe needs a **config-only guest export** (or the
existing decode to stop after image config). Otherwise Probe either costs a full
decode or stays a fake retry.

So Probe is: Go method first in the call order, plus a small shim/export so it
does not decompress the frame.

### Still later, unchanged

`DecodeFrame`, `Metadata`, allocating `Decode` / `DecodeReader` stay on the
list. They are not required to make Probe the first call.

If you want this written into a Plan 2 (or a short API sketch in `docs/`), say
so and we can do that next — Probe as cycle/step 1 of the public sequence,
formats still a test matrix behind the same `Decoder`.

---

## User — 9:28 PM

Give me the bulletted version first.

## Assistant

**Product**

- Image decode only (no compression/hash/JSON as a public API)
- One `Decoder` for all formats; formats are inputs, not types
- Primary path is pre-allocated destination, not `image.Decode`

**Call order**

- `Probe(src)` — first call; format + dimensions; no pixels
- Caller sizes `dst` and `Reserve`
- `DecodeRGBA(dst, src)` — pixels into caller’s `image.RGBA`
- Skip `Probe` only if dimensions are already known
- Empty `Rect` + `DstTooSmallError` retry is fallback, not the intended flow

**Now (keep)**

- `New()`
- `(*Decoder).DecodeRGBA(dst, src)`
- `(*Decoder).Reserve(dstBytes, srcBytes)`
- `(*Decoder).Version` / `VersionNum()`
- `Meta`
- `ErrUnknownFormat`, `ErrSrcTooLarge`, `ErrDecode`, `*DstTooSmallError`

**Add next**

- `(*Decoder).Probe(src) (*Meta, error)` — Wuffs `decode_image_config`
- Config-only wasm export so Probe does not decompress the frame
- `Meta` grows a format identifier if needed (no extra type unless `Meta` gets
  messy)

**Later, same `Decoder`**

- Allocating convenience: `Decode(src)` (and optionally `DecodeReader` /
  `image.RegisterFormat`)
- Animation: `FrameCount`, `DecodeFrame`, a `Frame` struct; `DecodeRGBA` stays
  frame 0
- Metadata: one `Metadata` struct + `(*Decoder).Metadata(src)`
- Other pixel types (`NRGBA`, gray, …) only if RGBA is no longer enough

**Not in this package**

- Per-format types (`PNGDecoder`, …)
- Public Wuffs `pixel_buffer` / `io_buffer` / quirk API
- AVIF, TIFF, HEIC, JPEG XL, SVG, PSD
- Compression/hash codecs as a public API

**Internals (not product surface)**

- `SlotLayout`, `MemoryLayout`, `ReadMeta`

This **Call order** section is the source of the later “Step 1 / Step 3”
numbering.

---

## User — 10:08 PM

Do you recommend a TDD plan?

## Assistant

**Yes — for Probe. Not for the whole remaining product.**

Probe is a real behavior change (new Go method + config-only wasm export). That
is what TDD plans are for: red test first, then guest + host until `Probe` then
`DecodeRGBA` is green.

Do **not** put JPEG/GIF/BMP/QOI, animation, metadata, and `image.RegisterFormat`
in the same plan. That is how Plan 1 vs Plan 2 felt fragmented. Those are a
**fixture matrix** and later plans.

**Recommended cut**

- **One TDD plan:** `Probe` + guest config export + `Probe` then `Reserve` then
  `DecodeRGBA` on an existing fixture (PNG is enough). Keep `DecodeRGBA`
  working. `DstTooSmallError` retry remains fallback.
- **Not in that plan:** other formats (add tests as you verify, same
  `Decoder`), `Frame` / `Metadata`, allocating `Decode`.

Write that plan when you want to implement Probe. Until then the bulleted API
list is enough.

---

TDD planning for `feat-probe-add` started after this (clarifying questions,
plan file, review). That work is not part of this record.
