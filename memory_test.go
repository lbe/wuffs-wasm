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

		// meta slot offset must be non-zero (guest rejects meta_off==0)
		if layout.metaOff == 0 {
			t.Error("meta slot offset must be non-zero (guest rejects meta_off==0)")
		}

		// meta slot size must be non-zero
		if layout.metaLen == 0 {
			t.Error("meta slot size must be non-zero")
		}

		// host slot region base >= 0x2000000
		if layout.hostBase < 0x2000000 {
			t.Errorf("host slot region base = 0x%X, want >= 0x2000000", layout.hostBase)
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
	})

	t.Run("Reserve grows memory and updates offsets", func(t *testing.T) {
		layout := d.currentLayout
		oldMetaOff := layout.metaOff
		oldSrcOff := layout.srcOff
		oldDstOff := layout.dstOff

		// Reserve with large values to trigger memory.Grow.
		// wasm initial memory is 64MiB; this request requires growth.
		err := d.Reserve(32*1024*1024, 2*1024*1024)
		if err != nil {
			t.Fatalf("Reserve(32MiB, 2MiB) failed: %v", err)
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
