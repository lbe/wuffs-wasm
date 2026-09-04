package wuffs_test

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// nieFourCC is the "NIE " FourCC literal local to this test package.
const nieFourCC = uint32(0x4E494520)

// NIE magic byte sequences, the UTF-8 encoding of "nïE" (still) and "nïA"
// (animated container) per ../wuffs/doc/spec/nie-spec.md. The guest sniff
// recognizes both after the higher-priority literal signatures (wasm/shim.c:
// src[0..4] is {0x6E, 0xC3, 0xAF, 0x45} or {0x6E, 0xC3, 0xAF, 0x41}).
var (
	nieStillMagic    = []byte{0x6E, 0xC3, 0xAF, 0x45}
	nieAnimatedMagic = []byte{0x6E, 0xC3, 0xAF, 0x41}
	// nieReversedMagic is the byte-reversed encoding of the two magic
	// sequences ("Aein"): the exact four bytes the pre-format-03 sniff_fourcc
	// matched via read_u32le(src) == 0x6E696541. That retired byte-reversed
	// sentinel is a near-signature mutation and must stay ErrUnknownFormat.
	nieReversedMagic = []byte{0x41, 0x65, 0x69, 0x6E}
)

// TestIntegrationNIEDecodeCharacterization verifies the NIE (Naive Image
// Format) contract end-to-end against the live wasm guest. The fixtures and
// their canonical evidence come from testdata/nie.golden.manifest, which
// records each vendored upstream fixture's reported dimensions, its wire
// pixel encoding, and the CRC-32 (IEEE) of its frame-zero straight RGBA
// pixels independently derived from the NIE specification and the Wuffs v0.4
// pixel-swizzler arithmetic:
//
//   - Every manifest fixture probes as FormatNIE with exact dimensions,
//     stride = Width*4, and BytesWritten 0. This includes both the nïE still
//     signature and the nïA container signature.
//   - Every manifest fixture decodes frame zero to the manifest's canonical
//     CRC-32 AND to the independently derived per-pixel RGBA bytes, with the
//     caller-owned destination layout preserved. The still fixtures cover
//     BGRA non-premultiplied and premultiplied pixels at 8 and 16 bits per
//     channel (bn4, bn8, bp4, bp8); the nïA container verifies only
//     frame-zero behavior (later frames are neither decoded nor validated by
//     FORMAT-03).
//   - Recognizable malformed frame-zero data or metadata required to reach
//     frame zero (truncated payload, invalid configuration byte, width with
//     the high bit set, inner NIE header mismatching the outer NIA config)
//     returns ErrDecode - never ErrUnknownFormat - with a nil Meta and a
//     prefilled sentinel destination byte-for-byte untouched.
//   - Near-signature mutations stay ErrUnknownFormat: the retired
//     byte-reversed "Aein" sequence, plus single-byte mutations of each of
//     the three distinctive nïE magic bytes.
//   - One canonical fixture per established recognizer keeps its own
//     classification: the NIE sniff must not shadow any existing format.
//   - After explicit Reserve and one warm-up DecodeRGBA per fixture,
//     repeated DecodeRGBA performs exactly zero Go heap allocations.
//
// The NIE format is asserted via the local uint32 FourCC literal rather than
// the exported constant so this runtime characterization stays independent of
// the exported name, matching the FORMAT-03 plan convention.
func TestIntegrationNIEDecodeCharacterization(t *testing.T) {
	fixtures := loadNIEManifest(t)
	if len(fixtures) == 0 {
		t.Fatal("testdata/nie.golden.manifest selected no fixtures")
	}

	t.Run("every manifest fixture probes as FormatNIE with exact geometry", func(t *testing.T) {
		for _, fc := range fixtures {
			t.Run(fc.file, func(t *testing.T) {
				src := loadFixture(t, fc.file)

				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, fc.width*fc.height*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve(%s): %v", fc.file, err)
				}
				assertNIEProbeMeta(t, d, fc.file, src, fc.width, fc.height)
			})
		}
	})

	t.Run("every manifest fixture decodes frame zero to its exact RGBA oracle", func(t *testing.T) {
		for _, fc := range fixtures {
			t.Run(fc.file, func(t *testing.T) {
				src := loadFixture(t, fc.file)

				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, fc.width*fc.height*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve(%s): %v", fc.file, err)
				}
				dst := image.NewRGBA(image.Rect(0, 0, fc.width, fc.height))
				assertNIEDecodeRGBA(t, d, fc.file, dst, src, uint32(fc.width), uint32(fc.height))

				want, _, _ := nieFrameZeroStraightRGBA(t, src)
				if !bytes.Equal(dst.Pix, want) {
					t.Errorf("DecodeRGBA(%s, %s) pixels differ from the independently derived RGBA oracle (first mismatch at byte %d)", fc.file, fc.encoding, firstMismatch(dst.Pix, want))
				}
				if got := crc32.ChecksumIEEE(dst.Pix); got != fc.crc {
					t.Errorf("DecodeRGBA(%s) pixel CRC-32 = 0x%08X (%d), want canonical 0x%08X (%d)", fc.file, got, got, fc.crc, fc.crc)
				}
			})
		}
	})

	t.Run("malformed frame-zero data returns ErrDecode with untouched memory", func(t *testing.T) {
		crude := loadFixture(t, "crude-flag.nie")
		nia := loadFixture(t, "animated-red-blue.nia")

		cases := []struct {
			name string
			src  []byte
		}{
			// The complete 16-byte header plus only four of six pixels: the
			// magic is recognized but the frame payload runs out mid-frame.
			{"truncated still payload", crude[:32]},
			// Valid magic, invalid configuration byte ('n' replaced by 'x'):
			// the header required to reach frame zero is rejected.
			{"invalid configuration byte", replaceBytes(crude, 6, 0x78)},
			// Valid magic and configuration; the width has its high bit set,
			// which the NIE spec forbids.
			{"width high bit set", replaceBytes(crude, 11, 0x80)},
			// The outer nïA header is well-formed but the inner NIE header's
			// configuration byte ('n' replaced by 'p') no longer matches the
			// outer container's bn4 config: frame zero never begins.
			{"nïA inner header config mismatch", replaceBytes(nia, 30, 0x70)},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, 64*48*4, len(tc.src)); err != nil {
					t.Fatalf("RequiredReserve: %v", err)
				}
				guestBefore := wuffs.CaptureGuestMemoryState(d)

				dst := image.NewRGBA(image.Rect(0, 0, 64, 48))
				for i := range dst.Pix {
					dst.Pix[i] = 0xA5
				}
				prefill := append([]byte(nil), dst.Pix...)
				dstBefore := captureRGBAState(dst)

				decMeta, err := d.DecodeRGBA(dst, tc.src)
				assertGuestMemoryPreserved(t, "DecodeRGBA", tc.name, d, guestBefore)
				assertDestinationPreserved(t, "DecodeRGBA", tc.name, dst, dstBefore)
				if err == nil {
					t.Fatal("DecodeRGBA(malformed NIE) error = nil, want ErrDecode")
				}
				if !errors.Is(err, wuffs.ErrDecode) {
					t.Errorf("DecodeRGBA(malformed NIE) error = %v, want errors.Is(err, ErrDecode)", err)
				}
				if errors.Is(err, wuffs.ErrUnknownFormat) {
					t.Errorf("DecodeRGBA(malformed NIE) error = %v, must not be ErrUnknownFormat", err)
				}
				if decMeta != nil {
					t.Errorf("DecodeRGBA(malformed NIE) returned non-nil Meta %+v, want nil", decMeta)
				}
				if !bytes.Equal(dst.Pix, prefill) {
					t.Error("DecodeRGBA(malformed NIE) mutated the sentinel destination pixels")
				}
			})
		}
	})

	t.Run("near-signature mutations stay ErrUnknownFormat", func(t *testing.T) {
		crude := loadFixture(t, "crude-flag.nie")

		cases := []struct {
			name string
			src  []byte
		}{
			// The byte-reversed "Aein" sequence is the exact four bytes the
			// retired pre-format-03 sniff matched via read_u32le(src) ==
			// 0x6E696541. The real NIE magic reads as 0x45AFC36E ("nïE") and
			// 0x41AFC36E ("nïA"), so the byte-reversed form must not be NIE.
			{"byte-reversed Aein sequence", append(append([]byte(nil), nieReversedMagic...), crude[4:]...)},
			// First magic byte 0x6E mutated to 0x6F.
			{"nïE magic byte 0 mutated", replaceBytes(crude, 0, 0x6F)},
			// Second magic byte 0xC3 mutated to 0xB0.
			{"nïE magic byte 1 mutated", replaceBytes(crude, 1, 0xB0)},
			// Fourth magic byte 0x45 mutated to 0x46.
			{"nïE magic byte 3 mutated", replaceBytes(crude, 3, 0x46)},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				for _, op := range []string{"Probe", "DecodeRGBA"} {
					assertRejectedPerOp(t, op, tc.name, tc.src)
				}
			})
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
			{"bricks-color.tga", uint32(wuffs.FormatTGA)},
			{"muybridge-frame-000.wbmp", uint32(wuffs.FormatWBMP)},
			{"bricks-color.etc2.pkm", uint32(wuffs.FormatETC2)},
		}, nieFourCC, "NIE", "NIE")
	})

	t.Run("repeated DecodeRGBA allocates zero heap objects per manifest fixture", func(t *testing.T) {
		for _, fc := range fixtures {
			t.Run(fc.file, func(t *testing.T) {
				src := loadFixture(t, fc.file)

				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, fc.width*fc.height*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve(%s): %v", fc.file, err)
				}

				dst := image.NewRGBA(image.Rect(0, 0, fc.width, fc.height))
				assertZeroAllocsPerRun(t, d, dst, src, fc.file)
			})
		}
	})
}

// assertNIEProbeMeta probes src and asserts the complete FormatNIE metadata
// contract: no error, non-nil Meta, exact FourCC, dimensions, stride
// (wantW*4), BytesWritten zero from Probe, and complete guest-memory
// preservation across the reserved Probe call.
func assertNIEProbeMeta(t *testing.T, d *wuffs.Decoder, name string, src []byte, wantW, wantH int) {
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
	if meta.Format != nieFourCC {
		t.Errorf("Probe(%s) Format = 0x%08X, want FormatNIE 0x%08X", name, meta.Format, nieFourCC)
	}
	if meta.Width != uint32(wantW) || meta.Height != uint32(wantH) {
		t.Errorf("Probe(%s) geometry = (%d,%d), want (%d,%d)", name, meta.Width, meta.Height, wantW, wantH)
	}
	if meta.Stride != uint32(wantW*4) {
		t.Errorf("Probe(%s) Stride = %d, want %d (Width*4)", name, meta.Stride, wantW*4)
	}
	if meta.BytesWritten != 0 {
		t.Errorf("Probe(%s) BytesWritten = %d, want 0", name, meta.BytesWritten)
	}
}

// assertNIEDecodeRGBA decodes src into d's reserved destination and asserts
// the FormatNIE decode contract shared by every positive fixture: no error, a
// non-nil Meta carrying FormatNIE, exact dimensions, stride (wantW*4),
// BytesWritten equal to wantW*wantH*4, an unchanged caller-owned destination
// layout, and an unchanged complete guest memory state (wasm backing pointer,
// byte length, and every slot field).
func assertNIEDecodeRGBA(t *testing.T, d *wuffs.Decoder, name string, dst *image.RGBA, src []byte, wantW, wantH uint32) {
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
	if decMeta.Format != nieFourCC {
		t.Errorf("DecodeRGBA(%s) Format = 0x%08X, want FormatNIE 0x%08X", name, decMeta.Format, nieFourCC)
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

// nieFixture mirrors one manifest line of testdata/nie.golden.manifest: a
// vendored upstream NIE/NIA fixture plus its canonical frame-zero decode
// evidence.
type nieFixture struct {
	file     string
	width    int
	height   int
	encoding string
	crc      uint32 // CRC-32 (IEEE) of the frame-zero straight RGBA pixels
}

// loadNIEManifest parses testdata/nie.golden.manifest with an explicit
// scanner and strconv (never regular expressions) into []nieFixture. It
// selects exactly the fixtures the manifest names; blank lines and '#'
// lines are ignored.
func loadNIEManifest(t *testing.T) []nieFixture {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "nie.golden.manifest"))
	if err != nil {
		t.Fatalf("opening testdata/nie.golden.manifest: %v", err)
	}
	defer f.Close()

	var fixtures []nieFixture
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 5 {
			t.Fatalf("testdata/nie.golden.manifest: malformed line %q", line)
		}
		w, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatalf("testdata/nie.golden.manifest: width of %q: %v", fields[0], err)
		}
		h, err := strconv.Atoi(fields[2])
		if err != nil {
			t.Fatalf("testdata/nie.golden.manifest: height of %q: %v", fields[0], err)
		}
		crc64, err := strconv.ParseUint(fields[4], 10, 32)
		if err != nil {
			t.Fatalf("testdata/nie.golden.manifest: crc32 of %q: %v", fields[0], err)
		}
		fixtures = append(fixtures, nieFixture{
			file:     fields[0],
			width:    w,
			height:   h,
			encoding: fields[3],
			crc:      uint32(crc64),
		})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading testdata/nie.golden.manifest: %v", err)
	}
	return fixtures
}

// nieFrameZeroStraightRGBA derives frame-zero straight RGBA bytes from a
// complete NIE still (nïE) or NIA container (nïA) independently of the wasm
// decoder, per ../wuffs/doc/spec/nie-spec.md:
//
//  1. the 16-byte header: magic, version-and-configuration {0xFF,'b','n' or
//     'p','4' or '8'}, little-endian width and height; the config byte 3
//     selects 4 or 8 bytes per pixel;
//  2. for a nïA container, frame zero starts after the 8-byte cumulative
//     display duration and the inner NIE header, whose config and dimensions
//     must equal the outer header's;
//  3. each wire pixel is converted into the guest's BGRA_PREMUL destination
//     bytes with the exact Wuffs v0.4 swizzler arithmetic
//     (wuffs-mirror-release-c/release/c/wuffs-v0.4.c): bn4 uses
//     color_u32_argb_nonpremul__as__color_u32_argb_premul (a*0x101*0x101,
//     ((c*a16)/0xFFFF)>>8), bn8 uses
//     color_u64_argb_nonpremul__as__color_u32_argb_premul
//     ((c16*a16)/0xFFFF, high byte out), bp4 copies, bp8 takes each 16-bit
//     channel's high byte via color_u64__as__color_u32;
//  4. finally the BGRA-PREMUL slot bytes become straight RGBA in Go image
//     order exactly like the host's documented convertBGRAToStraight: B/R
//     swap, and unpremultiply (c*255)/a for translucent alpha.
//
// The returned slice is width*height*4 bytes and the caller receives the
// derived width and height.
func nieFrameZeroStraightRGBA(t *testing.T, src []byte) ([]byte, int, int) {
	t.Helper()
	if len(src) < 16 {
		t.Fatalf("NIE oracle: source is only %d bytes, want a 16-byte header", len(src))
	}
	config := string(src[4:8])
	w := int(binary.LittleEndian.Uint32(src[8:12]))
	h := int(binary.LittleEndian.Uint32(src[12:16]))
	pos := 16
	if bytes.Equal(src[0:4], nieAnimatedMagic) {
		if len(src) < 40 {
			t.Fatalf("NIE oracle: nïA container is only %d bytes, want CDD plus inner header", len(src))
		}
		if !bytes.Equal(src[24:28], nieStillMagic) {
			t.Fatal("NIE oracle: frame zero of the nïA container is not a nïE still")
		}
		if string(src[28:32]) != config {
			t.Fatal("NIE oracle: inner NIE configuration does not match the outer nïA configuration")
		}
		w = int(binary.LittleEndian.Uint32(src[32:36]))
		h = int(binary.LittleEndian.Uint32(src[36:40]))
		pos = 40
	}

	bpp := 8
	if config[3] == '4' {
		bpp = 4
	} else if config[3] != '8' {
		t.Fatalf("NIE oracle: unsupported bytes-per-pixel byte 0x%02X", config[3])
	}
	need := w * h * bpp
	if len(src)-pos < need {
		t.Fatalf("NIE oracle: payload has %d bytes, want %d for %dx%d at %d bytes per pixel", len(src)-pos, need, w, h, bpp)
	}
	px := src[pos : pos+need]

	out := make([]byte, w*h*4)
	oi := 0
	for i := 0; i < len(px); {
		var b, g, r, a uint8
		switch config {
		case "\xffbn4":
			b, g, r, a = premul8(px[i+0], px[i+1], px[i+2], px[i+3])
			i += 4
		case "\xffbn8":
			b16 := binary.LittleEndian.Uint16(px[i+0:])
			g16 := binary.LittleEndian.Uint16(px[i+2:])
			r16 := binary.LittleEndian.Uint16(px[i+4:])
			a16 := binary.LittleEndian.Uint16(px[i+6:])
			b, g, r, a = premul16(b16, g16, r16, a16)
			i += 8
		case "\xffbp4":
			b, g, r, a = px[i+0], px[i+1], px[i+2], px[i+3]
			i += 4
		case "\xffbp8":
			b, g, r, a = px[i+1], px[i+3], px[i+5], px[i+7]
			i += 8
		default:
			t.Fatalf("NIE oracle: unsupported configuration %q", config)
		}
		switch a {
		case 0:
			out[oi+0], out[oi+1], out[oi+2], out[oi+3] = 0, 0, 0, 0
		case 255:
			out[oi+0], out[oi+1], out[oi+2], out[oi+3] = r, g, b, 255
		default:
			out[oi+0], out[oi+1], out[oi+2], out[oi+3] = unpremul8(r, a), unpremul8(g, a), unpremul8(b, a), a
		}
		oi += 4
	}
	return out, w, h
}

// premul8 converts one 8-bit non-premultiplied pixel to premultiplied bytes
// per the Wuffs v0.4 color_u32_argb_nonpremul__as__color_u32_argb_premul
// arithmetic: a16 = a*0x101*0x101 and each channel is ((c*a16)/0xFFFF)>>8.
func premul8(b, g, r, a uint8) (uint8, uint8, uint8, uint8) {
	a16 := uint32(a) * 0x101 * 0x101
	pR := uint8(((uint32(r) * a16) / 0xFFFF) >> 8)
	pG := uint8(((uint32(g) * a16) / 0xFFFF) >> 8)
	pB := uint8(((uint32(b) * a16) / 0xFFFF) >> 8)
	return pB, pG, pR, a
}

// premul16 converts one 16-bit non-premultiplied pixel to premultiplied
// 8-bit bytes per the Wuffs v0.4
// color_u64_argb_nonpremul__as__color_u32_argb_premul arithmetic: each
// channel is ((c16*a16)/0xFFFF)>>8 and alpha is a16>>8.
func premul16(b16, g16, r16, a16 uint16) (uint8, uint8, uint8, uint8) {
	r := uint16((uint32(r16) * uint32(a16)) / 0xFFFF)
	g := uint16((uint32(g16) * uint32(a16)) / 0xFFFF)
	b := uint16((uint32(b16) * uint32(a16)) / 0xFFFF)
	return uint8(b >> 8), uint8(g >> 8), uint8(r >> 8), uint8(a16 >> 8)
}

// unpremul8 converts one premultiplied channel to straight per the host's
// documented convertBGRAToStraight: (c*255)/a.
func unpremul8(c, a uint8) uint8 {
	return uint8((uint16(c) * 255) / uint16(a))
}

// replaceBytes returns a copy of src with src[index] replaced by value.
func replaceBytes(src []byte, index int, value byte) []byte {
	dst := append([]byte(nil), src...)
	dst[index] = value
	return dst
}

// firstMismatch returns the first index where a and b differ, or -1 when the
// slices are equal.
func firstMismatch(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return -1
}
