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

// etc2FourCC is the "ETC2" FourCC literal local to this test package.
const etc2FourCC = uint32(0x45544332) // "ETC2"

// TestIntegrationETC2DecodeCharacterization verifies the ETC2 (PKM) format
// contract end-to-end against the live wasm guest. The fixtures and their
// canonical evidence come from testdata/etc2.golden.manifest, which records
// each vendored upstream PKM fixture's reported dimensions, its PKM variant,
// and the CRC-32 (IEEE) of its decoded straight RGBA pixels as produced by
// the upstream Wuffs decoder (the same wuffs-v0.4.c embedded in the guest
// wasm, using the guest's own BGRA_PREMUL/blend-SRC decode parameters):
//
//   - Every manifest fixture probes as FormatETC2 with exact dimensions,
//     stride = Width*4, and BytesWritten 0.
//   - Every manifest fixture (ETC1, ETC1S, ETC2 RGB, 21×32
//     non-multiple-of-four geometry, one-bit alpha, and full
//     non-premultiplied alpha) decodes to the manifest's canonical CRC-32,
//     with the caller-owned destination layout preserved.
//   - A corrupt PKM with an intact 16-byte header still probes as 160×120
//     FormatETC2 but DecodeRGBA fails with errors.Is(err, ErrDecode) (never
//     ErrUnknownFormat), returns nil Meta, and leaves a prefilled sentinel
//     destination byte-for-byte untouched.
//   - Inputs lacking the complete literal "PKM " prefix (PK, PKM, PKMx)
//     return ErrUnknownFormat, because no other established recognizer owns
//     them.
//   - One canonical fixture per established recognizer keeps its own
//     classification: the PKM sniff must not shadow any existing format.
//   - After explicit Reserve and one warm-up DecodeRGBA per fixture,
//     repeated DecodeRGBA performs exactly zero Go heap allocations.
//
// The ETC2 format is asserted via the local uint32 FourCC literal rather than
// the exported constant so this runtime characterization stays independent of
// the exported name, matching the FORMAT-02 plan convention.
func TestIntegrationETC2DecodeCharacterization(t *testing.T) {
	fixtures := loadETC2Manifest(t)
	if len(fixtures) == 0 {
		t.Fatal("testdata/etc2.golden.manifest selected no fixtures")
	}

	t.Run("every manifest fixture probes as FormatETC2 with exact geometry", func(t *testing.T) {
		for _, fc := range fixtures {
			t.Run(fc.file, func(t *testing.T) {
				src := loadFixture(t, fc.file)

				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, fc.width*fc.height*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve(%s): %v", fc.file, err)
				}
				assertETC2ProbeMeta(t, d, fc.file, src, fc.width, fc.height)
			})
		}
	})

	t.Run("every manifest fixture decodes to its canonical upstream CRC", func(t *testing.T) {
		for _, fc := range fixtures {
			t.Run(fc.file, func(t *testing.T) {
				src := loadFixture(t, fc.file)

				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, fc.width*fc.height*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve(%s): %v", fc.file, err)
				}
				dst := image.NewRGBA(image.Rect(0, 0, fc.width, fc.height))
				assertETC2DecodeRGBA(t, d, fc.file, dst, src, uint32(fc.width), uint32(fc.height))

				got := crc32.ChecksumIEEE(dst.Pix)
				if got != fc.crc {
					t.Errorf("DecodeRGBA(%s) pixel CRC-32 = 0x%08X (%d), want canonical 0x%08X (%d) from the upstream Wuffs decoder",
						fc.file, got, got, fc.crc, fc.crc)
				}
			})
		}
	})

	t.Run("corrupt PKM with intact header fails DecodeRGBA with ErrDecode", func(t *testing.T) {
		// The complete 16-byte PKM header ("PKM " magic, version, format, the
		// four dimension fields) followed by a single 8-byte ETC2 block: the
		// header parses so the format is recognized, but the pixel payload
		// runs out mid-frame and decode must fail with ErrDecode.
		src := loadFixture(t, "bricks-color.etc2.pkm")
		corrupt := src[:24]

		d := wuffs.New()
		if err := wuffs.RequiredReserve(d, 160*120*4, len(corrupt)); err != nil {
			t.Fatalf("RequiredReserve: %v", err)
		}
		assertETC2ProbeMeta(t, d, "corrupt PKM", corrupt, 160, 120)

		guestBefore := wuffs.CaptureGuestMemoryState(d)

		dst := image.NewRGBA(image.Rect(0, 0, 160, 120))
		for i := range dst.Pix {
			dst.Pix[i] = 0xA5
		}
		prefill := append([]byte(nil), dst.Pix...)
		dstBefore := captureRGBAState(dst)

		decMeta, err := d.DecodeRGBA(dst, corrupt)
		assertGuestMemoryPreserved(t, "DecodeRGBA", "corrupt PKM", d, guestBefore)
		assertDestinationPreserved(t, "DecodeRGBA", "corrupt PKM", dst, dstBefore)
		if err == nil {
			t.Fatal("DecodeRGBA(corrupt PKM) error = nil, want ErrDecode")
		}
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Errorf("DecodeRGBA(corrupt PKM) error = %v, want errors.Is(err, ErrDecode)", err)
		}
		if errors.Is(err, wuffs.ErrUnknownFormat) {
			t.Errorf("DecodeRGBA(corrupt PKM) error = %v, must not be ErrUnknownFormat", err)
		}
		if decMeta != nil {
			t.Errorf("DecodeRGBA(corrupt PKM) returned non-nil Meta %+v, want nil", decMeta)
		}
		if !bytes.Equal(dst.Pix, prefill) {
			t.Error("DecodeRGBA(corrupt PKM) mutated the sentinel destination pixels")
		}
	})

	t.Run("inputs lacking the complete literal PKM prefix stay ErrUnknownFormat", func(t *testing.T) {
		full := loadFixture(t, "bricks-color.etc1.pkm")

		cases := []struct {
			name string
			src  []byte
		}{
			{"two bytes PK", full[:2]},
			{"three bytes PKM without the trailing space", full[:3]},
			{"four bytes PKMx in place of PKM space", append(append([]byte(nil), full[:3]...), 'X')},
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
		}, etc2FourCC, "ETC2", "PKM")
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

// assertETC2ProbeMeta probes src and asserts the complete FormatETC2 metadata
// contract: no error, non-nil Meta, exact FourCC, dimensions, stride
// (wantW*4), BytesWritten zero from Probe, and complete guest-memory
// preservation across the reserved Probe call.
func assertETC2ProbeMeta(t *testing.T, d *wuffs.Decoder, name string, src []byte, wantW, wantH int) {
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
	if meta.Format != etc2FourCC {
		t.Errorf("Probe(%s) Format = 0x%08X, want FormatETC2 0x%08X", name, meta.Format, etc2FourCC)
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

// assertETC2DecodeRGBA decodes src into d's reserved destination and asserts
// the FormatETC2 decode contract shared by every positive fixture: no error, a
// non-nil Meta carrying FormatETC2, exact dimensions, stride (wantW*4),
// BytesWritten equal to wantW*wantH*4, an unchanged caller-owned destination
// layout, and an unchanged complete guest memory state (wasm backing pointer,
// byte length, and every slot field).
func assertETC2DecodeRGBA(t *testing.T, d *wuffs.Decoder, name string, dst *image.RGBA, src []byte, wantW, wantH uint32) {
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
	if decMeta.Format != etc2FourCC {
		t.Errorf("DecodeRGBA(%s) Format = 0x%08X, want FormatETC2 0x%08X", name, decMeta.Format, etc2FourCC)
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

// etc2Fixture mirrors one manifest line of testdata/etc2.golden.manifest: a
// vendored upstream PKM fixture plus its canonical decode evidence.
type etc2Fixture struct {
	file   string
	width  int
	height int
	crc    uint32 // CRC-32 (IEEE) of the decoded straight RGBA pixels
}

// loadETC2Manifest parses testdata/etc2.golden.manifest with an explicit
// scanner and strconv (never regular expressions) into []etc2Fixture. It
// selects exactly the fixtures the manifest names; blank lines and '#' lines
// are ignored.
func loadETC2Manifest(t *testing.T) []etc2Fixture {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "etc2.golden.manifest"))
	if err != nil {
		t.Fatalf("opening testdata/etc2.golden.manifest: %v", err)
	}
	defer f.Close()

	var fixtures []etc2Fixture
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 5 {
			t.Fatalf("testdata/etc2.golden.manifest: malformed line %q", line)
		}
		w, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatalf("testdata/etc2.golden.manifest: width of %q: %v", fields[0], err)
		}
		h, err := strconv.Atoi(fields[2])
		if err != nil {
			t.Fatalf("testdata/etc2.golden.manifest: height of %q: %v", fields[0], err)
		}
		crc64, err := strconv.ParseUint(fields[3], 10, 32)
		if err != nil {
			t.Fatalf("testdata/etc2.golden.manifest: crc32 of %q: %v", fields[0], err)
		}
		fixtures = append(fixtures, etc2Fixture{
			file:   fields[0],
			width:  w,
			height: h,
			crc:    uint32(crc64),
		})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading testdata/etc2.golden.manifest: %v", err)
	}
	return fixtures
}
