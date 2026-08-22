package wuffs

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// benchDecodeRGBA benchmarks PNG decode of the named fixture into an RGBA
// image of the given decoded size (width×height×4). It sizes the decoder's
// wasm slots via RequiredReserve before the timed loop.
func benchDecodeRGBA(b *testing.B, pngPath string, dstBytes int) {
	pngSrc, err := os.ReadFile(pngPath)
	if err != nil {
		b.Fatalf("reading %s: %v", pngPath, err)
	}

	d := New()
	srcBytes := len(pngSrc)
	if resErr := RequiredReserve(d, dstBytes, srcBytes); resErr != nil {
		b.Fatalf("RequiredReserve: %v", resErr)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dst := image.NewRGBA(image.Rect(0, 0, 0, 0))
		if _, err := d.DecodeRGBA(dst, pngSrc); err != nil {
			b.Fatalf("DecodeRGBA: %v", err)
		}
	}
}

// BenchmarkDecodeRGBA_PNG_Harvesters benchmarks PNG decode for the large
// harvesters.png fixture (1165×859 RGBA, ~1.99 MB src, ~4 MiB dst).
func BenchmarkDecodeRGBA_PNG_Harvesters(b *testing.B) {
	benchDecodeRGBA(b, filepath.Join("testdata", "harvesters.png"), 1165*859*4)
}

// BenchmarkDecodeRGBA_PNG_BricksColor benchmarks PNG decode for the smaller
// bricks-color.png fixture (160×120 RGBA).
func BenchmarkDecodeRGBA_PNG_BricksColor(b *testing.B) {
	benchDecodeRGBA(b, filepath.Join("testdata", "bricks-color.png"), 160*120*4)
}

// benchStdlibPNG benchmarks image/png.Decode for a PNG fixture. The file is
// read once outside the timed loop; each iteration decodes from a fresh
// bytes.Reader. Allocs are reported for comparison with the wuffs benches.
func benchStdlibPNG(b *testing.B, pngPath string) {
	pngSrc, err := os.ReadFile(pngPath)
	if err != nil {
		b.Fatalf("reading %s: %v", pngPath, err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := png.Decode(bytes.NewReader(pngSrc)); err != nil {
			b.Fatalf("png.Decode: %v", err)
		}
	}
}

// BenchmarkStdlibPNG_Harvesters benchmarks image/png.Decode for the large
// harvesters.png fixture (1165×859 RGBA, ~1.99 MB src).
func BenchmarkStdlibPNG_Harvesters(b *testing.B) {
	benchStdlibPNG(b, filepath.Join("testdata", "harvesters.png"))
}

// BenchmarkStdlibPNG_BricksColor benchmarks image/png.Decode for the smaller
// bricks-color.png fixture (160×120 RGBA).
func BenchmarkStdlibPNG_BricksColor(b *testing.B) {
	benchStdlibPNG(b, filepath.Join("testdata", "bricks-color.png"))
}
