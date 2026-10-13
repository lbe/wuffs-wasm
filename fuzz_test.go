//go:build fuzz

package wuffs_test

import (
	"bytes"
	"errors"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"github.com/lbe/wuffs-wasm"
)

const fuzzMaxPixels = 32768

var fuzzSeedExt = map[string]bool{
	".png": true, ".webp": true, ".gif": true, ".jpeg": true, ".jpg": true,
	".bmp": true, ".qoi": true, ".tga": true, ".pgm": true, ".ppm": true,
	".wbmp": true, ".pkm": true, ".nie": true, ".nia": true, ".th": true,
	".handsum": true,
}

func isPublicErr(err error) bool {
	return err != nil && (errors.Is(err, wuffs.ErrUnknownFormat) ||
		errors.Is(err, wuffs.ErrSrcTooLarge) ||
		errors.Is(err, wuffs.ErrDecode) ||
		errors.Is(err, wuffs.ErrBadImage) ||
		errors.Is(err, wuffs.ErrDstTooSmall))
}

func loadFuzzSeeds(f *testing.F) [][]byte {
	seeds := [][]byte{
		{},
		{0x00},
		[]byte("XXXX"),
		[]byte("RIFF\x00\x00\x00\x00WEBP"),
	}
	added := 0
	err := filepath.WalkDir("testdata", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case "bench", "fuzz":
				return fs.SkipDir
			default:
				return nil
			}
		}
		if !fuzzSeedExt[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if len(b) == 0 || len(b) > wuffs.ShrunkMaxSrc() {
			return nil
		}
		seeds = append(seeds, b)
		added++
		return nil
	})
	if err != nil {
		f.Fatal(err)
	}
	if added < 10 {
		f.Fatalf("loadFuzzSeeds added %d testdata files, want at least 10", added)
	}
	return seeds
}

func seedFuzzSources(f *testing.F) {
	for _, src := range loadFuzzSeeds(f) {
		f.Add(src)
	}
}

func assertGuestUnchanged(t *testing.T, d *wuffs.Decoder, before wuffs.GuestMemoryState, where string) {
	t.Helper()
	if got := wuffs.CaptureGuestMemoryState(d); got != before {
		t.Fatalf("%s mutated guest memory layout: got %+v want %+v", where, got, before)
	}
}

func FuzzDecodeRGBA(f *testing.F) {
	seedFuzzSources(f)
	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) > wuffs.ShrunkMaxSrc() {
			return
		}
		d := wuffs.New()
		pre := wuffs.CaptureGuestMemoryState(d)
		meta, err := d.Probe(src)
		if err != nil {
			if !isPublicErr(err) {
				t.Fatalf("Probe: unexpected error %v", err)
			}
			assertGuestUnchanged(t, d, pre, "Probe")
			dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
			fuzzDecodeRGBA(t, d, dst, src)
			return
		}
		if meta == nil {
			t.Fatal("Probe returned nil Meta without error")
		}
		assertGuestUnchanged(t, d, pre, "Probe")
		w, h := int(meta.Width), int(meta.Height)
		if w <= 0 || h <= 0 || uint64(w)*uint64(h) > fuzzMaxPixels {
			return
		}
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		fuzzDecodeRGBA(t, d, dst, src)
	})
}

func fuzzDecodeRGBA(t *testing.T, d *wuffs.Decoder, dst *image.RGBA, src []byte) {
	t.Helper()
	pre := wuffs.CaptureGuestMemoryState(d)
	ptr := unsafe.SliceData(dst.Pix)
	ln, cp := len(dst.Pix), cap(dst.Pix)
	rect, stride := dst.Rect, dst.Stride
	pixCopy := append([]byte(nil), dst.Pix...)

	_, err := d.DecodeRGBA(dst, src)
	if err != nil && !isPublicErr(err) {
		t.Fatalf("DecodeRGBA: unexpected error %v", err)
	}
	if unsafe.SliceData(dst.Pix) != ptr || len(dst.Pix) != ln || cap(dst.Pix) != cp || dst.Rect != rect || dst.Stride != stride {
		t.Fatal("DecodeRGBA mutated dst Pix/Rect/Stride header")
	}
	if err != nil && !bytes.Equal(dst.Pix, pixCopy) {
		t.Fatal("failed DecodeRGBA mutated dst.Pix bytes")
	}
	assertGuestUnchanged(t, d, pre, "DecodeRGBA")
}

func FuzzDecodeGray(f *testing.F) {
	seedFuzzSources(f)
	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) > wuffs.ShrunkMaxSrc() {
			return
		}
		d := wuffs.New()
		pre := wuffs.CaptureGuestMemoryState(d)
		meta, err := d.Probe(src)
		if err != nil {
			if !isPublicErr(err) {
				t.Fatalf("Probe: unexpected error %v", err)
			}
			assertGuestUnchanged(t, d, pre, "Probe")
			dst := image.NewGray(image.Rect(0, 0, 1, 1))
			fuzzDecodeGray(t, d, dst, src)
			return
		}
		if meta == nil {
			t.Fatal("Probe returned nil Meta without error")
		}
		assertGuestUnchanged(t, d, pre, "Probe")
		w, h := int(meta.Width), int(meta.Height)
		if w <= 0 || h <= 0 || uint64(w)*uint64(h) > fuzzMaxPixels {
			return
		}
		dst := image.NewGray(image.Rect(0, 0, w, h))
		fuzzDecodeGray(t, d, dst, src)
	})
}

func seedFuzzSourcesIndexed(f *testing.F) {
	for _, src := range loadFuzzSeeds(f) {
		f.Add(src, 0)
	}
	f.Add([]byte("XXXX"), -1)
	f.Add([]byte("XXXX"), 1<<30)
}

func FuzzDecodeFrame(f *testing.F) {
	seedFuzzSourcesIndexed(f)
	f.Fuzz(func(t *testing.T, src []byte, index int) {
		if len(src) > wuffs.ShrunkMaxSrc() {
			return
		}
		d := wuffs.New()
		pre := wuffs.CaptureGuestMemoryState(d)
		meta, err := d.Probe(src)
		if err != nil {
			if !isPublicErr(err) {
				t.Fatalf("Probe: unexpected error %v", err)
			}
			assertGuestUnchanged(t, d, pre, "Probe")
			dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
			fuzzDecodeFrame(t, d, dst, src, index)
			return
		}
		if meta == nil {
			t.Fatal("Probe returned nil Meta without error")
		}
		assertGuestUnchanged(t, d, pre, "Probe")
		w, h := int(meta.Width), int(meta.Height)
		if w <= 0 || h <= 0 || uint64(w)*uint64(h) > fuzzMaxPixels {
			return
		}
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		fuzzDecodeFrame(t, d, dst, src, index)
	})
}

func fuzzDecodeGray(t *testing.T, d *wuffs.Decoder, dst *image.Gray, src []byte) {
	t.Helper()
	pre := wuffs.CaptureGuestMemoryState(d)
	ptr := unsafe.SliceData(dst.Pix)
	ln, cp := len(dst.Pix), cap(dst.Pix)
	rect, stride := dst.Rect, dst.Stride
	pixCopy := append([]byte(nil), dst.Pix...)

	_, err := d.DecodeGray(dst, src)
	if err != nil && !isPublicErr(err) {
		t.Fatalf("DecodeGray: unexpected error %v", err)
	}
	if unsafe.SliceData(dst.Pix) != ptr || len(dst.Pix) != ln || cap(dst.Pix) != cp || dst.Rect != rect || dst.Stride != stride {
		t.Fatal("DecodeGray mutated dst Pix/Rect/Stride header")
	}
	if err != nil && !bytes.Equal(dst.Pix, pixCopy) {
		t.Fatal("failed DecodeGray mutated dst.Pix bytes")
	}
	assertGuestUnchanged(t, d, pre, "DecodeGray")
}

func fuzzDecodeFrame(t *testing.T, d *wuffs.Decoder, dst *image.RGBA, src []byte, index int) {
	t.Helper()
	pre := wuffs.CaptureGuestMemoryState(d)
	ptr := unsafe.SliceData(dst.Pix)
	ln, cp := len(dst.Pix), cap(dst.Pix)
	rect, stride := dst.Rect, dst.Stride
	pixCopy := append([]byte(nil), dst.Pix...)

	_, err := d.DecodeFrame(dst, src, index)
	if err != nil && !isPublicErr(err) {
		t.Fatalf("DecodeFrame: unexpected error %v", err)
	}
	if unsafe.SliceData(dst.Pix) != ptr || len(dst.Pix) != ln || cap(dst.Pix) != cp || dst.Rect != rect || dst.Stride != stride {
		t.Fatal("DecodeFrame mutated dst Pix/Rect/Stride header")
	}
	if err != nil && !bytes.Equal(dst.Pix, pixCopy) {
		t.Fatal("failed DecodeFrame mutated dst.Pix bytes")
	}
	assertGuestUnchanged(t, d, pre, "DecodeFrame")
}

func FuzzMetadata(f *testing.F) {
	seedFuzzSources(f)
	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) > wuffs.ShrunkMaxSrc() {
			return
		}
		d := wuffs.New()
		pre := wuffs.CaptureGuestMemoryState(d)
		_, err := d.Metadata(src)
		if err != nil && !isPublicErr(err) {
			t.Fatalf("Metadata: unexpected error %v", err)
		}
		assertGuestUnchanged(t, d, pre, "Metadata")
	})
}
