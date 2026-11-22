package wuffs

import (
	"strconv"
	"testing"
)

// TestUnitGuestStaticDataBelowBumpBase pins the invariant Trim relies on: the
// stack, static data, and BSS end at or below bumpBase, so every page Trim
// releases holds only per-call data.
func TestUnitGuestStaticDataBelowBumpBase(t *testing.T) {
	d := New()
	defer d.Close()
	heapBase := uint32(d.module.Xwuffs_heap_base())
	if heapBase == 0 || heapBase > bumpBase {
		t.Fatalf("guest __heap_base = %#x, want in (0, %#x]", heapBase, bumpBase)
	}
}

// TestUnitGuestMemoryOffHeap checks that 64-bit unix and Windows decoders use
// the mapped memory rather than the Go-slice fallback.
func TestUnitGuestMemoryOffHeap(t *testing.T) {
	if strconv.IntSize == 32 {
		t.Skip("32-bit platforms use the Go-slice fallback")
	}
	d := New()
	defer d.Close()
	if _, isSlice := d.mem.(*sliceMemory); isSlice && guestMemoryMapped {
		t.Fatalf("guest memory is %T, want the mapped implementation", d.mem)
	}
}

// TestUnitGrowLen covers the memory.grow bounds shared by every guestMemory.
func TestUnitGrowLen(t *testing.T) {
	cases := []struct {
		name       string
		curLen     int
		delta, max int64
		wantOld    int64
		wantLen    int
		wantOK     bool
	}{
		{"zero delta", 512 << 16, 0, 4096, 512, 512 << 16, true},
		{"grow", 512 << 16, 8, 4096, 512, 520 << 16, true},
		{"to max", 512 << 16, 3584, 4096, 512, 4096 << 16, true},
		{"past max", 512 << 16, 3585, 4096, 512, 0, false},
		{"past module max", 0, 4097, 1 << 20, 0, 0, false},
		{"negative", 512 << 16, -1, 4096, 512, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old, newLen, ok := growLen(tc.curLen, tc.delta, tc.max)
			if old != tc.wantOld || newLen != tc.wantLen || ok != tc.wantOK {
				t.Fatalf("growLen = (%d, %d, %v), want (%d, %d, %v)", old, newLen, ok, tc.wantOld, tc.wantLen, tc.wantOK)
			}
		})
	}
}
