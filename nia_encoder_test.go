package wuffs

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"testing"
)

// niaEncodeRGBA wraps the straight, non-premultiplied RGBA pixels of img into
// a single-frame NIA v1-bn4 byte stream (non-premultiplied BGRA, 4 bytes per
// pixel), per ../wuffs/doc/spec/nie-spec.md.
//
// The returned bytes are the complete NIA image: the outer NIA header (magic
// 'n\u00EF' + 'A', version-and-config '0xFFbn4', little-endian width then
// height), the single-frame CDD (0), the inner NIE still image (magic 'n\u00EF'
// + 'E', same version/config, width and height, then the BGRA payload in
// row-major order), any odd-by-odd 4-byte zero padding, and the 8-byte footer
// (LoopCount 0 followed by [0x00,0x00,0x00,0x80]).
func niaEncodeRGBA(img *image.RGBA) []byte {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	var buf bytes.Buffer
	// writeHeader emits the 16-byte NIE/NIA header for the given magic byte.
	writeHeader := func(magic byte) {
		buf.Write([]byte{0x6e, 0xc3, 0xaf, magic, 0xff, 0x62, 0x6e, 0x34})
		_ = binary.Write(&buf, binary.LittleEndian, uint32(w))
		_ = binary.Write(&buf, binary.LittleEndian, uint32(h))
	}
	writeHeader('A')                                       // Outer NIA header.
	_ = binary.Write(&buf, binary.LittleEndian, uint64(0)) // Single-frame CDD.
	writeHeader('E')                                       // Inner NIE still image.

	// Payload: 4 bytes per pixel in row-major order, [B,G,R,A] on the wire.
	base := img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y)
	for y := 0; y < h; y++ {
		row := y * img.Stride
		for x := 0; x < w; x++ {
			i := base + row + x*4
			r, g, b, a := img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
			buf.Write([]byte{b, g, r, a})
		}
	}
	// 4 bytes of all-zero padding iff 4 bytes per pixel and odd width and height.
	if w&1 == 1 && h&1 == 1 {
		buf.Write([]byte{0, 0, 0, 0})
	}
	// Footer: little-endian LoopCount (0), then [0x00,0x00,0x00,0x80].
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))
	buf.Write([]byte{0, 0, 0, 0x80})
	return buf.Bytes()
}

// niaCRC32 is the CRC-32 (IEEE) of the single-frame NIA v1-bn4 encoding of the
// straight, non-premultiplied RGBA pixels of img.
func niaCRC32(img *image.RGBA) uint32 {
	return crc32.ChecksumIEEE(niaEncodeRGBA(img))
}

// TestUnitNIAEncoderGoldenCRC validates niaEncodeRGBA against an independent
// oracle: the standard library's image/png decoder produces the straight RGBA
// for bricks-color.png, whose NIA v1-bn4 encoding must checksum to a fixed,
// precomputed golden value (0x076CB375 / 124564341). The expected CRC is a
// literal, never derived from the wasm decoder under test.
//
// bricks-color.png is 160×120 (both even), so the stream has no padding.
func TestUnitNIAEncoderGoldenCRC(t *testing.T) {
	src := mustReadFixture(t, "bricks-color.png")
	dec, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		t.Fatalf("png.Decode(bricks-color.png) error = %v, want nil", err)
	}
	rgba := image.NewRGBA(dec.Bounds())
	for y := rgba.Rect.Min.Y; y < rgba.Rect.Max.Y; y++ {
		for x := rgba.Rect.Min.X; x < rgba.Rect.Max.X; x++ {
			i := rgba.PixOffset(x, y)
			r, g, b, a := dec.At(x, y).RGBA()
			rgba.Pix[i], rgba.Pix[i+1], rgba.Pix[i+2], rgba.Pix[i+3] =
				uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8)
		}
	}
	const wantCRC = uint32(0x076CB375)
	if got := niaCRC32(rgba); got != wantCRC {
		t.Errorf("niaCRC32(bricks-color.png) = 0x%08X, want 0x%08X", got, wantCRC)
	}
}
