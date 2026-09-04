package wuffs_test

import (
	"bufio"
	"bytes"
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

// thFourCC is the "TH  " FourCC literal local to this test package.
const thFourCC = uint32(0x54482020)

// thMagic is the three-byte cooked ThumbHash magic identifier "\xC3\xBE\xFE":
// "\xC3\xBE" is the UTF-8 encoding of 'þ' (U+00FE LATIN SMALL LETTER THORN)
// and "\xFE" is the ISO-8859-1 encoding of 'þ'. Wuffs' decode_thumbhash.wuffs
// requires this prefix before the raw payload unless
// QUIRK_JUST_RAW_THUMBHASH is enabled; the wasm guest leaves that quirk
// disabled, so only cooked files beginning {0xC3, 0xBE, 0xFE} are ThumbHash.
var thMagic = []byte{0xC3, 0xBE, 0xFE}

// TestIntegrationTHDecodeCharacterization verifies the ThumbHash contract
// end-to-end against the live wasm guest. The fixtures and their canonical
// evidence come from testdata/th.golden.manifest, which records each vendored
// upstream cooked ThumbHash fixture's reported dimensions, its variant
// properties (has_alpha, l_count, is_landscape), and the CRC-32 (IEEE) of its
// decoded straight RGBA pixels. Those CRCs were computed by an independent C
// oracle program against the upstream Wuffs v0.4 release C library (the exact
// decoder embedded in the wasm guest) using the guest's decode parameters,
// and validated against the canonical upstream thumbhash C test's final-pixel
// value 0xFF56632E for 3OcRJYB4d3h_iIeHeEh3eIhw-j3A.th.
//
//   - Every manifest fixture probes as FormatTH with exact Wuffs-derived
//     dimensions (always no greater than 32x32), stride = Width*4, and
//     BytesWritten 0. The fixture set spans alpha presence (0 and 1),
//     orientation (portrait, landscape, square), aspect ratio (18/32, 23/32,
//     32/23, 32/32), and encoded coefficient-count (l_count 4 and 5) variants.
//   - Every manifest fixture decodes to its manifest CRC-32 AND the full
//     decode contract asserted by assertTHDecodeRGBA, with the caller-owned
//     destination layout preserved.
//   - Recognizable truncated cooked files (magic plus a short header or a
//     payload that runs out mid-frame) return ErrDecode - never
//     ErrUnknownFormat - with a nil Meta, an untouched sentinel destination,
//     and unchanged complete guest memory.
//   - Raw payloads with the three-byte magic removed stay ErrUnknownFormat:
//     QUIRK_JUST_RAW_THUMBHASH is disabled in the guest.
//   - Near-signature mutations of the three magic bytes, and a reordering of
//     them, stay ErrUnknownFormat.
//   - One canonical fixture per established recognizer keeps its own
//     classification: the TH sniff must not shadow any existing format.
//   - After explicit Reserve and one warm-up DecodeRGBA per fixture,
//     repeated DecodeRGBA performs exactly zero Go heap allocations.
//
// The ThumbHash format is asserted via the local uint32 FourCC literal rather
// than the exported constant so this runtime characterization stays
// independent of the exported name, matching the FORMAT-03 plan convention.
func TestIntegrationTHDecodeCharacterization(t *testing.T) {
	fixtures := loadTHManifest(t)
	if len(fixtures) == 0 {
		t.Fatal("testdata/th.golden.manifest selected no fixtures")
	}

	t.Run("fixture set spans the upstream alpha, orientation, aspect, and coefficient variants", func(t *testing.T) {
		hasAlpha := map[int]bool{}
		landscape := map[int]bool{}
		lCount := map[int]bool{}
		aspect := map[string]bool{}
		for _, fc := range fixtures {
			if fc.width > 32 || fc.height > 32 {
				t.Errorf("manifest geometry (%d,%d) of %s exceeds the 32x32 thumbhash limit", fc.width, fc.height, fc.file)
			}
			hasAlpha[fc.hasAlpha] = true
			landscape[fc.isLandscape] = true
			lCount[fc.lCount] = true
			switch {
			case fc.width > fc.height:
				aspect["landscape"] = true
			case fc.width < fc.height:
				aspect["portrait"] = true
			default:
				aspect["square"] = true
			}
		}
		if !hasAlpha[0] || !hasAlpha[1] {
			t.Errorf("fixture set has_alpha coverage = %v, want both 0 and 1", hasAlpha)
		}
		if !landscape[0] || !landscape[1] {
			t.Errorf("fixture set is_landscape coverage = %v, want both 0 and 1", landscape)
		}
		if !lCount[4] || !lCount[5] {
			t.Errorf("fixture set l_count coverage = %v, want both 4 and 5", lCount)
		}
		if !aspect["landscape"] || !aspect["portrait"] || !aspect["square"] {
			t.Errorf("fixture set aspect coverage = %v, want landscape, portrait, and square", aspect)
		}
	})

	t.Run("every manifest fixture probes as FormatTH with exact geometry", func(t *testing.T) {
		for _, fc := range fixtures {
			t.Run(fc.file, func(t *testing.T) {
				src := loadFixture(t, fc.file)

				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, fc.width*fc.height*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve(%s): %v", fc.file, err)
				}
				guestBefore := wuffs.CaptureGuestMemoryState(d)
				meta, err := d.Probe(src)
				if err != nil {
					t.Fatalf("Probe(%s): %v", fc.file, err)
				}
				assertGuestMemoryPreserved(t, "Probe", fc.file, d, guestBefore)
				if meta == nil {
					t.Fatalf("Probe(%s) returned nil Meta", fc.file)
				}
				if meta.Format != thFourCC {
					t.Errorf("Probe(%s) Format = 0x%08X, want FormatTH 0x%08X", fc.file, meta.Format, thFourCC)
				}
				if meta.Width != uint32(fc.width) || meta.Height != uint32(fc.height) {
					t.Errorf("Probe(%s) geometry = (%d,%d), want (%d,%d)", fc.file, meta.Width, meta.Height, fc.width, fc.height)
				}
				if meta.Stride != uint32(fc.width*4) {
					t.Errorf("Probe(%s) Stride = %d, want %d (Width*4)", fc.file, meta.Stride, fc.width*4)
				}
				if meta.BytesWritten != 0 {
					t.Errorf("Probe(%s) BytesWritten = %d, want 0", fc.file, meta.BytesWritten)
				}
			})
		}
	})

	t.Run("every manifest fixture decodes to its canonical Wuffs CRC", func(t *testing.T) {
		for _, fc := range fixtures {
			t.Run(fc.file, func(t *testing.T) {
				src := loadFixture(t, fc.file)

				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, fc.width*fc.height*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve(%s): %v", fc.file, err)
				}
				dst := image.NewRGBA(image.Rect(0, 0, fc.width, fc.height))
				assertTHDecodeRGBA(t, d, fc.file, dst, src, uint32(fc.width), uint32(fc.height))

				if got := crc32.ChecksumIEEE(dst.Pix); got != fc.crc {
					t.Errorf("DecodeRGBA(%s) pixel CRC-32 = 0x%08X (%d), want canonical 0x%08X (%d)", fc.file, got, got, fc.crc, fc.crc)
				}
			})
		}
	})

	t.Run("truncated cooked files return ErrDecode with untouched memory", func(t *testing.T) {
		mona := loadFixture(t, "mona-lisa.21x32.th")
		alpha := loadFixture(t, "2IqDBQQnxnj0JoLYdM3f8ahpuDeHiHdwZw.th")

		cases := []struct {
			name string
			src  []byte
		}{
			// Magic plus only four of the five opaque-header bytes: the
			// 24-bit then 16-bit header fields run out mid-read.
			{"truncated opaque header", mona[:7]},
			// Magic, complete opaque header, but the payload runs out after
			// eight of the sixteen packed AC-coefficient bytes (22 L, 5 P,
			// and 5 Q nibbles).
			{"truncated opaque payload", mona[:16]},
			// Magic plus the eight non-alpha header bytes; has_alpha=1 needs
			// a ninth header byte (a_dc/a_scale) that is missing.
			{"truncated alpha header", alpha[:8]},
			// Magic, complete alpha header, but the payload runs out mid-AC.
			{"truncated alpha payload", alpha[:20]},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, 32*32*4, len(tc.src)); err != nil {
					t.Fatalf("RequiredReserve: %v", err)
				}
				guestBefore := wuffs.CaptureGuestMemoryState(d)

				dst := image.NewRGBA(image.Rect(0, 0, 32, 32))
				for i := range dst.Pix {
					dst.Pix[i] = 0xA5
				}
				prefill := append([]byte(nil), dst.Pix...)
				dstBefore := captureRGBAState(dst)

				decMeta, err := d.DecodeRGBA(dst, tc.src)
				assertGuestMemoryPreserved(t, "DecodeRGBA", tc.name, d, guestBefore)
				assertDestinationPreserved(t, "DecodeRGBA", tc.name, dst, dstBefore)
				if err == nil {
					t.Fatal("DecodeRGBA(truncated ThumbHash) error = nil, want ErrDecode")
				}
				if !errors.Is(err, wuffs.ErrDecode) {
					t.Errorf("DecodeRGBA(truncated ThumbHash) error = %v, want errors.Is(err, ErrDecode)", err)
				}
				if errors.Is(err, wuffs.ErrUnknownFormat) {
					t.Errorf("DecodeRGBA(truncated ThumbHash) error = %v, must not be ErrUnknownFormat", err)
				}
				if decMeta != nil {
					t.Errorf("DecodeRGBA(truncated ThumbHash) returned non-nil Meta %+v, want nil", decMeta)
				}
				if !bytes.Equal(dst.Pix, prefill) {
					t.Error("DecodeRGBA(truncated ThumbHash) mutated the sentinel destination pixels")
				}
			})
		}
	})

	t.Run("raw payloads without the three-byte magic stay ErrUnknownFormat", func(t *testing.T) {
		for _, fc := range fixtures {
			t.Run(fc.file, func(t *testing.T) {
				src := loadFixture(t, fc.file)
				raw := src[len(thMagic):]
				name := "raw payload of " + fc.file

				for _, op := range []string{"Probe", "DecodeRGBA"} {
					assertRejectedPerOp(t, op, name, raw)
				}
			})
		}
	})

	t.Run("near-signature mutations stay ErrUnknownFormat", func(t *testing.T) {
		mona := loadFixture(t, "mona-lisa.21x32.th")

		cases := []struct {
			name string
			src  []byte
		}{
			// First magic byte 0xC3 (the UTF-8 lead of 'þ') mutated to 0xC4.
			{"magic byte 0 mutated", replaceBytes(mona, 0, 0xC4)},
			// Second magic byte 0xBE (the UTF-8 continuation of 'þ') to 0xBF.
			{"magic byte 1 mutated", replaceBytes(mona, 1, 0xBF)},
			// Third magic byte 0xFE sanitized to 0xFF.
			{"magic byte 2 mutated", replaceBytes(mona, 2, 0xFF)},
			// The same three magic bytes in a different order.
			{"magic bytes reordered", append([]byte{0xBE, 0xFE, 0xC3}, mona[3:]...)},
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
			{"49.bn4.nie", uint32(wuffs.FormatNIE)},
		}, thFourCC, "TH", "TH")
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

// assertTHDecodeRGBA decodes src into d's reserved destination and asserts
// the FormatTH decode contract shared by every positive fixture: no error, a
// non-nil Meta carrying FormatTH, exact dimensions within the 32x32 thumbhash
// limit, stride (wantW*4), BytesWritten equal to wantW*wantH*4, an unchanged
// caller-owned destination layout, and an unchanged complete guest memory
// state (wasm backing pointer, byte length, and every slot field).
func assertTHDecodeRGBA(t *testing.T, d *wuffs.Decoder, name string, dst *image.RGBA, src []byte, wantW, wantH uint32) {
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
	if decMeta.Format != thFourCC {
		t.Errorf("DecodeRGBA(%s) Format = 0x%08X, want FormatTH 0x%08X", name, decMeta.Format, thFourCC)
	}
	if wantW > 32 || wantH > 32 {
		t.Errorf("DecodeRGBA(%s) manifest geometry (%d,%d) exceeds the 32x32 thumbhash limit", name, wantW, wantH)
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

// thFixture mirrors one manifest line of testdata/th.golden.manifest: a
// vendored upstream cooked ThumbHash fixture plus its canonical decode
// evidence and variant properties.
type thFixture struct {
	file        string
	width       int
	height      int
	crc         uint32 // CRC-32 (IEEE) of the decoded straight RGBA pixels
	hasAlpha    int    // 1 when the payload carries 14 A AC coefficients
	lCount      int    // 3..=7, the encoded L AC coefficient count selector
	isLandscape int    // 1 when width and height dimension codes are swapped
}

// loadTHManifest parses testdata/th.golden.manifest with an explicit scanner
// and strconv (never regular expressions) into []thFixture. It selects
// exactly the fixtures the manifest names; blank lines and '#' lines are
// ignored.
func loadTHManifest(t *testing.T) []thFixture {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "th.golden.manifest"))
	if err != nil {
		t.Fatalf("opening testdata/th.golden.manifest: %v", err)
	}
	defer f.Close()

	var fixtures []thFixture
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 7 {
			t.Fatalf("testdata/th.golden.manifest: malformed line %q", line)
		}
		// intField parses fields[i] as a decimal integer, naming the field
		// in the fatal message when the manifest line is malformed.
		intField := func(i int, name string) int {
			v, err := strconv.Atoi(fields[i])
			if err != nil {
				t.Fatalf("testdata/th.golden.manifest: %s of %q: %v", name, fields[0], err)
			}
			return v
		}
		crc64, err := strconv.ParseUint(fields[3], 10, 32)
		if err != nil {
			t.Fatalf("testdata/th.golden.manifest: crc32 of %q: %v", fields[0], err)
		}
		fixtures = append(fixtures, thFixture{
			file:        fields[0],
			width:       intField(1, "width"),
			height:      intField(2, "height"),
			crc:         uint32(crc64),
			hasAlpha:    intField(4, "has_alpha"),
			lCount:      intField(5, "l_count"),
			isLandscape: intField(6, "is_landscape"),
		})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading testdata/th.golden.manifest: %v", err)
	}
	return fixtures
}
