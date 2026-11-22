package wuffs

import "github.com/lbe/wuffs-wasm/internal/wuffswasm"

const (
	// initialMemoryBytes is the guest linear memory a new Decoder starts with,
	// matching --initial-memory in wasm/build.sh.
	initialMemoryBytes = 32 << 20 // 32 MiB

	// maxMemoryBytes is the address space reserved per Decoder: the module's
	// 4096-page maximum (--max-memory in wasm/build.sh).
	maxMemoryBytes = maxReservePages * wasmPageSize // 256 MiB

	// bumpBase is the start of the guest bump region (bump_base in
	// wasm/shim.c). Everything from here to the end of memory is per call:
	// each export rewinds the bump region, and the host rewrites its slots on
	// every call. The stack, static data, and BSS lie below it; the guest's
	// wuffs_heap_base export reports their end.
	bumpBase = 0x900000 // 9 MiB
)

// guestMemory is the wasm linear memory of one Decoder, imported by the
// generated module as env.memory. Platform files supply newGuestMemory: on
// 64-bit unix and Windows it reserves maxMemoryBytes of address space outside
// the Go heap, so pages become resident only when written and Grow never
// copies; elsewhere it falls back to a growable Go slice.
type guestMemory interface {
	wuffswasm.Memory
	// release frees the memory. Later calls are no-ops.
	release() error
	// trim returns the pages from offset from to the end of memory to the
	// operating system, keeping the memory size. Their contents read as zero
	// afterwards. It is a no-op for the Go-slice fallback.
	trim(from int)
}

// guestEnv supplies the generated module's env imports.
type guestEnv struct{ mem wuffswasm.Memory }

// Xmemory returns the imported linear memory.
func (e guestEnv) Xmemory() wuffswasm.Memory { return e.mem }

// growLen validates a wasm memory.grow of delta pages from curLen bytes
// against max pages and the reservation, returning the old page count and the
// new length in bytes. ok is false when the grow must fail.
func growLen(curLen int, delta, max int64) (old int64, newLen int, ok bool) {
	old = int64(curLen >> 16)
	limit := min(max, int64(maxReservePages))
	if delta < 0 || delta > limit || old+delta > limit {
		return old, 0, false
	}
	return old, int(old+delta) << 16, true
}

// sliceMemory is the Go-heap fallback: Grow appends and copies, as the
// generated code does for a non-imported memory.
type sliceMemory struct{ buf []byte }

func (m *sliceMemory) Slice() *[]byte { return &m.buf }

func (m *sliceMemory) Grow(delta, max int64) int64 {
	old, newLen, ok := growLen(len(m.buf), delta, max)
	if !ok {
		return -1
	}
	if delta > 0 {
		m.buf = append(m.buf, make([]byte, newLen-len(m.buf))...)
	}
	return old
}

func (m *sliceMemory) release() error {
	m.buf = nil
	return nil
}

func (m *sliceMemory) trim(int) {}
