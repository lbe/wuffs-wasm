package wuffs

import (
	"encoding/binary"
	"errors"
	"image"
	"math/big"
	"reflect"
	"testing"
	"time"
	"unsafe"
)

// TestIntegrationSourceSlotDefinesDecoderCapacity verifies that a fresh
// Decoder's source capacity is defined by currentLayout.srcLen (64 KiB) and
// that both Probe and DecodeRGBA reject a source one byte larger than that
// slot with ErrSrcTooLarge before invoking the guest. The rejection must leave
// the complete slotLayout, the wasm byte length, and the wasm slice backing
// pointer untouched — proving the decoder holds no independent source-capacity
// state that can disagree with currentLayout.srcLen.
func TestIntegrationSourceSlotDefinesDecoderCapacity(t *testing.T) {
	const wantSrcLen = 64 * 1024

	d := New()

	// A fresh decoder must report a 64 KiB source slot.
	layout := d.currentLayout
	if layout.srcLen != wantSrcLen {
		t.Fatalf("fresh currentLayout.srcLen = %d, want %d", layout.srcLen, wantSrcLen)
	}

	// A source exactly one byte larger than the source slot must be rejected.
	src := make([]byte, wantSrcLen+1)

	cases := []struct {
		name string
		run  func() (*Meta, error)
	}{
		{
			name: "Probe",
			run:  func() (*Meta, error) { return d.Probe(src) },
		},
		{
			name: "DecodeRGBA",
			run: func() (*Meta, error) {
				dst := &image.RGBA{Rect: image.Rect(0, 0, 1, 1), Stride: 4, Pix: make([]byte, 4)}
				return d.DecodeRGBA(dst, src)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture the full slot layout, wasm byte length, and backing
			// pointer before the rejected call.
			preSlice := d.module.Xmemory().Slice()
			preLen := len(*preSlice)
			prePtr := unsafe.Pointer(&(*preSlice)[0])
			preLayout := d.currentLayout

			meta, err := tc.run()
			if !errors.Is(err, ErrSrcTooLarge) {
				t.Fatalf("%s(SrcLen+1) err = %v, want errors.Is(err, ErrSrcTooLarge)", tc.name, err)
			}
			if meta != nil {
				t.Errorf("%s(SrcLen+1) meta = %v, want nil", tc.name, meta)
			}

			// The rejection must not mutate wasm memory: the slot layout, the
			// wasm byte length, and the backing slice pointer must be identical.
			postSlice := d.module.Xmemory().Slice()
			postLen := len(*postSlice)
			postPtr := unsafe.Pointer(&(*postSlice)[0])
			postLayout := d.currentLayout

			if postLayout != preLayout {
				t.Errorf("slotLayout changed after %s rejection:\n pre = %+v\n post = %+v", tc.name, preLayout, postLayout)
			}
			if postLen != preLen {
				t.Errorf("wasm byte length after %s rejection = %d, want %d", tc.name, postLen, preLen)
			}
			if postPtr != prePtr {
				t.Errorf("wasm backing pointer after %s rejection = %p, want %p", tc.name, postPtr, prePtr)
			}
		})
	}
}

// TestIntegrationSourceCapacityUsesCurrentLayout proves that currentLayout.srcLen
// is the sole source-capacity authority: the Decoder struct holds no independent
// srcCap field, and shrinking only currentLayout.srcLen causes a source of
// SrcLen+1 bytes to be rejected with ErrSrcTooLarge before any guest call, while
// leaving the complete slotLayout, the wasm byte length, and the wasm backing
// pointer untouched.
func TestIntegrationSourceCapacityUsesCurrentLayout(t *testing.T) {
	t.Run("Decoder has no independent srcCap field", func(t *testing.T) {
		// The decoder must not retain a duplicate source-capacity field that
		// can disagree with currentLayout.srcLen.
		decType := reflect.TypeOf(Decoder{})
		if f, ok := decType.FieldByName("srcCap"); ok {
			t.Fatalf("Decoder still carries independent srcCap field %+v; SrcLen must be the sole capacity authority", f)
		}
	})

	t.Run("currentLayout.srcLen controls rejection", func(t *testing.T) {
		// A small positive source slot, well below the old 64 KiB default.
		const reducedSrcLen = 16

		d := New()
		// Reduce ONLY currentLayout.srcLen. No Reserve, so the guest memory
		// layout is otherwise unchanged and the old 64 KiB capacity would have
		// accepted this source.
		d.currentLayout.srcLen = reducedSrcLen

		// Capture the full slot layout, wasm byte length, and backing pointer
		// before the rejected call.
		preSlice := d.module.Xmemory().Slice()
		preLen := len(*preSlice)
		prePtr := unsafe.Pointer(unsafe.SliceData(*preSlice))
		preLayout := d.currentLayout

		// A source one byte larger than the reduced slot, still far below the
		// old 64 KiB capacity. It must be rejected with ErrSrcTooLarge and the
		// guest must not be invoked.
		src := make([]byte, reducedSrcLen+1)
		meta, err := d.Probe(src)
		if !errors.Is(err, ErrSrcTooLarge) {
			t.Fatalf("Probe(SrcLen+1) err = %v, want errors.Is(err, ErrSrcTooLarge)", err)
		}
		if meta != nil {
			t.Errorf("Probe(SrcLen+1) meta = %v, want nil", meta)
		}

		// The rejection must not mutate wasm memory.
		postSlice := d.module.Xmemory().Slice()
		postLen := len(*postSlice)
		postPtr := unsafe.Pointer(unsafe.SliceData(*postSlice))
		postLayout := d.currentLayout

		if postLayout != preLayout {
			t.Errorf("slotLayout changed after rejection:\n pre = %+v\n post = %+v", preLayout, postLayout)
		}
		if postLen != preLen {
			t.Errorf("wasm byte length after rejection = %d, want %d", postLen, preLen)
		}
		if postPtr != prePtr {
			t.Errorf("wasm backing pointer after rejection = %p, want %p", postPtr, prePtr)
		}
	})
}

// TestUnitFrameMetaDuration verifies that readFrameMeta converts Wuffs flicks
// (1/705_600_000 s) to time.Duration by truncating to whole nanoseconds
// without intermediate overflow, and returns ErrDecode for the first flick
// count whose duration exceeds math.MaxInt64 nanoseconds.
func TestUnitFrameMetaDuration(t *testing.T) {
	const flicksPerSecond = 705_600_000

	// maxFlicks is the greatest flick count whose truncated nanosecond value
	// fits time.Duration: floor((MaxInt64+1) * flicksPerSecond / 1e9), as
	// (MaxInt64+1) * flicksPerSecond is not a multiple of 1e9.
	limit := new(big.Int).Lsh(big.NewInt(1), 63)
	limit.Mul(limit, big.NewInt(flicksPerSecond))
	limit.Quo(limit, big.NewInt(int64(time.Second)))
	maxFlicks := limit.Uint64()
	// truncated returns floor(flicks * 1e9 / flicksPerSecond) computed exactly.
	truncated := func(flicks uint64) time.Duration {
		n := new(big.Int).SetUint64(flicks)
		n.Mul(n, big.NewInt(int64(time.Second)))
		n.Quo(n, big.NewInt(flicksPerSecond))
		return time.Duration(n.Int64())
	}

	tests := []struct {
		name    string
		flicks  uint64
		want    time.Duration
		wantErr error
	}{
		{name: "zero", flicks: 0, want: 0},
		{name: "one flick truncates to 1ns", flicks: 1, want: 1},
		{name: "100ms", flicks: flicksPerSecond / 10, want: 100 * time.Millisecond},
		{name: "20s", flicks: 20 * flicksPerSecond, want: 20 * time.Second},
		{name: "GIF maximum delay 655.35s", flicks: 65535 * (flicksPerSecond / 100), want: 655350 * time.Millisecond},
		{name: "greatest representable duration", flicks: maxFlicks, want: truncated(maxFlicks)},
		{name: "first unrepresentable flick", flicks: maxFlicks + 1, wantErr: ErrDecode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := make([]byte, frameMetaSlotBytes)
			binary.LittleEndian.PutUint64(mem[24:32], tt.flicks)
			frame, err := readFrameMeta(mem, 0)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("readFrameMeta(%d flicks) error = %v, want %v", tt.flicks, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("readFrameMeta(%d flicks) error = %v, want nil", tt.flicks, err)
			}
			if frame.Duration != tt.want {
				t.Errorf("readFrameMeta(%d flicks).Duration = %d, want %d", tt.flicks, frame.Duration, tt.want)
			}
		})
	}
}
