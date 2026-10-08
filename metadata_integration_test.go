package wuffs_test

// Task 2 metadata integration: absent-metadata PNG and pack flag presence.

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// TestIntegrationMetadataAbsentBricksColor proves Metadata succeeds on a PNG
// with no ancillary metadata: Format is PNG, all blobs are nil, all presence
// flags are false, and guest memory layout is preserved.
func TestIntegrationMetadataAbsentBricksColor(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "bricks-color.png"))
	if err != nil {
		t.Fatalf("reading testdata/bricks-color.png: %v", err)
	}
	d := wuffs.New()
	if reserveErr := d.Reserve(0, len(src)); reserveErr != nil {
		t.Fatalf("Reserve(0, len(src)) error = %v, want nil", reserveErr)
	}
	before := wuffs.CaptureGuestMemoryState(d)
	md, err := d.Metadata(src)
	if err != nil {
		t.Fatalf("Metadata(bricks-color.png) error = %v, want nil", err)
	}
	if md.Format != wuffs.FormatPNG {
		t.Errorf("Metadata(bricks-color.png).Format = %#x, want %#x", md.Format, wuffs.FormatPNG)
	}
	if md.EXIF != nil {
		t.Errorf("Metadata(bricks-color.png).EXIF len = %d, want nil", len(md.EXIF))
	}
	if md.ICC != nil {
		t.Errorf("Metadata(bricks-color.png).ICC len = %d, want nil", len(md.ICC))
	}
	if md.XMP != nil {
		t.Errorf("Metadata(bricks-color.png).XMP len = %d, want nil", len(md.XMP))
	}
	if md.HasGamma {
		t.Errorf("Metadata(bricks-color.png).HasGamma = true, want false")
	}
	if md.HasChromaticities {
		t.Errorf("Metadata(bricks-color.png).HasChromaticities = true, want false")
	}
	if md.HasSRGB {
		t.Errorf("Metadata(bricks-color.png).HasSRGB = true, want false")
	}
	if md.HasModTime {
		t.Errorf("Metadata(bricks-color.png).HasModTime = true, want false")
	}
	assertGuestMemoryPreserved(t, "Metadata", "bricks-color.png", d, before)
}

// TestIntegrationMetadataPackFlagsBricksDither proves the guest pack header
// flags are populated on bricks-dither.png (cHRM, gAMA, sRGB parsed chunks).
// Value parsing and oracle tolerances remain Task 3.
func TestIntegrationMetadataPackFlagsBricksDither(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "bricks-dither.png"))
	if err != nil {
		t.Fatalf("reading testdata/bricks-dither.png: %v", err)
	}
	d := wuffs.New()
	if reserveErr := d.Reserve(0, len(src)); reserveErr != nil {
		t.Fatalf("Reserve(0, len(src)) error = %v, want nil", reserveErr)
	}
	before := wuffs.CaptureGuestMemoryState(d)
	md, err := d.Metadata(src)
	if err != nil {
		t.Fatalf("Metadata(bricks-dither.png) error = %v, want nil", err)
	}
	if !md.HasChromaticities {
		t.Errorf("Metadata(bricks-dither.png).HasChromaticities = false, want true")
	}
	if !md.HasGamma {
		t.Errorf("Metadata(bricks-dither.png).HasGamma = false, want true")
	}
	if !md.HasSRGB {
		t.Errorf("Metadata(bricks-dither.png).HasSRGB = false, want true")
	}
	assertGuestMemoryPreserved(t, "Metadata", "bricks-dither.png", d, before)
}

// approxEqual reports whether got is within tol of want.
func approxEqual(got, want, tol float64) bool {
	return math.Abs(got-want) <= tol
}

// TestIntegrationMetadataPNGParsedChunks proves host parsing of the guest
// pack matches ../wuffs/test/c/std/png.c
// test_wuffs_png_decode_metadata_chrm_gama_srgb for bricks-dither.png.
func TestIntegrationMetadataPNGParsedChunks(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "bricks-dither.png"))
	if err != nil {
		t.Fatalf("reading testdata/bricks-dither.png: %v", err)
	}
	d := wuffs.New()
	if reserveErr := d.Reserve(0, len(src)); reserveErr != nil {
		t.Fatalf("Reserve(0, len(src)) error = %v, want nil", reserveErr)
	}
	md, err := d.Metadata(src)
	if err != nil {
		t.Fatalf("Metadata(bricks-dither.png) error = %v, want nil", err)
	}
	if !md.HasChromaticities {
		t.Errorf("Metadata(bricks-dither.png).HasChromaticities = false, want true")
	}
	wants := []struct {
		name string
		got  float64
		want float64
	}{
		{"WhiteX", md.Chromaticities.WhiteX, 0.31270},
		{"WhiteY", md.Chromaticities.WhiteY, 0.32900},
		{"RedX", md.Chromaticities.RedX, 0.64},
		{"RedY", md.Chromaticities.RedY, 0.33},
		{"GreenX", md.Chromaticities.GreenX, 0.30},
		{"GreenY", md.Chromaticities.GreenY, 0.60},
		{"BlueX", md.Chromaticities.BlueX, 0.15},
		{"BlueY", md.Chromaticities.BlueY, 0.06},
	}
	for _, w := range wants {
		if !approxEqual(w.got, w.want, 1e-5) {
			t.Errorf("Metadata(bricks-dither.png).Chromaticities.%s = %v, want %v (tol 1e-5)", w.name, w.got, w.want)
		}
	}
	if !md.HasGamma {
		t.Errorf("Metadata(bricks-dither.png).HasGamma = false, want true")
	}
	if want := 100000.0 / 45455.0; !approxEqual(md.Gamma, want, 1e-9) {
		t.Errorf("Metadata(bricks-dither.png).Gamma = %v, want %v (tol 1e-9)", md.Gamma, want)
	}
	if !md.HasSRGB {
		t.Errorf("Metadata(bricks-dither.png).HasSRGB = false, want true")
	}
	if md.SRGB != 0 {
		t.Errorf("Metadata(bricks-dither.png).SRGB = %d, want 0", md.SRGB)
	}
	if md.HasModTime {
		t.Errorf("Metadata(bricks-dither.png).HasModTime = true, want false")
	}
}

// TestIntegrationMetadataPNGEXIF proves the guest forwards the raw EXIF blob
// on testdata/artificial-png/exif.png. Oracle: ../wuffs/test/c/std/png.c
// test_wuffs_png_decode_metadata_exif (raw range holds "LoremIpsum").
func TestIntegrationMetadataPNGEXIF(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "artificial-png", "exif.png"))
	if err != nil {
		t.Fatalf("reading testdata/artificial-png/exif.png: %v", err)
	}
	d := wuffs.New()
	if reserveErr := d.Reserve(0, len(src)); reserveErr != nil {
		t.Fatalf("Reserve(0, len(src)) error = %v, want nil", reserveErr)
	}
	md, err := d.Metadata(src)
	if err != nil {
		t.Fatalf("Metadata(exif.png) error = %v, want nil", err)
	}
	if len(md.EXIF) == 0 {
		t.Fatalf("Metadata(exif.png).EXIF len = 0, want > 0")
	}
	if !strings.Contains(string(md.EXIF), "LoremIpsum") {
		t.Errorf("Metadata(exif.png).EXIF missing LoremIpsum (len %d)", len(md.EXIF))
	}
}

// TestIntegrationMetadataPNGICCP proves the guest decompresses the PNG iCCP
// chunk via METADATA_RAW_TRANSFORM. Oracle: ../wuffs/test/c/std/png.c
// test_wuffs_png_decode_metadata_iccp (destination io_buffer `have` compared
// to test/data/DCI-P3-D65.icc, 604 bytes).
func TestIntegrationMetadataPNGICCP(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "red-blue-gradient.dcip3d65-no-chrm-no-gama.png"))
	if err != nil {
		t.Fatalf("reading red-blue-gradient fixture: %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "DCI-P3-D65.icc"))
	if err != nil {
		t.Fatalf("reading testdata/DCI-P3-D65.icc: %v", err)
	}
	if len(want) != 604 {
		t.Fatalf("DCI-P3-D65.icc len = %d, want 604", len(want))
	}
	d := wuffs.New()
	if reserveErr := d.Reserve(0, len(src)); reserveErr != nil {
		t.Fatalf("Reserve(0, len(src)) error = %v, want nil", reserveErr)
	}
	md, err := d.Metadata(src)
	if err != nil {
		t.Fatalf("Metadata(red-blue-gradient) error = %v, want nil", err)
	}
	if !bytes.Equal(md.ICC, want) {
		t.Errorf("Metadata(red-blue-gradient).ICC len = %d, want 604 matching DCI-P3-D65.icc", len(md.ICC))
	}
}

// TestIntegrationMetadataGIFChunks proves the guest forwards GIF ICCP and
// XMP application-extension blobs. Oracle: ../wuffs/test/c/std/gif.c
// do_test_wuffs_gif_decode_metadata. Payloads are pinned by the committed
// binary golden testdata/metadata-full.gif.metadata.golden (uint32 LE ICC
// len, uint32 LE XMP len, ICC bytes, XMP bytes).
func TestIntegrationMetadataGIFChunks(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "artificial-gif", "metadata-full.gif"))
	if err != nil {
		t.Fatalf("reading metadata-full.gif: %v", err)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "metadata-full.gif.metadata.golden"))
	if err != nil {
		t.Fatalf("reading metadata-full.gif.metadata.golden: %v", err)
	}
	if len(golden) < 8 {
		t.Fatalf("golden len = %d, want >= 8", len(golden))
	}
	iccLen := binary.LittleEndian.Uint32(golden[0:4])
	xmpLen := binary.LittleEndian.Uint32(golden[4:8])
	if uint64(8+iccLen+xmpLen) != uint64(len(golden)) {
		t.Fatalf("golden len = %d, want 8+%d+%d", len(golden), iccLen, xmpLen)
	}
	goldenICC := golden[8 : 8+iccLen]
	goldenXMP := golden[8+iccLen : 8+iccLen+xmpLen]
	d := wuffs.New()
	if reserveErr := d.Reserve(0, len(src)); reserveErr != nil {
		t.Fatalf("Reserve(0, len(src)) error = %v, want nil", reserveErr)
	}
	md, err := d.Metadata(src)
	if err != nil {
		t.Fatalf("Metadata(metadata-full.gif) error = %v, want nil", err)
	}
	if !bytes.Equal(md.ICC, goldenICC) {
		t.Errorf("Metadata(metadata-full.gif).ICC = % X (len %d), want golden (len %d)", md.ICC, len(md.ICC), len(goldenICC))
	}
	if !bytes.Equal(md.XMP, goldenXMP) {
		t.Errorf("Metadata(metadata-full.gif).XMP = % X (len %d), want golden (len %d)", md.XMP, len(md.XMP), len(goldenXMP))
	}

	src2, err := os.ReadFile(filepath.Join("testdata", "artificial-gif", "metadata-empty.gif"))
	if err != nil {
		t.Fatalf("reading metadata-empty.gif: %v", err)
	}
	d2 := wuffs.New()
	if reserveErr := d2.Reserve(0, len(src2)); reserveErr != nil {
		t.Fatalf("Reserve(0, len(src2)) error = %v, want nil", reserveErr)
	}
	md2, err := d2.Metadata(src2)
	if err != nil {
		t.Fatalf("Metadata(metadata-empty.gif) error = %v, want nil", err)
	}
	if md2.ICC != nil {
		t.Errorf("Metadata(metadata-empty.gif).ICC len = %d, want nil", len(md2.ICC))
	}
	if md2.XMP != nil {
		t.Errorf("Metadata(metadata-empty.gif).XMP len = %d, want nil", len(md2.XMP))
	}
}
