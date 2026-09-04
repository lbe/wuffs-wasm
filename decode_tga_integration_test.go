package wuffs_test

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// tgaFourCC is asserted as a local uint32 literal rather than the exported
// constant so this runtime characterization stays independent of the exported
// name, matching the FORMAT-02 plan convention.
const tgaFourCC = uint32(0x54474120)

// TestIntegrationTGADecodeCharacterization verifies the TGA format contract
// end-to-end against the live wasm guest:
//
//   - The vendored upstream fixtures (uncompressed true-color bricks-color.tga,
//     indexed-color bricks-nodither.tga, and grayscale/RLE bricks-gray.tga,
//     all 160×120) probe and decode as FormatTGA with the expected geometry
//     and stride, BytesWritten zero from Probe, and pixels identical to the
//     independent PNG oracles.
//   - Deterministic in-test fixtures cross the complete accepted matrix of
//     image types 1 and 9 at color-map depths 15, 24, and 32; types 2 and 10
//     at pixel depths 15, 16, 24, and 32; and types 3 and 11 at pixel
//     depth 8. Every class is covered with both permitted descriptor
//     vertical origins (0x00 and 0x20) and decodes to exact fixed pixels.
//   - 256×1 and 1×256 grayscale fixtures prove both bytes of each 16-bit
//     dimension are parsed.
//   - Complete, otherwise-valid inputs outside the conservative subset —
//     indexed types 1 and 9 with 16-bit color-map entries (clear and set
//     alpha bits), 15-bit true-color types 2 and 10 declaring one attribute
//     bit (pixels with clear and set alpha bits), and a 16-bit BGRA5551
//     declaring one attribute bit — stay ErrUnknownFormat with a sentinel
//     destination left byte-for-byte untouched.
//   - A structurally recognizable bricks-color.tga with corrupt (truncated)
//     pixel data probes cleanly but DecodeRGBA fails matching ErrDecode, not
//     ErrUnknownFormat, without mutating the destination; Decode preserves the
//     caller's Pix pointer, length, capacity, Rect, and Stride and never grows
//     guest memory after Reserve.
//   - Truncated headers and the generated collision corpus (17-byte prefixes,
//     zero-padded and sequential seeds, ELF bytes, zero dimension, image-type
//     byte, color-map fields, depths, and descriptor interleave/origin/
//     attribute rejections over every otherwise-valid baseline class) remain
//     ErrUnknownFormat.
//   - The exact higher-priority signature-bearing fixtures (PNG, WebP, BMP,
//     GIF, JPEG, QOI, PGM, PPM) still probe with their own FourCC, and the
//     ETC2, HNSM, NIE, and TH signature-marker sentinels retain ErrDecode.
//
// It fails against the current guest because sniff_fourcc does not recognize
// these valid TGA headers.
func TestIntegrationTGADecodeCharacterization(t *testing.T) {
	t.Run("vendored upstream fixtures match their PNG oracles", func(t *testing.T) {
		fixtures := []struct {
			name    string
			tgaFile string
			pngFile string
		}{
			{"uncompressed true-color", "bricks-color.tga", "bricks-color.png"},
			{"indexed-color", "bricks-nodither.tga", "bricks-nodither.png"},
			{"grayscale RLE", "bricks-gray.tga", "bricks-gray.png"},
		}
		for _, fc := range fixtures {
			t.Run(fc.name, func(t *testing.T) {
				const wantW, wantH = 160, 120
				src := loadFixture(t, fc.tgaFile)

				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, wantW*wantH*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve: %v", err)
				}
				assertTGAMeta(t, d, fc.tgaFile, src, wantW, wantH, wantW*4)

				dst := image.NewRGBA(image.Rect(0, 0, wantW, wantH))

				// Independent oracle: the same canvas decoded from the
				// pixel-identical PNG fixture.
				oracle := image.NewRGBA(image.Rect(0, 0, wantW, wantH))
				pngSrc := loadFixture(t, fc.pngFile)
				if _, err := wuffs.New().DecodeRGBA(oracle, pngSrc); err != nil {
					t.Fatalf("PNG oracle DecodeRGBA: %v", err)
				}

				decMeta := assertTGADecodeRGBA(t, d, fc.tgaFile, dst, src, oracle.Pix)
				if decMeta.Width != wantW || decMeta.Height != wantH || decMeta.Stride != wantW*4 {
					t.Errorf("DecodeRGBA geometry = (%d,%d) stride %d, want (%d,%d) stride %d",
						decMeta.Width, decMeta.Height, decMeta.Stride, wantW, wantH, wantW*4)
				}
			})
		}
	})

	t.Run("full accepted type/depth/origin matrix decodes to fixed pixels", func(t *testing.T) {
		for _, tc := range tgaPositiveMatrix() {
			t.Run(tc.name, func(t *testing.T) {
				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, int(tc.w)*int(tc.h)*4, len(tc.src)); err != nil {
					t.Fatalf("RequiredReserve: %v", err)
				}
				assertTGAMeta(t, d, tc.name, tc.src, int(tc.w), int(tc.h), int(tc.w)*4)

				dst := image.NewRGBA(image.Rect(0, 0, int(tc.w), int(tc.h)))
				assertTGADecodeRGBA(t, d, tc.name, dst, tc.src, tc.wantPix)
			})
		}
	})

	t.Run("256x1 and 1x256 prove both dimension bytes are parsed", func(t *testing.T) {
		cases := []struct {
			name string
			src  []byte
			w, h uint16
			want []byte
		}{
			{"256x1 grayscale", tgaGeometry(256, 1, 0x00), 256, 1, grayGradientPixels(256, 1)},
			{"1x256 grayscale", tgaGeometry(1, 256, 0x20), 1, 256, grayGradientPixels(1, 256)},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, int(tc.w)*int(tc.h)*4, len(tc.src)); err != nil {
					t.Fatalf("RequiredReserve: %v", err)
				}
				assertTGAMeta(t, d, tc.name, tc.src, int(tc.w), int(tc.h), int(tc.w)*4)

				dst := image.NewRGBA(image.Rect(0, 0, int(tc.w), int(tc.h)))
				assertTGADecodeRGBA(t, d, tc.name, dst, tc.src, tc.want)
			})
		}
	})

	t.Run("unsupported valid-looking inputs fail without destination mutation", func(t *testing.T) {
		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, 2*1*4, 64); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}
		for _, tc := range tgaUnsupportedFixtures() {
			t.Run(tc.name, func(t *testing.T) {
				meta, err := d.Probe(tc.src)
				if !errors.Is(err, wuffs.ErrUnknownFormat) {
					t.Errorf("Probe(%s) error = %v, want ErrUnknownFormat", tc.name, err)
				}
				if meta != nil {
					t.Errorf("Probe(%s) returned non-nil Meta %+v, want nil", tc.name, meta)
				}

				dst := image.NewRGBA(image.Rect(0, 0, 2, 1))
				for i := range dst.Pix {
					dst.Pix[i] = 0x5A
				}
				sentinel := append([]byte(nil), dst.Pix...)

				decMeta, err := d.DecodeRGBA(dst, tc.src)
				if !errors.Is(err, wuffs.ErrUnknownFormat) {
					t.Errorf("DecodeRGBA(%s) error = %v, want ErrUnknownFormat", tc.name, err)
				}
				if decMeta != nil {
					t.Errorf("DecodeRGBA(%s) returned non-nil Meta %+v, want nil", tc.name, decMeta)
				}
				if !bytes.Equal(dst.Pix, sentinel) {
					t.Errorf("DecodeRGBA(%s) mutated the sentinel destination: got % X, want % X", tc.name, dst.Pix, sentinel)
				}
			})
		}
	})

	t.Run("corrupt TGA pixel data fails with ErrDecode", func(t *testing.T) {
		const wantW, wantH = 160, 120
		src := loadFixture(t, "bricks-color.tga")
		// Keep the complete 18-byte header (ID length 0) plus 16 true-color
		// 24-bit pixels; the pixel stream is truncated mid-frame.
		corrupt := src[:18+48]

		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, wantW*wantH*4, len(src)); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}
		assertTGAMeta(t, d, "corrupt bricks-color.tga", corrupt, wantW, wantH, wantW*4)

		dst := image.NewRGBA(image.Rect(0, 0, wantW, wantH))
		for i := range dst.Pix {
			dst.Pix[i] = 0xA5
		}
		prefill := append([]byte(nil), dst.Pix...)

		decMeta, err := d.DecodeRGBA(dst, corrupt)
		if err == nil {
			t.Fatal("DecodeRGBA(corrupt TGA) error = nil, want ErrDecode")
		}
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Errorf("DecodeRGBA(corrupt TGA) error = %v, want errors.Is(err, ErrDecode)", err)
		}
		if errors.Is(err, wuffs.ErrUnknownFormat) {
			t.Errorf("DecodeRGBA(corrupt TGA) error = %v, must not be ErrUnknownFormat", err)
		}
		if decMeta != nil {
			t.Errorf("DecodeRGBA(corrupt TGA) returned non-nil Meta %+v, want nil", decMeta)
		}
		if !bytes.Equal(dst.Pix, prefill) {
			t.Error("DecodeRGBA(corrupt TGA) mutated the sentinel destination pixels")
		}
	})

	t.Run("truncated indexed palette is recognized then rejected by Wuffs", func(t *testing.T) {
		// Complete, otherwise-valid indexed TGA header for a 1x1 image with
		// one 24-bit palette entry (color-map type 1, first-entry index 0,
		// length 1, entry depth 24, index depth 8), with the palette and
		// pixel payload absent. sniff_tga must recognize the header from its
		// planned structural fields alone; Wuffs must then reject the
		// truncated payload with ErrDecode, not ErrUnknownFormat.
		src := []byte{
			0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x18, 0x00,
			0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x08, 0x00,
		}

		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, 1*1*4, len(src)); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}

		guestBeforeProbe := wuffs.CaptureGuestMemoryState(d)
		meta, err := d.Probe(src)
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Errorf("Probe(truncated indexed palette) error = %v, want errors.Is(err, ErrDecode)", err)
		}
		if errors.Is(err, wuffs.ErrUnknownFormat) {
			t.Errorf("Probe(truncated indexed palette) error = %v, must not be ErrUnknownFormat", err)
		}
		if meta != nil {
			t.Errorf("Probe(truncated indexed palette) returned non-nil Meta %+v, want nil", meta)
		}
		assertGuestMemoryPreserved(t, "Probe", "truncated indexed palette", d, guestBeforeProbe)

		dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
		for i := range dst.Pix {
			dst.Pix[i] = 0x5A
		}
		sentinel := append([]byte(nil), dst.Pix...)
		dstState := captureRGBAState(dst)
		guestBeforeDecode := wuffs.CaptureGuestMemoryState(d)

		decMeta, err := d.DecodeRGBA(dst, src)
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Errorf("DecodeRGBA(truncated indexed palette) error = %v, want errors.Is(err, ErrDecode)", err)
		}
		if errors.Is(err, wuffs.ErrUnknownFormat) {
			t.Errorf("DecodeRGBA(truncated indexed palette) error = %v, must not be ErrUnknownFormat", err)
		}
		if decMeta != nil {
			t.Errorf("DecodeRGBA(truncated indexed palette) returned non-nil Meta %+v, want nil", decMeta)
		}
		assertGuestMemoryPreserved(t, "DecodeRGBA", "truncated indexed palette", d, guestBeforeDecode)
		if !bytes.Equal(dst.Pix, sentinel) {
			t.Errorf("DecodeRGBA(truncated indexed palette) mutated the sentinel destination: got % X, want % X", dst.Pix, sentinel)
		}
		assertDestinationPreserved(t, "DecodeRGBA", "truncated indexed palette", dst, dstState)
	})

	t.Run("truncated headers remain ErrUnknownFormat", func(t *testing.T) {
		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, 160*120*4, 65536); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}
		baselines := [][]byte{
			tgaHeader(1, 0, 2, 0, 0, 0, 1, 1, 24, 0x00), // true-color
			tgaHeader(0, 1, 1, 0, 1, 24, 1, 1, 8, 0x00), // indexed
			tgaHeader(1, 0, 3, 0, 0, 0, 1, 1, 8, 0x00),  // grayscale
		}
		for bi, base := range baselines {
			for n := 1; n < len(base); n++ {
				expectUnknownProbe(t, d, fmt.Sprintf("truncated header[%d] len %d", bi, n), base[:n])
			}
		}
	})

	t.Run("collision corpus stays unknown", func(t *testing.T) {
		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, 160*120*4, 65536); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}

		// Fixed seeds.
		expectUnknownProbe(t, d, "18 zero bytes", make([]byte, 18))
		seq := make([]byte, 18)
		for i := range seq {
			seq[i] = byte(i)
		}
		expectUnknownProbe(t, d, "bytes 0x00 through 0x11", seq)
		elf := append([]byte{0x7F, 'E', 'L', 'F'}, make([]byte, 14)...)
		expectUnknownProbe(t, d, "ELF bytes padded to 18", elf)

		// True-color baseline: a structurally valid 1x1x24 header with a
		// one-byte image ID {01,00,02,...,18,00} followed by its ID byte.
		tcBase := tgaHeader(1, 0, 2, 0, 0, 0, 1, 1, 24, 0x00)
		tcID := append(append([]byte(nil), tcBase...), 0x00)

		// Width or height low byte set to zero.
		for _, off := range []int{12, 14} {
			mut := append([]byte(nil), tcID...)
			mut[off] = 0x00
			expectUnknownProbe(t, d, fmt.Sprintf("true-color zero %s", map[int]string{12: "width", 14: "height"}[off]), mut)
		}

		// Image-type byte at offset 2: every value outside {1,2,3,9,10,11}.
		for tv := 0; tv < 256; tv++ {
			bt := byte(tv)
			if bt == 1 || bt == 2 || bt == 3 || bt == 9 || bt == 10 || bt == 11 {
				continue
			}
			mut := append([]byte(nil), tcID...)
			mut[2] = bt
			expectUnknownProbe(t, d, fmt.Sprintf("type byte 0x%02X", bt), mut)
		}

		// Offset 1 and color-map-field bytes 3-7: every nonzero value.
		for _, off := range []int{1, 3, 4, 5, 6, 7} {
			for v := 1; v < 256; v++ {
				mut := append([]byte(nil), tcID...)
				mut[off] = byte(v)
				expectUnknownProbe(t, d, fmt.Sprintf("true-color offset %d value 0x%02X", off, v), mut)
			}
		}

		// True-color offset 16: every value outside {15,16,24,32}.
		for pv := 0; pv < 256; pv++ {
			bp := byte(pv)
			if bp == 15 || bp == 16 || bp == 24 || bp == 32 {
				continue
			}
			mut := append([]byte(nil), tcID...)
			mut[16] = bp
			expectUnknownProbe(t, d, fmt.Sprintf("true-color pixel depth 0x%02X", bp), mut)
		}

		// Indexed baselines (types 1 and 9): color-map type 1, length 1,
		// first-entry index 0, color-map depth 24, index depth 8.
		for _, imgType := range []byte{1, 9} {
			idxBase := tgaHeader(0, 1, imgType, 0, 1, 24, 1, 1, 8, 0x00)

			// Color-map length: 0 and every value 257..65535, covering
			// both bytes at offsets 5-6. Lengths 2..256 are inside the
			// accepted 1..256 rule and are recognized as TGA; Wuffs then
			// rejects the absent palette payload (see the truncated-indexed
			// subtest above), so they are not part of this non-TGA corpus.
			for l := 0; l <= 65535; l++ {
				if l >= 1 && l <= 256 {
					continue
				}
				mut := append([]byte(nil), idxBase...)
				mut[5] = byte(l)
				mut[6] = byte(l >> 8)
				expectUnknownProbe(t, d, fmt.Sprintf("indexed type %d color-map length %d", imgType, l), mut)
			}

			// Offset 1 (color-map type): every byte value except 1.
			for v := 0; v < 256; v++ {
				if byte(v) == 1 {
					continue
				}
				mut := append([]byte(nil), idxBase...)
				mut[1] = byte(v)
				expectUnknownProbe(t, d, fmt.Sprintf("indexed type %d color-map type 0x%02X", imgType, v), mut)
			}

			// Offsets 3 and 4 (first-entry index bytes): every nonzero value.
			for _, off := range []int{3, 4} {
				for v := 1; v < 256; v++ {
					mut := append([]byte(nil), idxBase...)
					mut[off] = byte(v)
					expectUnknownProbe(t, d, fmt.Sprintf("indexed type %d first-entry index offset %d value 0x%02X", imgType, off, v), mut)
				}
			}

			// Offset 7 (color-map depth): every value outside {15,24,32}.
			for dv := 0; dv < 256; dv++ {
				bd := byte(dv)
				if bd == 15 || bd == 24 || bd == 32 {
					continue
				}
				mut := append([]byte(nil), idxBase...)
				mut[7] = bd
				expectUnknownProbe(t, d, fmt.Sprintf("indexed type %d color-map depth 0x%02X", imgType, bd), mut)
			}

			// Offset 16 (index depth): every value except 8.
			for iv := 0; iv < 256; iv++ {
				if byte(iv) == 8 {
					continue
				}
				mut := append([]byte(nil), idxBase...)
				mut[16] = byte(iv)
				expectUnknownProbe(t, d, fmt.Sprintf("indexed type %d index depth 0x%02X", imgType, iv), mut)
			}
		}

		// Grayscale baseline: a structurally valid 1x1x8 header with a
		// one-byte image ID {01,00,03,...,08,00}, followed by its ID byte.
		grayBase := tgaHeader(1, 0, 3, 0, 0, 0, 1, 1, 8, 0x00)
		grayID := append(append([]byte(nil), grayBase...), 0x00)

		// Offset 1 and color-map-field bytes 3-7: every nonzero value.
		for _, off := range []int{1, 3, 4, 5, 6, 7} {
			for v := 1; v < 256; v++ {
				mut := append([]byte(nil), grayID...)
				mut[off] = byte(v)
				expectUnknownProbe(t, d, fmt.Sprintf("grayscale offset %d value 0x%02X", off, v), mut)
			}
		}
		// Grayscale offset 16: every value except 8.
		for gv := 0; gv < 256; gv++ {
			if byte(gv) == 8 {
				continue
			}
			mut := append([]byte(nil), grayID...)
			mut[16] = byte(gv)
			expectUnknownProbe(t, d, fmt.Sprintf("grayscale depth 0x%02X", gv), mut)
		}

		// Descriptor rejections over every otherwise-valid baseline class.
		for _, cls := range tgaDescriptorClasses() {
			for _, probe := range cls.rejections {
				expectUnknownProbe(t, d, fmt.Sprintf("%s descriptor 0x%02X", cls.name, probe), cls.src(probe))
			}
		}
	})

	t.Run("higher-priority signature formats retain their FourCC", func(t *testing.T) {
		priority := []struct {
			file       string
			wantFourCC uint32
		}{
			{"bricks-color.png", 0x504E4720},           // PNG
			{"bricks-color.lossless.webp", 0x57454250}, // WEBP
			{"bricks-color.bmp", 0x424D5020},           // BMP
			{"bricks-nodither.gif", 0x47494620},        // GIF
			{"hat.jpeg", 0x4A504547},                   // JPEG
			{"bricks-color.qoi", 0x514F4920},           // QOI
			{"hippopotamus.pgm", 0x4E50424D},           // NPBM (P5)
			{"hippopotamus.ppm", 0x4E50424D},           // NPBM (P6)
		}
		for _, fc := range priority {
			t.Run(fc.file, func(t *testing.T) {
				src := loadFixture(t, fc.file)
				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, 160*120*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve: %v", err)
				}
				meta, err := d.Probe(src)
				if err != nil {
					t.Fatalf("Probe(%s): %v", fc.file, err)
				}
				if meta == nil {
					t.Fatal("Probe returned nil Meta")
				}
				if meta.Format != fc.wantFourCC {
					t.Errorf("Probe(%s) Format = 0x%08X, want 0x%08X", fc.file, meta.Format, fc.wantFourCC)
				}
			})
		}
	})

	t.Run("ETC2 HNSM NIE and TH sentinels retain ErrDecode", func(t *testing.T) {
		sentinels := []struct {
			name string
			src  []byte
		}{
			{"ETC2 signature", []byte{0x13, 0xAB, 0xA1, 0x5C}},
			{"HNSM signature", []byte{'H', 'N', 'S', 'M'}},
			{"NIE little-endian FourCC", []byte{0x41, 0x65, 0x69, 0x6E}},
			{"TH signature", []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF}},
		}
		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, 160*120*4, 16); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}
		for _, sc := range sentinels {
			meta, err := d.Probe(sc.src)
			if !errors.Is(err, wuffs.ErrDecode) {
				t.Errorf("Probe(%s) error = %v, want ErrDecode (not FormatTGA, not ErrUnknownFormat)", sc.name, err)
			}
			if meta != nil {
				t.Errorf("Probe(%s) returned non-nil Meta %+v, want nil", sc.name, meta)
			}
		}
	})
}

// TestIntegrationTGADecodeRGBAAllocsPerRun measures heap allocations per
// DecodeRGBA call for reusable TGA decoders. Each input and its correctly
// sized caller-owned destination are prepared outside the measured closure,
// one warm-up decode runs first, and testing.AllocsPerRun then requires
// exactly zero allocations for every fixture in the complete accepted
// type/depth/origin/attribute matrix and both high-byte geometry fixtures.
func TestIntegrationTGADecodeRGBAAllocsPerRun(t *testing.T) {
	cases := append([]tgaCase{}, tgaPositiveMatrix()...)
	cases = append(cases,
		tgaCase{name: "256x1 grayscale", src: tgaGeometry(256, 1, 0x00), w: 256, h: 1},
		tgaCase{name: "1x256 grayscale", src: tgaGeometry(1, 256, 0x20), w: 1, h: 256},
	)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := wuffs.New()
			if err := wuffs.RequiredReserve(d, int(tc.w)*int(tc.h)*4, len(tc.src)); err != nil {
				t.Fatalf("RequiredReserve: %v", err)
			}

			dst := image.NewRGBA(image.Rect(0, 0, int(tc.w), int(tc.h)))
			if _, err := d.DecodeRGBA(dst, tc.src); err != nil {
				t.Fatalf("warm-up DecodeRGBA: %v", err)
			}

			allocs := testing.AllocsPerRun(100, func() {
				if _, err := d.DecodeRGBA(dst, tc.src); err != nil {
					t.Errorf("DecodeRGBA: %v", err)
				}
			})
			if allocs != 0 {
				t.Errorf("DecodeRGBA allocated %.0f heap objects per run, want 0", allocs)
			}
		})
	}
}

// assertTGAMeta probes src and asserts the complete FormatTGA metadata
// contract: no error, non-nil Meta, exact FourCC, dimensions, stride,
// BytesWritten zero from Probe, and complete guest-memory preservation across
// the reserved Probe call.
func assertTGAMeta(t *testing.T, d *wuffs.Decoder, name string, src []byte, wantW, wantH, wantStride int) {
	t.Helper()
	guestBefore := wuffs.CaptureGuestMemoryState(d)
	meta, err := d.Probe(src)
	if err != nil {
		t.Fatalf("Probe(%s): %v", name, err)
	}
	assertGuestMemoryPreserved(t, "Probe", name, d, guestBefore)
	if meta == nil {
		t.Fatalf("Probe(%s) returned nil Meta", name)
	}
	if meta.Format != tgaFourCC {
		t.Errorf("Probe(%s) Format = 0x%08X, want FormatTGA 0x%08X", name, meta.Format, tgaFourCC)
	}
	if meta.Width != uint32(wantW) || meta.Height != uint32(wantH) {
		t.Errorf("Probe(%s) geometry = (%d,%d), want (%d,%d)", name, meta.Width, meta.Height, wantW, wantH)
	}
	if meta.Stride != uint32(wantStride) {
		t.Errorf("Probe(%s) Stride = %d, want %d", name, meta.Stride, wantStride)
	}
	if meta.BytesWritten != 0 {
		t.Errorf("Probe(%s) BytesWritten = %d, want 0", name, meta.BytesWritten)
	}
}

// assertTGADecodeRGBA decodes src into d's reserved destination and asserts
// the DecodeRGBA contract shared by every positive TGA fixture: no error, a
// non-nil Meta carrying FormatTGA, BytesWritten equal to len(wantPix), pixels
// equal to wantPix, an unchanged caller-owned destination layout, and an
// unchanged complete guest memory state (wasm backing pointer, byte length,
// and every slot field). It returns the decoded Meta so callers can assert
// fixture-specific fields (e.g. exact geometry).
func assertTGADecodeRGBA(t *testing.T, d *wuffs.Decoder, name string, dst *image.RGBA, src, wantPix []byte) *wuffs.Meta {
	t.Helper()
	guestBefore := wuffs.CaptureGuestMemoryState(d)
	pixPtr := &dst.Pix[0]
	pixLen, pixCap := len(dst.Pix), cap(dst.Pix)
	rect, dstStride := dst.Rect, dst.Stride

	decMeta, err := d.DecodeRGBA(dst, src)
	if err != nil {
		t.Fatalf("DecodeRGBA(%s): %v", name, err)
	}
	assertGuestMemoryPreserved(t, "DecodeRGBA", name, d, guestBefore)
	if decMeta == nil {
		t.Fatal("DecodeRGBA returned nil Meta")
	}
	if decMeta.Format != tgaFourCC {
		t.Errorf("DecodeRGBA(%s) Format = 0x%08X, want FormatTGA 0x%08X", name, decMeta.Format, tgaFourCC)
	}
	if decMeta.BytesWritten != uint32(len(wantPix)) {
		t.Errorf("DecodeRGBA(%s) BytesWritten = %d, want %d", name, decMeta.BytesWritten, len(wantPix))
	}
	if !bytes.Equal(dst.Pix, wantPix) {
		// Dump both canvases only when the fixture is small; a full dump of a
		// 160×120 oracle would be noise.
		if len(dst.Pix)+len(wantPix) <= 64 {
			t.Errorf("DecodeRGBA(%s) pixels = % X, want % X", name, dst.Pix, wantPix)
		} else {
			t.Errorf("DecodeRGBA(%s) pixels differ from expected", name)
		}
	}
	if &dst.Pix[0] != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap ||
		dst.Rect != rect || dst.Stride != dstStride {
		t.Fatal("DecodeRGBA changed the caller-owned destination layout")
	}
	return decMeta
}

// expectUnknownProbe probes src and asserts ErrUnknownFormat with a nil Meta.
func expectUnknownProbe(t *testing.T, d *wuffs.Decoder, name string, src []byte) {
	t.Helper()
	meta, err := d.Probe(src)
	if !errors.Is(err, wuffs.ErrUnknownFormat) {
		t.Errorf("Probe(%s) error = %v, want ErrUnknownFormat", name, err)
	}
	if meta != nil {
		t.Errorf("Probe(%s) returned non-nil Meta %+v, want nil", name, meta)
	}
}

// tgaHeader builds an 18-byte TGA header.
func tgaHeader(idLen, cmapType, imgType byte, cmapFirst, cmapLen uint16, cmapDepth byte, w, h uint16, pixelDepth, desc byte) []byte {
	hdr := make([]byte, 18)
	hdr[0] = idLen
	hdr[1] = cmapType
	hdr[2] = imgType
	hdr[3] = byte(cmapFirst)
	hdr[4] = byte(cmapFirst >> 8)
	hdr[5] = byte(cmapLen)
	hdr[6] = byte(cmapLen >> 8)
	hdr[7] = cmapDepth
	hdr[12] = byte(w)
	hdr[13] = byte(w >> 8)
	hdr[14] = byte(h)
	hdr[15] = byte(h >> 8)
	hdr[16] = pixelDepth
	hdr[17] = desc
	return hdr
}

// expandTGA5 expands a 5-bit TGA channel to 8 bits using the Wuffs rule
// (c<<3)|(c>>2), which the targa decoder applies to every 5-5-5 color sample.
func expandTGA5(c byte) byte { return (c << 3) | (c >> 2) }

// tgaCase is one positive decode fixture in the full accepted matrix.
type tgaCase struct {
	name    string
	src     []byte
	w, h    uint16
	wantPix []byte
}

// tgaPositiveMatrix builds the complete accepted type/depth/origin matrix:
//
//   - image types 1 and 9 at each color-map depth 15, 24, and 32 with 8-bit
//     indices (12 cases);
//   - image types 2 and 10 at each pixel depth 15, 16, 24, and 32 (16 cases,
//     15- and 16-bit with attribute value 0, 32-bit with attribute bits 8);
//   - image types 3 and 11 at pixel depth 8 (4 cases).
//
// Every class is crossed with both permitted descriptor vertical origins
// (0x00 and 0x20). True-color 24-bit pixels are BGR and 32-bit pixels are
// BGRA with alpha 255; 15-/16-bit pixels use the 5-5-5 expansion with the
// attribute bit clear; grayscale is a single repeatable sample. RLE types
// (9, 10, 11) are 2×1 with a run packet so the run-length path is exercised.
func tgaPositiveMatrix() []tgaCase {
	indexedEntry := map[byte][]byte{
		15: {0x1F, 0x41},             // word 0x411F: B5=0x1F G5=0x08 R5=0x10, bit15 clear
		24: {0x11, 0x22, 0x33},       // B, G, R
		32: {0x11, 0x22, 0x33, 0xFF}, // B, G, R, A
	}
	indexedPix := map[byte][]byte{
		15: {expandTGA5(0x10), expandTGA5(0x08), expandTGA5(0x1F), 0xFF},
		24: {0x33, 0x22, 0x11, 0xFF},
		32: {0x33, 0x22, 0x11, 0xFF},
	}
	tcPixel := map[byte][]byte{
		15: {0x1F, 0x41},             // word 0x411F
		16: {0x47, 0x51},             // word 0x5147
		24: {0x10, 0x20, 0x30},       // B, G, R
		32: {0x10, 0x20, 0x30, 0xFF}, // B, G, R, A
	}
	tcPix := map[byte][]byte{
		15: {expandTGA5(0x10), expandTGA5(0x08), expandTGA5(0x1F), 0xFF},
		16: {expandTGA5(0x14), expandTGA5(0x0A), expandTGA5(0x07), 0xFF},
		24: {0x30, 0x20, 0x10, 0xFF},
		32: {0x30, 0x20, 0x10, 0xFF},
	}

	var cases []tgaCase
	// Indexed: types 1 and 9 at color-map depths 15, 24, and 32.
	for _, imgType := range []byte{1, 9} {
		for _, depth := range []byte{15, 24, 32} {
			for _, origin := range []byte{0x00, 0x20} {
				name := fmt.Sprintf("indexed type%d depth%d origin0x%02X", imgType, depth, origin)
				// RLE (type 9) fixtures are 2×1 with a two-pixel run packet so
				// the run-length path is exercised; raw (type 1) are 1×1.
				w, h := uint16(1), uint16(1)
				if imgType == 9 {
					w, h = 2, 1
				}
				src := tgaHeader(0, 1, imgType, 0, 1, depth, w, h, 8, origin)
				src = append(src, indexedEntry[depth]...)
				want := append([]byte(nil), indexedPix[depth]...)
				if imgType == 9 {
					// RLE run packet: two pixels of palette entry 0.
					src = append(src, 0x81, 0x00)
					want = append(want, want...)
				} else {
					src = append(src, 0x00)
				}
				cases = append(cases, tgaCase{name, src, w, h, want})
			}
		}
	}
	// True-color: types 2 and 10 at pixel depths 15, 16, 24, and 32.
	for _, imgType := range []byte{2, 10} {
		for _, depth := range []byte{15, 16, 24, 32} {
			attr := byte(0x00)
			if depth == 32 {
				attr = 0x08
			}
			for _, origin := range []byte{0x00, 0x20} {
				name := fmt.Sprintf("truecolor type%d depth%d origin0x%02X", imgType, depth, origin)
				// RLE (type 10) fixtures are 2×1 with a two-pixel run packet so
				// the run-length path is exercised; raw (type 2) are 1×1.
				w, h := uint16(1), uint16(1)
				if imgType == 10 {
					w, h = 2, 1
				}
				src := tgaHeader(0, 0, imgType, 0, 0, 0, w, h, depth, origin|attr)
				want := append([]byte(nil), tcPix[depth]...)
				if imgType == 10 {
					// RLE run packet: two pixels of the same value.
					src = append(src, 0x81)
					src = append(src, tcPixel[depth]...)
					want = append(want, want...)
				} else {
					src = append(src, tcPixel[depth]...)
				}
				cases = append(cases, tgaCase{name, src, w, h, want})
			}
		}
	}
	// Grayscale: types 3 and 11 at pixel depth 8.
	for _, imgType := range []byte{3, 11} {
		for _, origin := range []byte{0x00, 0x20} {
			name := fmt.Sprintf("grayscale type%d origin0x%02X", imgType, origin)
			// RLE (type 11) fixtures are 2×1 with a two-pixel run packet so
			// the run-length path is exercised; raw (type 3) are 1×1.
			w, h := uint16(1), uint16(1)
			if imgType == 11 {
				w, h = 2, 1
			}
			src := tgaHeader(0, 0, imgType, 0, 0, 0, w, h, 8, origin)
			want := []byte{0x55, 0x55, 0x55, 0xFF}
			if imgType == 11 {
				// RLE run packet: two pixels of the same sample.
				src = append(src, 0x81, 0x55)
				want = append(want, want...)
			} else {
				src = append(src, 0x55)
			}
			cases = append(cases, tgaCase{name, src, w, h, want})
		}
	}
	return cases
}

// tgaGeometry builds a complete 8-bit grayscale image of the given size with
// pixel i (row-major) equal to byte value i, so the expected RGBA canvas is a
// deterministic gradient.
func tgaGeometry(w, h uint16, desc byte) []byte {
	src := tgaHeader(0, 0, 3, 0, 0, 0, w, h, 8, desc)
	for i := 0; i < int(w)*int(h); i++ {
		src = append(src, byte(i))
	}
	return src
}

// grayGradientPixels returns the RGBA canvas for a w×h grayscale gradient
// whose sample at row-major index i is byte value i.
func grayGradientPixels(w, h uint16) []byte {
	pix := make([]byte, 0, int(w)*int(h)*4)
	for i := 0; i < int(w)*int(h); i++ {
		v := byte(i)
		pix = append(pix, v, v, v, 0xFF)
	}
	return pix
}

// tgaUnsupportedFixtures builds complete, otherwise-valid TGA inputs that lie
// outside the conservative shim subset and must stay ErrUnknownFormat:
//
//   - indexed types 1 and 9 with 16-bit color-map entries containing both
//     clear and set alpha bits (color-map depth 16 is outside {15,24,32});
//   - 15-bit true-color types 2 and 10 declaring one attribute bit
//     (descriptor attribute bits = 1), with pixels whose alpha bit is both
//     clear and set;
//   - a 16-bit BGRA5551 declaring one attribute bit with pixels whose alpha
//     bit is both clear and set.
func tgaUnsupportedFixtures() []struct {
	name string
	src  []byte
} {
	var fixtures []struct {
		name string
		src  []byte
	}
	appendFixture := func(name string, src []byte) {
		fixtures = append(fixtures, struct {
			name string
			src  []byte
		}{name, src})
	}

	// Indexed with 16-bit color-map entries: two palette entries, one with the
	// alpha bit clear and one with it set, then two 8-bit indices.
	for _, imgType := range []byte{1, 9} {
		src := tgaHeader(0, 1, imgType, 0, 2, 16, 2, 1, 8, 0x00)
		src = append(src, 0x1F, 0x41) // 0x411F: alpha bit clear
		src = append(src, 0x1F, 0xC1) // 0xC11F: alpha bit set
		if imgType == 9 {
			// RLE literal packet: two indices.
			src = append(src, 0x01, 0x00, 0x01)
		} else {
			src = append(src, 0x00, 0x01)
		}
		appendFixture(fmt.Sprintf("indexed type%d 16-bit color map", imgType), src)
	}

	// 15-bit true-color declaring one attribute bit, pixels with clear and set
	// alpha bits.
	for _, imgType := range []byte{2, 10} {
		src := tgaHeader(0, 0, imgType, 0, 0, 0, 2, 1, 15, 0x01)
		if imgType == 10 {
			src = append(src, 0x01) // RLE literal packet: two pixels.
		}
		src = append(src, 0x1F, 0x41) // 0x411F: alpha bit clear
		src = append(src, 0x1F, 0xC1) // 0xC11F: alpha bit set
		appendFixture(fmt.Sprintf("15-bit true-color type%d one attribute bit", imgType), src)
	}

	// 16-bit BGRA5551 declaring one attribute bit, pixels with clear and set
	// alpha bits.
	src := tgaHeader(0, 0, 2, 0, 0, 0, 2, 1, 16, 0x01)
	src = append(src, 0x47, 0x51) // 0x5147: alpha bit clear
	src = append(src, 0x47, 0xD1) // 0xD147: alpha bit set
	appendFixture("16-bit BGRA5551 one attribute bit", src)

	return fixtures
}

// tgaDescriptorClass describes one otherwise-valid baseline class (header
// only; Probe needs no pixel payload) plus the full descriptor rejection set
// derived from it: every invalid attribute-bit value and every interleave or
// right-to-left mask, with and without the vertical-origin bit.
type tgaDescriptorClass struct {
	name string
	src  func(desc byte) []byte

	// validAttr is the only permitted descriptor attribute-bit value for the
	// class: 0 for indexed, grayscale, and 15-/16-/24-bit true color; 8 for
	// 32-bit true color.
	validAttr byte

	// invalidAttrs contains every attribute value that must be rejected.
	invalidAttrs []byte

	// rejections holds the concrete descriptor bytes this class must reject,
	// filled by tgaDescriptorClasses after every class is registered.
	rejections []byte
}

// tgaDescriptorClasses returns every otherwise-valid baseline class from the
// accepted matrix.
func tgaDescriptorClasses() []tgaDescriptorClass {
	var classes []tgaDescriptorClass
	add := func(name string, mk func(byte) []byte, validAttr byte, invalidAttrs []byte) {
		classes = append(classes, tgaDescriptorClass{name: name, src: mk, validAttr: validAttr, invalidAttrs: invalidAttrs})
	}

	for _, imgType := range []byte{1, 9} {
		for _, depth := range []byte{15, 24, 32} {
			add(fmt.Sprintf("indexed type%d color-map depth%d", imgType, depth),
				func(desc byte) []byte { return tgaHeader(0, 1, imgType, 0, 1, depth, 1, 1, 8, desc) },
				0x00, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15})
		}
	}
	for _, imgType := range []byte{2, 10} {
		for _, depth := range []byte{15, 16, 24} {
			add(fmt.Sprintf("true-color type%d depth%d", imgType, depth),
				func(desc byte) []byte { return tgaHeader(0, 0, imgType, 0, 0, 0, 1, 1, depth, desc) },
				0x00, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15})
		}
		add(fmt.Sprintf("true-color type%d depth32", imgType),
			func(desc byte) []byte { return tgaHeader(0, 0, imgType, 0, 0, 0, 1, 1, 32, desc) },
			0x08, []byte{0, 1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 12, 13, 14, 15})
	}
	for _, imgType := range []byte{3, 11} {
		add(fmt.Sprintf("grayscale type%d", imgType),
			func(desc byte) []byte { return tgaHeader(0, 0, imgType, 0, 0, 0, 1, 1, 8, desc) },
			0x00, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15})
	}

	// rejections expands each class into its descriptor rejection probes: each
	// invalid attribute value with and without the vertical-origin bit, and
	// every interleave/right-to-left mask ORed over an otherwise-valid
	// descriptor, with and without the vertical-origin bit.
	for i := range classes {
		var rejections []byte
		for _, attr := range classes[i].invalidAttrs {
			rejections = append(rejections, attr, attr|0x20)
		}
		for _, mask := range []byte{0x10, 0x40, 0x80, 0xC0} {
			rejections = append(rejections,
				classes[i].validAttr|mask,
				classes[i].validAttr|0x20|mask)
		}
		classes[i].rejections = rejections
	}
	return classes
}
