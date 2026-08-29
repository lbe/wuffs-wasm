package wuffs

import (
	"errors"
	"fmt"
	"image"
	"math"

	wasihost "github.com/lbe/wasm2go-wasi-host"
	"github.com/lbe/wuffs-wasm/internal/wuffswasm"
)

// Decoder wraps the wasm2go-generated wuffs module and WASI host state.
type Decoder struct {
	module        *wuffswasm.Module
	wasi          *wasihost.State
	currentLayout slotLayout
	lastMeta      Meta // reused Meta return value to avoid per-decode allocation; returned pointer aliases this field
}

// New constructs a Decoder, initializing the wasm2go module and WASI host.
// The module pointer is declared before the WASI state so the host memory
// callback can capture its address before the module is instantiated.
func New() *Decoder {
	var module *wuffswasm.Module
	wasi := newWASIState(&module)
	module = wuffswasm.New(wasi)
	module.X_initialize()
	mem := module.Xmemory()
	memSize := uint32(len(*mem.Slice()))
	return &Decoder{
		module:        module,
		wasi:          wasi,
		currentLayout: computeLayout(memSize, uint32(initialDstSlotBytes), uint32(defaultSrcCap)),
	}
}

// Version returns the embedded Wuffs library version as a "major.minor.patch" string.
func (d *Decoder) Version() string {
	v := uint32(d.module.Xwuffs_version())
	return fmt.Sprintf("%d.%d.%d", (v>>16)&0xFF, (v>>8)&0xFF, v&0xFF)
}

// VersionNum returns the raw Wuffs version as a 32-bit integer.
// The encoding is: (major<<16 | minor<<8 | patch) with the high byte unused.
// For Wuffs 0.4 this equals 0x00040000.
func (d *Decoder) VersionNum() int32 {
	return d.module.Xwuffs_version()
}

// checkSrcCapacity rejects src larger than the reserved src-slot capacity,
// returning ErrSrcTooLarge before any guest call. The capacity authority is
// the decoder's currentLayout.srcLen (grown via Reserve); there is no separate
// capacity field. Comparing as uint64 keeps the bound correct when Go int and
// uint32 differ in width.
func (d *Decoder) checkSrcCapacity(src []byte) error {
	if uint64(len(src)) > uint64(d.currentLayout.srcLen) {
		return ErrSrcTooLarge
	}
	return nil
}

// DecodeRGBA decodes src (e.g. PNG or WEBP) into the caller-owned dst.
//
// The caller must pre-allocate dst with Rect, Stride, and Pix set to the
// decoded canvas: Probe for Width x Height (or use known dimensions), Rect.Min
// must be (0,0), Stride must be >= Dx*4, and len(Pix) must be >= Stride*Dy.
//
// On success pixels are written into the caller's Pix; the library never
// allocates or replaces Pix and never points Pix at wasm memory. The wasm
// destination slot is scratch only: the guest decodes BGRA bytes there and the
// host converts them into dst.Pix.
//
// Errors:
//   - ErrBadImage for a malformed or incompatible destination shape: an empty
//     rectangle, nil or empty Pix, non-zero Rect.Min, a decoded dimension
//     mismatch, or an unrepresentable host layout (e.g. a row stride that
//     overflows uint32).
//   - *DstTooSmallError (matching ErrDstTooSmall) when dimensions match but
//     Stride < Dx*4 or len(Pix) < Stride*Dy.
//
// The returned Meta carries the decoded image dimensions and pixel format.
func (d *Decoder) DecodeRGBA(dst *image.RGBA, src []byte) (*Meta, error) {
	if err := d.checkSrcCapacity(src); err != nil {
		return nil, err
	}

	lay := d.currentLayout

	// Copy source data into the wasm src slot.
	d.copySrcToSlot(lay, src)

	// Invoke the guest decode.
	ret := d.module.Xwuffs_decode_image(
		int32(lay.srcOff), int32(len(src)),
		int32(lay.dstOff), int32(lay.dstLen),
		int32(lay.metaOff),
	)

	// Map guest return codes to host errors.
	if ret != 0 {
		err := errFromGuestReturn(ret)
		if ret == guestErrDstTooSmall {
			// The guest has written image dimensions into the meta slot even
			// though the destination buffer was too small; surface them so the
			// caller can reserve a large enough slot and retry.
			decMeta := readMeta(*d.module.Xmemory().Slice(), lay.metaOff)
			var dts *DstTooSmallError
			if errors.As(err, &dts) {
				dts.MinBytes = decMeta.Stride * decMeta.Height
				dts.Width = decMeta.Width
				dts.Height = decMeta.Height
				dts.Stride = decMeta.Stride
			}
		}
		return nil, err
	}

	// Refresh memory view after guest call (guest may have grown memory).
	memBytes := *d.module.Xmemory().Slice()

	// Read decoded metadata from the guest meta slot.
	decMeta := readMeta(memBytes, lay.metaOff)
	width := int(decMeta.Width)
	height := int(decMeta.Height)

	// Validate the caller-owned destination against the decoded dimensions.
	// The caller's Pix, Rect, and Stride are never modified on the success
	// path; mismatches are returned as errors instead.
	if dst.Rect.Dx() == 0 || dst.Rect.Dy() == 0 {
		return nil, ErrBadImage
	}
	if len(dst.Pix) == 0 {
		return nil, ErrBadImage
	}
	if dst.Rect.Min != (image.Point{}) {
		return nil, ErrBadImage
	}
	if dst.Rect.Dx() != width || dst.Rect.Dy() != height {
		return nil, ErrBadImage
	}

	// Required tight row stride in bytes (width*4). width <= 0xFFFFFF, so this
	// fits uint32, but the product with height is checked below.
	rowBytes := width * 4
	if uint64(rowBytes)*uint64(height) > math.MaxUint32 {
		return nil, ErrBadImage
	}
	if dst.Stride < rowBytes {
		return nil, &DstTooSmallError{
			MinBytes: uint32(uint64(rowBytes) * uint64(height)),
			Width:    uint32(width),
			Height:   uint32(height),
			Stride:   uint32(rowBytes),
		}
	}
	// Pix length check: required bytes must fit uint32 and be present.
	totalPix := uint64(dst.Stride) * uint64(height)
	if totalPix > math.MaxUint32 {
		return nil, ErrBadImage
	}
	if uint64(len(dst.Pix)) < totalPix {
		return nil, &DstTooSmallError{
			MinBytes: uint32(totalPix),
			Width:    uint32(width),
			Height:   uint32(height),
			Stride:   uint32(dst.Stride),
		}
	}

	// Slice the guest destination slot as read-only source scratch for the
	// decoded width*height*4 BGRA bytes, then convert into the caller's Pix.
	// Form the decoded length in uint64 and check it once before casting so the
	// checked value is the single source of truth (no recomputed int product).
	decodedBytes := uint64(width) * uint64(height) * 4
	if decodedBytes > math.MaxUint32 {
		return nil, ErrBadImage
	}
	pixLen := uint32(decodedBytes)
	wasmBGRA := memBytes[lay.dstOff : lay.dstOff+pixLen]
	convertBGRAtoRGBA(dst.Pix, dst.Stride, wasmBGRA, width, height)

	d.lastMeta = decMeta
	return &d.lastMeta, nil
}

// Probe reports image dimensions and format from src without decoding pixels.
// It copies src into the already-reserved wasm src slot and calls the guest
// probe_image export, which sniffs the format and decodes the image config,
// then overrides the pixel config to the BGRA_PREMUL dest layout (stride =
// width*4) used by DecodeRGBA. Probe writes no destination pixels;
// BytesWritten is 0. Probe honors the reserved src-slot capacity via
// checkSrcCapacity and never resizes slots internally: it must not call
// Reserve. Callers grow the src slot with Reserve before probing a source
// larger than the default capacity.
func (d *Decoder) Probe(src []byte) (*Meta, error) {
	if err := d.checkSrcCapacity(src); err != nil {
		return nil, err
	}

	lay := d.currentLayout

	// Copy source data into the wasm src slot.
	d.copySrcToSlot(lay, src)

	// Invoke the guest probe.
	ret := d.module.Xwuffs_probe_image(
		int32(lay.srcOff), int32(len(src)),
		int32(lay.metaOff),
	)

	// Map guest return codes to host errors. The guest uses
	// WUFFS_WASM_ERR_BAD_ARG (zero src_len / meta_off) → ErrDecode, matching
	// decode_image's host mapping.
	if ret != 0 {
		return nil, errFromGuestReturn(ret)
	}

	// Refresh memory view after guest call (guest may have grown memory).
	memBytes := *d.module.Xmemory().Slice()

	// Read decoded metadata from the guest meta slot.
	decMeta := readMeta(memBytes, lay.metaOff)
	d.lastMeta = decMeta
	return &d.lastMeta, nil
}

// copySrcToSlot copies src into the wasm src slot at lay.srcOff. Callers must
// ensure len(src) <= the reserved src-slot capacity. The wasm memory view is
// refreshed so the copy targets the live backing slice.
func (d *Decoder) copySrcToSlot(lay slotLayout, src []byte) {
	mem := d.module.Xmemory()
	memBytes := *mem.Slice()
	copy(memBytes[lay.srcOff:lay.srcOff+uint32(len(src))], src)
}
