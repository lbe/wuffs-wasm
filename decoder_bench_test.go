package wuffs

import (
	"bytes"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/webp"
)

// benchDecodeRGBA benchmarks PNG decode of the named fixture into an RGBA
// image of the given decoded size (width×height). It allocates one correctly
// sized destination outside the timed loop, sizes the decoder's wasm slots
// via RequiredReserve before the timer, and reuses the same destination for
// every iteration so allocations occur only in setup.
func benchDecodeRGBA(b *testing.B, path string, width, height int) {
	src, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("reading %s: %v", path, err)
	}

	d := New()
	srcBytes := len(src)
	if resErr := RequiredReserve(d, width*height*4, srcBytes); resErr != nil {
		b.Fatalf("RequiredReserve: %v", resErr)
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := d.DecodeRGBA(dst, src); err != nil {
			b.Fatalf("DecodeRGBA: %v", err)
		}
	}
}

// BenchmarkDecodeRGBA_PNG_Harvesters benchmarks PNG decode for the large
// harvesters.png fixture (1165×859 RGBA, ~1.99 MB src, ~4 MiB dst).
func BenchmarkDecodeRGBA_PNG_Harvesters(b *testing.B) {
	requireBenchFixtures(b)
	benchDecodeRGBA(b, benchLHarvestersPNG, 1165, 859)
}

// BenchmarkDecodeRGBA_PNG_BricksColor benchmarks PNG decode for the smaller
// bricks-color.png fixture (160×120 RGBA).
func BenchmarkDecodeRGBA_PNG_BricksColor(b *testing.B) {
	benchDecodeRGBA(b, filepath.Join("testdata", "bricks-color.png"), 160, 120)
}

// BenchmarkDecodeRGBA_WEBP_BricksColor benchmarks lossless WebP decode for
// the bricks-color fixture (160×120 RGBA).
func BenchmarkDecodeRGBA_WEBP_BricksColor(b *testing.B) {
	benchDecodeRGBA(b, filepath.Join("testdata", "bricks-color.lossless.webp"), 160, 120)
}

// BenchmarkDecodeRGBA_GIF_BricksNodither benchmarks still GIF decode for the
// bricks-nodither fixture (160×120 RGBA, single frame).
func BenchmarkDecodeRGBA_GIF_BricksNodither(b *testing.B) {
	benchDecodeRGBA(b, filepath.Join("testdata", "bricks-nodither.gif"), 160, 120)
}

// BenchmarkDecodeRGBA_JPEG_Hat benchmarks JPEG decode for the hat fixture
// (90×112 RGBA, stride 360).
func BenchmarkDecodeRGBA_JPEG_Hat(b *testing.B) {
	benchDecodeRGBA(b, filepath.Join("testdata", "hat.jpeg"), 90, 112)
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

// benchStdlib benchmarks a stdlib image decoder for the given fixture. The
// file is read once outside the timed loop; each iteration decodes from a
// fresh bytes.Reader. Allocs are reported for comparison with the wuffs
// benches. Higher allocs than the wuffs hot path are expected.
func benchStdlib(b *testing.B, path, name string, decode func(io.Reader) (image.Image, error)) {
	src, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("reading %s: %v", path, err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := decode(bytes.NewReader(src)); err != nil {
			b.Fatalf("%s: %v", name, err)
		}
	}
}

// benchStdlibPNG benchmarks image/png.Decode for a PNG fixture. The file is
// read once outside the timed loop; each iteration decodes from a fresh
// bytes.Reader. Allocs are reported for comparison with the wuffs benches.
func benchStdlibPNG(b *testing.B, path string) {
	benchStdlib(b, path, "png.Decode", png.Decode)
}

// BenchmarkStdlibPNG_Harvesters benchmarks image/png.Decode for the large
// harvesters.png fixture (1165×859 RGBA, ~1.99 MB src).
func BenchmarkStdlibPNG_Harvesters(b *testing.B) {
	requireBenchFixtures(b)
	benchStdlibPNG(b, benchLHarvestersPNG)
}

// BenchmarkStdlibPNG_BricksColor benchmarks image/png.Decode for the smaller
// bricks-color.png fixture (160×120 RGBA).
func BenchmarkStdlibPNG_BricksColor(b *testing.B) {
	benchStdlibPNG(b, filepath.Join("testdata", "bricks-color.png"))
}

// BenchmarkStdlibWEBP_BricksColor benchmarks webp.Decode
// (golang.org/x/image/webp) for the lossless bricks-color fixture
// (160×120 RGBA).
func BenchmarkStdlibWEBP_BricksColor(b *testing.B) {
	benchStdlib(b, filepath.Join("testdata", "bricks-color.lossless.webp"), "webp.Decode", webp.Decode)
}

// BenchmarkStdlibGIF_BricksNodither benchmarks image/gif.Decode for the
// still bricks-nodither fixture (160×120, single frame).
func BenchmarkStdlibGIF_BricksNodither(b *testing.B) {
	benchStdlib(b, filepath.Join("testdata", "bricks-nodither.gif"), "gif.Decode", gif.Decode)
}

// BenchmarkStdlibJPEG_Hat benchmarks image/jpeg.Decode for the hat fixture
// (90×112).
func BenchmarkStdlibJPEG_Hat(b *testing.B) {
	benchStdlib(b, filepath.Join("testdata", "hat.jpeg"), "jpeg.Decode", jpeg.Decode)
}

func BenchmarkDecodeRGBA_PNG_HibiscusRegular(b *testing.B) {
	requireBenchFixtures(b)
	benchDecodeRGBA(b, benchMHibiscusPNG, 312, 442)
}

func BenchmarkStdlibPNG_HibiscusRegular(b *testing.B) {
	requireBenchFixtures(b)
	benchStdlibPNG(b, benchMHibiscusPNG)
}

func BenchmarkDecodeRGBA_JPEG_HibiscusRegular(b *testing.B) {
	requireBenchFixtures(b)
	benchDecodeRGBA(b, benchMHibiscusJPEG, 312, 442)
}

func BenchmarkStdlibJPEG_HibiscusRegular(b *testing.B) {
	requireBenchFixtures(b)
	benchStdlib(b, benchMHibiscusJPEG, "jpeg.Decode", jpeg.Decode)
}

func BenchmarkDecodeRGBA_WEBP_HibiscusRegular(b *testing.B) {
	requireBenchFixtures(b)
	benchDecodeRGBA(b, benchMHibiscusWEBP, 312, 442)
}

func BenchmarkStdlibWEBP_HibiscusRegular(b *testing.B) {
	requireBenchFixtures(b)
	benchStdlib(b, benchMHibiscusWEBP, "webp.Decode", webp.Decode)
}

func BenchmarkDecodeRGBA_GIF_HibiscusRegular(b *testing.B) {
	requireBenchFixtures(b)
	benchDecodeRGBA(b, benchMHibiscusGIF, 312, 442)
}

func BenchmarkStdlibGIF_HibiscusRegular(b *testing.B) {
	requireBenchFixtures(b)
	benchStdlib(b, benchMHibiscusGIF, "gif.Decode", gif.Decode)
}

func BenchmarkDecodeRGBA_JPEG_Harvesters(b *testing.B) {
	requireBenchFixtures(b)
	benchDecodeRGBA(b, benchLHarvestersJPEG, 1165, 859)
}

func BenchmarkStdlibJPEG_Harvesters(b *testing.B) {
	requireBenchFixtures(b)
	benchStdlib(b, benchLHarvestersJPEG, "jpeg.Decode", jpeg.Decode)
}

func BenchmarkDecodeRGBA_WEBP_HarvestersLossless(b *testing.B) {
	requireBenchFixtures(b)
	benchDecodeRGBA(b, benchLHarvestersWEBPLL, 1165, 859)
}

func BenchmarkStdlibWEBP_HarvestersLossless(b *testing.B) {
	requireBenchFixtures(b)
	benchStdlib(b, benchLHarvestersWEBPLL, "webp.Decode", webp.Decode)
}

func BenchmarkDecodeRGBA_WEBP_HarvestersLossy(b *testing.B) {
	requireBenchFixtures(b)
	benchDecodeRGBA(b, benchLHarvestersWEBPLY, 1165, 859)
}

func BenchmarkStdlibWEBP_HarvestersLossy(b *testing.B) {
	requireBenchFixtures(b)
	benchStdlib(b, benchLHarvestersWEBPLY, "webp.Decode", webp.Decode)
}

func BenchmarkDecodeRGBA_GIF_Harvesters(b *testing.B) {
	requireBenchFixtures(b)
	benchDecodeRGBA(b, benchLHarvestersGIF, 1165, 859)
}

func BenchmarkStdlibGIF_Harvesters(b *testing.B) {
	requireBenchFixtures(b)
	benchStdlib(b, benchLHarvestersGIF, "gif.Decode", gif.Decode)
}
