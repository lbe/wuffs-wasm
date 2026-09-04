package wuffs_test

import (
	"bytes"
	"errors"
	"image"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// TestIntegrationNPBMDecodeCharacterization verifies the Netpbm (NPBM) format
// contract end-to-end against the live wasm guest:
//
//   - The vendored hippopotamus fixtures (P5/P6, maxval 255, 36×28) probe and
//     decode as FormatNPBM with the expected geometry and exact first pixels,
//     exercising the 1-byte-per-sample path at both magic numbers.
//   - The in-repository maxval 65535 fixtures (P5/P6, 2×2, two-byte big-endian
//     samples) decode to identical RGBA bytes, proving that P5 and P6 with
//     equivalent sample values decode identically. The 16→8-bit conversion
//     takes the high byte of each sample.
//   - Literal minimal P1, P2, P3, and P4 inputs and syntactically valid P5/P6
//     inputs at the unsupported maxvals 1, 254, 256, and 65534 return
//     ErrDecode; a literal minimal P7 input returns ErrUnknownFormat because
//     the guest sniffer recognizes only P1-P6 as Netpbm. In every error case
//     both Probe and DecodeRGBA return a nil Meta, never FormatNPBM, and a
//     sentinel destination is left byte-for-byte untouched.
//
// The guest decoder (wasm/shim.c, wuffs_netpbm) supports only P5 and P6 with
// maxval exactly 255 or 65535; P1-P4, P7, and other maxvals are rejected at
// decode_image_config before any pixel is produced.
func TestIntegrationNPBMDecodeCharacterization(t *testing.T) {
	t.Run("vendored maxval 255 P5 and P6 decode as FormatNPBM", func(t *testing.T) {
		fixtures := []struct {
			name string
			file string
		}{
			{"P5", "hippopotamus.pgm"},
			{"P6", "hippopotamus.ppm"},
		}
		for _, fc := range fixtures {
			t.Run(fc.name, func(t *testing.T) {
				src := loadFixture(t, fc.file)

				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, 36*28*4, len(src)); err != nil {
					t.Fatalf("RequiredReserve: %v", err)
				}
				guestBefore := wuffs.CaptureGuestMemoryState(d)

				meta, err := d.Probe(src)
				if err != nil {
					t.Fatalf("Probe: %v", err)
				}
				assertGuestMemoryPreserved(t, "Probe", fc.file, d, guestBefore)
				if meta == nil {
					t.Fatal("Probe returned nil Meta")
				}
				if meta.Format != wuffs.FormatNPBM {
					t.Errorf("Probe Format = 0x%08X, want FormatNPBM 0x%08X", meta.Format, uint32(wuffs.FormatNPBM))
				}
				if meta.Width != 36 || meta.Height != 28 || meta.Stride != 144 {
					t.Errorf("Probe geometry = (%d,%d) stride %d, want (36,28) stride 144", meta.Width, meta.Height, meta.Stride)
				}
				if meta.BytesWritten != 0 {
					t.Errorf("Probe BytesWritten = %d, want 0", meta.BytesWritten)
				}

				dst := image.NewRGBA(image.Rect(0, 0, 36, 28))
				dstBefore := captureRGBAState(dst)
				decMeta, err := d.DecodeRGBA(dst, src)
				if err != nil {
					t.Fatalf("DecodeRGBA: %v", err)
				}
				assertGuestMemoryPreserved(t, "DecodeRGBA", fc.file, d, guestBefore)
				assertDestinationPreserved(t, "DecodeRGBA", fc.file, dst, dstBefore)
				if decMeta == nil {
					t.Fatal("DecodeRGBA returned nil Meta")
				}
				if decMeta.Format != wuffs.FormatNPBM {
					t.Errorf("DecodeRGBA Format = 0x%08X, want FormatNPBM 0x%08X", decMeta.Format, uint32(wuffs.FormatNPBM))
				}
				if decMeta.Width != 36 || decMeta.Height != 28 || decMeta.Stride != 144 {
					t.Errorf("DecodeRGBA geometry = (%d,%d) stride %d, want (36,28) stride 144", decMeta.Width, decMeta.Height, decMeta.Stride)
				}
				if decMeta.BytesWritten != 36*28*4 {
					t.Errorf("DecodeRGBA BytesWritten = %d, want %d", decMeta.BytesWritten, 36*28*4)
				}

				// First four 8-bit samples of the fixture are 'r','t','u','w'
				// (0x72 0x74 0x75 0x77); P5 maps each gray sample to
				// (g,g,g,255) and P6 maps each RGB triple, so the leading
				// pixels are deterministic.
				wantPrefix := []byte{
					0x72, 0x72, 0x72, 0xFF,
					0x74, 0x74, 0x74, 0xFF,
					0x75, 0x75, 0x75, 0xFF,
					0x77, 0x77, 0x77, 0xFF,
				}
				if got := dst.Pix[:16]; !bytes.Equal(got, wantPrefix) {
					t.Errorf("first four pixels = % X, want % X", got, wantPrefix)
				}
			})
		}
	})

	t.Run("maxval 65535 P5 and P6 with equivalent samples decode identically", func(t *testing.T) {
		// Both fixtures hold 2×2 pixels; the P6 triples repeat the P5 gray
		// sample (R=G=B), so the two files are sample-equivalent by
		// construction. The two-byte big-endian samples are, in row-major
		// order: 0x0000, 0x0100, 0x8000, 0xFFFF.
		pgmSrc := loadFixture(t, "npbm-maxval65535.pgm")
		ppmSrc := loadFixture(t, "npbm-maxval65535.ppm")

		wantPixels := []byte{
			0x00, 0x00, 0x00, 0xFF, // 0x0000 → 0x00 (high byte)
			0x01, 0x01, 0x01, 0xFF, // 0x0100 → 0x01 (high byte)
			0x80, 0x80, 0x80, 0xFF, // 0x8000 → 0x80 (high byte)
			0xFF, 0xFF, 0xFF, 0xFF, // 0xFFFF → 0xFF (high byte)
		}

		decodePixels := func(name string, src []byte) []byte {
			d := wuffs.New()
			if err := wuffs.RequiredReserve(d, 2*2*4, len(src)); err != nil {
				t.Fatalf("%s RequiredReserve: %v", name, err)
			}
			guestBefore := wuffs.CaptureGuestMemoryState(d)
			meta, err := d.Probe(src)
			if err != nil {
				t.Fatalf("%s Probe: %v", name, err)
			}
			assertGuestMemoryPreserved(t, "Probe", name, d, guestBefore)
			if meta == nil || meta.Format != wuffs.FormatNPBM {
				t.Fatalf("%s Probe = %+v, want FormatNPBM", name, meta)
			}
			if meta.Width != 2 || meta.Height != 2 || meta.Stride != 8 {
				t.Fatalf("%s Probe geometry = (%d,%d) stride %d, want (2,2) stride 8", name, meta.Width, meta.Height, meta.Stride)
			}
			dst := image.NewRGBA(image.Rect(0, 0, 2, 2))
			dstBefore := captureRGBAState(dst)
			if _, err := d.DecodeRGBA(dst, src); err != nil {
				t.Fatalf("%s DecodeRGBA: %v", name, err)
			}
			assertGuestMemoryPreserved(t, "DecodeRGBA", name, d, guestBefore)
			assertDestinationPreserved(t, "DecodeRGBA", name, dst, dstBefore)
			return dst.Pix
		}

		pgmPix := decodePixels("P5", pgmSrc)
		ppmPix := decodePixels("P6", ppmSrc)
		if !bytes.Equal(pgmPix, ppmPix) {
			t.Errorf("P5 pixels % X differ from P6 pixels % X, want identical", pgmPix, ppmPix)
		}
		if !bytes.Equal(pgmPix, wantPixels) {
			t.Errorf("P5 pixels = % X, want % X", pgmPix, wantPixels)
		}
	})

	t.Run("unsupported inputs fail without NPBM metadata or pixel writes", func(t *testing.T) {
		unsupported := []struct {
			name string
			src  []byte
			want error
		}{
			// Minimal literal P1-P4 inputs. The guest netpbm decoder
			// implements only P5 (binary gray) and P6 (binary RGB).
			{"P1 bitmap ASCII", []byte("P1\n1 1\n0\n"), wuffs.ErrDecode},
			{"P2 graymap ASCII", []byte("P2\n1 1\n255\n0\n"), wuffs.ErrDecode},
			{"P3 pixmap ASCII", []byte("P3\n1 1\n255\n0 0 0\n"), wuffs.ErrDecode},
			{"P4 bitmap binary", []byte("P4\n1 1\n\x00"), wuffs.ErrDecode},

			// Minimal literal P7 (PAM) input. The host sniffer recognizes
			// only P1-P6 as Netpbm, so P7 is an unknown format entirely.
			{"P7 PAM", []byte("P7\nWIDTH 1\nHEIGHT 1\nDEPTH 1\nMAXVAL 255\nTUPLTYPE GRAYSCALE\nENDHDR\n\x00"), wuffs.ErrUnknownFormat},

			// Syntactically valid P5/P6 inputs at maxvals the guest rejects.
			// Maxval < 256 uses single-byte samples; 256/65534 use two-byte
			// big-endian samples.
			{"P5 maxval 1", []byte("P5\n1 1\n1\n\x00"), wuffs.ErrDecode},
			{"P5 maxval 254", []byte("P5\n1 1\n254\n\x00"), wuffs.ErrDecode},
			{"P5 maxval 256", []byte("P5\n1 1\n256\n\x00\x00"), wuffs.ErrDecode},
			{"P5 maxval 65534", []byte("P5\n1 1\n65534\n\x00\x00"), wuffs.ErrDecode},
			{"P6 maxval 1", []byte("P6\n1 1\n1\n\x00\x00\x00"), wuffs.ErrDecode},
			{"P6 maxval 254", []byte("P6\n1 1\n254\n\x00\x00\x00"), wuffs.ErrDecode},
			{"P6 maxval 256", []byte("P6\n1 1\n256\n\x00\x00\x00\x00\x00\x00"), wuffs.ErrDecode},
			{"P6 maxval 65534", []byte("P6\n1 1\n65534\n\x00\x00\x00\x00\x00\x00"), wuffs.ErrDecode},
		}

		for _, tc := range unsupported {
			t.Run(tc.name, func(t *testing.T) {
				d := wuffs.New()
				if err := wuffs.RequiredReserve(d, 1*1*4, len(tc.src)); err != nil {
					t.Fatalf("RequiredReserve: %v", err)
				}

				meta, err := d.Probe(tc.src)
				if !errors.Is(err, tc.want) {
					t.Errorf("Probe error = %v, want errors.Is(..., %v)", err, tc.want)
				}
				if meta != nil {
					t.Errorf("Probe returned non-nil Meta %+v, want nil", meta)
				}

				dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
				for i := range dst.Pix {
					dst.Pix[i] = 0xAA
				}
				sentinel := append([]byte(nil), dst.Pix...)

				decMeta, err := d.DecodeRGBA(dst, tc.src)
				if !errors.Is(err, tc.want) {
					t.Errorf("DecodeRGBA error = %v, want errors.Is(..., %v)", err, tc.want)
				}
				if decMeta != nil {
					t.Errorf("DecodeRGBA returned non-nil Meta %+v, want nil", decMeta)
				}
				if !bytes.Equal(dst.Pix, sentinel) {
					t.Errorf("DecodeRGBA mutated destination: got % X, want % X", dst.Pix, sentinel)
				}
			})
		}
	})
}

// TestIntegrationNPBMDecodeRGBAAllocsPerRun measures heap allocations per
// DecodeRGBA call for reusable NPBM decoders. Each input and its correctly
// sized caller-owned destination are prepared outside the measured closure,
// one warm-up decode runs first, and testing.AllocsPerRun then requires
// exactly zero allocations per call for both P5 and P6 at maxval 255
// (hippopotamus, single-byte samples) and maxval 65535 (two-byte big-endian
// samples).
func TestIntegrationNPBMDecodeRGBAAllocsPerRun(t *testing.T) {
	fixtures := []struct {
		name         string
		file         string
		wantW, wantH int
	}{
		{"P5 maxval 255", "hippopotamus.pgm", 36, 28},
		{"P6 maxval 255", "hippopotamus.ppm", 36, 28},
		{"P5 maxval 65535", "npbm-maxval65535.pgm", 2, 2},
		{"P6 maxval 65535", "npbm-maxval65535.ppm", 2, 2},
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
