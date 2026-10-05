package wuffs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegrationRegisterFormatsWBMP verifies that the registration catalog
// routes the finite checked-in WBMP fixtures whose complete Type 0 and
// canonical-dimension prefix is registered literally through image.Decode
// with the canonical name "wbmp" and *image.RGBA pixels identical to
// wuffs.Decode, while near-prefix mutations and the fixed collision corpus
// stay image.ErrFormat.
//
// image.RegisterFormat's whole-byte literal/'?' matcher cannot recognize the
// full canonical WBMP varint language or reject arbitrary invalid dimensions:
// unlisted WBMP dimensions remain supported through wuffs.Decode and
// DecodeReader, not the image registry. No unconditional, empty, or 00 00
// wildcard fallback magic is installed; general WBMP recognition and payload
// validation stay in the existing Wuffs decode path.
func TestIntegrationRegisterFormatsWBMP(t *testing.T) {
	probe := `package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"os"

	wuffs "github.com/lbe/wuffs-wasm"
)

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "probe: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	wuffs.RegisterFormats()

	fixtures := []struct{ file string }{
		{"testdata/muybridge-frame-000.wbmp"},
		{"testdata/bricks-nodither.wbmp"},
	}
	for _, tc := range fixtures {
		src, err := os.ReadFile(tc.file)
		if err != nil {
			fail("read %s: %v", tc.file, err)
		}
		img, format, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			fail("Decode(%s): %v, want nil error with format %q", tc.file, err, "wbmp")
		}
		if format != "wbmp" {
			fail("Decode(%s) format = %q, want %q", tc.file, format, "wbmp")
		}
		rgba, ok := img.(*image.RGBA)
		if !ok {
			fail("Decode(%s) type = %T, want *image.RGBA", tc.file, img)
		}
		ref, _, err := wuffs.Decode(src)
		if err != nil {
			fail("wuffs.Decode(%s): %v", tc.file, err)
		}
		if !rgba.Bounds().Eq(ref.Bounds()) || rgba.Stride != ref.Stride || !bytes.Equal(rgba.Pix, ref.Pix) {
			fail("Decode(%s) pixels differ from wuffs.Decode output", tc.file)
		}
	}

	read := func(file string) []byte {
		src, err := os.ReadFile(file)
		if err != nil {
			fail("read %s: %v", file, err)
		}
		return src
	}
	mutate := func(src []byte, off int, v byte) []byte {
		out := append([]byte(nil), src...)
		out[off] = v
		return out
	}
	seq := make([]byte, 18)
	for i := range seq {
		seq[i] = byte(i)
	}
	unselected := []struct {
		name string
		src  []byte
	}{
		{"mutated muybridge width", mutate(read("testdata/muybridge-frame-000.wbmp"), 2, 0x1F)},
		{"mutated bricks width tail", mutate(read("testdata/bricks-nodither.wbmp"), 3, 0x21)},
		{"nonzero TypeField", mutate(read("testdata/muybridge-frame-000.wbmp"), 0, 0x01)},
		{"nonzero FixHeaderField", mutate(read("testdata/muybridge-frame-000.wbmp"), 1, 0x01)},
		{"18 zero bytes", make([]byte, 18)},
		{"bytes 0x00 through 0x11", seq},
		{"ELF bytes", []byte{0x7F, 0x45, 0x4C, 0x46}},
		{"empty input", []byte{}},
		{"bare 00 00 prefix", []byte{0x00, 0x00}},
	}
	for _, tc := range unselected {
		img, format, err := image.Decode(bytes.NewReader(tc.src))
		if !errors.Is(err, image.ErrFormat) {
			fail("%s: error = %v (format %q image %T), want image.ErrFormat", tc.name, err, format, img)
		}
		if format != "" {
			fail("%s: format = %q, want empty with image.ErrFormat", tc.name, format)
		}
		if img != nil {
			fail("%s: image = %T, want nil with image.ErrFormat", tc.name, img)
		}
	}

	// The registry matcher covers only the listed fixture prefixes, not the
	// full canonical varint language: a valid unlisted 1x1 Type 0 image must
	// stay image.ErrFormat through image.Decode while remaining supported
	// through wuffs.Decode and DecodeReader.
	unlisted := []byte{0x00, 0x00, 0x01, 0x01, 0x80}
	img, format, err := image.Decode(bytes.NewReader(unlisted))
	if !errors.Is(err, image.ErrFormat) {
		fail("unlisted 1x1 dimensions: error = %v (format %q image %T), want image.ErrFormat", err, format, img)
	}
	if format != "" {
		fail("unlisted 1x1 dimensions: format = %q, want empty with image.ErrFormat", format)
	}
	if img != nil {
		fail("unlisted 1x1 dimensions: image = %T, want nil with image.ErrFormat", img)
	}
	if _, _, err := wuffs.Decode(unlisted); err != nil {
		fail("wuffs.Decode(unlisted 1x1): %v, want nil error", err)
	}
	if _, _, err := wuffs.DecodeReader(bytes.NewReader(unlisted)); err != nil {
		fail("wuffs.DecodeReader(unlisted 1x1): %v, want nil error", err)
	}

	fmt.Println("probe: ok")
}
`
	dir := t.TempDir()
	probePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(probePath, []byte(probe), 0o600); err != nil {
		t.Fatalf("writing isolated probe: %v", err)
	}
	out, err := exec.Command("go", "run", probePath).CombinedOutput()
	if err != nil {
		t.Fatalf("isolated wbmp registration probe failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "probe: ok") {
		t.Fatalf("isolated wbmp registration probe output = %q, want it to report ok", string(out))
	}
}
