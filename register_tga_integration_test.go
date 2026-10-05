package wuffs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegrationRegisterFormatsTGA verifies that the registration catalog
// routes the finite checked-in TGA fixtures whose complete 18-byte headers
// are registered literally through image.Decode with the canonical name
// "tga" and *image.RGBA pixels identical to wuffs.Decode, while near-header
// mutations and the existing non-TGA and format-collision corpus stay
// image.ErrFormat.
//
// image.RegisterFormat's literal matcher cannot express the full TGA header
// language: unlisted TGA dimensions remain supported through wuffs.Decode
// and DecodeReader, not the image registry. Inputs matching a registered
// 18-byte header with an invalid or truncated payload may be selected as TGA
// and must return the existing decoder error; this intentional
// prefix-recognition boundary is documented. No wildcard or empty catch-all
// magic is installed; general TGA recognition and payload validation stay in
// the existing Wuffs decode path.
func TestIntegrationRegisterFormatsTGA(t *testing.T) {
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
		{"testdata/bricks-color.tga"},
		{"testdata/bricks-nodither.tga"},
		{"testdata/bricks-gray.tga"},
	}
	for _, tc := range fixtures {
		src, err := os.ReadFile(tc.file)
		if err != nil {
			fail("read %s: %v", tc.file, err)
		}
		img, format, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			fail("Decode(%s): %v, want nil error with format %q", tc.file, err, "tga")
		}
		if format != "tga" {
			fail("Decode(%s) format = %q, want %q", tc.file, format, "tga")
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
		{"mutated bricks-color type", mutate(read("testdata/bricks-color.tga"), 2, 0x00)},
		{"mutated bricks-nodither type", mutate(read("testdata/bricks-nodither.tga"), 2, 0x00)},
		{"mutated bricks-gray type", mutate(read("testdata/bricks-gray.tga"), 2, 0x00)},
		{"mutated bricks-color width", mutate(read("testdata/bricks-color.tga"), 12, 0x01)},
		{"mutated bricks-color depth", mutate(read("testdata/bricks-color.tga"), 16, 0x07)},
		{"mutated bricks-color descriptor", mutate(read("testdata/bricks-color.tga"), 17, 0x10)},
		{"18 zero bytes", make([]byte, 18)},
		{"bytes 0x00 through 0x11", seq},
		{"ELF bytes padded to 18", append([]byte{0x7F, 'E', 'L', 'F'}, make([]byte, 14)...)},
		{"empty input", []byte{}},
		{"truncated header", read("testdata/bricks-color.tga")[:17]},
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

	shadowed := []struct{ file, format string }{
		{"testdata/bricks-color.png", "png"},
		{"testdata/bricks-color.bmp", "bmp"},
		{"testdata/bricks-nodither.gif", "gif"},
		{"testdata/hat.jpeg", "jpeg"},
		{"testdata/bricks-color.qoi", "qoi"},
		{"testdata/hippopotamus.pgm", "npbm"},
		{"testdata/hippopotamus.ppm", "npbm"},
	}
	for _, tc := range shadowed {
		src, err := os.ReadFile(tc.file)
		if err != nil {
			fail("read %s: %v", tc.file, err)
		}
		img, format, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			fail("Decode(%s): %v, want nil error with format %q", tc.file, err, tc.format)
		}
		if format != tc.format {
			fail("Decode(%s) format = %q, want %q (TGA must not shadow)", tc.file, format, tc.format)
		}
		if _, ok := img.(*image.RGBA); !ok {
			fail("Decode(%s) type = %T, want *image.RGBA", tc.file, img)
		}
	}

	full := read("testdata/bricks-color.tga")
	corrupt := full[:18+48]
	img, format, err := image.Decode(bytes.NewReader(corrupt))
	if img != nil {
		fail("truncated payload returned %T, want nil image", img)
	}
	if format != "tga" {
		fail("truncated payload format = %q, want %q (registered header prefix)", format, "tga")
	}
	if !errors.Is(err, wuffs.ErrDecode) {
		fail("truncated payload error = %v, want wuffs.ErrDecode", err)
	}
	if errors.Is(err, image.ErrFormat) {
		fail("truncated payload error = %v, must not be image.ErrFormat", err)
	}

	unlisted := append([]byte{0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0x00, 0x01, 0x00, 0x18, 0x00}, bytes.Repeat([]byte{0x10, 0x20, 0x30}, 3)...)
	img, format, err = image.Decode(bytes.NewReader(unlisted))
	if !errors.Is(err, image.ErrFormat) {
		fail("unlisted 3x1 dimensions: error = %v (format %q image %T), want image.ErrFormat", err, format, img)
	}
	if format != "" {
		fail("unlisted 3x1 dimensions: format = %q, want empty with image.ErrFormat", format)
	}
	if img != nil {
		fail("unlisted 3x1 dimensions: image = %T, want nil with image.ErrFormat", img)
	}
	if _, _, err := wuffs.Decode(unlisted); err != nil {
		fail("wuffs.Decode(unlisted 3x1): %v, want nil error", err)
	}
	if _, _, err := wuffs.DecodeReader(bytes.NewReader(unlisted)); err != nil {
		fail("wuffs.DecodeReader(unlisted 3x1): %v, want nil error", err)
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
		t.Fatalf("isolated tga registration probe failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "probe: ok") {
		t.Fatalf("isolated tga registration probe output = %q, want it to report ok", string(out))
	}
}
