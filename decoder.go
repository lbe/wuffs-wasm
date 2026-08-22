package wuffs

import (
	"errors"
	"fmt"
	"image"

	"github.com/lbe/wuffs-wasm/internal/wuffswasm"
	wasihost "github.com/lbe/wasm2go-wasi-host"
)

// Decoder wraps the wasm2go-generated wuffs module and WASI host state.
type Decoder struct {
	module        *wuffswasm.Module
	wasi          *wasihost.State
	currentLayout SlotLayout
	srcCap        int  // max src bytes DecodeRGBA accepts; grown via Reserve
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
		currentLayout: computeLayout(memSize, uint32(initialDstSlotBytes), maxSrc),
		srcCap:        defaultSrcCap,
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

// readMeta reads the decoded metadata from mem at the meta offset described by layout.
func readMeta(mem []byte, layout SlotLayout) Meta {
	return ReadMeta(mem, layout.MetaOff)
}

// DecodeRGBA decodes src (e.g. PNG or WEBP) into the pre-allocated dst.
// If dst.Rect has zero bounds, the host performs no pre-check and the guest
// determines image dimensions. If dst.Rect has Dx>0 && Dy>0 and
// len(dst.Pix) is insufficient, a DstTooSmallError is returned before
// calling the guest.
//
// On success the returned Meta carries the decoded image dimensions and
// pixel format.
func (d *Decoder) DecodeRGBA(dst *image.RGBA, src []byte) (*Meta, error) {
	// Host pre-check: reject src that exceeds the src-slot capacity before
	// any guest call. Callers can raise the cap via Reserve.
	if len(src) > d.srcCap {
		return nil, ErrSrcTooLarge
	}

	// Host pre-check: if caller set a non-zero Rect, Pix must be large enough.
	if dst.Rect.Dx() > 0 && dst.Rect.Dy() > 0 {
		minBytes := dst.Stride * dst.Rect.Dy()
		if len(dst.Pix) < minBytes {
			return nil, &DstTooSmallError{MinBytes: uint32(minBytes)}
		}
	}

	// Reserve wasm memory for src, dst, and meta slots. Grow from the current
	// layout rather than shrinking back to the defaults, so that a caller who
	// already reserved larger slots via Reserve keeps them.
	dstBytes := max(initialDstSlotBytes, int(d.currentLayout.DstLen))
	srcBytes := max(len(src), int(d.currentLayout.SrcLen))
	if err := d.Reserve(dstBytes, srcBytes); err != nil {
		return nil, err
	}
	lay := d.currentLayout

	// Copy source data into the wasm src slot.
	mem := d.module.Xmemory()
	memBytes := *mem.Slice()
	copy(memBytes[lay.SrcOff:lay.SrcOff+uint32(len(src))], src)

	// Invoke the guest decode.
	ret := d.module.Xwuffs_decode_image(
		int32(lay.SrcOff), int32(len(src)),
		int32(lay.DstOff), int32(lay.DstLen),
		int32(lay.MetaOff),
	)

	// Map guest return codes to host errors.
	if ret != 0 {
		err := errFromGuestReturn(ret)
		if ret == guestErrDstTooSmall {
			// The guest has written image dimensions into the meta slot even
			// though the destination buffer was too small; surface them so the
			// caller can reserve a large enough slot and retry.
			decMeta := readMeta(memBytes, lay)
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
	memBytes = *d.module.Xmemory().Slice()

	// Read decoded metadata from the guest meta slot.
	decMeta := readMeta(memBytes, lay)
	width := int(decMeta.Width)
	height := int(decMeta.Height)

	// Set destination image dimensions and point Pix directly at the guest dst
	// slot in wasm linear memory. Slicing to the exact decoded size avoids a
	// per-decode heap allocation for the pixel buffer. The in-place BGRA→RGBA
	// conversion reads each pixel before writing it back, so sharing src and dst
	// is safe.
	dst.Rect = image.Rect(0, 0, width, height)
	dst.Stride = width * 4
	pixLen := uint32(width * height * 4)
	dst.Pix = memBytes[lay.DstOff : lay.DstOff+pixLen]

	// Convert BGRA premultiplied → straight RGBA in place.
	convertBGRAtoRGBA(dst.Pix, dst.Stride, dst.Pix, width, height)

	d.lastMeta = decMeta
	return &d.lastMeta, nil
}
