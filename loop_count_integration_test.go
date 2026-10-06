package wuffs_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// Recorded Wuffs num_animation_loops for testdata/muybridge.gif is 0:
// the file is animated (FrameCount 15) and 0 means loop forever per API.md.
// Pinned from the first successful GREEN run (2026-10-05).
const muybridgeLoopCount uint32 = 0

// TestIntegrationLoopCountCharacterization verifies the LoopCount contract:
// still fixtures report 0, muybridge.gif reports Wuffs loop metadata, empty
// input fails with ErrDecode, oversize input fails with ErrSrcTooLarge, and
// successful calls leave guest memory and the slot layout unchanged.
func TestIntegrationLoopCountCharacterization(t *testing.T) {
	load := func(t *testing.T, name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("reading testdata/%s: %v", name, err)
		}
		return data
	}

	still := []string{
		"bricks-nodither.png",
		"bricks-color.lossless.webp",
		"bricks-nodither.gif",
	}
	for _, name := range still {
		t.Run("still "+name+" reports zero loops", func(t *testing.T) {
			d := wuffs.New()
			src := load(t, name)
			before := wuffs.CaptureGuestMemoryState(d)
			n, err := d.LoopCount(src)
			if err != nil {
				t.Fatalf("LoopCount(%s) error = %v, want nil", name, err)
			}
			if n != 0 {
				t.Errorf("LoopCount(%s) = %d, want 0", name, n)
			}
			assertGuestMemoryPreserved(t, "LoopCount", name, d, before)
		})
	}

	t.Run("muybridge.gif reports recorded loop count", func(t *testing.T) {
		d := wuffs.New()
		src := load(t, "muybridge.gif")
		before := wuffs.CaptureGuestMemoryState(d)
		n, err := d.LoopCount(src)
		if err != nil {
			t.Fatalf("LoopCount(muybridge.gif) error = %v, want nil", err)
		}
		if n != muybridgeLoopCount {
			t.Errorf("LoopCount(muybridge.gif) = %d, want %d", n, muybridgeLoopCount)
		}
		assertGuestMemoryPreserved(t, "LoopCount", "muybridge.gif", d, before)
	})

	t.Run("empty source returns ErrDecode", func(t *testing.T) {
		d := wuffs.New()
		before := wuffs.CaptureGuestMemoryState(d)
		n, err := d.LoopCount(nil)
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Fatalf("LoopCount(nil) error = %v, want errors.Is(err, ErrDecode)", err)
		}
		if n != 0 {
			t.Errorf("LoopCount(nil) = %d, want 0", n)
		}
		assertGuestMemoryPreserved(t, "LoopCount", "empty", d, before)
	})

	t.Run("oversize source returns ErrSrcTooLarge", func(t *testing.T) {
		d := wuffs.New()
		src := make([]byte, 64*1024+1)
		before := wuffs.CaptureGuestMemoryState(d)
		n, err := d.LoopCount(src)
		if !errors.Is(err, wuffs.ErrSrcTooLarge) {
			t.Fatalf("LoopCount(oversize) error = %v, want errors.Is(err, ErrSrcTooLarge)", err)
		}
		if n != 0 {
			t.Errorf("LoopCount(oversize) = %d, want 0", n)
		}
		assertGuestMemoryPreserved(t, "LoopCount", "oversize", d, before)
	})
}
