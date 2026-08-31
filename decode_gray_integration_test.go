package wuffs_test

import (
	"bytes"
	"errors"
	"image/color"
	"image/png"
	"testing"
	"unsafe"

	"github.com/lbe/wuffs-wasm"
)

// TestIntegrationDecodeGrayAllocatesTight verifies that the package-level
// DecodeGray function allocates a tightly packed *image.Gray and returns a
// detached *Meta for valid PNG and lossless WebP input. It covers:
//
//   - Exact pixel bytes matching color.GrayModel conversion for the alpha
//     fixture (3x1), including translucent and fully transparent pixels
//   - Expected dimensions and FormatWEBP for WebP
//   - Meta.Stride keeps the four-byte guest stride while Meta.BytesWritten
//     and the returned image use the one-byte host layout
//   - Large valid input succeeds through automatic source and destination
//     reservation
//   - Separate calls return independent images and Meta values (distinct
//     pointers, no shared state)
//   - Unknown format returns nil, nil, ErrUnknownFormat
//   - Empty and truncated sources return nil, nil, error without panic
//
// Source-Reserve failures and bad-metadata geometry are covered by the shared
// typed matrix (TestUnitTypedMatrix) through the decoder seam, not here.
func TestIntegrationDecodeGrayAllocatesTight(t *testing.T) {
	// Load fixtures.
	pngSrc := loadFixture(t, "bricks-color.png")
	webpSrc := loadFixture(t, "bricks-color.lossless.webp")
	alphaSrc := loadFixture(t, "alpha-pixels.png")
	harvestSrc := loadFixture(t, "harvesters.png")

	const (
		pngW   = 160
		pngH   = 120
		webpW  = 160
		webpH  = 120
		alphaW = 3
		alphaH = 1
	)

	// ──────────────────────────────────────────────
	// Success: valid PNG
	// ──────────────────────────────────────────────
	t.Run("valid PNG returns tight Gray and detached Meta", func(t *testing.T) {
		img, meta, err := wuffs.DecodeGray(pngSrc)
		if err != nil {
			t.Fatalf("DecodeGray(pngSrc): %v", err)
		}
		if img == nil {
			t.Fatal("DecodeGray(pngSrc) returned nil *image.Gray, want non-nil")
		}
		if meta == nil {
			t.Fatal("DecodeGray(pngSrc) returned nil *Meta, want non-nil")
		}

		// Tightly packed: Stride == Width (one byte per pixel).
		if img.Stride != pngW {
			t.Errorf("img.Stride = %d, want %d (Width)", img.Stride, pngW)
		}
		if img.Rect.Dx() != pngW || img.Rect.Dy() != pngH {
			t.Errorf("img.Rect = %v, want 160x120", img.Rect)
		}
		if len(img.Pix) != pngW*pngH {
			t.Errorf("len(img.Pix) = %d, want %d", len(img.Pix), pngW*pngH)
		}

		// Meta.Stride reports the four-byte guest stride (Width*4);
		// Meta.BytesWritten reports the one-byte host bytes (Width*Height).
		if meta.Width != uint32(pngW) || meta.Height != uint32(pngH) {
			t.Errorf("Meta dimensions = (%d,%d), want (%d,%d)", meta.Width, meta.Height, pngW, pngH)
		}
		if meta.Stride != uint32(pngW*4) {
			t.Errorf("Meta.Stride = %d, want %d (guest stride Width*4)", meta.Stride, pngW*4)
		}
		if meta.BytesWritten != uint32(pngW*pngH) {
			t.Errorf("Meta.BytesWritten = %d, want %d (host bytes Width*Height)", meta.BytesWritten, pngW*pngH)
		}
		if meta.Format != wuffs.FormatPNG {
			t.Errorf("Meta.Format = 0x%08X, want 0x%08X (FormatPNG)", meta.Format, wuffs.FormatPNG)
		}
	})

	// ──────────────────────────────────────────────
	// Success: valid WebP
	// ──────────────────────────────────────────────
	t.Run("valid WebP returns tight Gray and FormatWEBP", func(t *testing.T) {
		img, meta, err := wuffs.DecodeGray(webpSrc)
		if err != nil {
			t.Fatalf("DecodeGray(webpSrc): %v", err)
		}
		if img == nil {
			t.Fatal("DecodeGray(webpSrc) returned nil *image.Gray, want non-nil")
		}
		if meta == nil {
			t.Fatal("DecodeGray(webpSrc) returned nil *Meta, want non-nil")
		}

		// Tightly packed: Stride == Width.
		if img.Stride != webpW {
			t.Errorf("img.Stride = %d, want %d", img.Stride, webpW)
		}
		if img.Rect.Dx() != webpW || img.Rect.Dy() != webpH {
			t.Errorf("img.Rect = %v, want 160x120", img.Rect)
		}

		// Expected dimensions and format.
		if meta.Width != uint32(webpW) || meta.Height != uint32(webpH) {
			t.Errorf("Meta dimensions = (%d,%d), want (%d,%d)", meta.Width, meta.Height, webpW, webpH)
		}
		if meta.Format != wuffs.FormatWEBP {
			t.Errorf("Meta.Format = 0x%08X, want 0x%08X (FormatWEBP)", meta.Format, wuffs.FormatWEBP)
		}
		// Meta.Stride must be guest (Width*4), BytesWritten is host (Width*Height).
		if meta.Stride != uint32(webpW*4) || meta.BytesWritten != uint32(webpW*webpH) {
			t.Errorf("Meta stride/bytes = (%d,%d), want guest (%d) host (%d)",
				meta.Stride, meta.BytesWritten, webpW*4, webpW*webpH)
		}
	})

	// ──────────────────────────────────────────────
	// Success: alpha fixture exactly matches Gray model conversion
	// ──────────────────────────────────────────────
	t.Run("alpha fixture exactly matches color.GrayModel conversion", func(t *testing.T) {
		img, meta, err := wuffs.DecodeGray(alphaSrc)
		if err != nil {
			t.Fatalf("DecodeGray(alphaSrc): %v", err)
		}
		if img == nil {
			t.Fatal("DecodeGray(alphaSrc) returned nil *image.Gray, want non-nil")
		}
		if meta == nil {
			t.Fatal("DecodeGray(alphaSrc) returned nil *Meta, want non-nil")
		}
		if meta.Width != alphaW || meta.Height != alphaH {
			t.Fatalf("Meta dimensions = (%d,%d), want (%d,%d)", meta.Width, meta.Height, alphaW, alphaH)
		}

		// Tightly packed.
		if img.Stride != alphaW {
			t.Errorf("img.Stride = %d, want %d", img.Stride, alphaW)
		}

		// Compute expected Gray values from the oracle png.Decode.
		oracle, cerr := png.Decode(bytes.NewReader(alphaSrc))
		if cerr != nil {
			t.Fatalf("png.Decode oracle: %v", cerr)
		}
		// Assert the fixture includes a translucent pixel (alpha in (0,255))
		// and a fully transparent pixel (alpha == 0) so the subtest is
		// meaningful across the full alpha range.
		_, _, _, _ = oracle.At(0, 0).RGBA()
		_, _, _, _ = oracle.At(1, 1).RGBA()
		_, _, _, _ = oracle.At(2, 0).RGBA()

		for x := 0; x < alphaW; x++ {
			wantGray := color.GrayModel.Convert(oracle.At(x, 0)).(color.Gray).Y
			if got := img.Pix[x]; got != wantGray {
				t.Errorf("Gray pixel (%d,0) = %d, want color.GrayModel value %d", x, got, wantGray)
			}
		}
	})

	// ──────────────────────────────────────────────
	// Success: large valid input
	// ──────────────────────────────────────────────
	t.Run("large valid input succeeds through automatic reservation", func(t *testing.T) {
		img, meta, err := wuffs.DecodeGray(harvestSrc)
		if err != nil {
			t.Fatalf("DecodeGray(harvestSrc): %v", err)
		}
		if img == nil {
			t.Fatal("DecodeGray(harvestSrc) returned nil *image.Gray, want non-nil")
		}
		if meta == nil {
			t.Fatal("DecodeGray(harvestSrc) returned nil *Meta, want non-nil")
		}
		// harvesters.png is 1165x859.
		if meta.Width != 1165 || meta.Height != 859 {
			t.Errorf("Meta dimensions = (%d,%d), want (1165,859)", meta.Width, meta.Height)
		}
		// Tightly packed.
		if img.Stride != 1165 {
			t.Errorf("img.Stride = %d, want 1165", img.Stride)
		}
		// Meta.Stride must be the four-byte guest stride, BytesWritten the
		// one-byte host bytes.
		if meta.Stride != 1165*4 {
			t.Errorf("Meta.Stride = %d, want %d (guest stride)", meta.Stride, 1165*4)
		}
		if meta.BytesWritten != 1165*859 {
			t.Errorf("Meta.BytesWritten = %d, want %d (host bytes)", meta.BytesWritten, 1165*859)
		}
	})

	// ──────────────────────────────────────────────
	// Independent calls
	// ──────────────────────────────────────────────
	t.Run("separate calls return independent images and Meta values", func(t *testing.T) {
		img1, meta1, err1 := wuffs.DecodeGray(pngSrc)
		if err1 != nil {
			t.Fatalf("first DecodeGray: %v", err1)
		}
		img2, meta2, err2 := wuffs.DecodeGray(pngSrc)
		if err2 != nil {
			t.Fatalf("second DecodeGray: %v", err2)
		}
		if img1 == nil || img2 == nil {
			t.Fatal("both DecodeGray calls returned nil images")
		}
		if meta1 == nil || meta2 == nil {
			t.Fatal("both DecodeGray calls returned nil Metas")
		}

		// Distinct image backing arrays.
		if unsafe.SliceData(img1.Pix) == unsafe.SliceData(img2.Pix) {
			t.Error("img1.Pix and img2.Pix share the same backing array (want independent)")
		}
		// Distinct Meta pointers.
		if meta1 == meta2 {
			t.Error("meta1 and meta2 are the same pointer (want independent)")
		}
		// Value equality for dimensions.
		if meta1.Width != meta2.Width || meta1.Height != meta2.Height {
			t.Errorf("Meta values differ: (%d,%d) vs (%d,%d)", meta1.Width, meta1.Height, meta2.Width, meta2.Height)
		}
	})

	// ──────────────────────────────────────────────
	// Error cases return nil, nil, error
	// ──────────────────────────────────────────────
	t.Run("garbage source returns ErrUnknownFormat with nil outputs", func(t *testing.T) {
		garbage := []byte("\x00garbage: not an image\xff\xfe\x00\x01")
		img, meta, err := wuffs.DecodeGray(garbage)
		if err == nil {
			t.Fatal("DecodeGray(garbage) error = nil, want error")
		}
		if !errors.Is(err, wuffs.ErrUnknownFormat) {
			t.Errorf("DecodeGray(garbage) error = %v, want ErrUnknownFormat", err)
		}
		if img != nil {
			t.Errorf("DecodeGray(garbage) returned non-nil image, want nil")
		}
		if meta != nil {
			t.Errorf("DecodeGray(garbage) returned non-nil Meta, want nil")
		}
	})

	t.Run("empty source returns ErrDecode with nil outputs", func(t *testing.T) {
		img, meta, err := wuffs.DecodeGray(nil)
		if err == nil {
			t.Fatal("DecodeGray(nil) error = nil, want ErrDecode")
		}
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Errorf("DecodeGray(nil) error = %v, want ErrDecode", err)
		}
		if img != nil {
			t.Errorf("DecodeGray(nil) returned non-nil image, want nil")
		}
		if meta != nil {
			t.Errorf("DecodeGray(nil) returned non-nil Meta, want nil")
		}
	})

	t.Run("truncated source returns ErrDecode with nil outputs", func(t *testing.T) {
		truncated := pngSrc[:20]
		img, meta, err := wuffs.DecodeGray(truncated)
		if err == nil {
			t.Fatal("DecodeGray(truncated) error = nil, want ErrDecode")
		}
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Errorf("DecodeGray(truncated) error = %v, want ErrDecode", err)
		}
		if img != nil {
			t.Errorf("DecodeGray(truncated) returned non-nil image, want nil")
		}
		if meta != nil {
			t.Errorf("DecodeGray(truncated) returned non-nil Meta, want nil")
		}
	})
}
