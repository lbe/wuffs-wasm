package wuffs

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
