package wuffs_test

// NIE nïA multi-frame characterization against
// testdata/nie.animation.golden.manifest: pins FrameCount and per-frame
// full-canvas CRCs for animated-red-blue.nia, with a frame-zero cross-check
// against the FORMAT-03 evidence in testdata/nie.golden.manifest.

import (
	"hash/crc32"
	"image"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/lbe/wuffs-wasm"
)

// nieFrameZeroCanonicalCRC is the animated-red-blue.nia frame-zero CRC from
// the existing testdata/nie.golden.manifest (nia-bn4 line): the full-canvas
// straight RGBA CRC that frame-zero decode must keep matching.
const nieFrameZeroCanonicalCRC = 4230696731

// TestIntegrationNIEAnimationCharacterization verifies NIE nïA multi-frame
// behavior: animated-red-blue.nia reports 4 frames, every frame decodes on a
// fresh zeroed canvas to its pinned CRC in nie.animation.golden.manifest,
// and frame 0 still matches the FORMAT-03 NIE evidence (4230696731).
// Every successful decode preserves caller Pix identity and guest memory.
func TestIntegrationNIEAnimationCharacterization(t *testing.T) {
	manifest := loadAnimationManifest(t, "nie.animation.golden.manifest")

	t.Run("animated-red-blue.nia has 4 frames", func(t *testing.T) {
		d := wuffs.New()
		src, err := os.ReadFile(filepath.Join("testdata", "animated-red-blue.nia"))
		if err != nil {
			t.Fatalf("reading testdata/animated-red-blue.nia: %v", err)
		}
		before := wuffs.CaptureGuestMemoryState(d)
		n, err := d.FrameCount(src)
		if err != nil {
			t.Fatalf("FrameCount(animated-red-blue.nia) error = %v, want nil", err)
		}
		if n != 4 {
			t.Errorf("FrameCount(animated-red-blue.nia) = %d, want 4", n)
		}
		assertGuestMemoryPreserved(t, "FrameCount", "animated-red-blue.nia", d, before)
	})

	t.Run("per-frame CRCs match manifest", func(t *testing.T) {
		src, err := os.ReadFile(filepath.Join("testdata", "animated-red-blue.nia"))
		if err != nil {
			t.Fatalf("reading testdata/animated-red-blue.nia: %v", err)
		}
		d := wuffs.New()
		probe, err := d.Probe(src)
		if err != nil {
			t.Fatalf("Probe(animated-red-blue.nia) error = %v, want nil", err)
		}
		w, h := int(probe.Width), int(probe.Height)
		if w != 64 || h != 48 {
			t.Fatalf("Probe(animated-red-blue.nia) = %dx%d, want 64x48", w, h)
		}
		if err := d.Reserve(int(probe.Stride)*h, len(src)); err != nil {
			t.Fatalf("Reserve error = %v, want nil", err)
		}
		for i := 0; i < 4; i++ {
			dst := image.NewRGBA(image.Rect(0, 0, w, h))
			pixPtr := unsafe.SliceData(dst.Pix)
			pixLen, pixCap := len(dst.Pix), cap(dst.Pix)
			rect, stride := dst.Rect, dst.Stride
			before := wuffs.CaptureGuestMemoryState(d)
			frame, err := d.DecodeFrame(dst, src, i)
			if err != nil {
				t.Fatalf("DecodeFrame(animated-red-blue.nia, %d) error = %v, want nil", i, err)
			}
			if frame == nil {
				t.Fatalf("DecodeFrame(animated-red-blue.nia, %d) returned nil Frame", i)
			}
			if frame.Index != i {
				t.Errorf("frame %d: Frame.Index = %d, want %d", i, frame.Index, i)
			}
			want, ok := manifest["animated-red-blue.nia"][i]
			if !ok {
				t.Fatalf("frame %d: no manifest entry for animated-red-blue.nia", i)
			}
			if got := crc32.ChecksumIEEE(dst.Pix); got != want {
				t.Errorf("frame %d: full-canvas CRC = 0x%08X, want manifest 0x%08X", i, got, want)
			}
			if unsafe.SliceData(dst.Pix) != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != stride {
				t.Errorf("frame %d: DecodeFrame changed the caller-owned destination layout", i)
			}
			assertGuestMemoryPreserved(t, "DecodeFrame", "animated-red-blue.nia", d, before)
		}
	})

	t.Run("frame-0 matches FORMAT-03 NIE evidence", func(t *testing.T) {
		src, err := os.ReadFile(filepath.Join("testdata", "animated-red-blue.nia"))
		if err != nil {
			t.Fatalf("reading testdata/animated-red-blue.nia: %v", err)
		}
		d := wuffs.New()
		probe, err := d.Probe(src)
		if err != nil {
			t.Fatalf("Probe(animated-red-blue.nia) error = %v, want nil", err)
		}
		w, h := int(probe.Width), int(probe.Height)
		if reserveErr := d.Reserve(int(probe.Stride)*h, len(src)); reserveErr != nil {
			t.Fatalf("Reserve error = %v, want nil", reserveErr)
		}
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		pixPtr := unsafe.SliceData(dst.Pix)
		pixLen, pixCap := len(dst.Pix), cap(dst.Pix)
		rect, stride := dst.Rect, dst.Stride
		before := wuffs.CaptureGuestMemoryState(d)
		frame, err := d.DecodeFrame(dst, src, 0)
		if err != nil {
			t.Fatalf("DecodeFrame(animated-red-blue.nia, 0) error = %v, want nil", err)
		}
		if frame == nil {
			t.Fatal("DecodeFrame(animated-red-blue.nia, 0) returned nil Frame")
		}
		if frame.Index != 0 {
			t.Errorf("Frame.Index = %d, want 0", frame.Index)
		}
		if got := crc32.ChecksumIEEE(dst.Pix); got != nieFrameZeroCanonicalCRC {
			t.Errorf("frame 0: full-canvas CRC = %d, want nie.golden.manifest evidence %d", got, uint32(nieFrameZeroCanonicalCRC))
		}
		if unsafe.SliceData(dst.Pix) != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != stride {
			t.Error("frame 0: DecodeFrame changed the caller-owned destination layout")
		}
		assertGuestMemoryPreserved(t, "DecodeFrame", "animated-red-blue.nia", d, before)
	})
}
