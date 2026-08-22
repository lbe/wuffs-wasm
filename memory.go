package wuffs

import "fmt"

// Memory layout constants for guest linear memory slots.
const (
	// maxSrc is the maximum source slot size in bytes. Reserve may grow
	// the src slot beyond this, but New() allocates at least this much.
	maxSrc = 2 * 1024 * 1024 // 2 MiB

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
	// Matches the C wuffs_wasm_decode_meta struct (5 × uint32 = 20 bytes).
	metaSlotBytes = 20

	// hostSlotRegionBase is the minimum base offset in wasm linear memory
	// where host-accessible slots begin. Guest code rejects meta_off == 0,
	// so this must be non-zero and sufficiently large.
	hostSlotRegionBase = 0x2000000 // 32 MiB

	// wasmPageSize is the wasm linear memory page size in bytes.
	wasmPageSize = 64 * 1024 // 64 KiB
)

// align8 rounds n up to the next multiple of 8.
func align8(n uint32) uint32 { return (n + 7) &^ 7 }

// slotTotal returns the aligned total size of the meta, src, and dst slots.
func slotTotal(dstBytes, srcBytes uint32) uint32 {
	total := uint32(metaSlotBytes) + srcBytes + dstBytes
	return align8(total)
}

// nonNegUint32 converts n to uint32, clamping negative values to zero.
func nonNegUint32(n int) uint32 {
	if n < 0 {
		return 0
	}
	return uint32(n)
}

// errReserveGrowFailed is returned when memory.Grow fails during Reserve.
var errReserveGrowFailed = fmt.Errorf("wuffs: Reserve failed: memory.Grow returned error")

// SlotLayout describes the memory slot arrangement in wasm linear memory.
// All offsets and lengths are in bytes relative to the start of wasm linear memory.
type SlotLayout struct {
	MetaOff  uint32 // offset of metadata slot
	MetaLen  uint32 // size of metadata slot
	SrcOff   uint32 // offset of source data slot
	SrcLen   uint32 // allocated size of source slot
	DstOff   uint32 // offset of destination data slot
	DstLen   uint32 // allocated size of destination slot
	HostBase uint32 // base of host slot region
}

// computeLayout computes the slot layout for the given memory size and slot
// sizes. Slots are placed at the end of linear memory so that memory growth
// shifts their offsets (guest needs to re-read meta after grow).
func computeLayout(memSize uint32, dstBytes, srcBytes uint32) SlotLayout {
	totalSlots := slotTotal(dstBytes, srcBytes)

	// Place slots at the end of memory, but not below the host slot region base.
	base := memSize - totalSlots
	if base < hostSlotRegionBase {
		base = hostSlotRegionBase
	}

	metaOff := base
	srcOff := metaOff + uint32(metaSlotBytes)
	dstOff := srcOff + srcBytes

	return SlotLayout{
		MetaOff:  metaOff,
		MetaLen:  uint32(metaSlotBytes),
		SrcOff:   srcOff,
		SrcLen:   srcBytes,
		DstOff:   dstOff,
		DstLen:   dstBytes,
		HostBase: base,
	}
}

// MemoryLayout returns the current memory slot layout.
func (d *Decoder) MemoryLayout() SlotLayout {
	return d.currentLayout
}

// Reserve grows wasm memory and updates slot layout for the given destination
// and source sizes. It calls memory.Grow when the required layout exceeds the
// current wasm page count, then refreshes the active memory view.
func (d *Decoder) Reserve(dstBytes, srcBytes int) error {
	// Raise the source capacity to accommodate the requested src slot.
	if srcBytes > d.srcCap {
		d.srcCap = srcBytes
	}

	dst := nonNegUint32(dstBytes)
	src := nonNegUint32(srcBytes)

	mem := d.module.Xmemory()
	currentBytes := uint32(len(*mem.Slice()))

	// Tentatively compute layout with current memory to check if growth is needed.
	trial := computeLayout(currentBytes, dst, src)
	if trial.HostBase >= hostSlotRegionBase {
		// Slots fit within current memory.
		d.currentLayout = trial
		return nil
	}

	// Slots don't fit: grow memory so that hostSlotRegionBase + totalSlots <= newMemSize.
	totalSlots := slotTotal(dst, src)
	requiredMem := uint64(hostSlotRegionBase) + uint64(totalSlots)
	pagesNeeded := (requiredMem + wasmPageSize - 1) / wasmPageSize
	currentPages := uint64(currentBytes) / wasmPageSize
	delta := int64(pagesNeeded - currentPages)
	if delta > 0 {
		const maxPages = int64(0x10000) // 1 GiB
		oldPages := mem.Grow(delta, maxPages)
		if oldPages < 0 {
			return errReserveGrowFailed
		}
		currentBytes = uint32(len(*mem.Slice()))
	}

	d.currentLayout = computeLayout(currentBytes, dst, src)
	return nil
}
