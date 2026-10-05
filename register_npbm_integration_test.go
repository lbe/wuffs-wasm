package wuffs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegrationRegisterFormatsNetpbm verifies that the registration catalog
// routes the verified binary P5 and P6 Netpbm classes through image.Decode
// with the canonical name "npbm" and *image.RGBA pixels identical to
// wuffs.Decode, while unsupported Netpbm classes, near-prefix mutations, and
// every other verified format stay unshadowed.
func TestIntegrationRegisterFormatsNetpbm(t *testing.T) {
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
		{"testdata/hippopotamus.pgm"},
		{"testdata/hippopotamus.ppm"},
		{"testdata/npbm-maxval65535.pgm"},
		{"testdata/npbm-maxval65535.ppm"},
	}
	for _, tc := range fixtures {
		src, err := os.ReadFile(tc.file)
		if err != nil {
			fail("read %s: %v", tc.file, err)
		}
		img, format, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			fail("Decode(%s): %v, want nil error with format %q", tc.file, err, "npbm")
		}
		if format != "npbm" {
			fail("Decode(%s) format = %q, want %q", tc.file, format, "npbm")
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

	unsupported := []struct {
		name string
		src  []byte
	}{
		{"P1 bitmap ASCII", []byte("P1\n1 1\n0\n")},
		{"P2 graymap ASCII", []byte("P2\n1 1\n255\n0\n")},
		{"P3 pixmap ASCII", []byte("P3\n1 1\n255\n0 0 0\n")},
		{"P4 bitmap binary", []byte("P4\n1 1\n\x00")},
		{"P7 PAM", []byte("P7\nWIDTH 1\nHEIGHT 1\nDEPTH 1\nMAXVAL 255\nTUPLTYPE GRAYSCALE\nENDHDR\n\x00")},
		{"X5 near prefix", []byte("X5\n1 1\n255\n\x00")},
		{"Q6 near prefix", []byte("Q6\n1 1\n255\n\x00\x00\x00")},
		{"empty input", []byte{}},
		{"single P byte", []byte("P")},
	}
	for _, tc := range unsupported {
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
	nearPrefix := []struct {
		name string
		src  []byte
	}{
		{"mutated PGM magic", mutate(read("testdata/hippopotamus.pgm"), 0, 'X')},
		{"mutated PPM magic", mutate(read("testdata/hippopotamus.ppm"), 0, 'X')},
		{"mutated PGM class", mutate(read("testdata/hippopotamus.pgm"), 1, '1')},
		{"mutated PPM class", mutate(read("testdata/hippopotamus.ppm"), 1, '7')},
	}
	for _, tc := range nearPrefix {
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
			fail("Decode(%s) format = %q, want %q (NPBM must not shadow)", tc.file, format, tc.format)
		}
		if _, ok := img.(*image.RGBA); !ok {
			fail("Decode(%s) type = %T, want *image.RGBA", tc.file, img)
		}
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
		t.Fatalf("isolated npbm registration probe failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "probe: ok") {
		t.Fatalf("isolated npbm registration probe output = %q, want it to report ok", string(out))
	}
}
