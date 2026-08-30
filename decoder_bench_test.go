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
// image of the given decoded size (width×height). It allocates one correctly
// sized destination outside the timed loop, sizes the decoder's wasm slots
// via RequiredReserve before the timer, and reuses the same destination for
// every iteration so allocations occur only in setup.
func benchDecodeRGBA(b *testing.B, pngPath string, width, height int) {
	pngSrc, err := os.ReadFile(pngPath)
	if err != nil {
		b.Fatalf("reading %s: %v", pngPath, err)
	}

	d := New()
	srcBytes := len(pngSrc)
	if resErr := RequiredReserve(d, width*height*4, srcBytes); resErr != nil {
		b.Fatalf("RequiredReserve: %v", resErr)
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := d.DecodeRGBA(dst, pngSrc); err != nil {
			b.Fatalf("DecodeRGBA: %v", err)
		}
	}
}

// BenchmarkDecodeRGBA_PNG_Harvesters benchmarks PNG decode for the large
// harvesters.png fixture (1165×859 RGBA, ~1.99 MB src, ~4 MiB dst).
func BenchmarkDecodeRGBA_PNG_Harvesters(b *testing.B) {
	benchDecodeRGBA(b, filepath.Join("testdata", "harvesters.png"), 1165, 859)
}

// BenchmarkDecodeRGBA_PNG_BricksColor benchmarks PNG decode for the smaller
// bricks-color.png fixture (160×120 RGBA).
func BenchmarkDecodeRGBA_PNG_BricksColor(b *testing.B) {
	benchDecodeRGBA(b, filepath.Join("testdata", "bricks-color.png"), 160, 120)
}

// BenchmarkDecodeNRGBA_PNG_AlphaPixels benchmarks DecodeNRGBA with a reused
// decoder and caller-owned destination after reservation and warm-up.
func BenchmarkDecodeNRGBA_PNG_AlphaPixels(b *testing.B) {
	pngSrc, err := os.ReadFile(filepath.Join("testdata", "alpha-pixels.png"))
	if err != nil {
		b.Fatalf("reading alpha fixture: %v", err)
	}
	d := New()
	if err := RequiredReserve(d, 3*1*4, len(pngSrc)); err != nil {
		b.Fatalf("RequiredReserve: %v", err)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	if _, err := d.DecodeNRGBA(dst, pngSrc); err != nil {
		b.Fatalf("warmup DecodeNRGBA: %v", err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := d.DecodeNRGBA(dst, pngSrc); err != nil {
			b.Fatalf("DecodeNRGBA: %v", err)
		}
	}
}

// benchDecodeGray benchmarks PNG decode into a reused Gray destination.
func benchDecodeGray(b *testing.B, pngPath string, width, height int) {
	pngSrc, err := os.ReadFile(pngPath)
	if err != nil {
		b.Fatalf("reading %s: %v", pngPath, err)
	}

	d := New()
	if resErr := RequiredReserve(d, width*height*4, len(pngSrc)); resErr != nil {
		b.Fatalf("RequiredReserve: %v", resErr)
	}
	dst := image.NewGray(image.Rect(0, 0, width, height))
	if _, err := d.DecodeGray(dst, pngSrc); err != nil {
		b.Fatalf("warmup DecodeGray: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := d.DecodeGray(dst, pngSrc); err != nil {
			b.Fatalf("DecodeGray: %v", err)
		}
	}
}

// BenchmarkDecodeGray_PNG_BricksColor benchmarks Gray decode for the
// bricks-color.png fixture (160×120), reusing decoder and destination storage.
func BenchmarkDecodeGray_PNG_BricksColor(b *testing.B) {
	benchDecodeGray(b, filepath.Join("testdata", "bricks-color.png"), 160, 120)
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
