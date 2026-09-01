package wuffs

import (
	"bytes"
	"errors"
	"image"
	"testing"
)

// TestIntegrationJPEGDecodeCharacterization verifies the JPEG format contract
// end-to-end against the live wasm guest:
//
//   - Probe reports Width 90, Height 112, Stride 360, Format 0x4A504547
//     ("JPEG"), and BytesWritten 0, without decoding pixels, growing guest
//     memory, or relocating the slot layout.
//   - After an explicit scratch reservation, DecodeRGBA reports BytesWritten
//     40320 (90×112×4) while preserving the caller Pix pointer, length,
//     capacity, Rect, and Stride. Every pixel is opaque, and converting the
//     caller-owned straight, non-premultiplied RGBA bytes to the canonical
//     non-premultiplied BGRA of a single-frame NIA v1-bn4 stream checksums to
//     the checked-in golden value in testdata/hat.jpeg.golden.nia.crc32.
//   - A deterministic truncation of the scan span (the corrupt JPEG) still
//     probes as 90×112 FormatJPEG, but DecodeRGBA fails with
//     errors.Is(err, ErrDecode) (and not ErrUnknownFormat), leaving a
//     prefilled destination and its fields unchanged.
//
// The format is asserted via the local uint32 FourCC literal rather than the
// exported constant so this runtime characterization stays independent of the
// exported name, matching the FORMAT-01 plan convention.
func TestIntegrationJPEGDecodeCharacterization(t *testing.T) {
	const (
		jpegFourCC  = uint32(0x4A504547)
		wantWidth   = 90
		wantHeight  = 112
		wantStride  = 360
		wantBytes   = 40320
		corruptTail = 16
	)

	jpegSrc := mustReadFixture(t, "hat.jpeg")

	// corruptJPEG keeps the SOI, the quantization, Huffman, and start-of-frame
	// tables (so Probe still sniffs JPEG and reads 90×112) plus the SOS header
	// and only the first corruptTail scan bytes, so the truncated entropy-coded
	// pixel stream must fail with ErrDecode.
	//
	// The SOS marker (0xFFDA) opens with a 2-byte length; the entropy-coded
	// scan follows immediately after that length field.
	sos := bytes.Index(jpegSrc, []byte{0xFF, 0xDA})
	if sos < 0 {
		t.Fatal("hat.jpeg has no SOS marker (0xFFDA)")
	}
	scanStart := sos + 2 + int(jpegSrc[sos+2])<<8 + int(jpegSrc[sos+3])
	if scanStart+corruptTail > len(jpegSrc) {
		t.Fatalf("scan truncation %d exceeds hat.jpeg length %d", scanStart+corruptTail, len(jpegSrc))
	}
	corrupt := jpegSrc[:scanStart+corruptTail]

	t.Run("Probe reports JPEG config without pixels or memory growth", func(t *testing.T) {
		d := New()
		pre := captureMemState(t, d)
		meta, err := d.Probe(jpegSrc)
		if err != nil {
			t.Fatalf("Probe(hat.jpeg) error = %v, want nil", err)
		}
		if meta == nil {
			t.Fatal("Probe(hat.jpeg) returned nil Meta, want non-nil")
		}
		assertMemUnchanged(t, d, pre, "Probe(hat.jpeg)")
		if meta.Width != wantWidth || meta.Height != wantHeight || meta.Stride != wantStride {
			t.Errorf("Probe Meta = {W:%d H:%d S:%d}, want {W:%d H:%d S:%d}",
				meta.Width, meta.Height, meta.Stride, wantWidth, wantHeight, wantStride)
		}
		if meta.Format != jpegFourCC {
			t.Errorf("Probe Meta.Format = 0x%08X, want 0x%08X", meta.Format, jpegFourCC)
		}
		if meta.BytesWritten != 0 {
			t.Errorf("Probe Meta.BytesWritten = %d, want 0", meta.BytesWritten)
		}
	})

	t.Run("DecodeRGBA after reservation is opaque and matches the golden NIA CRC", func(t *testing.T) {
		d := New()
		probe, err := d.Probe(jpegSrc)
		if err != nil {
			t.Fatalf("Probe(hat.jpeg) error = %v, want nil", err)
		}
		if probe == nil || probe.Width != wantWidth || probe.Height != wantHeight || probe.Stride != wantStride {
			t.Fatalf("Probe Meta = %+v, want {W:%d H:%d S:%d}", probe, wantWidth, wantHeight, wantStride)
		}

		if reserveErr := d.Reserve(wantBytes, len(jpegSrc)); reserveErr != nil {
			t.Fatalf("Reserve(%d, %d) error = %v, want nil", wantBytes, len(jpegSrc), reserveErr)
		}

		dst := image.NewRGBA(image.Rect(0, 0, wantWidth, wantHeight))
		pixPtr := &dst.Pix[0]
		pixLen, pixCap := len(dst.Pix), cap(dst.Pix)
		rect, dstStride := dst.Rect, dst.Stride

		meta, err := d.DecodeRGBA(dst, jpegSrc)
		if err != nil {
			t.Fatalf("DecodeRGBA(hat.jpeg) error = %v, want nil", err)
		}
		if meta == nil {
			t.Fatal("DecodeRGBA(hat.jpeg) returned nil Meta, want non-nil")
		}
		if meta.Width != wantWidth || meta.Height != wantHeight || meta.Stride != wantStride {
			t.Errorf("DecodeRGBA Meta = {W:%d H:%d S:%d}, want {W:%d H:%d S:%d}",
				meta.Width, meta.Height, meta.Stride, wantWidth, wantHeight, wantStride)
		}
		if meta.Format != jpegFourCC {
			t.Errorf("DecodeRGBA Meta.Format = 0x%08X, want 0x%08X", meta.Format, jpegFourCC)
		}
		if meta.BytesWritten != wantBytes {
			t.Errorf("DecodeRGBA Meta.BytesWritten = %d, want %d", meta.BytesWritten, wantBytes)
		}
		if &dst.Pix[0] != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != dstStride {
			t.Fatal("DecodeRGBA changed the caller-owned destination layout")
		}

		// hat.jpeg is an RGB (opaque) image; every decoded pixel must carry
		// fully-covering alpha.
		for i := 3; i < len(dst.Pix); i += 4 {
			if dst.Pix[i] != 0xFF {
				t.Fatalf("DecodeRGBA(hat.jpeg) produced a non-opaque pixel (alpha=0x%02X at Pix[%d])", dst.Pix[i], i)
			}
		}

		// The straight, non-premultiplied RGBA bytes converted to the canonical
		// non-premultiplied BGRA of a single-frame NIA v1-bn4 stream must
		// checksum to the checked-in golden value (0x2298F3CA / 580449226).
		gotCRC := niaCRC32(dst)
		if wantCRC := readGoldenCRC32(t, "hat.jpeg.golden.nia.crc32"); gotCRC != wantCRC {
			t.Errorf("NIA CRC-32 of DecodeRGBA(hat.jpeg) = 0x%08X, want golden 0x%08X", gotCRC, wantCRC)
		}
	})

	t.Run("corrupt JPEG probes cleanly but decode fails with ErrDecode", func(t *testing.T) {
		d := New()
		meta, err := d.Probe(corrupt)
		if err != nil {
			t.Fatalf("Probe(corrupt JPEG) error = %v, want nil", err)
		}
		if meta == nil {
			t.Fatal("Probe(corrupt JPEG) returned nil Meta, want non-nil")
		}
		if meta.Width != wantWidth || meta.Height != wantHeight || meta.Stride != wantStride {
			t.Errorf("Probe(corrupt JPEG) Meta = {W:%d H:%d S:%d}, want {W:%d H:%d S:%d}",
				meta.Width, meta.Height, meta.Stride, wantWidth, wantHeight, wantStride)
		}
		if meta.Format != jpegFourCC {
			t.Errorf("Probe(corrupt JPEG) Meta.Format = 0x%08X, want 0x%08X", meta.Format, jpegFourCC)
		}
		if meta.BytesWritten != 0 {
			t.Errorf("Probe(corrupt JPEG) Meta.BytesWritten = %d, want 0", meta.BytesWritten)
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
			t.Fatal("DecodeRGBA(corrupt JPEG) error = nil, want ErrDecode")
		}
		if !errors.Is(err, ErrDecode) {
			t.Errorf("DecodeRGBA(corrupt JPEG) error = %v, want errors.Is(err, ErrDecode)", err)
		}
		if errors.Is(err, ErrUnknownFormat) {
			t.Errorf("DecodeRGBA(corrupt JPEG) error = %v, must not be ErrUnknownFormat", err)
		}
		if !bytes.Equal(dst.Pix, prefill) {
			t.Error("DecodeRGBA(corrupt JPEG) mutated the prefilled destination pixels")
		}
		if &dst.Pix[0] != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != dstStride {
			t.Fatal("DecodeRGBA(corrupt JPEG) changed the caller-owned destination layout")
		}
	})
}
