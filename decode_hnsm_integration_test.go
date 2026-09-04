package wuffs_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// hnsmFourCC is asserted as a local uint32 literal rather than the exported
// constant so this runtime characterization stays independent of the exported
// name, matching the FORMAT-03 plan convention.
const hnsmFourCC = uint32(0x484E534D)

// TestIntegrationHNSMDecodeCharacterization verifies the Handsum (HNSM)
// contract end-to-end against the live wasm guest. The fixtures and their
// canonical evidence come from testdata/hnsm.golden.manifest, which records
// each vendored upstream handsum fixture's encoded length, decoded
// dimensions, its variant fields (color class and quality bit values decoded
// from the fixture's own header), and the CRC-32 (IEEE) of its decoded
// straight RGBA pixels. Those CRCs were computed by an independent C oracle
// program (tmp/hnsm_oracle.c) against the upstream Wuffs v0.4 release C
// library (the exact decoder embedded in the wasm guest) using the guest's
// decode parameters, and then transformed with the host's own
// convertBGRAToStraight B/R swap and unpremultiply rules.
//
//   - Every manifest fixture probes as FormatHNSM with exact Wuffs-derived
//     dimensions (always one dimension of 16 and the other from 1 through
//     16), stride = Width*4, and BytesWritten 0. The fixture set spans the
//     three accepted color classes (grayscale 0, RGB 2, RGBA 3), all four
//     quality bits (0 through 3), and all three geometries (landscape 16x12,
//     portrait 11x16, square 16x16), and the vendored bytes' own header must
//     agree with the recorded variant fields and encoded length.
//   - Every manifest fixture decodes to its manifest CRC-32 AND the full
//     decode contract asserted by assertHNSMDecodeRGBA, with the
//     caller-owned destination layout preserved.
//   - The complete accepted structural matrix (every color class 0, 2, and 3
//     at every quality 0..3 and every geometry code 0..30 whose top 15
//     header bits equal 0x7F6B) probes as FormatHNSM with exact dimensions.
//   - A structurally recognized truncated payload (a bare valid three-byte
//     header, or a real fixture cut off mid-payload) returns ErrDecode never
//     ErrUnknownFormat, with a nil Meta, an untouched sentinel destination,
//     and unchanged complete guest memory.
//   - Color class 1, the reserved geometry code 0x1F, header prefixes whose
//     top 15 bits are not 0x7F6B, headers shorter than three bytes, the
//     retired literal "HNSM" check, and the generated collision corpus all
//     stay ErrUnknownFormat.
//   - One canonical fixture per established recognizer keeps its own
//     classification: the HNSM sniff must not shadow any existing format.
//   - After explicit Reserve and one warm-up DecodeRGBA per fixture,
//     repeated DecodeRGBA performs exactly zero Go heap allocations.
//
// It fails against the current guest because sniff_fourcc still carries the
// retired literal "HNSM" check instead of parsing the conservative three-byte
// structural header, so valid HNSM fixtures probe as ErrUnknownFormat.
func TestIntegrationHNSMDecodeCharacterization(t *testing.T) {
	fixtures := loadHNSMManifest(t)
	if len(fixtures) == 0 {
		t.Fatal("testdata/hnsm.golden.manifest selected no fixtures")
	}

	t.Run("fixture set spans the upstream color, quality, and geometry variants", func(t *testing.T) {
		colors := map[int]bool{}
		qualities := map[int]bool{}
		aspect := map[string]bool{}
		for _, fc := range fixtures {
			src := loadFixture(t, fc.file)
			if len(src) != fc.length {
				t.Errorf("encoded length of %s = %d, want manifest %d", fc.file, len(src), fc.length)
			}

			// The vendored fixture's own header must decode to the recorded
			// variant fields and geometry.
			v := (uint32(src[0]) << 16) | (uint32(src[1]) << 8) | uint32(src[2])
			if v>>9 != 0x7F6B {
				t.Errorf("%s header top 15 bits = 0x%04X, want 0x7F6B", fc.file, v>>9)
			}
			if gotColor, gotQuality := int((v>>7)&3), int((v>>5)&3); gotColor != fc.color || gotQuality != fc.quality {
				t.Errorf("%s header color/quality = %d/%d, want manifest %d/%d", fc.file, gotColor, gotQuality, fc.color, fc.quality)
			}
			gotW, gotH := hnsmGeometry(byte(v & 31))
			if gotW != uint32(fc.width) || gotH != uint32(fc.height) {
				t.Errorf("%s header geometry = (%d,%d), want manifest (%d,%d)", fc.file, gotW, gotH, fc.width, fc.height)
			}

			colors[fc.color] = true
			qualities[fc.quality] = true
			switch {
			case fc.width > fc.height:
				aspect["landscape"] = true
			case fc.width < fc.height:
				aspect["portrait"] = true
			default:
				aspect["square"] = true
			}
			if (fc.width != 16 && fc.height != 16) || fc.width < 1 || fc.width > 16 || fc.height < 1 || fc.height > 16 {
				t.Errorf("%s geometry (%d,%d) must have one dimension of 16 and the other 1..16", fc.file, fc.width, fc.height)
			}
		}
		if !colors[0] || !colors[2] || !colors[3] {
			t.Errorf("fixture set color coverage = %v, want classes {0 grayscale, 2 RGB, 3 RGBA}", colors)
		}
		if colors[1] {
			t.Errorf("fixture set color coverage %v includes the rejected class 1", colors)
		}
		for q := 0; q <= 3; q++ {
			if !qualities[q] {
				t.Errorf("fixture set quality coverage = %v, want 0 through 3", qualities)
			}
		}
		if !aspect["landscape"] || !aspect["portrait"] || !aspect["square"] {
			t.Errorf("fixture set aspect coverage = %v, want landscape, portrait, and square", aspect)
		}
	})

	t.Run("every manifest fixture probes as FormatHNSM with exact geometry", func(t *testing.T) {
		for _, fc := range fixtures {
			t.Run(fc.file, func(t *testing.T) {
				src := loadFixture(t, fc.file)

				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, 16*16*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve(%s): %v", fc.file, err)
				}
				assertHNSMMeta(t, d, fc.file, src, uint32(fc.width), uint32(fc.height))
			})
		}
	})

	t.Run("every manifest fixture decodes to its canonical Wuffs CRC", func(t *testing.T) {
		for _, fc := range fixtures {
			t.Run(fc.file, func(t *testing.T) {
				src := loadFixture(t, fc.file)

				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, 16*16*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve(%s): %v", fc.file, err)
				}
				dst := image.NewRGBA(image.Rect(0, 0, fc.width, fc.height))
				assertHNSMDecodeRGBA(t, d, fc.file, dst, src, uint32(fc.width), uint32(fc.height))

				if got := crc32.ChecksumIEEE(dst.Pix); got != fc.crc {
					t.Errorf("DecodeRGBA(%s) pixel CRC-32 = 0x%08X (%d), want canonical 0x%08X (%d)", fc.file, got, got, fc.crc, fc.crc)
				}
			})
		}
	})

	t.Run("full accepted structural matrix probes as FormatHNSM", func(t *testing.T) {
		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, 16*16*4, 4); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}
		for _, color := range []byte{0, 2, 3} {
			for quality := byte(0); quality <= 3; quality++ {
				for geom := byte(0); geom <= 30; geom++ {
					name := fmt.Sprintf("color %d quality %d geometry 0x%02X", color, quality, geom)
					w, h := hnsmGeometry(geom)
					assertHNSMMeta(t, d, name, hnsmHeader(color, quality, geom), w, h)
				}
			}
		}
	})

	t.Run("truncated structurally recognized payloads return ErrDecode", func(t *testing.T) {
		mona := loadFixture(t, "mona-lisa.21x32.c3q4.handsum")
		bricks := loadFixture(t, "bricks-color.c3q4.handsum")

		cases := []struct {
			name         string
			src          []byte
			wantW, wantH uint32
			label        string
		}{
			// A bare valid three-byte header: structurally recognized by the
			// probe, but the encoded coefficient payload is absent.
			{"three-byte mona-lisa header", mona[:3], 11, 16, "mona-lisa header"},
			// A real fixture cut off mid-payload: header intact, coefficients
			// truncated.
			{"mona-lisa payload truncated", mona[:100], 11, 16, "mona-lisa payload"},
			{"bricks-color payload truncated", bricks[:40], 16, 12, "bricks-color payload"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, 16*16*4, len(tc.src)); err != nil {
					t.Fatalf("RequiredReserve: %v", err)
				}

				// Probe still recognizes the structural header.
				assertHNSMMeta(t, d, tc.label, tc.src, tc.wantW, tc.wantH)

				guestBefore := wuffs.CaptureGuestMemoryState(d)

				dst := image.NewRGBA(image.Rect(0, 0, int(tc.wantW), int(tc.wantH)))
				for i := range dst.Pix {
					dst.Pix[i] = 0xA5
				}
				prefill := append([]byte(nil), dst.Pix...)
				dstBefore := captureRGBAState(dst)

				decMeta, err := d.DecodeRGBA(dst, tc.src)
				assertGuestMemoryPreserved(t, "DecodeRGBA", tc.label, d, guestBefore)
				assertDestinationPreserved(t, "DecodeRGBA", tc.label, dst, dstBefore)
				if err == nil {
					t.Fatal("DecodeRGBA(truncated Handsum) error = nil, want ErrDecode")
				}
				if !errors.Is(err, wuffs.ErrDecode) {
					t.Errorf("DecodeRGBA(truncated Handsum) error = %v, want errors.Is(err, ErrDecode)", err)
				}
				if errors.Is(err, wuffs.ErrUnknownFormat) {
					t.Errorf("DecodeRGBA(truncated Handsum) error = %v, must not be ErrUnknownFormat", err)
				}
				if decMeta != nil {
					t.Errorf("DecodeRGBA(truncated Handsum) returned non-nil Meta %+v, want nil", decMeta)
				}
				if !bytes.Equal(dst.Pix, prefill) {
					t.Error("DecodeRGBA(truncated Handsum) mutated the sentinel destination pixels")
				}
			})
		}
	})

	t.Run("invalid structural headers and the collision corpus stay ErrUnknownFormat", func(t *testing.T) {
		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, 16*16*4, 64); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}
		dst := image.NewRGBA(image.Rect(0, 0, 16, 16))
		for i := range dst.Pix {
			dst.Pix[i] = 0x5A
		}

		// Color class 1 is rejected at every quality and geometry code.
		for quality := byte(0); quality <= 3; quality++ {
			for geom := byte(0); geom <= 30; geom++ {
				assertHNSMRejected(t, d, dst,
					fmt.Sprintf("color class 1 quality %d geometry 0x%02X", quality, geom),
					hnsmHeader(1, quality, geom))
			}
		}

		// The reserved geometry code 0x1F is rejected at every color class
		// and quality.
		for _, color := range []byte{0, 2, 3} {
			for quality := byte(0); quality <= 3; quality++ {
				assertHNSMRejected(t, d, dst,
					fmt.Sprintf("quality %d reserved geometry 0x1F", quality),
					hnsmHeader(color, quality, 0x1F))
			}
		}

		// Header prefixes whose top 15 bits are not 0x7F6B, headers shorter
		// than the three structural bytes, and the retired literal "HNSM"
		// check all stay unknown.
		prefixCases := []struct {
			name string
			src  []byte
		}{
			{"first header byte 0xFF", []byte{0xFF, 0xD6, 0x00}},
			{"first header byte 0xFD", []byte{0xFD, 0xD6, 0x00}},
			{"second header byte 0xD5", []byte{0xFE, 0xD5, 0x00}},
			{"second header byte 0xD8", []byte{0xFE, 0xD8, 0x00}},
			{"second header byte 0x00", []byte{0xFE, 0x00, 0x00}},
			{"second header byte 0xFF", []byte{0xFE, 0xFF, 0x00}},
			{"all-zero header", []byte{0x00, 0x00, 0x00}},
			{"misplaced raw prefix", []byte{0x7F, 0x6B, 0x00}},
			{"one header byte", []byte{0xFE}},
			{"two header bytes", []byte{0xFE, 0xD6}},
			{"retired HNSM literal", []byte{'H', 'N', 'S', 'M'}},
			{"18 zero bytes", make([]byte, 18)},
			{"bytes 0x00 through 0x11", []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11}},
			{"ELF bytes padded to 18", append([]byte{0x7F, 'E', 'L', 'F'}, make([]byte, 14)...)},
		}
		for _, pc := range prefixCases {
			assertHNSMRejected(t, d, dst, pc.name, pc.src)
		}
	})

	t.Run("existing recognizers keep priority and classification", func(t *testing.T) {
		assertRecognizerPriority(t, []recognizerCase{
			{"bricks-color.bmp", uint32(wuffs.FormatBMP)},
			{"bricks-color.png", uint32(wuffs.FormatPNG)},
			{"hat.jpeg", uint32(wuffs.FormatJPEG)},
			{"bricks-nodither.gif", uint32(wuffs.FormatGIF)},
			{"bricks-color.lossless.webp", uint32(wuffs.FormatWEBP)},
			{"bricks-color.qoi", uint32(wuffs.FormatQOI)},
			{"hippopotamus.ppm", uint32(wuffs.FormatNPBM)},
			{"hippopotamus.pgm", uint32(wuffs.FormatNPBM)},
			{"muybridge-frame-000.wbmp", uint32(wuffs.FormatWBMP)},
			{"bricks-color.tga", uint32(wuffs.FormatTGA)},
			{"bricks-color.etc2.pkm", uint32(wuffs.FormatETC2)},
			{"49.bn4.nie", uint32(wuffs.FormatNIE)},
			{"mona-lisa.21x32.th", uint32(wuffs.FormatTH)},
		}, hnsmFourCC, "HNSM", "HNSM")
	})

	t.Run("repeated DecodeRGBA allocates zero heap objects per manifest fixture", func(t *testing.T) {
		for _, fc := range fixtures {
			t.Run(fc.file, func(t *testing.T) {
				src := loadFixture(t, fc.file)

				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, 16*16*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve(%s): %v", fc.file, err)
				}

				dst := image.NewRGBA(image.Rect(0, 0, fc.width, fc.height))
				assertZeroAllocsPerRun(t, d, dst, src, fc.file)
			})
		}
	})
}

// hnsmFixture mirrors one manifest line of testdata/hnsm.golden.manifest: a
// vendored upstream handsum fixture plus its canonical decode evidence and
// variant fields.
type hnsmFixture struct {
	file    string
	length  int    // encoded byte length
	width   int    // decoded width
	height  int    // decoded height
	color   int    // 0=grayscale, 2=RGB, 3=RGBA (1 is rejected)
	quality int    // 0=worst .. 3=best
	crc     uint32 // CRC-32 (IEEE) of the decoded straight RGBA pixels
}

// loadHNSMManifest parses testdata/hnsm.golden.manifest with an explicit
// scanner and strconv (never regular expressions) into []hnsmFixture. It
// selects exactly the fixtures the manifest names; blank lines and '#'
// lines are ignored.
func loadHNSMManifest(t *testing.T) []hnsmFixture {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "hnsm.golden.manifest"))
	if err != nil {
		t.Fatalf("opening testdata/hnsm.golden.manifest: %v", err)
	}
	defer f.Close()

	var fixtures []hnsmFixture
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 7 {
			t.Fatalf("testdata/hnsm.golden.manifest: malformed line %q", line)
		}
		// intField parses fields[i] as a decimal integer, naming the field
		// in the fatal message when the manifest line is malformed.
		intField := func(i int, name string) int {
			v, err := strconv.Atoi(fields[i])
			if err != nil {
				t.Fatalf("testdata/hnsm.golden.manifest: %s of %q: %v", name, fields[0], err)
			}
			return v
		}
		crc64, err := strconv.ParseUint(fields[6], 10, 32)
		if err != nil {
			t.Fatalf("testdata/hnsm.golden.manifest: crc32 of %q: %v", fields[0], err)
		}
		fixtures = append(fixtures, hnsmFixture{
			file:    fields[0],
			length:  intField(1, "encoded length"),
			width:   intField(2, "width"),
			height:  intField(3, "height"),
			color:   intField(4, "color"),
			quality: intField(5, "quality"),
			crc:     uint32(crc64),
		})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading testdata/hnsm.golden.manifest: %v", err)
	}
	return fixtures
}

// hnsmHeader returns the three-byte HNSM structural header encoding the
// given color class (0 grayscale, 2 RGB, 3 RGBA; 1 is rejected), quality bit
// value (0..3), and geometry code g (0..30; 0x1F is reserved). The top 15
// bits always equal 0x7F6B, matching do_decode_image_config's bit layout:
// bits 8-7 color, bits 6-5 quality, bits 4-0 geometry.
func hnsmHeader(color, quality, geom byte) []byte {
	v := (uint32(0x7F6B) << 9) | (uint32(color) << 7) | (uint32(quality) << 5) | uint32(geom)
	return []byte{byte(v >> 16), byte(v >> 8), byte(v)}
}

// hnsmGeometry resolves a geometry code byte to the decoded dimensions:
// bit 4 clear means 16 x (g&15 + 1), bit 4 set means (g&15 + 1) x 16.
func hnsmGeometry(geom byte) (uint32, uint32) {
	if geom&16 == 0 {
		return 16, uint32(geom&15) + 1
	}
	return uint32(geom&15) + 1, 16
}

// assertHNSMMeta probes src and asserts the complete FormatHNSM metadata
// contract: no error, non-nil Meta, exact FourCC, dimensions, stride
// equal to Width*4, BytesWritten zero from Probe, and complete guest-memory
// preservation across the reserved Probe call.
func assertHNSMMeta(t *testing.T, d *wuffs.Decoder, name string, src []byte, wantW, wantH uint32) {
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
	if meta.Format != hnsmFourCC {
		t.Errorf("Probe(%s) Format = 0x%08X, want FormatHNSM 0x%08X", name, meta.Format, hnsmFourCC)
	}
	if meta.Width != wantW || meta.Height != wantH {
		t.Errorf("Probe(%s) geometry = (%d,%d), want (%d,%d)", name, meta.Width, meta.Height, wantW, wantH)
	}
	if meta.Stride != wantW*4 {
		t.Errorf("Probe(%s) Stride = %d, want %d (Width*4)", name, meta.Stride, wantW*4)
	}
	if meta.BytesWritten != 0 {
		t.Errorf("Probe(%s) BytesWritten = %d, want 0", name, meta.BytesWritten)
	}
}

// assertHNSMDecodeRGBA decodes src into d's reserved destination and asserts
// the FormatHNSM decode contract shared by every positive fixture: no error,
// a non-nil Meta carrying FormatHNSM, exact dimensions, stride wantW*4,
// BytesWritten equal to wantW*wantH*4, an unchanged caller-owned destination
// layout, and an unchanged complete guest memory state (wasm backing pointer,
// byte length, and every slot field).
func assertHNSMDecodeRGBA(t *testing.T, d *wuffs.Decoder, name string, dst *image.RGBA, src []byte, wantW, wantH uint32) {
	t.Helper()
	guestBefore := wuffs.CaptureGuestMemoryState(d)
	dstBefore := captureRGBAState(dst)

	decMeta, err := d.DecodeRGBA(dst, src)
	if err != nil {
		t.Fatalf("DecodeRGBA(%s): %v", name, err)
	}
	assertGuestMemoryPreserved(t, "DecodeRGBA", name, d, guestBefore)
	assertDestinationPreserved(t, "DecodeRGBA", name, dst, dstBefore)
	if decMeta == nil {
		t.Fatalf("DecodeRGBA(%s) returned nil Meta", name)
	}
	if decMeta.Format != hnsmFourCC {
		t.Errorf("DecodeRGBA(%s) Format = 0x%08X, want FormatHNSM 0x%08X", name, decMeta.Format, hnsmFourCC)
	}
	if decMeta.Width != wantW || decMeta.Height != wantH {
		t.Errorf("DecodeRGBA(%s) geometry = (%d,%d), want (%d,%d)", name, decMeta.Width, decMeta.Height, wantW, wantH)
	}
	if decMeta.Stride != wantW*4 {
		t.Errorf("DecodeRGBA(%s) Stride = %d, want %d (Width*4)", name, decMeta.Stride, wantW*4)
	}
	if want := wantW * wantH * 4; decMeta.BytesWritten != want {
		t.Errorf("DecodeRGBA(%s) BytesWritten = %d, want %d", name, decMeta.BytesWritten, want)
	}
}

// assertHNSMRejected asserts that src stays ErrUnknownFormat through both
// Probe and DecodeRGBA - never ErrDecode - with a nil Meta, an untouched
// sentinel destination, and unchanged complete guest memory. name labels the
// case in every diagnostic. d and dst are shared across the whole rejection
// corpus.
func assertHNSMRejected(t *testing.T, d *wuffs.Decoder, dst *image.RGBA, name string, src []byte) {
	t.Helper()
	sentinel := append([]byte(nil), dst.Pix...)

	guestBefore := wuffs.CaptureGuestMemoryState(d)
	meta, err := d.Probe(src)
	assertGuestMemoryPreserved(t, "Probe", name, d, guestBefore)
	if err == nil {
		t.Errorf("Probe(%s) error = nil, want ErrUnknownFormat", name)
	}
	if !errors.Is(err, wuffs.ErrUnknownFormat) {
		t.Errorf("Probe(%s) error = %v, want errors.Is(err, ErrUnknownFormat)", name, err)
	}
	if errors.Is(err, wuffs.ErrDecode) {
		t.Errorf("Probe(%s) error = %v, must not be ErrDecode", name, err)
	}
	if meta != nil {
		t.Errorf("Probe(%s) returned non-nil Meta %+v, want nil", name, meta)
	}

	dstBefore := captureRGBAState(dst)
	guestBefore = wuffs.CaptureGuestMemoryState(d)
	decMeta, err := d.DecodeRGBA(dst, src)
	assertGuestMemoryPreserved(t, "DecodeRGBA", name, d, guestBefore)
	assertDestinationPreserved(t, "DecodeRGBA", name, dst, dstBefore)
	if err == nil {
		t.Errorf("DecodeRGBA(%s) error = nil, want ErrUnknownFormat", name)
	}
	if !errors.Is(err, wuffs.ErrUnknownFormat) {
		t.Errorf("DecodeRGBA(%s) error = %v, want errors.Is(err, ErrUnknownFormat)", name, err)
	}
	if errors.Is(err, wuffs.ErrDecode) {
		t.Errorf("DecodeRGBA(%s) error = %v, must not be ErrDecode", name, err)
	}
	if decMeta != nil {
		t.Errorf("DecodeRGBA(%s) returned non-nil Meta %+v, want nil", name, decMeta)
	}
	if !bytes.Equal(dst.Pix, sentinel) {
		t.Errorf("DecodeRGBA(%s) mutated the sentinel destination: got % X, want % X", name, dst.Pix, sentinel)
	}
}
