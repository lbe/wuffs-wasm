// Package wuffs provides image decoding through a reusable Decoder backed by
// Wuffs compiled to WebAssembly (wasm2go). There is no CGO.
//
// # Caller-owned pixels
//
// The reusable hot path is Probe → Reserve → DecodeRGBA (or DecodeNRGBA /
// DecodeGray) into a pre-allocated image.Image destination. Decode methods
// write into the caller's Pix slice; they never allocate, replace, or alias
// Pix to wasm memory. Guest dst scratch is internal; only explicit Reserve
// grows wasm linear memory.
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
// without returning frame metadata.
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
// (or external serialization) for concurrent callers.
//
// The authoritative public contract is API.md; README.md shows typical usage.
package wuffs
