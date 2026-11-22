package wuffs_test

import (
	"bytes"
	"image"
	"testing"

	wuffs "github.com/lbe/wuffs-wasm"
)

// TestIntegrationTrimThenDecodeMatches decodes a fixture, trims the decoder,
// decodes again, and requires identical pixels: Trim releases only per-call
// guest pages, so the second decode must not depend on them.
func TestIntegrationTrimThenDecodeMatches(t *testing.T) {
	fixtures := []string{
		"bricks-color.png",
		"bricks-color.bmp",
		"bricks-nodither.gif",
		"bricks-color.lossless.webp",
		"bricks-color.qoi",
		"bricks-color.tga",
		"bricks-nodither.wbmp",
		"bricks-color.etc2.pkm",
		"bricks-color.c3q4.handsum",
		"crude-flag.nie",
		"mona-lisa.21x32.th",
		"hat.jpeg",
		"hippopotamus.ppm",
	}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			src := loadFixture(t, name)
			d := wuffs.New()
			defer d.Close()
			m := reserveForDecode(t, d, src)
			rect := image.Rect(0, 0, int(m.Width), int(m.Height))

			first := image.NewRGBA(rect)
			if _, err := d.DecodeRGBA(first, src); err != nil {
				t.Fatalf("first DecodeRGBA: %v", err)
			}
			d.Trim()
			second := image.NewRGBA(rect)
			if _, err := d.DecodeRGBA(second, src); err != nil {
				t.Fatalf("DecodeRGBA after Trim: %v", err)
			}
			if !bytes.Equal(first.Pix, second.Pix) {
				t.Fatal("pixels after Trim differ from the first decode")
			}
		})
	}
}

// TestIntegrationTrimBetweenAnimationFrames decodes every frame of an animation
// twice with a Trim in between and requires identical pixels and Frame metadata.
// The *Frame returned by DecodeFrame aliases decoder storage, so the first
// result is copied before the second call.
func TestIntegrationTrimBetweenAnimationFrames(t *testing.T) {
	for _, name := range []string{"animated-red-blue.gif", "animated-red-blue.nia"} {
		t.Run(name, func(t *testing.T) {
			src := loadFixture(t, name)
			d := wuffs.New()
			defer d.Close()
			m := reserveForDecode(t, d, src)
			rect := image.Rect(0, 0, int(m.Width), int(m.Height))

			n, err := d.FrameCount(src)
			if err != nil {
				t.Fatalf("FrameCount: %v", err)
			}
			if n < 2 {
				t.Fatalf("FrameCount = %d, want at least 2 frames", n)
			}
			for i := range n {
				dstA := image.NewRGBA(rect)
				fA, err := d.DecodeFrame(dstA, src, i)
				if err != nil {
					t.Fatalf("DecodeFrame(%d) before Trim: %v", i, err)
				}
				frameA := *fA

				d.Trim()

				dstB := image.NewRGBA(rect)
				fB, err := d.DecodeFrame(dstB, src, i)
				if err != nil {
					t.Fatalf("DecodeFrame(%d) after Trim: %v", i, err)
				}
				if !bytes.Equal(dstA.Pix, dstB.Pix) {
					t.Fatalf("frame %d pixels after Trim differ from the first decode", i)
				}
				if frameA != *fB {
					t.Fatalf("frame %d metadata after Trim = %+v, want %+v", i, *fB, frameA)
				}
			}
		})
	}
}
