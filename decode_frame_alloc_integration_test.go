package wuffs_test

import (
	"image"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// TestIntegrationDecodeFrameAllocsPerRun measures heap allocations per
// DecodeFrame, FrameCount, and LoopCount call after RequiredReserve has sized
// the wasm memory slots and warm-up calls have been performed. The target is
// exactly zero heap allocations on each hot path: destinations are allocated
// once outside the timed loops and reused.
func TestIntegrationDecodeFrameAllocsPerRun(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-nodither.png")
	gifSrc := loadFixture(t, "muybridge.gif")

	d := wuffs.New()

	// Size reserve for the bricks-nodither.png fixture: 160x120 RGBA.
	const (
		wantW    = 160
		wantH    = 120
		dstBytes = wantW * wantH * 4
	)
	srcBytes := len(pngSrc)
	if len(gifSrc) > srcBytes {
		srcBytes = len(gifSrc)
	}
	if resErr := wuffs.RequiredReserve(d, dstBytes, srcBytes); resErr != nil {
		t.Fatalf("RequiredReserve: %v", resErr)
	}

	// One correctly sized destination, allocated outside the timed loop.
	dst := image.NewRGBA(image.Rect(0, 0, wantW, wantH))

	// Warm up so any one-time setup (e.g. d.lastMeta/lastFrame aliasing) is
	// complete before measurement.
	if _, err := d.DecodeFrame(dst, pngSrc, 0); err != nil {
		t.Fatalf("warm-up DecodeFrame: %v", err)
	}
	if _, err := d.FrameCount(gifSrc); err != nil {
		t.Fatalf("warm-up FrameCount: %v", err)
	}
	if _, err := d.LoopCount(gifSrc); err != nil {
		t.Fatalf("warm-up LoopCount: %v", err)
	}

	allocs := testing.AllocsPerRun(5, func() {
		if _, err := d.DecodeFrame(dst, pngSrc, 0); err != nil {
			t.Errorf("DecodeFrame: %v", err)
		}
	})

	t.Logf("allocs per DecodeFrame: %.0f (target: 0)", allocs)

	if allocs != 0 {
		t.Errorf("DecodeFrame allocated %.0f heap objects per run, want 0", allocs)
	}

	frameCountAllocs := testing.AllocsPerRun(5, func() {
		if _, err := d.FrameCount(gifSrc); err != nil {
			t.Errorf("FrameCount: %v", err)
		}
	})

	t.Logf("allocs per FrameCount: %.0f (target: 0)", frameCountAllocs)

	if frameCountAllocs != 0 {
		t.Errorf("FrameCount allocated %.0f heap objects per run, want 0", frameCountAllocs)
	}

	loopCountAllocs := testing.AllocsPerRun(5, func() {
		if _, err := d.LoopCount(gifSrc); err != nil {
			t.Errorf("LoopCount: %v", err)
		}
	})

	t.Logf("allocs per LoopCount: %.0f (target: 0)", loopCountAllocs)

	if loopCountAllocs != 0 {
		t.Errorf("LoopCount allocated %.0f heap objects per run, want 0", loopCountAllocs)
	}
}
