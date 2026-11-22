package wuffs_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"testing"

	wuffs "github.com/lbe/wuffs-wasm"
)

// largePNGSide is past the size at which a workbuf in the fixed guest bump
// region overran the host slots (about 15 MiB of workbuf at the 24 MiB floor).
const largePNGSide = 2048

// compressiblePNG8 returns an 8-bit RGBA PNG whose deflate stream is far
// smaller than its decoded size, plus the premultiplied RGBA it decodes to.
func compressiblePNG8(t *testing.T, n int) ([]byte, *image.RGBA) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for i := range img.Pix {
		img.Pix[i] = byte(i/4%251 ^ (i/(4*n))%7)
	}
	return encodePNG(t, img), toRGBA(img)
}

// compressiblePNG16 returns an opaque 16-bit RGBA PNG whose 16-bit samples
// are v*257, so the 8-bit decode is exactly v.
func compressiblePNG16(t *testing.T, n int) ([]byte, *image.RGBA) {
	t.Helper()
	img := image.NewNRGBA64(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			v := uint16(byte(x%251^y%7)) * 257
			img.SetNRGBA64(x, y, color.NRGBA64{R: v, G: v, B: ^v, A: 0xFFFF})
		}
	}
	return encodePNG(t, img), toRGBA(img)
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func toRGBA(img image.Image) *image.RGBA {
	out := image.NewRGBA(img.Bounds())
	draw.Draw(out, out.Rect, img, image.Point{}, draw.Src)
	return out
}

func assertSamePix(t *testing.T, got, want *image.RGBA) {
	t.Helper()
	if !bytes.Equal(got.Pix, want.Pix) {
		for i := range got.Pix {
			if got.Pix[i] != want.Pix[i] {
				t.Fatalf("Pix[%d] = %d, want %d", i, got.Pix[i], want.Pix[i])
			}
		}
	}
}

// TestIntegrationDecodeLargeCompressiblePNG decodes PNGs whose work buffer
// exceeds the fixed guest bump region, through DecodeRGBA and DecodeFrame, and
// requires output identical to image/png.
func TestIntegrationDecodeLargeCompressiblePNG(t *testing.T) {
	cases := []struct {
		name string
		gen  func(*testing.T, int) ([]byte, *image.RGBA)
	}{
		{"8-bit", compressiblePNG8},
		{"16-bit", compressiblePNG16},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, want := tc.gen(t, largePNGSide)

			t.Run("DecodeRGBA", func(t *testing.T) {
				d := wuffs.New()
				m := reserveForDecode(t, d, src)
				if m.WorkbufLen == 0 {
					t.Fatal("Probe WorkbufLen = 0, want the PNG work buffer length")
				}
				got := image.NewRGBA(want.Rect)
				dm, err := d.DecodeRGBA(got, src)
				if err != nil {
					t.Fatalf("DecodeRGBA: %v", err)
				}
				if dm.WorkbufLen != m.WorkbufLen {
					t.Errorf("decode WorkbufLen = %d, Probe = %d", dm.WorkbufLen, m.WorkbufLen)
				}
				assertSamePix(t, got, want)
			})

			t.Run("DecodeFrame", func(t *testing.T) {
				d := wuffs.New()
				reserveForDecode(t, d, src)
				got := image.NewRGBA(want.Rect)
				if _, err := d.DecodeFrame(got, src, 0); err != nil {
					t.Fatalf("DecodeFrame: %v", err)
				}
				assertSamePix(t, got, want)
			})
		})
	}
}

// TestIntegrationDecodeLargePNGWorkbufShortageRetry reserves only the canvas,
// requires a guest DstTooSmallError whose MinBytes includes the work buffer,
// and requires Reserve(MinBytes) to make the retry succeed.
func TestIntegrationDecodeLargePNGWorkbufShortageRetry(t *testing.T) {
	src, want := compressiblePNG8(t, largePNGSide)
	d := wuffs.New()
	if err := d.Reserve(0, len(src)); err != nil {
		t.Fatalf("Reserve src: %v", err)
	}
	m, err := d.Probe(src)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	canvas := int(m.Stride) * int(m.Height)
	if err = d.Reserve(canvas, len(src)); err != nil {
		t.Fatalf("Reserve canvas: %v", err)
	}

	got := image.NewRGBA(want.Rect)
	_, err = d.DecodeRGBA(got, src)
	var dts *wuffs.DstTooSmallError
	if !errors.As(err, &dts) {
		t.Fatalf("DecodeRGBA error = %v, want *DstTooSmallError", err)
	}
	if wantMin := uint32(canvas) + m.WorkbufLen; dts.MinBytes != wantMin {
		t.Fatalf("MinBytes = %d, want Stride*Height+WorkbufLen = %d", dts.MinBytes, wantMin)
	}

	if err = d.Reserve(int(dts.MinBytes), len(src)); err != nil {
		t.Fatalf("Reserve(MinBytes): %v", err)
	}
	if _, err = d.DecodeRGBA(got, src); err != nil {
		t.Fatalf("retry DecodeRGBA: %v", err)
	}
	assertSamePix(t, got, want)
}
