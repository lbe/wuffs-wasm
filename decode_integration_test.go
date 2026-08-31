package wuffs_test

import (
	"bytes"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/lbe/wuffs-wasm"
)

// TestIntegrationDecodeAllocatesTightRGBA verifies that the package-level
// Decode function allocates a tightly packed *image.RGBA and returns a
// detached *Meta for valid PNG and lossless WebP input. It covers:
//
//   - Exact pixel bytes for the alpha fixture (3×1)
//   - Expected dimensions and FormatWEBP for WebP
//   - Host stride and BytesWritten reported by Meta
//   - Large valid input succeeds through automatic source and destination
//     reservation
//   - Separate calls return independent images and Meta values (distinct
//     pointers, no shared state)
//   - Unknown format returns nil, nil, ErrUnknownFormat
//   - Empty and truncated sources return nil, nil, error without panic
//
// Source-Reserve failures and bad-metadata geometry are covered by the shared
// typed matrix (TestUnitTypedMatrix) through the decoder seam, not here.
func TestIntegrationDecodeAllocatesTightRGBA(t *testing.T) {
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
	t.Run("valid PNG returns tight RGBA and detached Meta", func(t *testing.T) {
		img, meta, err := wuffs.Decode(pngSrc)
		if err != nil {
			t.Fatalf("Decode(pngSrc): %v", err)
		}
		if img == nil {
			t.Fatal("Decode(pngSrc) returned nil *image.RGBA, want non-nil")
		}
		if meta == nil {
			t.Fatal("Decode(pngSrc) returned nil *Meta, want non-nil")
		}

		// Tightly packed: Stride == Dx*4, len(Pix) == Stride*Dy.
		wantStride := pngW * 4
		if img.Stride != wantStride {
			t.Errorf("img.Stride = %d, want %d (Width*4)", img.Stride, wantStride)
		}
		if img.Rect.Dx() != pngW || img.Rect.Dy() != pngH {
			t.Errorf("img.Rect = %v, want 160x120", img.Rect)
		}
		if len(img.Pix) != wantStride*pngH {
			t.Errorf("len(img.Pix) = %d, want %d", len(img.Pix), wantStride*pngH)
		}

		// Meta reports host stride (Width*4) and BytesWritten.
		if meta.Width != uint32(pngW) || meta.Height != uint32(pngH) {
			t.Errorf("Meta dimensions = (%d,%d), want (%d,%d)", meta.Width, meta.Height, pngW, pngH)
		}
		if meta.Stride != uint32(wantStride) {
			t.Errorf("Meta.Stride = %d, want %d", meta.Stride, wantStride)
		}
		if meta.BytesWritten != uint32(wantStride*pngH) {
			t.Errorf("Meta.BytesWritten = %d, want %d", meta.BytesWritten, wantStride*pngH)
		}
		if meta.Format != wuffs.FormatPNG {
			t.Errorf("Meta.Format = 0x%08X, want 0x%08X (FormatPNG)", meta.Format, wuffs.FormatPNG)
		}

		// CRC32 must match golden.
		gotCRC := crc32.ChecksumIEEE(img.Pix)
		raw, rerr := os.ReadFile(filepath.Join("testdata", "bricks-color.golden.crc32"))
		if rerr != nil {
			t.Fatalf("reading golden CRC: %v", rerr)
		}
		wantCRC, perr := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 32)
		if perr != nil {
			t.Fatalf("parsing golden CRC: %v", perr)
		}
		if gotCRC != uint32(wantCRC) {
			t.Errorf("CRC32 of decoded Pix = 0x%08X, want golden 0x%08X", gotCRC, uint32(wantCRC))
		}
	})

	// ──────────────────────────────────────────────
	// Success: valid WebP
	// ──────────────────────────────────────────────
	t.Run("valid WebP returns tight RGBA and FormatWEBP", func(t *testing.T) {
		img, meta, err := wuffs.Decode(webpSrc)
		if err != nil {
			t.Fatalf("Decode(webpSrc): %v", err)
		}
		if img == nil {
			t.Fatal("Decode(webpSrc) returned nil *image.RGBA, want non-nil")
		}
		if meta == nil {
			t.Fatal("Decode(webpSrc) returned nil *Meta, want non-nil")
		}

		// Tightly packed.
		wantStride := webpW * 4
		if img.Stride != wantStride {
			t.Errorf("img.Stride = %d, want %d", img.Stride, wantStride)
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
	})

	// ──────────────────────────────────────────────
	// Success: alpha fixture exact pixels
	// ──────────────────────────────────────────────
	t.Run("alpha fixture has exact expected pixel bytes", func(t *testing.T) {
		img, meta, err := wuffs.Decode(alphaSrc)
		if err != nil {
			t.Fatalf("Decode(alphaSrc): %v", err)
		}
		if img == nil {
			t.Fatal("Decode(alphaSrc) returned nil *image.RGBA, want non-nil")
		}
		if meta == nil {
			t.Fatal("Decode(alphaSrc) returned nil *Meta, want non-nil")
		}
		if meta.Width != alphaW || meta.Height != alphaH {
			t.Fatalf("Meta dimensions = (%d,%d), want (%d,%d)", meta.Width, meta.Height, alphaW, alphaH)
		}

		// Tightly packed.
		wantStride := alphaW * 4
		if img.Stride != wantStride {
			t.Errorf("img.Stride = %d, want %d", img.Stride, wantStride)
		}

		// Expected pixel bytes matching the NRGBA integration test pattern.
		wantPixels := []byte{
			0x12, 0x34, 0x56, 0xFF, // opaque pixel
			0xFF, 0x00, 0xFF, 0x80, // translucent pixel
			0x00, 0x00, 0x00, 0x00, // fully transparent pixel
		}
		if !bytes.Equal(img.Pix[:alphaW*4], wantPixels) {
			t.Errorf("img.Pix = % X, want % X", img.Pix[:alphaW*4], wantPixels)
		}
	})

	// ──────────────────────────────────────────────
	// Success: large valid input
	// ──────────────────────────────────────────────
	t.Run("large valid input succeeds through automatic reservation", func(t *testing.T) {
		img, meta, err := wuffs.Decode(harvestSrc)
		if err != nil {
			t.Fatalf("Decode(harvestSrc): %v", err)
		}
		if img == nil {
			t.Fatal("Decode(harvestSrc) returned nil *image.RGBA, want non-nil")
		}
		if meta == nil {
			t.Fatal("Decode(harvestSrc) returned nil *Meta, want non-nil")
		}
		// harvesters.png is 1165×859
		if meta.Width != 1165 || meta.Height != 859 {
			t.Errorf("Meta dimensions = (%d,%d), want (1165,859)", meta.Width, meta.Height)
		}
		// Tightly packed.
		wantStride := 1165 * 4
		if img.Stride != wantStride {
			t.Errorf("img.Stride = %d, want %d", img.Stride, wantStride)
		}
	})

	// ──────────────────────────────────────────────
	// Independent calls
	// ──────────────────────────────────────────────
	t.Run("separate calls return independent images and Meta values", func(t *testing.T) {
		img1, meta1, err1 := wuffs.Decode(pngSrc)
		if err1 != nil {
			t.Fatalf("first Decode: %v", err1)
		}
		img2, meta2, err2 := wuffs.Decode(pngSrc)
		if err2 != nil {
			t.Fatalf("second Decode: %v", err2)
		}
		if img1 == nil || img2 == nil {
			t.Fatal("both Decode calls returned nil images")
		}
		if meta1 == nil || meta2 == nil {
			t.Fatal("both Decode calls returned nil Metas")
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
	// Bad metadata cases
	// ──────────────────────────────────────────────
	t.Run("garbage source returns ErrUnknownFormat with nil outputs", func(t *testing.T) {
		garbage := []byte("\x00garbage: not an image\xff\xfe\x00\x01")
		img, meta, err := wuffs.Decode(garbage)
		if err == nil {
			t.Fatal("Decode(garbage) error = nil, want error")
		}
		if !errors.Is(err, wuffs.ErrUnknownFormat) {
			t.Errorf("Decode(garbage) error = %v, want ErrUnknownFormat", err)
		}
		if img != nil {
			t.Errorf("Decode(garbage) returned non-nil image, want nil")
		}
		if meta != nil {
			t.Errorf("Decode(garbage) returned non-nil Meta, want nil")
		}
	})

	t.Run("empty source returns ErrDecode with nil outputs", func(t *testing.T) {
		img, meta, err := wuffs.Decode(nil)
		if err == nil {
			t.Fatal("Decode(nil) error = nil, want ErrDecode")
		}
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Errorf("Decode(nil) error = %v, want ErrDecode", err)
		}
		if img != nil {
			t.Errorf("Decode(nil) returned non-nil image, want nil")
		}
		if meta != nil {
			t.Errorf("Decode(nil) returned non-nil Meta, want nil")
		}
	})

	t.Run("truncated source returns ErrDecode with nil outputs", func(t *testing.T) {
		truncated := pngSrc[:20]
		img, meta, err := wuffs.Decode(truncated)
		if err == nil {
			t.Fatal("Decode(truncated) error = nil, want ErrDecode")
		}
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Errorf("Decode(truncated) error = %v, want ErrDecode", err)
		}
		if img != nil {
			t.Errorf("Decode(truncated) returned non-nil image, want nil")
		}
		if meta != nil {
			t.Errorf("Decode(truncated) returned non-nil Meta, want nil")
		}
	})
}
