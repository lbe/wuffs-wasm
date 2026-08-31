package wuffs_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// TestIntegrationDecodeConfigReturnsStandardImageConfig verifies that the
// package-level DecodeConfig function returns exact width, height, and
// color.RGBAModel for valid PNG and lossless WebP input, including input
// larger than the default source slot. It also covers error paths: empty,
// unknown-format, and non-config-readable truncated input.
func TestIntegrationDecodeConfigReturnsStandardImageConfig(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")
	webpSrc := loadFixture(t, "bricks-color.lossless.webp")

	const (
		wantW = 160
		wantH = 120
	)

	t.Run("valid PNG returns RGBAModel with correct dimensions", func(t *testing.T) {
		cfg, err := wuffs.DecodeConfig(pngSrc)
		if err != nil {
			t.Fatalf("DecodeConfig(PNG) unexpected error: %v", err)
		}
		if cfg.ColorModel != color.RGBAModel {
			t.Errorf("DecodeConfig(PNG) ColorModel = %v, want color.RGBAModel", cfg.ColorModel)
		}
		if cfg.Width != wantW {
			t.Errorf("DecodeConfig(PNG) Width = %d, want %d", cfg.Width, wantW)
		}
		if cfg.Height != wantH {
			t.Errorf("DecodeConfig(PNG) Height = %d, want %d", cfg.Height, wantH)
		}
	})

	t.Run("valid lossless WebP returns RGBAModel with correct dimensions", func(t *testing.T) {
		cfg, err := wuffs.DecodeConfig(webpSrc)
		if err != nil {
			t.Fatalf("DecodeConfig(WebP) unexpected error: %v", err)
		}
		if cfg.ColorModel != color.RGBAModel {
			t.Errorf("DecodeConfig(WebP) ColorModel = %v, want color.RGBAModel", cfg.ColorModel)
		}
		if cfg.Width != wantW {
			t.Errorf("DecodeConfig(WebP) Width = %d, want %d", cfg.Width, wantW)
		}
		if cfg.Height != wantH {
			t.Errorf("DecodeConfig(WebP) Height = %d, want %d", cfg.Height, wantH)
		}
	})

	t.Run("input larger than default source slot yields correct config", func(t *testing.T) {
		// Pad the PNG payload to exceed the 64 KiB default source slot, then
		// verify DecodeConfig grows the slot internally and returns correct metadata.
		large := make([]byte, 128*1024) // 128 KiB > defaultSrcCap (64 KiB)
		copy(large, pngSrc)
		// Wuffs accepts trailing garbage after a valid image stream.

		cfg, err := wuffs.DecodeConfig(large)
		if err != nil {
			t.Fatalf("DecodeConfig(large PNG) unexpected error: %v", err)
		}
		if cfg.ColorModel != color.RGBAModel {
			t.Errorf("DecodeConfig(large PNG) ColorModel = %v, want color.RGBAModel", cfg.ColorModel)
		}
		if cfg.Width != wantW {
			t.Errorf("DecodeConfig(large PNG) Width = %d, want %d", cfg.Width, wantW)
		}
		if cfg.Height != wantH {
			t.Errorf("DecodeConfig(large PNG) Height = %d, want %d", cfg.Height, wantH)
		}
	})

	t.Run("truncated config-readable input succeeds when Probe succeeds", func(t *testing.T) {
		// A truncated PNG header that Probe can parse. Wuffs needs at least
		// 50 bytes of PNG data (signature + IHDR + partial scan data) to
		// decode the image config.
		truncated := pngSrc[:50]

		// Verify Probe succeeds on this truncated input.
		meta, pErr := wuffs.Probe(truncated)
		if pErr != nil {
			t.Fatalf("Probe(truncated PNG) must succeed for config-readable input, got: %v", pErr)
		}
		if meta == nil {
			t.Fatal("Probe(truncated PNG) returned nil Meta")
		}

		// DecodeConfig must also succeed and yield the corresponding config.
		cfg, err := wuffs.DecodeConfig(truncated)
		if err != nil {
			t.Fatalf("DecodeConfig(truncated PNG) expected success matching Probe, got: %v", err)
		}
		if cfg.ColorModel != color.RGBAModel {
			t.Errorf("DecodeConfig(truncated PNG) ColorModel = %v, want color.RGBAModel", cfg.ColorModel)
		}
		if cfg.Width != int(meta.Width) {
			t.Errorf("DecodeConfig(truncated PNG) Width = %d, want %d (from Probe)", cfg.Width, meta.Width)
		}
		if cfg.Height != int(meta.Height) {
			t.Errorf("DecodeConfig(truncated PNG) Height = %d, want %d (from Probe)", cfg.Height, meta.Height)
		}
	})

	t.Run("empty input returns zero config and exact ErrDecode", func(t *testing.T) {
		cfg, err := wuffs.DecodeConfig(nil)
		//nolint:errorlint // exact identity required by contract; primitive sentinel must match
		if err != wuffs.ErrDecode {
			t.Fatalf("DecodeConfig(nil) err = %v, want exact %v", err, wuffs.ErrDecode)
		}
		if cfg != (image.Config{}) {
			t.Errorf("DecodeConfig(nil) Config = %v, want zero image.Config", cfg)
		}
	})

	t.Run("unknown format returns zero config and exact ErrUnknownFormat", func(t *testing.T) {
		cfg, err := wuffs.DecodeConfig([]byte("not a known image format header"))
		//nolint:errorlint // exact identity required by contract; primitive sentinel must match
		if err != wuffs.ErrUnknownFormat {
			t.Fatalf("DecodeConfig(unknown) err = %v, want exact %v", err, wuffs.ErrUnknownFormat)
		}
		if cfg != (image.Config{}) {
			t.Errorf("DecodeConfig(unknown) Config = %v, want zero image.Config", cfg)
		}
	})

	t.Run("non-config-readable truncated input returns zero config and exact primitive error", func(t *testing.T) {
		// A one-byte PNG "header" is too short for any format sniffing.
		tooShort := []byte{0x89}

		// Verify Probe fails on this input.
		meta, pErr := wuffs.Probe(tooShort)
		if pErr == nil {
			t.Fatal("Probe(tooShort) must fail for non-config-readable input")
		}
		if meta != nil {
			t.Errorf("Probe(tooShort) returned non-nil Meta %v, want nil", meta)
		}

		// DecodeConfig must also fail with the exact same primitive error.
		cfg, err := wuffs.DecodeConfig(tooShort)
		//nolint:errorlint // exact identity required by contract; primitive sentinel must match
		if err != pErr {
			t.Fatalf("DecodeConfig(tooShort) err = %v, want exact %v", err, pErr)
		}
		if cfg != (image.Config{}) {
			t.Errorf("DecodeConfig(tooShort) Config = %v, want zero image.Config", cfg)
		}
	})
}
