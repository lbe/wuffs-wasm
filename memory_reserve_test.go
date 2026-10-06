package wuffs

import (
	"math"
	"math/bits"
	"testing"
	"unsafe"
)

// TestReserveGrowsOnlyRequestedSlotCapacity encodes the acceptance criteria
// for Reserve: each requested slot is treated independently as
// max(current length, requested length). Reserve must preserve the untouched
// slot's capacity, leave state unchanged for equal/smaller/negative requests,
// reject requests that cannot be represented safely in uint32 slot or layout
// arithmetic, and reject requests whose complete layout exceeds the generated
// wasm module's authoritative maximum of 4096 pages (256 MiB).
func TestReserveGrowsOnlyRequestedSlotCapacity(t *testing.T) {
	const (
		maxPages    = 4096
		maxMemBytes = maxPages * wasmPageSize // 256 MiB
	)

	wasmLen := func(d *Decoder) int { return len(*d.module.Xmemory().Slice()) }
	wasmPtr := func(d *Decoder) uintptr {
		s := d.module.Xmemory().Slice()
		if len(*s) == 0 {
			return 0
		}
		return uintptr(unsafe.Pointer(&(*s)[0]))
	}

	// assertReserveUnchanged asserts the complete slotLayout, wasm byte length,
	// and backing pointer are identical to the captured values. Used to prove a
	// rejected or no-growth Reserve is failure-atomic.
	assertReserveUnchanged := func(t *testing.T, d *Decoder, before slotLayout, beforeLen int, beforePtr uintptr) {
		t.Helper()
		after := d.currentLayout
		if after != before {
			t.Errorf("slotLayout changed: before=%+v after=%+v", before, after)
		}
		if wasmLen(d) != beforeLen {
			t.Errorf("wasm byte length changed: before=%d after=%d", beforeLen, wasmLen(d))
		}
		if wasmPtr(d) != beforePtr {
			t.Errorf("wasm backing pointer changed")
		}
	}

	// assertSlotsValid asserts the installed slots are ordered, non-overlapping,
	// and entirely within the wasm memory slice, using the actual computed
	// slotLayout from d.currentLayout.
	assertSlotsValid := func(t *testing.T, d *Decoder, lay slotLayout) {
		t.Helper()
		wlen := wasmLen(d)
		// In bounds: every slot extent [off, off+len) must satisfy off+len <= wasmLen.
		if end := lay.metaOff + lay.metaLen; end > uint32(wlen) {
			t.Errorf("meta slot out of bounds: off=%d len=%d end=%d wasmLen=%d", lay.metaOff, lay.metaLen, end, wlen)
		}
		if end := lay.srcOff + lay.srcLen; end > uint32(wlen) {
			t.Errorf("src slot out of bounds: off=%d len=%d end=%d wasmLen=%d", lay.srcOff, lay.srcLen, end, wlen)
		}
		if end := lay.dstOff + lay.dstLen; end > uint32(wlen) {
			t.Errorf("dst slot out of bounds: off=%d len=%d end=%d wasmLen=%d", lay.dstOff, lay.dstLen, end, wlen)
		}
		// Ordered: animation scratch < meta < src < dst are monotonically
		// increasing offsets.
		if lay.countOutOff != lay.hostBase {
			t.Errorf("countOutOff = %d, want hostBase %d", lay.countOutOff, lay.hostBase)
		}
		if lay.loopsOutOff != lay.countOutOff+uint32(frameCountOutSlotBytes) {
			t.Errorf("loopsOutOff = %d, want countOutOff+%d", lay.loopsOutOff, lay.countOutOff+uint32(frameCountOutSlotBytes))
		}
		if lay.frameMetaOff != lay.countOutOff+8 {
			t.Errorf("frameMetaOff = %d, want countOutOff+8", lay.frameMetaOff)
		}
		if lay.metaOff != lay.hostBase+uint32(hostAnimationScratchBytes) {
			t.Errorf("metaOff = %d, want hostBase+%d", lay.metaOff, lay.hostBase+uint32(hostAnimationScratchBytes))
		}
		// Frame meta (48 bytes at frameMetaOff) must end at/before metaOff.
		if lay.frameMetaOff+uint32(frameMetaSlotBytes) > lay.metaOff {
			t.Errorf("frame meta overlaps decode meta: frameMetaEnd=%d metaOff=%d", lay.frameMetaOff+uint32(frameMetaSlotBytes), lay.metaOff)
		}
		if lay.metaOff >= lay.srcOff || lay.srcOff >= lay.dstOff {
			t.Errorf("slots not monotonically ordered: meta=%d src=%d dst=%d", lay.metaOff, lay.srcOff, lay.dstOff)
		}
		// Non-overlapping: src starts at/after meta end; dst starts at/after src end.
		if lay.srcOff < lay.metaOff+lay.metaLen {
			t.Errorf("src overlaps meta: srcOff=%d metaEnd=%d", lay.srcOff, lay.metaOff+lay.metaLen)
		}
		if lay.dstOff < lay.srcOff+lay.srcLen {
			t.Errorf("dst overlaps src: dstOff=%d srcEnd=%d", lay.dstOff, lay.srcOff+lay.srcLen)
		}
	}

	// Grow only the destination slot (src requested as 0) preserves SrcLen and
	// increases DstLen.
	t.Run("grows only destination slot", func(t *testing.T) {
		d := New()
		before := d.currentLayout
		if err := d.Reserve(4*1024*1024, 0); err != nil {
			t.Fatalf("Reserve(4MiB,0) returned error: %v", err)
		}
		after := d.currentLayout
		if after.srcLen != before.srcLen {
			t.Errorf("SrcLen not preserved: before=%d after=%d", before.srcLen, after.srcLen)
		}
		if after.dstLen <= before.dstLen {
			t.Errorf("DstLen did not grow: before=%d after=%d", before.dstLen, after.dstLen)
		}
	})

	// Grow only the source slot (dst requested as 0) preserves DstLen and
	// increases SrcLen.
	t.Run("grows only source slot", func(t *testing.T) {
		d := New()
		before := d.currentLayout
		if err := d.Reserve(0, 4*1024*1024); err != nil {
			t.Fatalf("Reserve(0,4MiB) returned error: %v", err)
		}
		after := d.currentLayout
		if after.dstLen != before.dstLen {
			t.Errorf("DstLen not preserved: before=%d after=%d", before.dstLen, after.dstLen)
		}
		if after.srcLen <= before.srcLen {
			t.Errorf("SrcLen did not grow: before=%d after=%d", before.srcLen, after.srcLen)
		}
	})

	// Equal, smaller, negative, and negative-one-slot requests leave the
	// complete slotLayout, wasm byte length, and backing pointer unchanged and
	// return a nil error.
	t.Run("equal smaller negative leave state unchanged", func(t *testing.T) {
		cases := []struct {
			name     string
			dst, src int
		}{
			{"equal (1,1)", 1, 1},
			{"smaller than current (100,100)", 100, 100},
			{"negative both (-100,-100)", -100, -100},
			{"negative dst only (-100,0)", -100, 0},
			{"negative src only (0,-100)", 0, -100},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				d := New()
				before := d.currentLayout
				beforeLen := wasmLen(d)
				beforePtr := wasmPtr(d)
				if err := d.Reserve(c.dst, c.src); err != nil {
					t.Fatalf("Reserve(%d,%d) returned error: %v", c.dst, c.src, err)
				}
				after := d.currentLayout
				if after != before {
					t.Errorf("slotLayout changed: before=%+v after=%+v", before, after)
				}
				if wasmLen(d) != beforeLen {
					t.Errorf("wasm byte length changed: before=%d after=%d", beforeLen, wasmLen(d))
				}
				if wasmPtr(d) != beforePtr {
					t.Errorf("wasm backing pointer changed")
				}
			})
		}
	})

	// A positive request that cannot be represented safely in uint32 slot or
	// layout arithmetic is rejected: non-nil error and unchanged state.
	t.Run("rejects uint32 overflow request", func(t *testing.T) {
		d := New()
		before := d.currentLayout
		beforeLen := wasmLen(d)
		beforePtr := wasmPtr(d)
		err := d.Reserve(math.MaxUint32, math.MaxUint32)
		if err == nil {
			t.Fatalf("Reserve(MaxUint32,MaxUint32) returned nil; want non-nil error (slot/layout arithmetic overflow)")
		}
		after := d.currentLayout
		if after != before {
			t.Errorf("slotLayout changed after rejected overflow request: before=%+v after=%+v", before, after)
		}
		if wasmLen(d) != beforeLen {
			t.Errorf("wasm byte length changed after rejected overflow request: before=%d after=%d", beforeLen, wasmLen(d))
		}
		if wasmPtr(d) != beforePtr {
			t.Errorf("wasm backing pointer changed after rejected overflow request")
		}
	})

	// A request whose complete slot layout would exceed the generated wasm
	// module's authoritative maximum of 4096 pages (256 MiB) is rejected:
	// non-nil error and unchanged state.
	t.Run("rejects layout exceeding 4096 pages", func(t *testing.T) {
		d := New()
		before := d.currentLayout
		beforeLen := wasmLen(d)
		beforePtr := wasmPtr(d)
		// dst=250MiB, src=0 => total ~250MiB; requiredMem = hostSlotRegionBase + 250MiB > 256MiB.
		err := d.Reserve(250*1024*1024, 0)
		if err == nil {
			t.Fatalf("Reserve(250MiB,0) returned nil; want non-nil error (exceeds 4096-page module max)")
		}
		after := d.currentLayout
		if after != before {
			t.Errorf("slotLayout changed after rejected over-limit request: before=%+v after=%+v", before, after)
		}
		if wasmLen(d) != beforeLen {
			t.Errorf("wasm byte length changed after rejected over-limit request: before=%d after=%d", beforeLen, wasmLen(d))
		}
		if wasmPtr(d) != beforePtr {
			t.Errorf("wasm backing pointer changed after rejected over-limit request")
		}
	})

	// Directly assert the host memory limit used for growth is 4096 pages via
	// observable behavior, not the private maxMem field.
	t.Run("hostLimitIs4096Pages", func(t *testing.T) {
		d := New()
		// After New, initial wasm byte length must not exceed 4096 pages.
		if got := wasmLen(d); got > maxMemBytes {
			t.Fatalf("initial wasm byte length %d exceeds 4096-page limit %d", got, maxMemBytes)
		}

		// Request whose required layout fits within 4096 pages is accepted.
		// dst=200MiB, src=0 => total ~200MiB; requiredMem = 32MiB + 200MiB = 232MiB <= 256MiB.
		if err := d.Reserve(200*1024*1024, 0); err != nil {
			t.Errorf("Reserve(200MiB,0) rejected but layout fits within 4096 pages: %v", err)
		}

		// Request whose required layout is above 4096 pages is rejected.
		// dst=250MiB, src=0 => requiredMem ~282MiB (> 256MiB).
		d2 := New()
		if err := d2.Reserve(250*1024*1024, 0); err == nil {
			t.Errorf("Reserve(250MiB,0) accepted but required layout exceeds 4096-page limit")
		}
	})

	// Direct positive-int overflow per position: a request whose magnitude
	// exceeds math.MaxUint32 cannot be represented safely in uint32 slot
	// arithmetic and must be rejected, with all state unchanged, at each
	// request position independently.
	t.Run("rejects per-position int overflow above MaxUint32", func(t *testing.T) {
		if bits.UintSize <= 32 {
			t.Skipf("int is %d bits; no positive int greater than math.MaxUint32 is representable, so the per-position overflow path is exercised only on wider int", bits.UintSize)
		}
		// Build the value at runtime so it is a valid positive int on 64-bit
		// platforms and never a compile-time int overflow.
		tooLargeUint64 := uint64(math.MaxUint32)
		tooLargeUint64++
		tooLarge := int(tooLargeUint64)

		t.Run("destination only", func(t *testing.T) {
			d := New()
			before := d.currentLayout
			beforeLen := wasmLen(d)
			beforePtr := wasmPtr(d)
			if err := d.Reserve(tooLarge, 0); err == nil {
				t.Fatalf("Reserve(%d,0) returned nil on %d-bit int; want non-nil error", tooLarge, bits.UintSize)
			}
			assertReserveUnchanged(t, d, before, beforeLen, beforePtr)
		})

		t.Run("source only", func(t *testing.T) {
			d := New()
			before := d.currentLayout
			beforeLen := wasmLen(d)
			beforePtr := wasmPtr(d)
			if err := d.Reserve(0, tooLarge); err == nil {
				t.Fatalf("Reserve(0,%d) returned nil on %d-bit int; want non-nil error", tooLarge, bits.UintSize)
			}
			assertReserveUnchanged(t, d, before, beforeLen, beforePtr)
		})
	})

	// Repeated mixed requests on a single decoder prove the two reserved
	// capacities are independent and monotonic: growing one slot leaves the
	// other at its prior value, and no smaller/equal/zero/negative request
	// ever shrinks either capacity or perturbs state for no-growth calls.
	t.Run("monotonic independent capacities across mixed requests", func(t *testing.T) {
		d := New()
		prev := d.currentLayout
		steps := []struct {
			name         string
			dst, src     int
			grow         bool // false => request no growth; layout/identity must be preserved
			srcUnchanged bool // src must equal its value at call start
			dstUnchanged bool // dst must equal its value at call start
		}{
			{"grow dst only; src unchanged", 4 * 1024 * 1024, -1, true, true, false},
			{"grow src only; dst unchanged at prior grown value", -1, 4 * 1024 * 1024, true, false, true},
			{"smaller mixed", 1 * 1024 * 1024, 1 * 1024 * 1024, false, false, false},
			{"equal mixed", 4 * 1024 * 1024, 4 * 1024 * 1024, false, false, false},
			{"zero mixed", 0, 0, false, false, false},
			{"negative mixed", -100, -100, false, false, false},
		}
		for _, s := range steps {
			t.Run(s.name, func(t *testing.T) {
				before := d.currentLayout
				beforeLen := wasmLen(d)
				beforePtr := wasmPtr(d)
				if err := d.Reserve(s.dst, s.src); err != nil {
					t.Fatalf("Reserve(%d,%d) returned error: %v", s.dst, s.src, err)
				}
				after := d.currentLayout

				// Neither capacity may decrease within the call.
				if after.srcLen < before.srcLen {
					t.Errorf("SrcLen decreased within call: before=%d after=%d", before.srcLen, after.srcLen)
				}
				if after.dstLen < before.dstLen {
					t.Errorf("DstLen decreased within call: before=%d after=%d", before.dstLen, after.dstLen)
				}

				// Grow-only steps must leave the untouched slot exactly unchanged.
				if s.srcUnchanged && after.srcLen != before.srcLen {
					t.Errorf("SrcLen changed on dst-only grow: before=%d after=%d", before.srcLen, after.srcLen)
				}
				if s.dstUnchanged && after.dstLen != before.dstLen {
					t.Errorf("DstLen changed on src-only grow: before=%d after=%d", before.dstLen, after.dstLen)
				}

				// No-growth requests preserve full layout and wasm identity.
				if !s.grow {
					assertReserveUnchanged(t, d, before, beforeLen, beforePtr)
				}

				// Installed slots remain ordered, non-overlapping, in bounds.
				assertSlotsValid(t, d, after)
			})

			// Across the whole sequence neither reserved capacity may decrease.
			cur := d.currentLayout
			if cur.srcLen < prev.srcLen {
				t.Errorf("SrcLen decreased across sequence at %q: prev=%d cur=%d", s.name, prev.srcLen, cur.srcLen)
			}
			if cur.dstLen < prev.dstLen {
				t.Errorf("DstLen decreased across sequence at %q: prev=%d cur=%d", s.name, prev.dstLen, cur.dstLen)
			}
			prev = cur
		}
	})
}
