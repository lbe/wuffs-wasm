package wuffs_test

import (
	"bytes"
	"errors"
	"hash/crc32"
	"image"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// wbmpFourCC is asserted as a local uint32 literal rather than the exported
// constant so this runtime characterization stays independent of the exported
// name, matching the FORMAT-02 plan convention.
const wbmpFourCC = uint32(0x57424D50)

// wbmpMuybridgeGoldenCRC and wbmpBricksGoldenCRC pin the expected decoded
// RGBA pixels of the two vendored fixtures via CRC-32 (IEEE) of the packed
// straight-RGBA bytes. The values were produced by the independent Wuffs v0.4
// release C library (compiled with gcc) and cross-checked against the in-test
// WBMP bit unpacker in testdata/README; they are never derived from the wasm
// decoder under test.
const (
	wbmpMuybridgeGoldenCRC = uint32(0x339C150B)
	wbmpBricksGoldenCRC    = uint32(0xDDBF25F3)
)

// TestIntegrationWBMPDecodeCharacterization verifies the WBMP (Type 0)
// format contract end-to-end against the live wasm guest:
//
//   - The vendored upstream fixtures (muybridge-frame-000.wbmp, 30×20 with
//     single-byte width and height varints, and bricks-nodither.wbmp, 160×120
//     with a two-byte width varint) probe and decode as FormatWBMP with the
//     expected geometry and stride, BytesWritten zero from Probe, and pixels
//     identical to an independent WBMP bit unpacker pinned by CRC-32.
//   - Exact-boundary Type 0 headers with width 0xFF_FFFF and height 1, and
//     width 1 and height 0xFF_FFFF, probe as FormatWBMP with their exact image
//     configuration without requiring any pixel payload.
//   - A recognizable truncated payload {00,00,01,01} (complete 1×1 header,
//     missing pixel byte) probes cleanly but DecodeRGBA fails matching
//     ErrDecode, not ErrUnknownFormat, without mutating the destination.
//   - The exact negative table - nonzero TypeField and FixHeaderField,
//     unterminated and non-shortest dimension varints, dimensions above
//     0xFF_FFFF, zero dimensions, and the fixed non-WBMP binary corpus (18
//     zero bytes, bytes 0x00 through 0x11, ELF bytes) - stays
//     ErrUnknownFormat.
//   - The exact higher-priority fixtures (PNG, WebP, BMP, GIF, JPEG, QOI,
//     PGM, PPM, TGA) still probe with their own FourCC, and the ETC2, HNSM,
//     NIE, and TH signature-marker sentinels retain ErrDecode.
//
// The recognizer needs no literal magic string: valid Type 0 files begin with
// TypeField and FixHeaderField zero bytes, so sniffing routes the two-byte
// header plus canonical width and height varints to the FormatWBMP decoder
// and leaves pixel validation to the Wuffs decoder.
func TestIntegrationWBMPDecodeCharacterization(t *testing.T) {
	t.Run("vendored fixtures decode to oracle pixels", func(t *testing.T) {
		fixtures := []struct {
			name    string
			file    string
			w, h    int
			wantCRC uint32
		}{
			{"single-byte dimensions", "muybridge-frame-000.wbmp", 30, 20, wbmpMuybridgeGoldenCRC},
			{"multi-byte width", "bricks-nodither.wbmp", 160, 120, wbmpBricksGoldenCRC},
		}
		for _, fc := range fixtures {
			t.Run(fc.name, func(t *testing.T) {
				src := loadFixture(t, fc.file)

				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, fc.w*fc.h*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve: %v", err)
				}
				assertWBMPProbe(t, d, fc.file, src, fc.w, fc.h)

				// Independent oracle: unpack the fixture bitmap bits directly,
				// pinned against the release-decoder CRC recorded above.
				oracle := wbmpOraclePixels(t, src, fc.w, fc.h)
				if got := crc32.ChecksumIEEE(oracle); got != fc.wantCRC {
					t.Fatalf("oracle CRC-32 = %08X, want %08X", got, fc.wantCRC)
				}

				dst := image.NewRGBA(image.Rect(0, 0, fc.w, fc.h))
				assertWBMPDecodeRGBA(t, d, fc.file, dst, src, oracle, fc.w, fc.h)
			})
		}
	})

	t.Run("exact-boundary dimensions probe without pixel payloads", func(t *testing.T) {
		cases := []struct {
			name string
			src  []byte
			w, h int
		}{
			{"width 0xFF_FFFF height 1", []byte{0x00, 0x00, 0x87, 0xFF, 0xFF, 0x7F, 0x01}, 0xFF_FFFF, 1},
			{"width 1 height 0xFF_FFFF", []byte{0x00, 0x00, 0x01, 0x87, 0xFF, 0xFF, 0x7F}, 1, 0xFF_FFFF},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, 4, len(tc.src)); err != nil {
					t.Fatalf("RequiredReserve: %v", err)
				}
				assertWBMPProbe(t, d, tc.name, tc.src, tc.w, tc.h)
			})
		}
	})

	t.Run("recognizable truncated payload fails with ErrDecode", func(t *testing.T) {
		// A complete 1×1 Type 0 header; the single pixel byte is missing.
		trunc := []byte{0x00, 0x00, 0x01, 0x01}

		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, 4, len(trunc)); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}

		guestBeforeProbe := wuffs.CaptureGuestMemoryState(d)
		meta, err := d.Probe(trunc)
		if err != nil {
			t.Fatalf("Probe(truncated): %v", err)
		}
		assertGuestMemoryPreserved(t, "Probe", "truncated payload", d, guestBeforeProbe)
		if meta == nil || meta.Format != wbmpFourCC {
			t.Errorf("Probe(truncated) Meta = %+v, want FormatWBMP 1×1", meta)
		}

		dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
		dst.Pix = []byte{0xA5, 0x5A, 0xA5, 0x5A}
		sentinel := append([]byte(nil), dst.Pix...)

		decMeta, err := d.DecodeRGBA(dst, trunc)
		if err == nil {
			t.Fatal("DecodeRGBA(truncated) error = nil, want ErrDecode")
		}
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Errorf("DecodeRGBA(truncated) error = %v, want errors.Is(err, ErrDecode)", err)
		}
		if errors.Is(err, wuffs.ErrUnknownFormat) {
			t.Errorf("DecodeRGBA(truncated) error = %v, must not be ErrUnknownFormat", err)
		}
		if decMeta != nil {
			t.Errorf("DecodeRGBA(truncated) returned non-nil Meta %+v, want nil", decMeta)
		}
		if !bytes.Equal(dst.Pix, sentinel) {
			t.Errorf("DecodeRGBA(truncated) mutated the sentinel destination: got % X, want % X", dst.Pix, sentinel)
		}
	})

	t.Run("exact negative table stays ErrUnknownFormat", func(t *testing.T) {
		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, 160*120*4, 65536); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}
		seq := make([]byte, 18)
		for i := range seq {
			seq[i] = byte(i)
		}
		negatives := []struct {
			name string
			src  []byte
		}{
			{"nonzero TypeField", []byte{0x01, 0x00, 0x01, 0x01, 0x00}},
			{"nonzero FixHeaderField", []byte{0x00, 0x01, 0x01, 0x01, 0x00}},
			{"unterminated width", []byte{0x00, 0x00, 0x81}},
			{"unterminated height", []byte{0x00, 0x00, 0x01, 0x81}},
			{"non-shortest width", []byte{0x00, 0x00, 0x80, 0x01, 0x01, 0x00}},
			{"non-shortest height", []byte{0x00, 0x00, 0x01, 0x80, 0x01, 0x00}},
			{"width 0x1000000", []byte{0x00, 0x00, 0x88, 0x80, 0x80, 0x00, 0x01}},
			{"height 0x1000000", []byte{0x00, 0x00, 0x01, 0x88, 0x80, 0x80, 0x00}},
			{"zero width", []byte{0x00, 0x00, 0x00, 0x01, 0x00}},
			{"zero height", []byte{0x00, 0x00, 0x01, 0x00}},
			{"18 zero bytes", make([]byte, 18)},
			{"bytes 0x00 through 0x11", seq},
			{"ELF bytes", []byte{0x7F, 0x45, 0x4C, 0x46}},
		}
		for _, tc := range negatives {
			t.Run(tc.name, func(t *testing.T) {
				expectUnknownProbe(t, d, tc.name, tc.src)
			})
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
			{"bricks-color.tga", 0x54474120},           // TGA
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
				t.Errorf("Probe(%s) error = %v, want ErrDecode (not FormatWBMP, not ErrUnknownFormat)", sc.name, err)
			}
			if meta != nil {
				t.Errorf("Probe(%s) returned non-nil Meta %+v, want nil", sc.name, meta)
			}
		}
	})
}

// TestIntegrationWBMPDecodeRGBAAllocsPerRun measures heap allocations per
// DecodeRGBA call for reusable WBMP decoders. Each input and its correctly
// sized caller-owned destination are prepared outside the measured closure,
// one warm-up decode runs first, and testing.AllocsPerRun then requires
// exactly zero allocations per call for both varint encodings: the
// single-byte-dimension muybridge-frame-000.wbmp and the multi-byte-width
// bricks-nodither.wbmp.
func TestIntegrationWBMPDecodeRGBAAllocsPerRun(t *testing.T) {
	fixtures := []struct {
		name         string
		file         string
		wantW, wantH int
	}{
		{"single-byte dimensions", "muybridge-frame-000.wbmp", 30, 20},
		{"multi-byte width", "bricks-nodither.wbmp", 160, 120},
	}

	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			src := loadFixture(t, tc.file)

			d := wuffs.New()
			if err := wuffs.RequiredReserve(d, tc.wantW*tc.wantH*4, len(src)); err != nil {
				t.Fatalf("RequiredReserve: %v", err)
			}

			dst := image.NewRGBA(image.Rect(0, 0, tc.wantW, tc.wantH))
			if _, err := d.DecodeRGBA(dst, src); err != nil {
				t.Fatalf("warm-up DecodeRGBA: %v", err)
			}

			allocs := testing.AllocsPerRun(100, func() {
				if _, err := d.DecodeRGBA(dst, src); err != nil {
					t.Errorf("DecodeRGBA: %v", err)
				}
			})
			if allocs != 0 {
				t.Errorf("DecodeRGBA allocated %.0f heap objects per run, want 0", allocs)
			}
		})
	}
}

// assertWBMPProbe probes src and asserts the complete FormatWBMP metadata
// contract: no error, non-nil Meta, exact FourCC, dimensions, stride,
// BytesWritten zero from Probe, and complete guest-memory preservation across
// the reserved Probe call.
func assertWBMPProbe(t *testing.T, d *wuffs.Decoder, name string, src []byte, wantW, wantH int) {
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
	if meta.Format != wbmpFourCC {
		t.Errorf("Probe(%s) Format = 0x%08X, want FormatWBMP 0x%08X", name, meta.Format, wbmpFourCC)
	}
	if meta.Width != uint32(wantW) || meta.Height != uint32(wantH) {
		t.Errorf("Probe(%s) geometry = (%d,%d), want (%d,%d)", name, meta.Width, meta.Height, wantW, wantH)
	}
	if meta.Stride != uint32(wantW*4) {
		t.Errorf("Probe(%s) Stride = %d, want %d", name, meta.Stride, wantW*4)
	}
	if meta.BytesWritten != 0 {
		t.Errorf("Probe(%s) BytesWritten = %d, want 0", name, meta.BytesWritten)
	}
}

// assertWBMPDecodeRGBA decodes src into the reserved d's destination and
// asserts the FormatWBMP decode contract: no error, a non-nil Meta carrying
// FormatWBMP, BytesWritten equal to len(wantPix), pixels equal to wantPix, an
// unchanged caller-owned destination layout, and an unchanged complete guest
// memory state (wasm backing pointer, byte length, and every slot field).
func assertWBMPDecodeRGBA(t *testing.T, d *wuffs.Decoder, name string, dst *image.RGBA, src, wantPix []byte, wantW, wantH int) {
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
	if decMeta.Format != wbmpFourCC {
		t.Errorf("DecodeRGBA(%s) Format = 0x%08X, want FormatWBMP 0x%08X", name, decMeta.Format, wbmpFourCC)
	}
	if decMeta.BytesWritten != uint32(len(wantPix)) {
		t.Errorf("DecodeRGBA(%s) BytesWritten = %d, want %d", name, decMeta.BytesWritten, len(wantPix))
	}
	if !bytes.Equal(dst.Pix, wantPix) {
		if len(wantPix) <= 64 {
			t.Errorf("DecodeRGBA(%s) pixels = % X, want % X", name, dst.Pix, wantPix)
		} else {
			t.Errorf("DecodeRGBA(%s) pixels differ from the oracle", name)
		}
	}
	if &dst.Pix[0] != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap ||
		dst.Rect != rect || dst.Stride != dstStride {
		t.Fatal("DecodeRGBA changed the caller-owned destination layout")
	}
}

// wbmpOraclePixels unpacks a WBMP Type 0 source into straight RGBA pixels:
// width and height are canonical base-128 varints after the two zero header
// bytes, and each row's bits are MSB-first within bytes with rows aligned to
// byte boundaries, so every row's trailing padding bits are skipped before the
// next row begins. Bit 1 maps to opaque white and bit 0 to opaque black,
// matching the Wuffs decoder's Y-source semantics. It reads src without
// mutating it, leaving the fixture bytes intact for the decoder under test, and
// is an independent oracle sharing no code with the wasm decoder.
func wbmpOraclePixels(t *testing.T, src []byte, wantW, wantH int) []byte {
	t.Helper()
	if len(src) < 4 || src[0] != 0 || src[1] != 0 {
		t.Fatal("oracle: not a WBMP Type 0 header")
	}
	readVarint := func(i int) (int, int) {
		val, n := 0, 0
		for {
			if i >= len(src) {
				t.Fatal("oracle: truncated multi-byte integer")
			}
			c := src[i]
			i++
			n++
			if n > 4 {
				t.Fatal("oracle: dimension exceeds four varint bytes")
			}
			val = val<<7 | int(c&0x7F)
			if c&0x80 == 0 {
				return val, i
			}
		}
	}
	w, i := readVarint(2)
	h, i := readVarint(i)
	if w != wantW || h != wantH {
		t.Fatalf("oracle header = %dx%d, want %dx%d", w, h, wantW, wantH)
	}

	pix := make([]byte, wantW*wantH*4)
	for y := 0; y < wantH; y++ {
		bit := 0 // remaining bits in the current byte; 0 means a new byte is needed
		for x := 0; x < wantW; x++ {
			if bit == 0 {
				if i >= len(src) {
					t.Fatalf("oracle: pixel data exhausted at row %d", y)
				}
				bit = 8
			}
			p := (y*wantW + x) * 4
			if src[i]&(1<<uint(bit-1)) != 0 {
				pix[p], pix[p+1], pix[p+2], pix[p+3] = 0xFF, 0xFF, 0xFF, 0xFF
			} else {
				pix[p], pix[p+1], pix[p+2], pix[p+3] = 0x00, 0x00, 0x00, 0xFF
			}
			bit--
			if bit == 0 {
				i++
			}
		}
		// WBMP Type 0 aligns every row to a byte boundary: skip the current
		// byte's unused padding bits and advance to the next byte.
		if bit != 0 {
			i++
		}
	}
	return pix
}
