package wuffs_test

import (
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// TestIntegrationPackageProbeMatchesDecoderProbe verifies that the
// package-level Probe function returns the same metadata as the
// per-decoder (*Decoder).Probe for valid PNG and lossless WebP input,
// including Width, Height, Stride, Format, and BytesWritten == 0 (req 1).
func TestIntegrationPackageProbeMatchesDecoderProbe(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")
	webpSrc := loadFixture(t, "bricks-color.lossless.webp")

	const (
		wantW = 160
		wantH = 120
	)

	tests := []struct {
		name    string
		src     []byte
		wantFmt uint32
	}{
		{"PNG", pngSrc, wuffs.FormatPNG},
		{"lossless WebP", webpSrc, wuffs.FormatWEBP},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Decoder-level Probe: pre-reserve then probe.
			d := wuffs.New()
			if err := d.Reserve(0, len(tc.src)); err != nil {
				t.Fatalf("decoder Reserve: %v", err)
			}
			decMeta, decErr := d.Probe(tc.src)
			if decErr != nil {
				t.Fatalf("decoder Probe: %v", decErr)
			}
			if decMeta == nil {
				t.Fatal("decoder Probe returned nil Meta")
			}

			// Package-level Probe.
			pkgMeta, pkgErr := wuffs.Probe(tc.src)
			if pkgErr != nil {
				t.Fatalf("package Probe: %v", pkgErr)
			}
			if pkgMeta == nil {
				t.Fatal("package Probe returned nil Meta")
			}

			// Fields must match.
			if pkgMeta.Err != decMeta.Err {
				t.Errorf("Meta.Err = %d, want %d", pkgMeta.Err, decMeta.Err)
			}
			if pkgMeta.Width != decMeta.Width {
				t.Errorf("Meta.Width = %d, want %d", pkgMeta.Width, decMeta.Width)
			}
			if pkgMeta.Height != decMeta.Height {
				t.Errorf("Meta.Height = %d, want %d", pkgMeta.Height, decMeta.Height)
			}
			if pkgMeta.Stride != decMeta.Stride {
				t.Errorf("Meta.Stride = %d, want %d", pkgMeta.Stride, decMeta.Stride)
			}
			if pkgMeta.Format != decMeta.Format {
				t.Errorf("Meta.Format = 0x%08X, want 0x%08X", pkgMeta.Format, decMeta.Format)
			}
			if pkgMeta.Format != tc.wantFmt {
				t.Errorf("Meta.Format = 0x%08X, want 0x%08X", pkgMeta.Format, tc.wantFmt)
			}

			// BytesWritten must be 0 (no pixel decode).
			if pkgMeta.BytesWritten != 0 {
				t.Errorf("Meta.BytesWritten = %d, want 0", pkgMeta.BytesWritten)
			}
			if decMeta.BytesWritten != 0 {
				t.Errorf("decoder Meta.BytesWritten = %d, want 0", decMeta.BytesWritten)
			}

			// Dimensions must match expected.
			if pkgMeta.Width != wantW || pkgMeta.Height != wantH {
				t.Errorf("package Meta dimensions = (%d,%d), want (%d,%d)", pkgMeta.Width, pkgMeta.Height, wantW, wantH)
			}
		})
	}
}

// TestIntegrationPackageProbeLargeInputSucceeds verifies that valid encoded
// input larger than the default source slot (64 KiB) succeeds through the
// package-level Probe (req 2).
func TestIntegrationPackageProbeLargeInputSucceeds(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	// Pad to 128 KiB, exceeding the default 64 KiB source slot.
	large := make([]byte, 128*1024)
	copy(large, pngSrc)

	meta, err := wuffs.Probe(large)
	if err != nil {
		t.Fatalf("Probe(large): %v", err)
	}
	if meta == nil {
		t.Fatal("Probe(large) returned nil Meta")
	}

	const wantW, wantH = 160, 120
	if meta.Width != wantW || meta.Height != wantH {
		t.Errorf("Meta dimensions = (%d,%d), want (%d,%d)", meta.Width, meta.Height, wantW, wantH)
	}
	if meta.Format != wuffs.FormatPNG {
		t.Errorf("Meta.Format = 0x%08X, want 0x%08X", meta.Format, wuffs.FormatPNG)
	}
	if meta.BytesWritten != 0 {
		t.Errorf("Meta.BytesWritten = %d, want 0", meta.BytesWritten)
	}
}

// TestIntegrationPackageProbeTruncatedConfigReadable proves that
// config-readable truncated PNG succeeds exactly when (*Decoder).Probe
// succeeds, and returns matching metadata (req 3).
func TestIntegrationPackageProbeTruncatedConfigReadable(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")
	truncated := pngSrc[:50] // Wuffs needs ~50 bytes for config

	// Decoder-level Probe must succeed on this truncated input.
	d := wuffs.New()
	if err := d.Reserve(0, len(truncated)); err != nil {
		t.Fatalf("decoder Reserve: %v", err)
	}
	decMeta, decErr := d.Probe(truncated)
	if decErr != nil {
		t.Fatalf("decoder Probe(truncated) must succeed for config-readable input, got: %v", decErr)
	}
	if decMeta == nil {
		t.Fatal("decoder Probe(truncated) returned nil Meta")
	}

	// Package-level Probe must also succeed and match metadata.
	pkgMeta, pkgErr := wuffs.Probe(truncated)
	if pkgErr != nil {
		t.Fatalf("package Probe(truncated) must succeed when decoder Probe succeeds, got: %v", pkgErr)
	}
	if pkgMeta == nil {
		t.Fatal("package Probe(truncated) returned nil Meta")
	}

	if pkgMeta.Width != decMeta.Width || pkgMeta.Height != decMeta.Height {
		t.Errorf("package Meta dimensions = (%d,%d), decoder Meta = (%d,%d)", pkgMeta.Width, pkgMeta.Height, decMeta.Width, decMeta.Height)
	}
	if pkgMeta.BytesWritten != 0 {
		t.Errorf("package Meta.BytesWritten = %d, want 0", pkgMeta.BytesWritten)
	}
}

// TestIntegrationPackageProbeErrorInputs proves that empty, unknown-format,
// and non-config-readable truncated input return the exact error emitted by
// (*Decoder).Probe and nil Meta (req 4).
func TestIntegrationPackageProbeErrorInputs(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	tests := []struct {
		name string
		src  []byte
	}{
		{"empty (nil)", nil},
		{"empty (zero-length)", []byte{}},
		{"unknown format", []byte("not a known image format header")},
		{"non-config-readable truncated", pngSrc[:1]},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Decoder-level Probe: pre-reserve then probe.
			d := wuffs.New()
			if err := d.Reserve(0, len(tc.src)); err != nil {
				t.Fatalf("decoder Reserve: %v", err)
			}
			_, decErr := d.Probe(tc.src)
			if decErr == nil {
				t.Fatal("decoder Probe must fail for this input")
			}

			// Package-level Probe must return the exact same error.
			pkgMeta, pkgErr := wuffs.Probe(tc.src)
			//nolint:errorlint // exact identity required by contract; both calls return the same sentinel
			if pkgErr != decErr {
				t.Fatalf("package Probe err = %v, want exact decoder Probe err = %v", pkgErr, decErr)
			}
			if pkgMeta != nil {
				t.Errorf("package Probe returned non-nil Meta %v, want nil", pkgMeta)
			}
		})
	}
}

// TestIntegrationPackageProbeIndependentMetas proves that repeated calls
// to the package-level Probe return independent Meta pointers; mutating one
// does not affect another (req 8).
func TestIntegrationPackageProbeIndependentMetas(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	meta1, err1 := wuffs.Probe(pngSrc)
	if err1 != nil {
		t.Fatalf("first Probe: %v", err1)
	}
	if meta1 == nil {
		t.Fatal("first Probe returned nil Meta")
	}

	meta2, err2 := wuffs.Probe(pngSrc)
	if err2 != nil {
		t.Fatalf("second Probe: %v", err2)
	}
	if meta2 == nil {
		t.Fatal("second Probe returned nil Meta")
	}

	// Must be distinct pointers.
	if meta1 == meta2 {
		t.Error("meta1 and meta2 are the same pointer, want independent pointers")
	}

	// Must be value-equal.
	if *meta1 != *meta2 {
		t.Errorf("Meta values differ: %+v vs %+v", *meta1, *meta2)
	}

	// Mutate meta1; meta2 must remain unchanged.
	captured2 := *meta2
	meta1.Width = 0xFFFFFFFF
	meta1.Height = 0xFFFFFFFF
	if *meta2 != captured2 {
		t.Errorf("meta2 was mutated after meta1 change: got %+v, want %+v", *meta2, captured2)
	}
}
