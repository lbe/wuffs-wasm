package wuffs

import (
	"bytes"
	"errors"
	"image"
	"testing"
)

// TestIntegrationGIFDecodeCharacterization verifies the GIF format contract
// end-to-end against the live wasm guest:
//
//   - Probe reports Width 160, Height 120, Stride 640, Format 0x47494620
//     ("GIF "), and BytesWritten 0, without decoding pixels, growing guest
//     memory, or relocating the slot layout.
//   - After an explicit scratch reservation, DecodeRGBA reports BytesWritten
//     76800 and pixel-identical output to the bricks-nodither.png oracle, while
//     preserving the caller Pix pointer, length, capacity, Rect, and Stride.
//   - A deterministic truncation of the pixel stream (the corrupt GIF) still
//     probes as 160×120 FormatGIF, but DecodeRGBA fails with
//     errors.Is(err, ErrDecode) (and not ErrUnknownFormat), leaving a
//     prefilled destination and its fields unchanged.
//
// The format is asserted via the local uint32 FourCC literal rather than the
// exported constant so this runtime characterization stays independent of the
// exported name, matching the FORMAT-01 plan convention.
func TestIntegrationGIFDecodeCharacterization(t *testing.T) {
	const (
		gifFourCC   = uint32(0x47494620)
		wantWidth   = 160
		wantHeight  = 120
		wantStride  = 640
		wantBytes   = 76800
		corruptTail = 10
	)

	gifSrc := mustReadFixture(t, "bricks-nodither.gif")
	pngSrc := mustReadFixture(t, "bricks-nodither.png")

	// corruptGIF keeps the 6-byte signature, 7-byte logical screen descriptor,
	// and the full global color table, plus the image descriptor and the first
	// few sub-block bytes, so Probe still sniffs GIF and reads 160×120 while
	// the truncated LZW pixel stream must fail with ErrDecode.
	//
	// The logical screen descriptor's low three bits hold the log2 of the
	// color-table size minus one; its byte count is 3 * 2^(bits+1) and it
	// immediately follows the 13-byte signature + descriptor header.
	gctSize := 1 << ((gifSrc[10] & 0x07) + 1)
	colorTableEnd := 13 + gctSize*3
	imgDesc := bytes.Index(gifSrc[colorTableEnd:], []byte{0x2C})
	if imgDesc < 0 {
		t.Fatal("bricks-nodither.gif has no image descriptor (0x2C)")
	}
	corrupt := gifSrc[:colorTableEnd+imgDesc+corruptTail]

	t.Run("Probe reports GIF config without pixels or memory growth", func(t *testing.T) {
		d := New()
		pre := captureMemState(t, d)
		meta, err := d.Probe(gifSrc)
		if err != nil {
			t.Fatalf("Probe(bricks-nodither.gif) error = %v, want nil", err)
		}
		if meta == nil {
			t.Fatal("Probe(bricks-nodither.gif) returned nil Meta, want non-nil")
		}
		assertMemUnchanged(t, d, pre, "Probe(bricks-nodither.gif)")
		if meta.Width != wantWidth || meta.Height != wantHeight || meta.Stride != wantStride {
			t.Errorf("Probe Meta = {W:%d H:%d S:%d}, want {W:%d H:%d S:%d}",
				meta.Width, meta.Height, meta.Stride, wantWidth, wantHeight, wantStride)
		}
		if meta.Format != gifFourCC {
			t.Errorf("Probe Meta.Format = 0x%08X, want 0x%08X", meta.Format, gifFourCC)
		}
		if meta.BytesWritten != 0 {
			t.Errorf("Probe Meta.BytesWritten = %d, want 0", meta.BytesWritten)
		}
	})

	t.Run("DecodeRGBA after reservation matches the PNG oracle", func(t *testing.T) {
		d := New()
		probe, err := d.Probe(gifSrc)
		if err != nil {
			t.Fatalf("Probe(bricks-nodither.gif) error = %v, want nil", err)
		}
		if probe == nil || probe.Width != wantWidth || probe.Height != wantHeight || probe.Stride != wantStride {
			t.Fatalf("Probe Meta = %+v, want {W:%d H:%d S:%d}", probe, wantWidth, wantHeight, wantStride)
		}

		if reserveErr := d.Reserve(wantBytes, len(gifSrc)); reserveErr != nil {
			t.Fatalf("Reserve(%d, %d) error = %v, want nil", wantBytes, len(gifSrc), reserveErr)
		}

		dst := image.NewRGBA(image.Rect(0, 0, wantWidth, wantHeight))
		pixPtr := &dst.Pix[0]
		pixLen, pixCap := len(dst.Pix), cap(dst.Pix)
		rect, dstStride := dst.Rect, dst.Stride

		meta, err := d.DecodeRGBA(dst, gifSrc)
		if err != nil {
			t.Fatalf("DecodeRGBA(bricks-nodither.gif) error = %v, want nil", err)
		}
		if meta == nil {
			t.Fatal("DecodeRGBA(bricks-nodither.gif) returned nil Meta, want non-nil")
		}
		if meta.Width != wantWidth || meta.Height != wantHeight || meta.Stride != wantStride {
			t.Errorf("DecodeRGBA Meta = {W:%d H:%d S:%d}, want {W:%d H:%d S:%d}",
				meta.Width, meta.Height, meta.Stride, wantWidth, wantHeight, wantStride)
		}
		if meta.Format != gifFourCC {
			t.Errorf("DecodeRGBA Meta.Format = 0x%08X, want 0x%08X", meta.Format, gifFourCC)
		}
		if meta.BytesWritten != wantBytes {
			t.Errorf("DecodeRGBA Meta.BytesWritten = %d, want %d", meta.BytesWritten, wantBytes)
		}

		// Oracle: the same canvas decoded from the pixel-identical PNG fixture.
		oracle := image.NewRGBA(image.Rect(0, 0, wantWidth, wantHeight))
		if _, err := New().DecodeRGBA(oracle, pngSrc); err != nil {
			t.Fatalf("PNG oracle DecodeRGBA error = %v, want nil", err)
		}
		if !bytes.Equal(dst.Pix, oracle.Pix) {
			t.Error("DecodeRGBA(bricks-nodither.gif) pixels differ from the bricks-nodither.png oracle")
		}

		if &dst.Pix[0] != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != dstStride {
			t.Fatal("DecodeRGBA changed the caller-owned destination layout")
		}
	})

	t.Run("corrupt GIF probes cleanly but decode fails with ErrDecode", func(t *testing.T) {
		d := New()
		meta, err := d.Probe(corrupt)
		if err != nil {
			t.Fatalf("Probe(corrupt GIF) error = %v, want nil", err)
		}
		if meta == nil {
			t.Fatal("Probe(corrupt GIF) returned nil Meta, want non-nil")
		}
		if meta.Width != wantWidth || meta.Height != wantHeight || meta.Stride != wantStride {
			t.Errorf("Probe(corrupt GIF) Meta = {W:%d H:%d S:%d}, want {W:%d H:%d S:%d}",
				meta.Width, meta.Height, meta.Stride, wantWidth, wantHeight, wantStride)
		}
		if meta.Format != gifFourCC {
			t.Errorf("Probe(corrupt GIF) Meta.Format = 0x%08X, want 0x%08X", meta.Format, gifFourCC)
		}
		if meta.BytesWritten != 0 {
			t.Errorf("Probe(corrupt GIF) Meta.BytesWritten = %d, want 0", meta.BytesWritten)
		}

		if reserveErr := d.Reserve(wantBytes, len(corrupt)); reserveErr != nil {
			t.Fatalf("Reserve(%d, %d) error = %v, want nil", wantBytes, len(corrupt), reserveErr)
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
		if err == nil {
			t.Fatal("DecodeRGBA(corrupt GIF) error = nil, want ErrDecode")
		}
		if !errors.Is(err, ErrDecode) {
			t.Errorf("DecodeRGBA(corrupt GIF) error = %v, want errors.Is(err, ErrDecode)", err)
		}
		if errors.Is(err, ErrUnknownFormat) {
			t.Errorf("DecodeRGBA(corrupt GIF) error = %v, must not be ErrUnknownFormat", err)
		}
		if !bytes.Equal(dst.Pix, prefill) {
			t.Error("DecodeRGBA(corrupt GIF) mutated the prefilled destination pixels")
		}
		if &dst.Pix[0] != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != dstStride {
			t.Fatal("DecodeRGBA(corrupt GIF) changed the caller-owned destination layout")
		}
	})
}
