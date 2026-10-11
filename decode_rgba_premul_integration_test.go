package wuffs_test

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/lbe/wuffs-wasm"
	"golang.org/x/image/webp"
)

// stdlibPremulRGBA decodes src with png.Decode and composites it into a
// tightly packed image.RGBA with draw.Src, per the plan oracle rule.
func stdlibPremulRGBA(t *testing.T, src []byte) *image.RGBA {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		t.Fatalf("png.Decode: %v", err)
	}
	want := image.NewRGBA(img.Bounds())
	draw.Draw(want, want.Bounds(), img, image.Point{}, draw.Src)
	return want
}

func decodeRGBAForTest(t *testing.T, src []byte) (*image.RGBA, *wuffs.Meta) {
	t.Helper()
	d := wuffs.New()
	probe, err := d.Probe(src)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if reserveErr := d.Reserve(int(probe.Stride)*int(probe.Height), len(src)); reserveErr != nil {
		t.Fatalf("Reserve: %v", reserveErr)
	}
	dst := image.NewRGBA(image.Rect(0, 0, int(probe.Width), int(probe.Height)))
	meta, err := d.DecodeRGBA(dst, src)
	if err != nil {
		t.Fatalf("DecodeRGBA: %v", err)
	}
	return dst, meta
}

// TestIntegrationDecodeRGBA_AlphaPixelsMatchesStdlibPremul verifies that
// DecodeRGBA output Pix matches the stdlib premultiplied oracle (png.Decode
// composited into image.NewRGBA with draw.Src) on testdata/alpha-pixels.png.
func TestIntegrationDecodeRGBA_AlphaPixelsMatchesStdlibPremul(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "alpha-pixels.png"))
	if err != nil {
		t.Fatalf("reading testdata/alpha-pixels.png: %v", err)
	}
	want := stdlibPremulRGBA(t, src)
	got, meta := decodeRGBAForTest(t, src)

	if meta.Width != uint32(want.Rect.Dx()) || meta.Height != uint32(want.Rect.Dy()) {
		t.Fatalf("Meta dimensions = (%d,%d), want (%d,%d)", meta.Width, meta.Height, want.Rect.Dx(), want.Rect.Dy())
	}
	if got.Stride != want.Rect.Dx()*4 {
		t.Fatalf("got.Stride = %d, want tight %d", got.Stride, want.Rect.Dx()*4)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Errorf("DecodeRGBA Pix = %v, want stdlib premul %v", got.Pix, want.Pix)
	}
}

// TestIntegrationDecodeRGBA_DrawCompositesTranslucentPixel verifies that the
// translucent middle pixel of alpha-pixels.png decodes premultiplied: the
// stdlib draw.Draw composite of the fixture over an opaque backdrop must
// equal compositing DecodeRGBA output the same way.
func TestIntegrationDecodeRGBA_DrawCompositesTranslucentPixel(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "alpha-pixels.png"))
	if err != nil {
		t.Fatalf("reading testdata/alpha-pixels.png: %v", err)
	}
	wantImg := stdlibPremulRGBA(t, src)
	got, _ := decodeRGBAForTest(t, src)

	// Composite both over an opaque white backdrop with draw.Over.
	backdrop := image.Rect(0, 0, wantImg.Rect.Dx(), wantImg.Rect.Dy())
	compWant := image.NewRGBA(backdrop)
	draw.Draw(compWant, backdrop, &image.Uniform{C: image.White}, image.Point{}, draw.Src)
	draw.Draw(compWant, backdrop, wantImg, image.Point{}, draw.Over)

	compGot := image.NewRGBA(backdrop)
	draw.Draw(compGot, backdrop, &image.Uniform{C: image.White}, image.Point{}, draw.Src)
	draw.Draw(compGot, backdrop, got, image.Point{}, draw.Over)

	if !bytes.Equal(compGot.Pix, compWant.Pix) {
		t.Errorf("Over composite Pix = %v, want %v", compGot.Pix, compWant.Pix)
	}
}

// TestIntegrationDecodeRGBA_WEBP_AlphaMatchesStdlibPremul verifies that
// DecodeRGBA output Pix matches the stdlib premultiplied oracle
// (webp.Decode composited into image.NewRGBA with draw.Src) on
// testdata/bricks-color.lossless.webp (160x120).
func TestIntegrationDecodeRGBA_WEBP_AlphaMatchesStdlibPremul(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "bricks-color.lossless.webp"))
	if err != nil {
		t.Fatalf("reading testdata/bricks-color.lossless.webp: %v", err)
	}
	img, err := webp.Decode(bytes.NewReader(src))
	if err != nil {
		t.Fatalf("webp.Decode: %v", err)
	}
	want := image.NewRGBA(img.Bounds())
	draw.Draw(want, want.Bounds(), img, image.Point{}, draw.Src)
	got, meta := decodeRGBAForTest(t, src)

	if meta.Width != uint32(want.Rect.Dx()) || meta.Height != uint32(want.Rect.Dy()) {
		t.Fatalf("Meta dimensions = (%d,%d), want (%d,%d)", meta.Width, meta.Height, want.Rect.Dx(), want.Rect.Dy())
	}
	if got.Stride != want.Rect.Dx()*4 {
		t.Fatalf("got.Stride = %d, want tight %d", got.Stride, want.Rect.Dx()*4)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Errorf("DecodeRGBA Pix = %v, want stdlib premul %v", got.Pix, want.Pix)
	}
}
