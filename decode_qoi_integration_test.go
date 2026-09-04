package wuffs_test

import (
	"bytes"
	"errors"
	"image"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// TestIntegrationQOIDecodeCharacterization verifies the QOI (Quite OK Image)
// format contract end-to-end against the live wasm guest:
//
//   - The vendored bricks-color.qoi fixture (RGB mode, 160×120, sRGB
//     colorspace) probes and decodes as FormatQOI with the expected geometry,
//     and DecodeRGBA output is pixel-identical to the existing bricks-color.png
//     oracle while preserving the caller-owned destination layout.
//   - The in-repository alpha-pixels.qoi fixture (RGBA mode, 3×1, straight
//     non-premultiplied pixels) decodes to exact fixed pixels: opaque,
//     translucent, and fully transparent.
//   - A deterministic truncation of the bricks-color.qoi pixel stream (the
//     corrupt QOI) still probes as 160×120 FormatQOI, but DecodeRGBA fails
//     with errors.Is(err, ErrDecode) (and not ErrUnknownFormat), leaving a
//     prefilled sentinel destination byte-for-byte untouched.
//
// The format is asserted via the local uint32 FourCC literal rather than the
// exported constant so this runtime characterization stays independent of the
// exported name, matching the FORMAT-02 plan convention.
func TestIntegrationQOIDecodeCharacterization(t *testing.T) {
	const (
		qoiFourCC  = uint32(0x514F4920)
		wantWidth  = 160
		wantHeight = 120
		wantStride = 640
		wantBytes  = 76800
		corruptLen = 1000
	)

	qoiSrc := loadFixture(t, "bricks-color.qoi")
	alphaSrc := loadFixture(t, "alpha-pixels.qoi")

	// corruptQOI keeps the complete 14-byte header and image configuration but
	// truncates the pixel stream, so Probe still sniffs QOI and reads 160×120
	// while pixel decode runs out of input and must fail with ErrDecode.
	corrupt := qoiSrc[:corruptLen]

	t.Run("vendored RGB bricks-color.qoi matches the PNG oracle", func(t *testing.T) {
		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, wantBytes, len(qoiSrc)); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}
		guestBefore := wuffs.CaptureGuestMemoryState(d)

		meta, err := d.Probe(qoiSrc)
		if err != nil {
			t.Fatalf("Probe(bricks-color.qoi): %v", err)
		}
		assertGuestMemoryPreserved(t, "Probe", "bricks-color.qoi", d, guestBefore)
		if meta == nil {
			t.Fatal("Probe(bricks-color.qoi) returned nil Meta, want non-nil")
		}
		if meta.Format != qoiFourCC {
			t.Errorf("Probe Format = 0x%08X, want FormatQOI 0x%08X", meta.Format, qoiFourCC)
		}
		if meta.Width != wantWidth || meta.Height != wantHeight || meta.Stride != wantStride {
			t.Errorf("Probe geometry = (%d,%d) stride %d, want (%d,%d) stride %d",
				meta.Width, meta.Height, meta.Stride, wantWidth, wantHeight, wantStride)
		}
		if meta.BytesWritten != 0 {
			t.Errorf("Probe BytesWritten = %d, want 0", meta.BytesWritten)
		}

		dst := image.NewRGBA(image.Rect(0, 0, wantWidth, wantHeight))
		pixPtr := &dst.Pix[0]
		pixLen, pixCap := len(dst.Pix), cap(dst.Pix)
		rect, dstStride := dst.Rect, dst.Stride

		decMeta, err := d.DecodeRGBA(dst, qoiSrc)
		if err != nil {
			t.Fatalf("DecodeRGBA(bricks-color.qoi): %v", err)
		}
		assertGuestMemoryPreserved(t, "DecodeRGBA", "bricks-color.qoi", d, guestBefore)
		if decMeta == nil {
			t.Fatal("DecodeRGBA(bricks-color.qoi) returned nil Meta, want non-nil")
		}
		if decMeta.Format != qoiFourCC {
			t.Errorf("DecodeRGBA Format = 0x%08X, want FormatQOI 0x%08X", decMeta.Format, qoiFourCC)
		}
		if decMeta.Width != wantWidth || decMeta.Height != wantHeight || decMeta.Stride != wantStride {
			t.Errorf("DecodeRGBA geometry = (%d,%d) stride %d, want (%d,%d) stride %d",
				decMeta.Width, decMeta.Height, decMeta.Stride, wantWidth, wantHeight, wantStride)
		}
		if decMeta.BytesWritten != wantBytes {
			t.Errorf("DecodeRGBA BytesWritten = %d, want %d", decMeta.BytesWritten, wantBytes)
		}

		// Oracle: the same canvas decoded from the pixel-identical PNG fixture.
		oracle := image.NewRGBA(image.Rect(0, 0, wantWidth, wantHeight))
		pngSrc := loadFixture(t, "bricks-color.png")
		if _, err := wuffs.New().DecodeRGBA(oracle, pngSrc); err != nil {
			t.Fatalf("PNG oracle DecodeRGBA: %v", err)
		}
		if !bytes.Equal(dst.Pix, oracle.Pix) {
			t.Error("DecodeRGBA(bricks-color.qoi) pixels differ from the bricks-color.png oracle")
		}

		if &dst.Pix[0] != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != dstStride {
			t.Fatal("DecodeRGBA changed the caller-owned destination layout")
		}
	})

	t.Run("in-repository RGBA alpha-pixels.qoi decodes to fixed straight pixels", func(t *testing.T) {
		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, 3*1*4, len(alphaSrc)); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}
		guestBefore := wuffs.CaptureGuestMemoryState(d)

		meta, err := d.Probe(alphaSrc)
		if err != nil {
			t.Fatalf("Probe(alpha-pixels.qoi): %v", err)
		}
		assertGuestMemoryPreserved(t, "Probe", "alpha-pixels.qoi", d, guestBefore)
		if meta == nil {
			t.Fatal("Probe(alpha-pixels.qoi) returned nil Meta, want non-nil")
		}
		if meta.Format != qoiFourCC {
			t.Errorf("Probe Format = 0x%08X, want FormatQOI 0x%08X", meta.Format, qoiFourCC)
		}
		if meta.Width != 3 || meta.Height != 1 || meta.Stride != 12 {
			t.Errorf("Probe geometry = (%d,%d) stride %d, want (3,1) stride 12",
				meta.Width, meta.Height, meta.Stride)
		}
		if meta.BytesWritten != 0 {
			t.Errorf("Probe BytesWritten = %d, want 0", meta.BytesWritten)
		}

		dst := image.NewRGBA(image.Rect(0, 0, 3, 1))
		dstBefore := captureRGBAState(dst)
		decMeta, err := d.DecodeRGBA(dst, alphaSrc)
		if err != nil {
			t.Fatalf("DecodeRGBA(alpha-pixels.qoi): %v", err)
		}
		assertGuestMemoryPreserved(t, "DecodeRGBA", "alpha-pixels.qoi", d, guestBefore)
		assertDestinationPreserved(t, "DecodeRGBA", "alpha-pixels.qoi", dst, dstBefore)
		if decMeta == nil {
			t.Fatal("DecodeRGBA(alpha-pixels.qoi) returned nil Meta, want non-nil")
		}
		if decMeta.Format != qoiFourCC {
			t.Errorf("DecodeRGBA Format = 0x%08X, want FormatQOI 0x%08X", decMeta.Format, qoiFourCC)
		}
		if decMeta.BytesWritten != 3*1*4 {
			t.Errorf("DecodeRGBA BytesWritten = %d, want %d", decMeta.BytesWritten, 3*1*4)
		}

		// Straight (non-premultiplied) RGBA: identical values to the
		// alpha-pixels.png NRGBA fixture: opaque, translucent, transparent.
		wantPixels := []byte{
			0x12, 0x34, 0x56, 0xFF,
			0xFF, 0x00, 0xFF, 0x80,
			0x00, 0x00, 0x00, 0x00,
		}
		if !bytes.Equal(dst.Pix, wantPixels) {
			t.Errorf("DecodeRGBA(alpha-pixels.qoi) pixels = % X, want % X", dst.Pix, wantPixels)
		}
	})

	t.Run("corrupt QOI probes cleanly but decode fails with ErrDecode", func(t *testing.T) {
		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, wantBytes, len(corrupt)); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}
		guestBefore := wuffs.CaptureGuestMemoryState(d)

		meta, err := d.Probe(corrupt)
		if err != nil {
			t.Fatalf("Probe(corrupt QOI): %v", err)
		}
		assertGuestMemoryPreserved(t, "Probe", "corrupt QOI", d, guestBefore)
		if meta == nil {
			t.Fatal("Probe(corrupt QOI) returned nil Meta, want non-nil")
		}
		if meta.Format != qoiFourCC {
			t.Errorf("Probe(corrupt QOI) Format = 0x%08X, want 0x%08X", meta.Format, qoiFourCC)
		}
		if meta.Width != wantWidth || meta.Height != wantHeight || meta.Stride != wantStride {
			t.Errorf("Probe(corrupt QOI) geometry = (%d,%d) stride %d, want (%d,%d) stride %d",
				meta.Width, meta.Height, meta.Stride, wantWidth, wantHeight, wantStride)
		}
		if meta.BytesWritten != 0 {
			t.Errorf("Probe(corrupt QOI) BytesWritten = %d, want 0", meta.BytesWritten)
		}

		dst := image.NewRGBA(image.Rect(0, 0, wantWidth, wantHeight))
		for i := range dst.Pix {
			dst.Pix[i] = 0xA5
		}
		prefill := append([]byte(nil), dst.Pix...)
		pixPtr := &dst.Pix[0]
		pixLen, pixCap := len(dst.Pix), cap(dst.Pix)
		rect, dstStride := dst.Rect, dst.Stride

		_, err = d.DecodeRGBA(dst, corrupt)
		assertGuestMemoryPreserved(t, "DecodeRGBA", "corrupt QOI", d, guestBefore)
		if err == nil {
			t.Fatal("DecodeRGBA(corrupt QOI) error = nil, want ErrDecode")
		}
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Errorf("DecodeRGBA(corrupt QOI) error = %v, want errors.Is(err, ErrDecode)", err)
		}
		if errors.Is(err, wuffs.ErrUnknownFormat) {
			t.Errorf("DecodeRGBA(corrupt QOI) error = %v, must not be ErrUnknownFormat", err)
		}
		if !bytes.Equal(dst.Pix, prefill) {
			t.Error("DecodeRGBA(corrupt QOI) mutated the sentinel destination pixels")
		}
		if &dst.Pix[0] != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != dstStride {
			t.Fatal("DecodeRGBA(corrupt QOI) changed the caller-owned destination layout")
		}
	})
}

// TestIntegrationQOIDecodeRGBAAllocsPerRun measures heap allocations per
// DecodeRGBA call for reusable QOI decoders. Each input and its correctly
// sized caller-owned destination are prepared outside the measured closure,
// one warm-up decode runs first, and testing.AllocsPerRun then requires
// exactly zero allocations per call for both the RGB (bricks-color.qoi) and
// RGBA (alpha-pixels.qoi) channel modes.
func TestIntegrationQOIDecodeRGBAAllocsPerRun(t *testing.T) {
	fixtures := []struct {
		name         string
		file         string
		wantW, wantH int
	}{
		{"RGB 160×120", "bricks-color.qoi", 160, 120},
		{"RGBA 3×1", "alpha-pixels.qoi", 3, 1},
	}

	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			src := loadFixture(t, tc.file)

			d := wuffs.New()
			if err := wuffs.RequiredReserve(d, tc.wantW*tc.wantH*4, len(src)); err != nil {
				t.Fatalf("RequiredReserve: %v", err)
			}

			dst := image.NewRGBA(image.Rect(0, 0, tc.wantW, tc.wantH))
			if _, err := d.DecodeRGBA(dst, src); err != nil {
				t.Fatalf("warm-up DecodeRGBA: %v", err)
			}

			allocs := testing.AllocsPerRun(100, func() {
				if _, err := d.DecodeRGBA(dst, src); err != nil {
					t.Errorf("DecodeRGBA: %v", err)
				}
			})
			if allocs != 0 {
				t.Errorf("DecodeRGBA allocated %.0f heap objects per run, want 0", allocs)
			}
		})
	}
}
