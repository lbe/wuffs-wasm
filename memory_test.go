package wuffs

import "testing"

func TestUnitMemoryLayout(t *testing.T) {
	d := New()
	if d == nil {
		t.Fatal("New() returned nil decoder")
	}

	t.Run("constants meet minimums", func(t *testing.T) {
		if maxSrc < 2*1024*1024 {
			t.Errorf("maxSrc = %d, want >= %d (2 MiB)", maxSrc, 2*1024*1024)
		}
		if defaultInitialDstSlotBytes < 128*1024 {
			t.Errorf("defaultInitialDstSlotBytes = %d, want >= %d (128 KiB)", defaultInitialDstSlotBytes, 128*1024)
		}
	})

	t.Run("slot layout at New", func(t *testing.T) {
		layout := d.MemoryLayout()

		// meta slot offset must be non-zero (guest rejects meta_off==0)
		if layout.MetaOff == 0 {
			t.Error("meta slot offset must be non-zero (guest rejects meta_off==0)")
		}

		// meta slot size must be non-zero
		if layout.MetaLen == 0 {
			t.Error("meta slot size must be non-zero")
		}

		// host slot region base >= 0x2000000
		if layout.HostBase < 0x2000000 {
			t.Errorf("host slot region base = 0x%X, want >= 0x2000000", layout.HostBase)
		}

		// Slots must not overlap
		if overlaps(layout.MetaOff, layout.MetaLen, layout.SrcOff, layout.SrcLen) {
			t.Error("meta and src slots overlap")
		}
		if overlaps(layout.MetaOff, layout.MetaLen, layout.DstOff, layout.DstLen) {
			t.Error("meta and dst slots overlap")
		}
		if overlaps(layout.SrcOff, layout.SrcLen, layout.DstOff, layout.DstLen) {
			t.Error("src and dst slots overlap")
		}
	})

	t.Run("Reserve grows memory and updates offsets", func(t *testing.T) {
		layout := d.MemoryLayout()
		oldMetaOff := layout.MetaOff
		oldSrcOff := layout.SrcOff
		oldDstOff := layout.DstOff

		// Reserve with large values to trigger memory.Grow.
		// wasm initial memory is 64MiB; this request requires growth.
		err := d.Reserve(32*1024*1024, 2*1024*1024)
		if err != nil {
			t.Fatalf("Reserve(32MiB, 2MiB) failed: %v", err)
		}

		layout = d.MemoryLayout()
		if layout.MetaOff == oldMetaOff && layout.SrcOff == oldSrcOff && layout.DstOff == oldDstOff {
			t.Error("Reserve did not update any offsets after memory.Grow")
		}

		// After grow, meta offset must still be non-zero
		if layout.MetaOff == 0 {
			t.Error("meta slot offset must be non-zero after Reserve")
		}

		// Slots must still not overlap after grow
		if overlaps(layout.MetaOff, layout.MetaLen, layout.SrcOff, layout.SrcLen) {
			t.Error("meta and src slots overlap after Reserve")
		}
		if overlaps(layout.MetaOff, layout.MetaLen, layout.DstOff, layout.DstLen) {
			t.Error("meta and dst slots overlap after Reserve")
		}
		if overlaps(layout.SrcOff, layout.SrcLen, layout.DstOff, layout.DstLen) {
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
