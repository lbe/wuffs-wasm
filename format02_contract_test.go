package wuffs_test

// FORMAT-02 contract helpers. These shared helpers prove the two ownership
// identities every positive FORMAT-02 case must demonstrate:
//
//   - guest-memory identity: a reserved Probe or DecodeRGBA call leaves the
//     complete wasm memory slice (backing pointer and byte length) and every
//     field of the decoder's slot layout unchanged;
//   - caller-destination identity: DecodeRGBA leaves the caller's image.RGBA
//     layout (Pix backing pointer, length, capacity, Rect, and Stride)
//     unchanged.
//
// Both failure paths name the operation and fixture so a regression points at
// the exact call site. These are helpers only; they deliberately add no
// top-level Test* function.

import (
	"image"
	"testing"
	"unsafe"

	"github.com/lbe/wuffs-wasm"
)

// rgbaLayoutState is a comparable snapshot of the caller-owned destination
// properties that DecodeRGBA must never change or replace.
type rgbaLayoutState struct {
	PixBase unsafe.Pointer // &dst.Pix[0] via unsafe.SliceData
	PixLen  int
	PixCap  int
	Rect    image.Rectangle
	Stride  int
}

// captureRGBAState snapshots dst's complete layout. A nil Pix yields a nil
// base pointer, matching unsafe.SliceData's nil result for a nil slice.
func captureRGBAState(dst *image.RGBA) rgbaLayoutState {
	return rgbaLayoutState{
		PixBase: unsafe.Pointer(unsafe.SliceData(dst.Pix)),
		PixLen:  len(dst.Pix),
		PixCap:  cap(dst.Pix),
		Rect:    dst.Rect,
		Stride:  dst.Stride,
	}
}

// assertGuestMemoryPreserved fails the test unless d's complete guest memory
// state still equals before after the named operation.
func assertGuestMemoryPreserved(t *testing.T, op, fixture string, d *wuffs.Decoder, before wuffs.GuestMemoryState) {
	t.Helper()
	after := wuffs.CaptureGuestMemoryState(d)
	if after != before {
		t.Errorf("%s(%s) changed guest memory: before=%+v after=%+v", op, fixture, before, after)
	}
}

// assertDestinationPreserved fails the test unless dst's complete layout still
// equals before after the named operation.
func assertDestinationPreserved(t *testing.T, op, fixture string, dst *image.RGBA, before rgbaLayoutState) {
	t.Helper()
	after := captureRGBAState(dst)
	if after != before {
		t.Errorf("%s(%s) changed destination layout: before=%+v after=%+v", op, fixture, before, after)
	}
}
