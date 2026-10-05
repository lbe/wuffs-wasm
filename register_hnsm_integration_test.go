package wuffs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegrationRegisterFormatsHandsum verifies that the registration catalog
// routes the complete verified HNSM structural header matrix through
// image.Decode with the canonical name "hnsm" and *image.RGBA pixels identical
// to wuffs.Decode, while invalid structural encodings, the retired literal
// HNSM marker, and the full collision corpus stay image.ErrFormat, and the
// HNSM patterns do not shadow any of the other twelve registered formats.
func TestIntegrationRegisterFormatsHandsum(t *testing.T) {
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

func hnsmHeader(color, quality, geom byte) []byte {
	v := (uint32(0x7F6B) << 9) | (uint32(color) << 7) | (uint32(quality) << 5) | uint32(geom)
	return []byte{byte(v >> 16), byte(v >> 8), byte(v)}
}

func wantErrFormat(name string, src []byte) {
	img, format, err := image.Decode(bytes.NewReader(src))
	if !errors.Is(err, image.ErrFormat) {
		fail("%s: error = %v (format %q image %T), want image.ErrFormat", name, err, format, img)
	}
	if format != "" {
		fail("%s: format = %q, want empty with image.ErrFormat", name, format)
	}
	if img != nil {
		fail("%s: image = %T, want nil with image.ErrFormat", name, img)
	}
}

func main() {
	wuffs.RegisterFormats()

	fixtures := []struct{ file string }{
		{"testdata/bricks-gray.c1q1.handsum"},
		{"testdata/bricks-color.c3q4.handsum"},
		{"testdata/mona-lisa.21x32.c3q4.handsum"},
		{"testdata/49.c4q4.handsum"},
		{"testdata/49.c3q2.handsum"},
	}
	for _, tc := range fixtures {
		src, err := os.ReadFile(tc.file)
		if err != nil {
			fail("read %s: %v", tc.file, err)
		}
		img, format, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			fail("Decode(%s): %v, want nil error with format %q", tc.file, err, "hnsm")
		}
		if format != "hnsm" {
			fail("Decode(%s) format = %q, want %q", tc.file, format, "hnsm")
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

	for _, color := range []byte{0, 2, 3} {
		for quality := byte(0); quality <= 3; quality++ {
			for geom := byte(0); geom <= 30; geom++ {
				src := hnsmHeader(color, quality, geom)
				img, format, err := image.Decode(bytes.NewReader(src))
				if img != nil {
					fail("accepted color %d quality %d geometry 0x%02X returned %T, want nil image", color, quality, geom, img)
				}
				if format != "hnsm" {
					fail("accepted color %d quality %d geometry 0x%02X format = %q, want %q", color, quality, geom, format, "hnsm")
				}
				if !errors.Is(err, wuffs.ErrDecode) {
					fail("accepted color %d quality %d geometry 0x%02X error = %v, want wuffs.ErrDecode", color, quality, geom, err)
				}
				if errors.Is(err, image.ErrFormat) {
					fail("accepted color %d quality %d geometry 0x%02X error = %v, must not be image.ErrFormat", color, quality, geom, err)
				}
			}
		}
	}

	for quality := byte(0); quality <= 3; quality++ {
		for geom := byte(0); geom <= 30; geom++ {
			wantErrFormat("rejected color class 1", hnsmHeader(1, quality, geom))
		}
	}
	for _, color := range []byte{0, 2, 3} {
		for quality := byte(0); quality <= 3; quality++ {
			wantErrFormat("rejected reserved geometry 0x1F", hnsmHeader(color, quality, 0x1F))
		}
	}

	seq := make([]byte, 18)
	for i := range seq {
		seq[i] = byte(i)
	}
	unselected := []struct {
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
		{"bytes 0x00 through 0x11", seq},
		{"ELF bytes padded to 18", append([]byte{0x7F, 'E', 'L', 'F'}, make([]byte, 14)...)},
	}
	for _, tc := range unselected {
		wantErrFormat(tc.name, tc.src)
	}

	shadowed := []struct{ file, format string }{
		{"testdata/bricks-color.png", "png"},
		{"testdata/bricks-color.lossless.webp", "webp"},
		{"testdata/bricks-color.bmp", "bmp"},
		{"testdata/bricks-nodither.gif", "gif"},
		{"testdata/hat.jpeg", "jpeg"},
		{"testdata/bricks-color.qoi", "qoi"},
		{"testdata/bricks-color.etc2.pkm", "etc2"},
		{"testdata/crude-flag.nie", "nie"},
		{"testdata/mona-lisa.21x32.th", "th"},
		{"testdata/hippopotamus.pgm", "npbm"},
		{"testdata/hippopotamus.ppm", "npbm"},
		{"testdata/muybridge-frame-000.wbmp", "wbmp"},
		{"testdata/bricks-color.tga", "tga"},
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
			fail("Decode(%s) format = %q, want %q (HNSM must not shadow)", tc.file, format, tc.format)
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
		t.Fatalf("isolated hnsm registration probe failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "probe: ok") {
		t.Fatalf("isolated hnsm registration probe output = %q, want it to report ok", string(out))
	}
}
