// Package wuffs provides image decoding through a reusable Decoder backed by
// Wuffs compiled to WebAssembly (wasm2go). There is no CGO.
//
// # Caller-owned pixels
//
// The reusable hot path is Probe → Reserve → DecodeRGBA (or DecodeNRGBA /
// DecodeGray) into a pre-allocated image.Image destination. Decode methods
// write into the caller's Pix slice; they never allocate, replace, or alias
// Pix to wasm memory. DecodeRGBA and DecodeFrame emit alpha-premultiplied
// image.RGBA bytes; DecodeNRGBA emits straight NRGBA bytes. A successful
// DecodeNRGBA or DecodeGray call populates its entire decoded canvas and
// retains no pixels from an earlier use of the Decoder. Guest dst scratch
// is internal. New starts with 32 MiB of guest linear memory; only explicit
// Reserve grows it, up to the 256 MiB generated maximum. Decoding needs
// Stride*Height + WorkbufLen bytes of guest dst scratch, both reported by
// Probe.
//
// On 64-bit unix and Windows, guest memory lives outside the Go heap: each
// Decoder reserves 256 MiB of address space, and pages become resident only
// when written. GOMEMLIMIT and Go heap profiles do not count it; measure it
// with RSS. Close releases a Decoder's guest memory at once; a Decoder dropped
// without Close is released when garbage collected. Trim returns the guest
// pages above the stack and static data to the operating system, and the
// Decoder stays usable with its current Reserve sizes.
//
// # Source capacity
//
// A new Decoder accepts sources up to 64 KiB until Reserve raises the src
// slot. Probe, decode, animation, and Metadata methods share that capacity;
// oversized src returns ErrSrcTooLarge.
//
// # Animation
//
// FrameCount, LoopCount, and DecodeFrame expose multi-frame GIF and verified
// NIE nïA animation. DecodeRGBA is equivalent to DecodeFrame(dst, src, 0)
// without returning frame metadata. Frame.Duration converts Wuffs
// flicks to time.Duration, truncated to whole nanoseconds; DecodeFrame
// returns ErrDecode for a duration outside time.Duration.
//
// # Metadata
//
// Metadata reads EXIF, ICC, XMP, gamma, chromaticities, sRGB intent, and
// modification time without decoding pixels. It requires the guest dst scratch
// slot to be at least 64 KiB (New defaults to 128 KiB). Returned blob slices
// are copied to the Go heap; the *Metadata pointer aliases decoder storage
// until the next successful Metadata call.
//
// # Allocating helpers
//
// Package-level Probe, Decode, DecodeNRGBA, DecodeGray, and DecodeConfig
// allocate a temporary Decoder and destination where needed. DecodeReader and
// DecodeConfigReader buffer an io.Reader. These paths are not zero-allocation.
//
// # Standard library registration
//
// RegisterFormats opts in to registering the thirteen verified formats with
// image.Decode and image.DecodeConfig. Importing this package does not touch
// the global registry.
//
// # Concurrency
//
// A Decoder is used by one goroutine at a time. Use separate Decoder values
// (or external serialization) for concurrent callers; a Decoder remains
// single-goroutine.
//
// The authoritative public contract is API.md; README.md shows typical usage.
package wuffs
