package wuffs

import (
	"fmt"
	"math"
)

// Memory layout constants for guest linear memory slots.
const (
	// defaultInitialDstSlotBytes is the initial destination slot size in bytes.
	defaultInitialDstSlotBytes = 128 * 1024 // 128 KiB
)

// initialDstSlotBytes is the initial destination slot size in bytes.
// It is a variable so integration tests can temporarily shrink it to force
// the guest DstTooSmallError path without rebuilding the wasm binary.
var initialDstSlotBytes = defaultInitialDstSlotBytes

const (
	// defaultSrcCap is the source-slot capacity a fresh Decoder accepts
	// before callers raise it via Reserve.
	defaultSrcCap = 64 * 1024 // 64 KiB
	// metaSlotBytes is the size of the metadata slot in bytes.
	// Matches the C wuffs_wasm_decode_meta struct (6 × uint32 = 24 bytes).
	metaSlotBytes = 24

	// hostSlotRegionBase is the minimum base offset in wasm linear memory
	// where host-accessible slots begin. Guest code rejects meta_off == 0,
	// so this must be non-zero and sufficiently large.
	hostSlotRegionBase = 0x2000000 // 32 MiB

	// wasmPageSize is the wasm linear memory page size in bytes.
	wasmPageSize = 64 * 1024 // 64 KiB

	// maxReservePages is the generated wasm module's authoritative maximum
	// linear memory size, expressed in wasm pages (64 KiB each). 4096 pages
	// equals 256 MiB and is enforced on every memory.Grow call.
	maxReservePages = 4096
)

// align8 rounds n up to the next multiple of 8.
func align8(n uint32) uint32 { return (n + 7) &^ 7 }

// slotTotal returns the aligned total size of the meta, src, and dst slots.
func slotTotal(dstBytes, srcBytes uint32) uint32 {
	total := uint32(metaSlotBytes) + srcBytes + dstBytes
	return align8(total)
}

// errReserveGrowFailed is returned when memory.Grow fails during Reserve.
var errReserveGrowFailed = fmt.Errorf("wuffs: Reserve failed: memory.Grow returned error")

// errReserveInvalid is returned when a Reserve request cannot be represented
// safely in uint32 slot/layout arithmetic or exceeds the generated wasm
// module's authoritative maximum of 4096 pages (256 MiB).
var errReserveInvalid = fmt.Errorf("wuffs: Reserve failed: request exceeds representable slot layout")

// resolveSlot returns max(current, requested) as a uint32. A non-positive
// request leaves the current slot unchanged. It returns errReserveInvalid when
// the positive request cannot be represented in uint32 slot arithmetic.
func resolveSlot(current uint32, requested int) (uint32, error) {
	if requested <= 0 {
		return current, nil
	}
	if uint64(requested) > math.MaxUint32 {
		return 0, errReserveInvalid
	}
	nd := uint32(requested)
	if nd > current {
		return nd, nil
	}
	return current, nil
}

// slotLayout describes the memory slot arrangement in wasm linear memory.
// All offsets and lengths are in bytes relative to the start of wasm linear memory.
// It is an internal implementation detail of guest scratch management, not part
// of the public API.
type slotLayout struct {
	metaOff  uint32 // offset of metadata slot
	metaLen  uint32 // size of metadata slot
	srcOff   uint32 // offset of source data slot
	srcLen   uint32 // allocated size of source slot
	dstOff   uint32 // offset of destination data slot
	dstLen   uint32 // allocated size of destination slot
	hostBase uint32 // base of host slot region
}

// computeLayout computes the slot layout for the given memory size and slot
// sizes. Slots are placed at the end of linear memory so that memory growth
// shifts their offsets (guest needs to re-read meta after grow).
func computeLayout(memSize uint32, dstBytes, srcBytes uint32) slotLayout {
	totalSlots := slotTotal(dstBytes, srcBytes)

	// Place slots at the end of memory, but not below the host slot region base.
	base := memSize - totalSlots
	if base < hostSlotRegionBase {
		base = hostSlotRegionBase
	}

	metaOff := base
	srcOff := metaOff + uint32(metaSlotBytes)
	dstOff := srcOff + srcBytes

	return slotLayout{
		metaOff:  metaOff,
		metaLen:  uint32(metaSlotBytes),
		srcOff:   srcOff,
		srcLen:   srcBytes,
		dstOff:   dstOff,
		dstLen:   dstBytes,
		hostBase: base,
	}
}

// Reserve grows wasm memory and updates slot layout for the given destination
// and source sizes. Each requested slot is treated independently as
// max(current length, requested length): a negative or smaller request leaves
// the existing slot capacity and the complete slot layout unchanged. Requests
// that cannot be represented safely in uint32 slot or layout arithmetic, or
// whose complete layout would exceed the module's 4096-page maximum, are
// rejected with a non-nil error and leave all state unchanged.
func (d *Decoder) Reserve(dstBytes, srcBytes int) error {
	cur := d.currentLayout

	newDst, err := resolveSlot(cur.dstLen, dstBytes)
	if err != nil {
		return err
	}
	newSrc, err := resolveSlot(cur.srcLen, srcBytes)
	if err != nil {
		return err
	}

	// Validate the complete slot layout arithmetic in uint64 before any
	// conversion to uint32 or mutation of d.currentLayout.
	total := uint64(metaSlotBytes) + uint64(newSrc) + uint64(newDst)
	if total < uint64(newSrc) || total < uint64(newDst) {
		return errReserveInvalid
	}
	total = (total + 7) &^ 7 // align8
	requiredMem := uint64(hostSlotRegionBase) + total
	if requiredMem < uint64(hostSlotRegionBase) {
		return errReserveInvalid
	}
	maxBytes := uint64(maxReservePages) * uint64(wasmPageSize)
	if requiredMem > maxBytes {
		return errReserveInvalid
	}

	// Grow the host linear memory only if the required layout does not already
	// fit within the current page count. Host growth uses the 4096-page limit.
	mem := d.module.Xmemory()
	currentBytes := uint32(len(*mem.Slice()))
	pagesNeeded := (requiredMem + uint64(wasmPageSize) - 1) / uint64(wasmPageSize)
	currentPages := uint64(currentBytes) / uint64(wasmPageSize)
	if pagesNeeded > currentPages {
		delta := int64(pagesNeeded - currentPages)
		if mem.Grow(delta, maxReservePages) < 0 {
			return errReserveGrowFailed
		}
		currentBytes = uint32(len(*mem.Slice()))
	}

	d.currentLayout = computeLayout(currentBytes, newDst, newSrc)
	return nil
}
