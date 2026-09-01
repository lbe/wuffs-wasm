package wuffs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"testing"
)

// TestIntegrationBMPDecodeCharacterization verifies the BMP format contract
// end-to-end against the live wasm guest:
//
//   - Probe reports Width 160, Height 120, Stride 640, Format 0x424D5020
//     ("BMP "), and BytesWritten 0, without decoding pixels, growing guest
//     memory, or relocating the slot layout.
//   - After an explicit scratch reservation, DecodeRGBA reports BytesWritten
//     76800 and pixel-identical output to the bricks-color.png oracle, while
//     preserving the caller Pix pointer, length, capacity, Rect, and Stride.
//   - A deterministic truncation of the pixel stream (the corrupt BMP) still
//     probes as 160×120 FormatBMP, but DecodeRGBA fails with
//     errors.Is(err, ErrDecode) (and not ErrUnknownFormat), leaving a
//     prefilled destination and its fields unchanged.
//
// The format is asserted via the local uint32 FourCC literal rather than the
// exported constant so this runtime characterization stays independent of the
// exported name, matching the FORMAT-01 plan convention.
func TestIntegrationBMPDecodeCharacterization(t *testing.T) {
	const (
		bmpFourCC   = uint32(0x424D5020)
		wantWidth   = 160
		wantHeight  = 120
		wantStride  = 640
		wantBytes   = 76800
		corruptRows = 4
	)

	bmpSrc := mustReadFixture(t, "bricks-color.bmp")
	pngSrc := mustReadFixture(t, "bricks-color.png")

	// corruptBMP keeps the complete headers and only the first corruptRows
	// 24-bit pixel rows, so Probe still sniffs BMP and reads 160×120 while
	// pixel decode runs out of input and must fail with ErrDecode. The pixel
	// data offset (bfOffBits) is read from the file header so the cut survives
	// fixture header changes.
	pixelOff := binary.LittleEndian.Uint32(bmpSrc[10:14])
	corrupt := bmpSrc[:int(pixelOff)+corruptRows*wantWidth*3]

	t.Run("Probe reports BMP config without pixels or memory growth", func(t *testing.T) {
		d := New()
		pre := captureMemState(t, d)
		meta, err := d.Probe(bmpSrc)
		if err != nil {
			t.Fatalf("Probe(bricks-color.bmp) error = %v, want nil", err)
		}
		if meta == nil {
			t.Fatal("Probe(bricks-color.bmp) returned nil Meta, want non-nil")
		}
		assertMemUnchanged(t, d, pre, "Probe(bricks-color.bmp)")
		if meta.Width != wantWidth || meta.Height != wantHeight || meta.Stride != wantStride {
			t.Errorf("Probe Meta = {W:%d H:%d S:%d}, want {W:%d H:%d S:%d}",
				meta.Width, meta.Height, meta.Stride, wantWidth, wantHeight, wantStride)
		}
		if meta.Format != bmpFourCC {
			t.Errorf("Probe Meta.Format = 0x%08X, want 0x%08X", meta.Format, bmpFourCC)
		}
		if meta.BytesWritten != 0 {
			t.Errorf("Probe Meta.BytesWritten = %d, want 0", meta.BytesWritten)
		}
	})

	t.Run("DecodeRGBA after reservation matches the PNG oracle", func(t *testing.T) {
		d := New()
		probe, err := d.Probe(bmpSrc)
		if err != nil {
			t.Fatalf("Probe(bricks-color.bmp) error = %v, want nil", err)
		}
		if probe == nil || probe.Width != wantWidth || probe.Height != wantHeight || probe.Stride != wantStride {
			t.Fatalf("Probe Meta = %+v, want {W:%d H:%d S:%d}", probe, wantWidth, wantHeight, wantStride)
		}

		if reserveErr := d.Reserve(wantBytes, len(bmpSrc)); reserveErr != nil {
			t.Fatalf("Reserve(%d, %d) error = %v, want nil", wantBytes, len(bmpSrc), reserveErr)
		}

		dst := image.NewRGBA(image.Rect(0, 0, wantWidth, wantHeight))
		pixPtr := &dst.Pix[0]
		pixLen, pixCap := len(dst.Pix), cap(dst.Pix)
		rect, dstStride := dst.Rect, dst.Stride

		meta, err := d.DecodeRGBA(dst, bmpSrc)
		if err != nil {
			t.Fatalf("DecodeRGBA(bricks-color.bmp) error = %v, want nil", err)
		}
		if meta == nil {
			t.Fatal("DecodeRGBA(bricks-color.bmp) returned nil Meta, want non-nil")
		}
		if meta.Width != wantWidth || meta.Height != wantHeight || meta.Stride != wantStride {
			t.Errorf("DecodeRGBA Meta = {W:%d H:%d S:%d}, want {W:%d H:%d S:%d}",
				meta.Width, meta.Height, meta.Stride, wantWidth, wantHeight, wantStride)
		}
		if meta.Format != bmpFourCC {
			t.Errorf("DecodeRGBA Meta.Format = 0x%08X, want 0x%08X", meta.Format, bmpFourCC)
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
			t.Error("DecodeRGBA(bricks-color.bmp) pixels differ from the bricks-color.png oracle")
		}

		if &dst.Pix[0] != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != dstStride {
			t.Fatal("DecodeRGBA changed the caller-owned destination layout")
		}
	})

	t.Run("corrupt BMP probes cleanly but decode fails with ErrDecode", func(t *testing.T) {
		d := New()
		meta, err := d.Probe(corrupt)
		if err != nil {
			t.Fatalf("Probe(corrupt BMP) error = %v, want nil", err)
		}
		if meta == nil {
			t.Fatal("Probe(corrupt BMP) returned nil Meta, want non-nil")
		}
		if meta.Width != wantWidth || meta.Height != wantHeight || meta.Stride != wantStride {
			t.Errorf("Probe(corrupt BMP) Meta = {W:%d H:%d S:%d}, want {W:%d H:%d S:%d}",
				meta.Width, meta.Height, meta.Stride, wantWidth, wantHeight, wantStride)
		}
		if meta.Format != bmpFourCC {
			t.Errorf("Probe(corrupt BMP) Meta.Format = 0x%08X, want 0x%08X", meta.Format, bmpFourCC)
		}
		if meta.BytesWritten != 0 {
			t.Errorf("Probe(corrupt BMP) Meta.BytesWritten = %d, want 0", meta.BytesWritten)
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
			t.Fatal("DecodeRGBA(corrupt BMP) error = nil, want ErrDecode")
		}
		if !errors.Is(err, ErrDecode) {
			t.Errorf("DecodeRGBA(corrupt BMP) error = %v, want errors.Is(err, ErrDecode)", err)
		}
		if errors.Is(err, ErrUnknownFormat) {
			t.Errorf("DecodeRGBA(corrupt BMP) error = %v, must not be ErrUnknownFormat", err)
		}
		if !bytes.Equal(dst.Pix, prefill) {
			t.Error("DecodeRGBA(corrupt BMP) mutated the prefilled destination pixels")
		}
		if &dst.Pix[0] != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != dstStride {
			t.Fatal("DecodeRGBA(corrupt BMP) changed the caller-owned destination layout")
		}
	})
}
