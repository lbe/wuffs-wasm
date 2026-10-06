package wuffs_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// TestIntegrationFrameCountStillImage verifies the FrameCount contract on
// still images: a single-frame PNG and a single-frame GIF both report 1,
// empty input fails with ErrDecode, input over the reserved src-slot capacity
// fails with ErrSrcTooLarge, and successful calls leave guest memory and the
// slot layout unchanged.
func TestIntegrationFrameCountStillImage(t *testing.T) {
	load := func(t *testing.T, name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("reading testdata/%s: %v", name, err)
		}
		return data
	}

	t.Run("PNG still image reports one frame", func(t *testing.T) {
		d := wuffs.New()
		src := load(t, "bricks-nodither.png")
		before := wuffs.CaptureGuestMemoryState(d)
		n, err := d.FrameCount(src)
		if err != nil {
			t.Fatalf("FrameCount(bricks-nodither.png) error = %v, want nil", err)
		}
		if n != 1 {
			t.Errorf("FrameCount(bricks-nodither.png) = %d, want 1", n)
		}
		assertGuestMemoryPreserved(t, "FrameCount", "bricks-nodither.png", d, before)
	})

	t.Run("GIF still image reports one frame", func(t *testing.T) {
		d := wuffs.New()
		src := load(t, "bricks-nodither.gif")
		before := wuffs.CaptureGuestMemoryState(d)
		n, err := d.FrameCount(src)
		if err != nil {
			t.Fatalf("FrameCount(bricks-nodither.gif) error = %v, want nil", err)
		}
		if n != 1 {
			t.Errorf("FrameCount(bricks-nodither.gif) = %d, want 1", n)
		}
		assertGuestMemoryPreserved(t, "FrameCount", "bricks-nodither.gif", d, before)
	})

	t.Run("empty source returns ErrDecode", func(t *testing.T) {
		d := wuffs.New()
		before := wuffs.CaptureGuestMemoryState(d)
		n, err := d.FrameCount(nil)
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Fatalf("FrameCount(nil) error = %v, want errors.Is(err, ErrDecode)", err)
		}
		if n != 0 {
			t.Errorf("FrameCount(nil) = %d, want 0", n)
		}
		assertGuestMemoryPreserved(t, "FrameCount", "empty", d, before)
	})

	t.Run("oversize source returns ErrSrcTooLarge", func(t *testing.T) {
		d := wuffs.New()
		src := make([]byte, 64*1024+1)
		before := wuffs.CaptureGuestMemoryState(d)
		n, err := d.FrameCount(src)
		if !errors.Is(err, wuffs.ErrSrcTooLarge) {
			t.Fatalf("FrameCount(oversize) error = %v, want errors.Is(err, ErrSrcTooLarge)", err)
		}
		if n != 0 {
			t.Errorf("FrameCount(oversize) = %d, want 0", n)
		}
		assertGuestMemoryPreserved(t, "FrameCount", "oversize", d, before)
	})
}
