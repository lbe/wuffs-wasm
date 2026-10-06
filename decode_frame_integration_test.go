package wuffs

// Task 4 RED test: core DecodeFrame contract on a still GIF plus the first
// animated frame oracle. Fails until the guest wuffs_decode_frame export and
// host (*Decoder).DecodeFrame exist.

import (
	"bytes"
	"errors"
	"hash/crc32"
	"image"
	"testing"
)

// animatedFrame1DeltaCRC is the CRC32-IEEE of the Frame.Bounds
// sub-rectangle (derived from Frame.Bounds at runtime) of frame 1 of
// testdata/animated-red-blue.gif decoded with a fresh zeroed canvas: the
// indexed frame's own (delta) pixels in straight RGBA row-major bytes,
// not a composed canvas. Pinned from the delta-at-Bounds GREEN run; guest
// reports Duration 200ms, DisposalNone, Overwrite false, canvas 64x48.
const animatedFrame1DeltaCRC uint32 = 0xA1BFE543

// TestIntegrationDecodeFrameCore verifies the DecodeFrame core contract:
//
//   - On bricks-nodither.gif after Probe + Reserve, DecodeFrame(dst, src, 0)
//     returns a non-nil *Frame with Index 0, full-canvas Bounds
//     (0,0)-(160,120), zero Duration (still GIF), and DisposalNone.
//   - dst pixels match the existing PNG oracle (same assertion approach as
//     decode_gif_integration_test.go).
//   - dst.Pix pointer / len / cap / Rect / Stride identity is preserved.
//   - index == 1 when FrameCount == 1 fails with errors.Is(err, ErrDecode).
//   - a negative index fails with ErrDecode.
//   - On animated-red-blue.gif: FrameCount == 4; DecodeFrame(dst, src, 1)
//     succeeds with Frame.Index == 1, non-full-canvas Bounds, and pixels
//     inside Bounds matching the pinned CRC constant.
func TestIntegrationDecodeFrameCore(t *testing.T) {
	const (
		wantWidth  = 160
		wantHeight = 120
		wantStride = 640
		wantBytes  = 76800
	)

	t.Run("still GIF frame 0 matches PNG oracle and preserves dst", func(t *testing.T) {
		d := New()
		gifSrc := mustReadFixture(t, "bricks-nodither.gif")
		pngSrc := mustReadFixture(t, "bricks-nodither.png")

		probe, err := d.Probe(gifSrc)
		if err != nil {
			t.Fatalf("Probe(bricks-nodither.gif) error = %v, want nil", err)
		}
		if probe == nil || probe.Width != wantWidth || probe.Height != wantHeight {
			t.Fatalf("Probe Meta = %+v, want {W:%d H:%d}", probe, wantWidth, wantHeight)
		}
		if reserveErr := d.Reserve(wantBytes, len(gifSrc)); reserveErr != nil {
			t.Fatalf("Reserve(%d, %d) error = %v, want nil", wantBytes, len(gifSrc), reserveErr)
		}

		dst := image.NewRGBA(image.Rect(0, 0, wantWidth, wantHeight))
		pixPtr := &dst.Pix[0]
		pixLen, pixCap := len(dst.Pix), cap(dst.Pix)
		rect, dstStride := dst.Rect, dst.Stride

		frame, err := d.DecodeFrame(dst, gifSrc, 0)
		if err != nil {
			t.Fatalf("DecodeFrame(bricks-nodither.gif, 0) error = %v, want nil", err)
		}
		if frame == nil {
			t.Fatal("DecodeFrame(bricks-nodither.gif, 0) returned nil Frame, want non-nil")
		}
		if frame.Index != 0 {
			t.Errorf("Frame.Index = %d, want 0", frame.Index)
		}
		wantBounds := image.Rect(0, 0, wantWidth, wantHeight)
		if frame.Bounds != wantBounds {
			t.Errorf("Frame.Bounds = %v, want %v", frame.Bounds, wantBounds)
		}
		if frame.Duration != 0 {
			t.Errorf("Frame.Duration = %v, want 0 (still GIF)", frame.Duration)
		}
		if frame.Disposal != DisposalNone {
			t.Errorf("Frame.Disposal = %d, want DisposalNone", frame.Disposal)
		}

		// Oracle: the same canvas decoded from the pixel-identical PNG fixture.
		oracle := image.NewRGBA(image.Rect(0, 0, wantWidth, wantHeight))
		if _, err := New().DecodeRGBA(oracle, pngSrc); err != nil {
			t.Fatalf("PNG oracle DecodeRGBA error = %v, want nil", err)
		}
		if !bytes.Equal(dst.Pix, oracle.Pix) {
			t.Error("DecodeFrame(bricks-nodither.gif, 0) pixels differ from the bricks-nodither.png oracle")
		}

		if &dst.Pix[0] != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != dstStride {
			t.Fatal("DecodeFrame changed the caller-owned destination layout")
		}
	})

	t.Run("index past single frame returns ErrDecode", func(t *testing.T) {
		d := New()
		src := mustReadFixture(t, "bricks-nodither.gif")
		n, err := d.FrameCount(src)
		if err != nil {
			t.Fatalf("FrameCount(bricks-nodither.gif) error = %v, want nil", err)
		}
		if n != 1 {
			t.Fatalf("FrameCount(bricks-nodither.gif) = %d, want 1", n)
		}
		if err := d.Reserve(wantBytes, len(src)); err != nil {
			t.Fatalf("Reserve error = %v, want nil", err)
		}
		dst := image.NewRGBA(image.Rect(0, 0, wantWidth, wantHeight))
		if _, err := d.DecodeFrame(dst, src, 1); !errors.Is(err, ErrDecode) {
			t.Errorf("DecodeFrame(src, 1) error = %v, want errors.Is(err, ErrDecode)", err)
		}
	})

	t.Run("negative index returns ErrDecode", func(t *testing.T) {
		d := New()
		src := mustReadFixture(t, "bricks-nodither.gif")
		if err := d.Reserve(wantBytes, len(src)); err != nil {
			t.Fatalf("Reserve error = %v, want nil", err)
		}
		dst := image.NewRGBA(image.Rect(0, 0, wantWidth, wantHeight))
		if _, err := d.DecodeFrame(dst, src, -1); !errors.Is(err, ErrDecode) {
			t.Errorf("DecodeFrame(src, -1) error = %v, want errors.Is(err, ErrDecode)", err)
		}
	})

	t.Run("animated GIF frame 1 has sub-canvas bounds and pinned CRC", func(t *testing.T) {
		d := New()
		src := mustReadFixture(t, "animated-red-blue.gif")

		n, err := d.FrameCount(src)
		if err != nil {
			t.Fatalf("FrameCount(animated-red-blue.gif) error = %v, want nil", err)
		}
		if n != 4 {
			t.Fatalf("FrameCount(animated-red-blue.gif) = %d, want 4", n)
		}

		probe, err := d.Probe(src)
		if err != nil {
			t.Fatalf("Probe(animated-red-blue.gif) error = %v, want nil", err)
		}
		canvasW, canvasH := int(probe.Width), int(probe.Height)
		if reserveErr := d.Reserve(int(probe.Stride)*canvasH, len(src)); reserveErr != nil {
			t.Fatalf("Reserve error = %v, want nil", reserveErr)
		}

		dst := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
		frame, err := d.DecodeFrame(dst, src, 1)
		if err != nil {
			t.Fatalf("DecodeFrame(animated-red-blue.gif, 1) error = %v, want nil", err)
		}
		if frame == nil {
			t.Fatal("DecodeFrame(animated-red-blue.gif, 1) returned nil Frame, want non-nil")
		}
		if frame.Index != 1 {
			t.Errorf("Frame.Index = %d, want 1", frame.Index)
		}
		full := image.Rect(0, 0, canvasW, canvasH)
		if frame.Bounds == full {
			t.Errorf("Frame.Bounds = %v, want non-full-canvas bounds", frame.Bounds)
		}
		if !frame.Bounds.In(full) {
			t.Errorf("Frame.Bounds = %v not inside canvas %v", frame.Bounds, full)
		}

		// CRC over straight-RGBA row-major bytes strictly inside Bounds.
		b := frame.Bounds
		var raw []byte
		for y := b.Min.Y; y < b.Max.Y; y++ {
			off := y*dst.Stride + b.Min.X*4
			raw = append(raw, dst.Pix[off:off+b.Dx()*4]...)
		}
		if got := crc32.ChecksumIEEE(raw); got != animatedFrame1DeltaCRC {
			t.Errorf("frame-1 sub-rect CRC = 0x%08X, want pinned 0x%08X", got, animatedFrame1DeltaCRC)
		}
	})
}
