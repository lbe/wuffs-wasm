package wuffs

import "testing"

func TestUnitMemoryLayout(t *testing.T) {
	d := New()
	if d == nil {
		t.Fatal("New() returned nil decoder")
	}

	t.Run("constants meet minimums", func(t *testing.T) {
		if defaultInitialDstSlotBytes < 128*1024 {
			t.Errorf("defaultInitialDstSlotBytes = %d, want >= %d (128 KiB)", defaultInitialDstSlotBytes, 128*1024)
		}
	})

	t.Run("slot layout at New", func(t *testing.T) {
		layout := d.currentLayout

		// A new decoder starts with exactly 32 MiB of guest linear memory
		// from the generated module.
		memSize := uint32(len(*d.module.Xmemory().Slice()))
		if memSize != 32*1024*1024 {
			t.Errorf("guest linear memory = %d bytes, want exactly %d (32 MiB)", memSize, 32*1024*1024)
		}

		// Default slots must end within that 32 MiB memory.
		slotEnd := layout.dstOff + layout.dstLen
		if uint64(slotEnd) > uint64(memSize) {
			t.Errorf("default slot end = %d, exceeds guest linear memory %d", slotEnd, memSize)
		}

		// Host slots begin at the 24 MiB floor.
		if hostSlotRegionBase != 0x01800000 {
			t.Errorf("hostSlotRegionBase = 0x%X, want 0x01800000 (24 MiB)", hostSlotRegionBase)
		}

		// meta slot offset must be non-zero (guest rejects meta_off==0)
		if layout.metaOff == 0 {
			t.Error("meta slot offset must be non-zero (guest rejects meta_off==0)")
		}

		// meta slot size must be non-zero
		if layout.metaLen == 0 {
			t.Error("meta slot size must be non-zero")
		}

		// Host slots begin at or above the 24 MiB floor.
		if layout.hostBase < 0x01800000 {
			t.Errorf("host slot region base = 0x%X, want >= 0x01800000 (24 MiB)", layout.hostBase)
		}

		// Slots must not overlap
		if overlaps(layout.metaOff, layout.metaLen, layout.srcOff, layout.srcLen) {
			t.Error("meta and src slots overlap")
		}
		if overlaps(layout.metaOff, layout.metaLen, layout.dstOff, layout.dstLen) {
			t.Error("meta and dst slots overlap")
		}
		if overlaps(layout.srcOff, layout.srcLen, layout.dstOff, layout.dstLen) {
			t.Error("src and dst slots overlap")
		}

		// Animation scratch sits at the start of the host slot region:
		// count out at hostBase, loops out 4 bytes later, frame meta at
		// the 8-byte-aligned offset hostBase+8, decode meta after 56 bytes.
		if layout.countOutOff != layout.hostBase {
			t.Errorf("countOutOff = %d, want hostBase %d", layout.countOutOff, layout.hostBase)
		}
		if layout.loopsOutOff != layout.countOutOff+4 {
			t.Errorf("loopsOutOff = %d, want countOutOff+4 = %d", layout.loopsOutOff, layout.countOutOff+4)
		}
		if layout.frameMetaOff != layout.countOutOff+8 {
			t.Errorf("frameMetaOff = %d, want countOutOff+8 = %d", layout.frameMetaOff, layout.countOutOff+8)
		}
		if layout.metaOff != layout.hostBase+uint32(hostAnimationScratchBytes) {
			t.Errorf("metaOff = %d, want hostBase+%d = %d", layout.metaOff, hostAnimationScratchBytes, layout.hostBase+uint32(hostAnimationScratchBytes))
		}
		if hostAnimationScratchBytes != 56 {
			t.Errorf("hostAnimationScratchBytes = %d, want 56 (align8(4+4+48))", hostAnimationScratchBytes)
		}
		if layout.frameMetaOff%8 != 0 {
			t.Errorf("frameMetaOff = %d, want 8-byte alignment", layout.frameMetaOff)
		}
	})

	t.Run("Reserve grows memory and updates offsets", func(t *testing.T) {
		layout := d.currentLayout
		oldMetaOff := layout.metaOff
		oldSrcOff := layout.srcOff
		oldDstOff := layout.dstOff

		// Reserve with large values to trigger memory.Grow. The generated
		// 32 MiB baseline cannot hold a 32 MiB dst slot plus a 2 MiB src
		// slot above the 24 MiB floor, so this request grows memory.
		beforeBytes := uint32(len(*d.module.Xmemory().Slice()))
		err := d.Reserve(32*1024*1024, 2*1024*1024)
		if err != nil {
			t.Fatalf("Reserve(32MiB, 2MiB) failed: %v", err)
		}
		afterBytes := uint32(len(*d.module.Xmemory().Slice()))
		if afterBytes <= beforeBytes {
			t.Errorf("Reserve did not grow guest linear memory: before %d, after %d", beforeBytes, afterBytes)
		}

		layout = d.currentLayout
		if layout.metaOff == oldMetaOff && layout.srcOff == oldSrcOff && layout.dstOff == oldDstOff {
			t.Error("Reserve did not update any offsets after memory.Grow")
		}

		// After grow, meta offset must still be non-zero
		if layout.metaOff == 0 {
			t.Error("meta slot offset must be non-zero after Reserve")
		}

		// Slots must still not overlap after grow
		if overlaps(layout.metaOff, layout.metaLen, layout.srcOff, layout.srcLen) {
			t.Error("meta and src slots overlap after Reserve")
		}
		if overlaps(layout.metaOff, layout.metaLen, layout.dstOff, layout.dstLen) {
			t.Error("meta and dst slots overlap after Reserve")
		}
		if overlaps(layout.srcOff, layout.srcLen, layout.dstOff, layout.dstLen) {
			t.Error("src and dst slots overlap after Reserve")
		}
	})
}

// overlaps reports whether two memory regions [off1, off1+len1) and
// [off2, off2+len2) overlap. A zero offset means the slot is not allocated.
func overlaps(off1, len1, off2, len2 uint32) bool {
	if off1 == 0 || off2 == 0 {
		return false
	}
	return off1 < off2+len2 && off2 < off1+len1
}
