package wuffs

// Task 7 metadata errors, detachment, and decode interaction integration
// tests. Internal package so tests can inspect d.currentLayout and mutate
// guest dst scratch through d.module.Xmemory, matching the wasm-memory
// mutation pattern in decoder_typed_matrix_test.go.

import (
	"bytes"
	"errors"
	"hash/crc32"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// mustReadMetadataFixture reads a testdata fixture or fails the test.
func mustReadMetadataFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading testdata/%s: %v", name, err)
	}
	return data
}

// TestIntegrationMetadataErrors proves Metadata enforces the same src cap
// rules as Probe: empty src returns ErrDecode, oversized src returns
// ErrSrcTooLarge, and an unrecognized header returns ErrUnknownFormat.
func TestIntegrationMetadataErrors(t *testing.T) {
	t.Run("empty src returns ErrDecode", func(t *testing.T) {
		d := New()
		for _, src := range [][]byte{nil, {}} {
			if _, err := d.Metadata(src); !errors.Is(err, ErrDecode) {
				t.Errorf("Metadata(%d bytes) error = %v, want errors.Is(err, ErrDecode)", len(src), err)
			}
		}
	})

	t.Run("oversized src returns ErrSrcTooLarge", func(t *testing.T) {
		d := New()
		src := make([]byte, defaultSrcCap+1)
		if _, err := d.Metadata(src); !errors.Is(err, ErrSrcTooLarge) {
			t.Errorf("Metadata(%d bytes) error = %v, want errors.Is(err, ErrSrcTooLarge)", len(src), err)
		}
	})

	t.Run("garbage header returns ErrUnknownFormat", func(t *testing.T) {
		d := New()
		src := []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01}
		if _, err := d.Metadata(src); !errors.Is(err, ErrUnknownFormat) {
			t.Errorf("Metadata(garbage) error = %v, want errors.Is(err, ErrUnknownFormat)", err)
		}
	})
}

// TestIntegrationMetadataCorruptEXIF proves Metadata returns ErrDecode when
// the eXIf chunk declares a length exceeding the file: flipping only byte
// 0x24 (the LSB of the big-endian eXIf chunk length, 0x0a -> 0xf5) makes the
// declared length exceed the 93-byte file.
func TestIntegrationMetadataCorruptEXIF(t *testing.T) {
	raw := mustReadMetadataFixture(t, filepath.Join("artificial-png", "exif.png"))
	src := append([]byte(nil), raw...)
	src[0x24] ^= 0xFF

	d := New()
	if err := d.Reserve(0, len(src)); err != nil {
		t.Fatalf("Reserve(0, len(src)): %v", err)
	}
	if _, err := d.Metadata(src); !errors.Is(err, ErrDecode) {
		t.Errorf("Metadata(corrupt EXIF) error = %v, want errors.Is(err, ErrDecode)", err)
	}
}

// TestIntegrationMetadataDoesNotAliasWasm proves returned blob slices are
// heap copies, not wasm views: mutating the guest pack region after Metadata
// leaves the returned EXIF unchanged, and a second Metadata call returns
// fresh backing arrays.
func TestIntegrationMetadataDoesNotAliasWasm(t *testing.T) {
	src := mustReadMetadataFixture(t, filepath.Join("artificial-png", "exif.png"))

	d := New()
	if err := d.Reserve(0, len(src)); err != nil {
		t.Fatalf("Reserve(0, len(src)): %v", err)
	}
	md, err := d.Metadata(src)
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if len(md.EXIF) == 0 {
		t.Fatal("Metadata EXIF is empty, want non-nil blob for detachment test")
	}
	savedEXIF := append([]byte(nil), md.EXIF...)

	// Mutate the wasm pack region (header + EXIF blob) at the dst scratch slot.
	lay := d.currentLayout
	memBytes := *d.module.Xmemory().Slice()
	end := lay.dstOff + metadataPackHeaderSize + uint32(len(md.EXIF))
	if uint64(end) > uint64(len(memBytes)) {
		t.Fatalf("pack region end %d exceeds wasm memory len %d", end, len(memBytes))
	}
	for i := lay.dstOff; i < end; i++ {
		memBytes[i] = 0xFF
	}

	// The first returned blob is a heap copy: unchanged by wasm mutation.
	if !bytes.Equal(md.EXIF, savedEXIF) {
		t.Error("md.EXIF changed after wasm pack mutation; returned blob aliases wasm memory")
	}

	// A second Metadata call returns fresh heap copies with equal contents.
	md2, err := d.Metadata(src)
	if err != nil {
		t.Fatalf("second Metadata: %v", err)
	}
	if !bytes.Equal(md2.EXIF, savedEXIF) {
		t.Error("second Metadata EXIF differs from first-call snapshot")
	}
	if len(md2.EXIF) > 0 && len(savedEXIF) > 0 && &md2.EXIF[0] == &savedEXIF[0] {
		t.Error("second Metadata EXIF shares backing array with snapshot; want fresh heap copy")
	}
}

// TestIntegrationMetadataPreservesDecodeState proves Probe then Metadata on
// the same decoder leaves decode state intact: DecodeRGBA still succeeds and
// the pixel CRC32 matches the checked-in golden value.
func TestIntegrationMetadataPreservesDecodeState(t *testing.T) {
	src := mustReadMetadataFixture(t, "bricks-color.png")

	d := New()
	if _, err := d.Probe(src); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if _, err := d.Metadata(src); err != nil {
		t.Fatalf("Metadata: %v", err)
	}

	dst := image.NewRGBA(image.Rect(0, 0, 160, 120))
	if _, err := d.DecodeRGBA(dst, src); err != nil {
		t.Fatalf("DecodeRGBA after Probe+Metadata: %v", err)
	}

	gotCRC := crc32.ChecksumIEEE(dst.Pix)
	raw, err := os.ReadFile(filepath.Join("testdata", "bricks-color.golden.crc32"))
	if err != nil {
		t.Fatalf("reading golden CRC32: %v", err)
	}
	wantCRC, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 32)
	if err != nil {
		t.Fatalf("parsing golden CRC32: %v", err)
	}
	if gotCRC != uint32(wantCRC) {
		t.Errorf("CRC32 of decoded Pix = 0x%08X, want golden 0x%08X", gotCRC, uint32(wantCRC))
	}
}

// TestIntegrationMetadataProbeMetaUnchangedAfterMetadata proves Metadata does
// not mutate Probe state: the Probe-returned Meta fields and d.lastMeta are
// unchanged by a Metadata call on the same src.
func TestIntegrationMetadataProbeMetaUnchangedAfterMetadata(t *testing.T) {
	src := mustReadMetadataFixture(t, "bricks-color.png")

	d := New()
	meta, err := d.Probe(src)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if meta == nil {
		t.Fatal("Probe returned nil Meta")
	}
	captured := *meta

	if _, err := d.Metadata(src); err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if *meta != captured {
		t.Errorf("Probe Meta changed by Metadata: got %+v, want %+v", *meta, captured)
	}
	if d.lastMeta != captured {
		t.Errorf("d.lastMeta changed by Metadata: got %+v, want %+v", d.lastMeta, captured)
	}
}

// TestIntegrationMetadataModTimeAbsent proves HasModTime is false and ModTime
// is zero on fixtures without modification-time metadata.
func TestIntegrationMetadataModTimeAbsent(t *testing.T) {
	for _, name := range []string{"bricks-color.png", "bricks-dither.png"} {
		t.Run(name, func(t *testing.T) {
			src := mustReadMetadataFixture(t, name)
			d := New()
			if err := d.Reserve(0, len(src)); err != nil {
				t.Fatalf("Reserve(0, len(src)): %v", err)
			}
			md, err := d.Metadata(src)
			if err != nil {
				t.Fatalf("Metadata: %v", err)
			}
			if md.HasModTime {
				t.Errorf("HasModTime = true, want false for %s", name)
			}
			if !md.ModTime.IsZero() {
				t.Errorf("ModTime = %v, want zero for %s", md.ModTime, name)
			}
		})
	}
}

// TestIntegrationMetadataDstSlotTooSmall proves Metadata returns ErrDecode
// without auto-Reserve when the guest dst slot is smaller than 64 KiB.
func TestIntegrationMetadataDstSlotTooSmall(t *testing.T) {
	restore := SetInitialDstSlotBytes(1024)
	defer restore()

	d := New()
	src := mustReadMetadataFixture(t, "bricks-color.png")
	if _, err := d.Metadata(src); !errors.Is(err, ErrDecode) {
		t.Errorf("Metadata with 1024-byte dst slot error = %v, want errors.Is(err, ErrDecode)", err)
	}
	if got := CurrentDstSlotLen(d); got != 1024 {
		t.Errorf("CurrentDstSlotLen(d) = %d, want 1024 (Metadata must not auto-Reserve)", got)
	}
}
