# wuffs-wasm

[![Go Reference](https://pkg.go.dev/badge/github.com/lbe/wuffs-wasm.svg)](https://pkg.go.dev/github.com/lbe/wuffs-wasm)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.26.5-blue.svg)](https://go.dev/dl/)
[![Release](https://github.com/lbe/wuffs-wasm/actions/workflows/releases.yml/badge.svg)](https://github.com/lbe/wuffs-wasm/actions/workflows/releases.yml)
[![CI](https://github.com/lbe/wuffs-wasm/actions/workflows/ci.yml/badge.svg)](https://github.com/lbe/wuffs-wasm/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/lbe/wuffs-wasm/branch/main/graph/badge.svg)](https://codecov.io/gh/lbe/wuffs-wasm)

Pure Go bindings for the [Wuffs](https://github.com/google/wuffs) image decoder.
Wuffs runs as a WebAssembly guest compiled to Go via [wasm2go](https://github.com/lbe/wasm2go-wasi-host)—no CGO, no native Wuffs library at link time.

`DecodeRGBA` decodes supported inputs into `image.RGBA` with one Go heap allocation per decode (the `image.RGBA` shell; pixel data lives in wasm linear memory until copied out).

## Format support

Support is being added incrementally. The wasm guest already compiles all Wuffs
image decoders; the Go package only **verifies** the formats listed under
**Supported** today.

### Supported (tested)

| Format | Notes                       |
| ------ | --------------------------- |
| PNG    | Still images                |
| WebP   | Lossless WebP only (tested) |

### Not yet verified

These decoders are compiled into the wasm guest and may work via `DecodeRGBA`,
but they are not covered by integration tests yet. Behavior is not guaranteed
until we add fixtures and tests.

| Format               | Notes                                           |
| -------------------- | ----------------------------------------------- |
| JPEG                 |                                                 |
| GIF                  | First frame only (see limitations)              |
| WebP                 | Lossy variants not yet tested                   |
| BMP                  |                                                 |
| TGA                  |                                                 |
| QOI                  |                                                 |
| Netpbm (PBM/PGM/PPM) |                                                 |
| WBMP                 |                                                 |
| NIE                  |                                                 |
| ETC2                 | GPU texture format                              |
| ThumbHash            | Placeholder hash, not a conventional image file |
| Handsum (HNSM)       |                                                 |

### Not supported

Formats outside [Wuffs image decoders](https://github.com/google/wuffs/blob/main/doc/std/image-decoders.md) are out of scope, including AVIF, TIFF, ICO/CUR, HEIC, JPEG XL, SVG, and PSD.

### Current limitations

- **Still images / first frame only** — animated GIF and WebP decode frame 0; no multi-frame API yet.
- **Output** — `image.RGBA` (straight, non-premultiplied) only.
- **No metadata API** — EXIF, ICC profiles, and similar are not exposed.

## Install

```bash
go get github.com/lbe/wuffs-wasm
```

Requires Go 1.26.5 or later.

## Usage

```go
package main

import (
	"fmt"
	"image"
	"os"

	"github.com/lbe/wuffs-wasm"
)

func main() {
	pngSrc, err := os.ReadFile("image.png")
	if err != nil {
		panic(err)
	}

	d := wuffs.New()
	dst := image.NewRGBA(image.Rect(0, 0, 0, 0))

	meta, err := d.DecodeRGBA(dst, pngSrc)
	if err != nil {
		panic(err)
	}

	fmt.Printf("decoded %dx%d (%s)\n", meta.Width, meta.Height, d.Version())
	_ = dst // *image.RGBA with decoded pixels
}
```

Pass `dst` with `Rect` at `(0,0,0,0)` and empty `Pix` to let the decoder size the image. For large inputs, call `Reserve` on the decoder first to grow wasm memory slots:

```go
d := wuffs.New()
if err := d.Reserve(4*1024*1024, len(pngSrc)); err != nil {
	return err
}
```

## API

| Symbol                                            | Description                                                      |
| ------------------------------------------------- | ---------------------------------------------------------------- |
| `New()`                                           | Construct a decoder (initializes the wasm guest).                |
| `(*Decoder) DecodeRGBA(dst, src)`                 | Decode a supported image into `dst`; returns `*Meta` on success. |
| `(*Decoder) Reserve(dstBytes, srcBytes)`          | Grow wasm src/dst slots before decode.                           |
| `(*Decoder) Version()` / `VersionNum()`           | Embedded Wuffs library version.                                  |
| `ErrUnknownFormat`, `ErrSrcTooLarge`, `ErrDecode` | Sentinel errors.                                                 |
| `*DstTooSmallError`                               | Structured error with required buffer size and image dimensions. |

A `Decoder` is not safe for concurrent use. Use one decoder per goroutine, or serialize access.

## Performance

Benchmarks pair wuffs-go with `image/png` on the same fixtures. On typical hardware, wuffs-go is faster with far fewer Go heap allocations; decoded pixels are not counted in `benchmem` because they live in wasm memory.

```bash
go test -bench='PNG_' -benchmem -count=3 -run=^$
```

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for benchmark interpretation and the full developer workflow.

## Development

To rebuild the wasm guest or regenerate `internal/wuffswasm/wuffs.go`, see [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md). Consumers only need `go get`; the generated bindings are committed in the repository.

```bash
make test
```

## License

MIT. See [LICENSE](LICENSE).
