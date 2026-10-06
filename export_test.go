package wuffs

import "unsafe"

// Exported for tests: exposes internal helpers so that tests in other packages
// (or bench tests) can drive decoder behavior without widening the public API.

// ShrunkMaxSrc returns a compressed source-slot capacity used by host-side
// ErrSrcTooLarge enforcement. It is a test-only hook so that integration tests
// can drive the source-too-large path without megabytes of input.
func ShrunkMaxSrc() int { return 64 * 1024 }

// RequiredReserve is a test helper that reserves memory for decode.
// It calls d.Reserve(dstBytes, srcBytes) to size the wasm memory slots
// before a timed decode loop.
func RequiredReserve(d *Decoder, dstBytes, srcBytes int) error {
	return d.Reserve(dstBytes, srcBytes)
}

// SetInitialDstSlotBytes overrides the initial destination slot size for the
// current process. It returns a function that restores the original value.
// Callers must defer the returned function to avoid leaking the override to
// other tests.
func SetInitialDstSlotBytes(n int) func() {
	old := initialDstSlotBytes
	initialDstSlotBytes = n
	return func() { initialDstSlotBytes = old }
}

// CurrentDstSlotLen is a test-only helper that returns the current destination
// slot length of d. It exposes only that single value so external-package
// tests can assert destination-slot behavior without widening the public API or
// exposing the complete memory layout.
func CurrentDstSlotLen(d *Decoder) uint32 { return d.currentLayout.dstLen }

// GuestMemoryState is a test-only snapshot of the complete wasm linear memory
// and the decoder's slot layout. It is comparable, so tests can assert with a
// single equality that a guest call left the wasm backing store, its byte
// length, and every slot offset and size unchanged.
type GuestMemoryState struct {
	MemoryBase   unsafe.Pointer // backing pointer of the wasm memory slice
	MemoryLen    int            // byte length of the wasm memory slice
	CountOutOff  uint32         // currentLayout.countOutOff
	LoopsOutOff  uint32         // currentLayout.loopsOutOff
	FrameMetaOff uint32         // currentLayout.frameMetaOff
	MetaOff      uint32         // currentLayout.metaOff
	MetaLen      uint32         // currentLayout.metaLen
	SrcOff       uint32         // currentLayout.srcOff
	SrcLen       uint32         // currentLayout.srcLen
	DstOff       uint32         // currentLayout.dstOff
	DstLen       uint32         // currentLayout.dstLen
	HostBase     uint32         // currentLayout.hostBase
}

// CaptureGuestMemoryState snapshots the complete wasm memory slice (backing
// pointer via unsafe.SliceData and byte length) plus every field of the
// decoder's current slot layout. It exists only behind _test.go so it never
// widens the production API.
func CaptureGuestMemoryState(d *Decoder) GuestMemoryState {
	memBytes := *d.module.Xmemory().Slice()
	return GuestMemoryState{
		MemoryBase:   unsafe.Pointer(unsafe.SliceData(memBytes)),
		MemoryLen:    len(memBytes),
		CountOutOff:  d.currentLayout.countOutOff,
		LoopsOutOff:  d.currentLayout.loopsOutOff,
		FrameMetaOff: d.currentLayout.frameMetaOff,
		MetaOff:      d.currentLayout.metaOff,
		MetaLen:      d.currentLayout.metaLen,
		SrcOff:       d.currentLayout.srcOff,
		SrcLen:       d.currentLayout.srcLen,
		DstOff:       d.currentLayout.dstOff,
		DstLen:       d.currentLayout.dstLen,
		HostBase:     d.currentLayout.hostBase,
	}
}
