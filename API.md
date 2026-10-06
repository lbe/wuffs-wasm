# API

Final public API for `github.com/lbe/wuffs-wasm`. This is the product
contract, not a snapshot of the tree.

Package `wuffs` is a Go image decoder. Wuffs runs as a wasm guest (wasm2go,
no CGO). One `Decoder` handles every Wuffs image format. Formats are inputs
(`Meta.Format`), not types.

Primary job: decode into a caller-owned `image.RGBA` (or other `image` buffer
below) without allocating or replacing `Pix`.

---

## Constraints

- No CGO. Consumer types live in the root package.
- Hot path takes `[]byte` source, not `io.Reader`.
- `New()` does not reserve 50MP (or any large cap). Guest memory grows with
  `Reserve`.
- Wuffs rejects width or height above `0xFFFFFF` (16_777_215). This package
  does the same.
- A `Decoder` is not safe for concurrent use. Create one per goroutine.
- Not in this package: per-format decoder types; Wuffs `pixel_buffer` /
  `io_buffer` / quirk APIs; compression/hash/JSON codecs; AVIF, TIFF, ICO,
  HEIC, JPEG XL, SVG, PSD.

---

## Call order

Happy path (still image or frame 0):

```go
d := wuffs.New()
meta, err := d.Probe(src)
if err != nil { /* ... */ }
if err := d.Reserve(int(meta.Stride)*int(meta.Height), len(src)); err != nil { /* ... */ }
dst := image.NewRGBA(image.Rect(0, 0, int(meta.Width), int(meta.Height)))
meta, err = d.DecodeRGBA(dst, src)
```

- `Probe` first unless width and height are already known.
- Caller allocates `dst` (and `Reserve`s guest scratch) from `Meta`.
- `DecodeRGBA` writes into that `Pix`. It does not allocate `Pix` and does not
  point `Pix` at wasm memory.
- Reuse `d` and `dst` for further decodes of the same size. After `Reserve` and
  with `dst` already sized, `DecodeRGBA` / `Probe` allocate zero Go heap
  objects on success.

Guest decode still uses a wasm pixel slot (scratch). Host `Pix` is the product
buffer. `Reserve` sizes the guest slot, not the Go image.

---

## Types

```go
type Decoder struct { /* unexported */ }

type Meta struct {
    Err          int32
    Width        uint32
    Height       uint32
    Stride       uint32 // bytes per row for this package’s dest layout (Width*4 for RGBA)
    BytesWritten uint32 // 0 after Probe; pixel bytes after a successful decode
    Format       uint32 // image FourCC (FormatPNG, FormatJPEG, …)
}

type Frame struct {
    Index       int             // 0-based; first frame is 0
    Bounds      image.Rectangle // frame rectangle inside the overall canvas (half-open)
    Duration    time.Duration   // 0 means still / display forever (Wuffs flicks → Duration)
    Disposal    Disposal
    Opaque      bool            // conservative: all pixels in Bounds are opaque
    Overwrite   bool            // true: replace canvas pixels; false: blend over
    Background  color.RGBA      // canvas background, straight RGBA
    IOPosition  uint64          // source offset of this frame config (Wuffs io_position)
}

type Disposal uint8

const (
    DisposalNone              Disposal = 0 // leave the frame; draw the next on top
    DisposalRestoreBackground Disposal = 1
    DisposalRestorePrevious   Disposal = 2
)

type Chromaticities struct {
    WhiteX, WhiteY float64
    RedX, RedY     float64
    GreenX, GreenY float64
    BlueX, BlueY   float64
}

type Metadata struct {
    Format uint32 // image FourCC of the file these blobs came from

    EXIF []byte // raw TIFF/EXIF payload; nil if absent
    ICC  []byte // raw ICC profile; nil if absent
    XMP  []byte // raw XMP; nil if absent

    HasGamma bool
    Gamma    float64 // file gamma when HasGamma

    HasChromaticities bool
    Chromaticities    Chromaticities

    HasSRGB   bool
    SRGB      uint32 // Wuffs SRGB rendering-intent FourCC payload as reported

    HasModTime bool
    ModTime    time.Time // from MTIM when present
}

type DstTooSmallError struct {
    MinBytes uint32 // required len(Pix) for Stride*Height
    Width    uint32
    Height   uint32
    Stride   uint32
}

func (e *DstTooSmallError) Error() string
func (e *DstTooSmallError) Is(target error) bool // true if target == ErrDstTooSmall
```

`Duration` is converted from Wuffs flicks (1 flick = 1/705_600_000 s).

---

## Constants

Image format FourCCs (`Meta.Format`). Values are `WUFFS_BASE__FOURCC__*`.

```go
const (
	FormatBMP   uint32 = 0x424D5020 // "BMP "
	FormatETC2  uint32 = 0x45544332 // "ETC2"
	FormatGIF   uint32 = 0x47494620 // "GIF "
	FormatHNSM  uint32 = 0x484E534D // "HNSM"
	FormatJPEG  uint32 = 0x4A504547 // "JPEG"
	FormatNIE   uint32 = 0x4E494520 // "NIE "
	FormatNPBM  uint32 = 0x4E50424D // "NPBM"
	FormatPNG   uint32 = 0x504E4720 // "PNG "
	FormatQOI   uint32 = 0x514F4920 // "QOI "
	FormatTGA   uint32 = 0x54474120 // "TGA "
	FormatTH    uint32 = 0x54482020 // "TH  "
	FormatWBMP  uint32 = 0x57424D50 // "WBMP"
	FormatWEBP  uint32 = 0x57454250 // "WEBP"
)
```

Metadata FourCCs (for `Metadata` and any future opt-in). Not image types.

```go
const (
    MetaEXIF uint32 = 0x45584946 // "EXIF"
    MetaICCP uint32 = 0x49434350 // "ICCP"
    MetaXMP  uint32 = 0x584D5020 // "XMP "
    MetaGAMA uint32 = 0x47414D41 // "GAMA"
    MetaCHRM uint32 = 0x4348524D // "CHRM"
    MetaSRGB uint32 = 0x53524742 // "SRGB"
    MetaMTIM uint32 = 0x4D54494D // "MTIM"
)
```

---

## Errors

```go
var (
    ErrUnknownFormat error // source is not a recognized image header
    ErrSrcTooLarge   error // len(src) exceeds the decoder’s src-slot cap; Reserve first
    ErrDecode        error // decode/probe failed (corrupt, truncated payload, bad args)
    ErrDstTooSmall   error // sentinel; also *DstTooSmallError
    ErrBadImage      error // dst Rect/Stride/Pix do not match the decoded image
)
```

`errors.Is(err, ErrDstTooSmall)` is true for `*DstTooSmallError`.

`ErrBadImage` is returned when `dst` is non-empty but `Dx`/`Dy`/`Stride`/`Pix`
do not match the decoded canvas (wrong size, not merely “too small to hold
it”).

---

## Decoder

```go
func New() *Decoder

func (d *Decoder) Version() string    // "major.minor.patch"
func (d *Decoder) VersionNum() int32  // major<<16 | minor<<8 | patch
```

### Probe

```go
func (d *Decoder) Probe(src []byte) (*Meta, error)
```

Sniff format and decode image config. No pixels. `BytesWritten` is 0.
`Stride` is `Width*4` (RGBA dest layout). Does not grow the guest pixel slot.
Does not touch any `image.RGBA`.

`len(src) > srcCap` → `ErrSrcTooLarge`. Empty `src` → `ErrDecode`.
Unrecognized header → `ErrUnknownFormat`.

### Reserve

```go
func (d *Decoder) Reserve(dstBytes, srcBytes int) error
```

Grows wasm src and dst **scratch** slots. Call before `Probe` if `src` exceeds
the default src cap (64 KiB). Call before decode with
`dstBytes >= int(meta.Stride)*int(meta.Height)`.

This is the only intended allocation point besides `New()`.

`DecodeRGBA` / `DecodeFrame` / `Probe` do not auto-grow guest memory past
what `Reserve` (or `New` defaults) already provided. Insufficient guest dst
slot → `*DstTooSmallError` with dimensions filled in.

### Decode still image / frame 0

```go
func (d *Decoder) DecodeRGBA(dst *image.RGBA, src []byte) (*Meta, error)
func (d *Decoder) DecodeNRGBA(dst *image.NRGBA, src []byte) (*Meta, error)
func (d *Decoder) DecodeGray(dst *image.Gray, src []byte) (*Meta, error)
```

`DecodeRGBA` is the required minimum. `DecodeNRGBA` and `DecodeGray` use the
same contract on their respective `Pix`/`Stride`/`Rect`.

Contract for all three:

- Caller has set `Rect`, `Stride`, and `Pix` to the canvas size (`Probe`
  `Width`×`Height`, or already known).
- `Rect.Min` is `(0,0)`. `Rect.Dx()` and `Rect.Dy()` equal decoded width and
  height. `Stride >= Dx()*bytesPerPixel`. `len(Pix) >= Stride*Dy()`.
- On success, pixels are written into **that** `Pix`. The library never
  allocates `Pix` and never replaces the `Pix` slice header with wasm memory.
- After return, `Pix` is still the caller’s backing array. Later `Probe`,
  `Reserve`, or decode on this `Decoder` must not change those bytes.
- Empty `Rect` or nil/empty `Pix` is an error (`ErrBadImage` or
  `*DstTooSmallError`), not a cue to invent a buffer.
- Output is straight (non-premultiplied) color. Guest scratch is BGRA premul
  for RGBA/NRGBA; converted into `dst.Pix`.
- `DecodeRGBA` is frame 0 of an animation. Overall canvas size, not the first
  frame’s dirty rect, unless they coincide.

Returned `*Meta` matches `Probe` for the same `src`, with `BytesWritten` set
to the number of pixel bytes written into `Pix`.

For `DecodeGray`, `Pix` contains one byte per pixel whose values follow
`color.GrayModel`, and `BytesWritten` is `Width*Height`. `Meta.Stride` still
reports the guest BGRA scratch stride, `Width*4`, because guest reservation
and metadata use the common decode path; the caller-owned Gray stride is
validated and used only for one-byte host output.

### Animation

```go
func (d *Decoder) FrameCount(src []byte) (int, error)
func (d *Decoder) LoopCount(src []byte) (uint32, error)
func (d *Decoder) DecodeFrame(dst *image.RGBA, src []byte, index int) (*Frame, error)
```

`FrameCount` walks frame configs only (no pixel decompress). Still images
return 1.

`LoopCount` is Wuffs `num_animation_loops`. `0` means loop forever when the
file is animated; still images return `0`.

`DecodeFrame`:

- `index` is `0 .. FrameCount-1`. Out of range → `ErrDecode`.
- `dst` is the **overall canvas** (`Probe` width×height), same Pix contract as
  `DecodeRGBA`.
- Pixels for that frame are written into `dst` at `Frame.Bounds`. The method
  does not clear the rest of `Pix`; the caller composites using `Disposal`,
  `Overwrite`, and `Background` if they need a full composed canvas.
- `DecodeRGBA(dst, src)` is defined as `DecodeFrame(dst, src, 0)` ignoring the
  `Frame` value except for filling `Meta`.

### Metadata

```go
func (d *Decoder) Metadata(src []byte) (*Metadata, error)
```

Opt-in read of EXIF, ICC, XMP, gamma, chromaticities, sRGB, modification time.
Absent fields are zero / nil. Does not decode pixels. Same `src` cap rules as
`Probe`.

---

## Package-level convenience (allocating)

These create a `Decoder` internally, allocate destination images, and are not
the hot path.

```go
func Probe(src []byte) (*Meta, error)

func Decode(src []byte) (*image.RGBA, *Meta, error)
func DecodeNRGBA(src []byte) (*image.NRGBA, *Meta, error)
func DecodeGray(src []byte) (*image.Gray, *Meta, error)

func DecodeConfig(src []byte) (image.Config, error)
```

`Decode` is `New` + `Reserve(0, len(src))` + `Probe` + validated geometry +
`Reserve(guestLen, len(src))` + `image.NewRGBA` + `DecodeRGBA`. It
allocates a temporary decoder and tightly packed `*image.RGBA`, then returns a
detached `*Meta` that does not alias the decoder's internal state. On any
failure both the image and Meta return values are `nil`. `DecodeNRGBA` mirrors
`Decode` exactly but allocates a tightly packed `*image.NRGBA` via
`image.NewNRGBA` and decodes with `DecodeNRGBA`. `DecodeGray` mirrors
`Decode` but allocates a tightly packed `*image.Gray` via `image.NewGray`
and decodes with `DecodeGray`. The guest scratch slot remains four bytes per
pixel (BGRA), so `Meta.Stride` retains the guest stride (`Width*4`), while
`Meta.BytesWritten` reports the host bytes written (`Width*Height`). On any
failure both the image and Meta return values are `nil`. Automatic source
reservation (`Reserve(0, len(src))`) precedes probing so the source slot fits
`src` before any guest call; automatic destination reservation follows
validated geometry and precedes host allocation.

`DecodeConfig` returns `image.Config{ColorModel: color.RGBAModel, Width, Height}`
from the same internal probe path — no pixels are decoded, no destination is
allocated. Each call builds its own temporary decoder, automatically reserves
the source slot to fit `src` with `Reserve(0, len(src))`, and returns only the
image configuration. The `ColorModel` is always `color.RGBAModel` for compatible
Wuffs input (the package's default decode target). On failure `DecodeConfig`
returns a zero-value `image.Config` and the error.

`Probe` (package function) is `New` + `Reserve(0, len(src))` +
`(*Decoder).Probe`. It sniffs format and decodes image config without touching
pixels or allocating a destination. Each call builds its own temporary decoder,
grows the source slot to fit `src`, and returns a detached `*Meta` that does not
alias the decoder's internal state. The `Decoder.Probe` method is the zero-alloc
reusable path.

### stdlib `image` adapter

```go
func DecodeReader(r io.Reader) (image.Image, string, error)
```

`DecodeReader` reads `r` fully to EOF into a `[]byte`, then delegates to the
allocating `Decode` helper. It returns a newly allocated `*image.RGBA` as an
`image.Image`, together with the canonical format name (`"png"`, `"webp"`,
`"bmp"`, `"gif"`, `"jpeg"`, `"npbm"`, `"qoi"`, `"tga"`, `"wbmp"`, `"etc2"`,
`"hnsm"`, `"nie"`, or `"th"`) for the verified formats. Reader errors and
decode errors retain their exact identity; failures return a nil image and an
empty format. Full buffering and image allocation are intentional. The adapter
does not register formats with the standard library's global image decoder
registry.

```go
func DecodeConfigReader(r io.Reader) (image.Config, error)
```

`DecodeConfigReader` reads `r` fully to EOF into a `[]byte`, then applies the
same configuration-only decode as `DecodeConfig`. It returns
`image.Config{ColorModel: color.RGBAModel, Width, Height}` on success. Reader
and decode errors retain their exact identity; failures return a zero
`image.Config`. Full buffering is intentional, and the adapter does not
register formats with the standard library's global image decoder registry.

### stdlib `image` registration

```go
func RegisterFormats()
```

`RegisterFormats` is explicit opt-in registration of exactly the thirteen
verified formats with the standard library's global image decoder registry,
so image.Decode and image.DecodeConfig recognize them. It registers each
format only once via sync.Once; it is safe for concurrent use. The global
image registry is first-match-wins, so earlier registrations remain
authoritative. The TGA entries use exact-header 18-byte magics and the WBMP
entries use literal-prefix magics; both keep their intentional
prefix-recognition false-positive boundaries.

```go
wuffs.RegisterFormats()
img, format, err := image.Decode(f)
cfg, err := image.DecodeConfig(f)
```

### Verified format subsets

The verified thirteen-format set is PNG, WebP, BMP, GIF, JPEG, NPBM, QOI, TGA,
WBMP, ETC2, HNSM, NIE, and TH. Four formats are verified on documented scope:

- **HNSM** is Handsum; **NIE** covers still nïE images (still images, frame zero) plus verified nïA multi-frame animation.
- **TH** supports only the cooked C3 BE FE form while raw ThumbHash is unsupported.
- **ETC2** covers the PKM fixtures in `testdata/` (ETC1, ETC1S, ETC2 RGB,
  non-multiple-of-four geometry, one-bit alpha, and full non-premultiplied
  alpha).

Three formats are verified on documented subsets:

- **NPBM** supports binary PGM P5 and PPM P6 with Maxval exactly 255 or 65535;
  all other maxima are unsupported.
- **TGA** supports indexed types 1 and 9 with 8-bit indices and 15-, 24-, or 32-bit
  palettes, true-color types 2 and 10 at 15, 16, 24, or 32 bits, and
  grayscale types 3 and 11 at 8 bits. Indexed palettes must start at entry zero
  and contain 1 to 256 entries; true-color and grayscale images must have no color map;
  interleaving and right-to-left origins are not permitted; either
  vertical origin is supported; attribute bits are 0 for indexed, 15-, 16-,
  and 24-bit true-color and 8-bit grayscale, and 8 for 32-bit true-color.
  Indexed 16-bit palettes and 15- or 16-bit true-color with one attribute bit
  are unsupported.
- **WBMP** supports Type 0 with canonical shortest-form dimension encodings
  and nonzero dimensions no greater than 0xFFFFFF.
- **GIF** animation is verified multi-frame via DecodeFrame GIF animation.
- **PNG** remains first-frame-only still images.
- **WebP** remains still-only (lossless, no alpha, no animation).

### Animation under verified scope

Multi-frame animation is verified for GIF and NIE nïA multi-frame via
`FrameCount`, `LoopCount`, and `DecodeFrame`; `DecodeRGBA` decodes frame 0.
PNG remains first-frame-only still images, and WebP remains still-only
(lossless, no alpha, no animation).

---

## Version

```go
func (d *Decoder) Version() string
func (d *Decoder) VersionNum() int32
```

`Version` is `"major.minor.patch"` of the embedded Wuffs library.
`VersionNum` is `(major<<16 | minor<<8 | patch)`.

---

## Out of scope

- Public quirk keys / `set_quirk`
- Incremental `io.Reader` decode without buffering `src`
- Encoding / compression
- Vector or document formats (SVG, PDF, PSD)
- Formats Wuffs does not decode (AVIF, TIFF, HEIC, JPEG XL, ICO/CUR)
