package registrationtest

import (
	"bytes"
	"errors"
	"image"
	"testing"

	wuffs "github.com/lbe/wuffs-wasm"
)

func runTGA(t *testing.T) {
	t.Helper()

	fixtures := []string{
		"testdata/bricks-color.tga",
		"testdata/bricks-nodither.tga",
		"testdata/bricks-gray.tga",
	}
	for _, file := range fixtures {
		src := readFixture(t, file)
		img, format, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			t.Fatalf("Decode(%s): %v", file, err)
		}
		if format != "tga" {
			t.Fatalf("Decode(%s) format = %q, want tga", file, format)
		}
		rgba, ok := img.(*image.RGBA)
		if !ok {
			t.Fatalf("Decode(%s) type = %T", file, img)
		}
		ref, _, err := wuffs.Decode(src)
		if err != nil {
			t.Fatalf("wuffs.Decode(%s): %v", file, err)
		}
		if !rgba.Bounds().Eq(ref.Bounds()) || rgba.Stride != ref.Stride || !bytes.Equal(rgba.Pix, ref.Pix) {
			t.Fatalf("Decode(%s) pixels differ from wuffs.Decode", file)
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
		{"mutated bricks-color type", mutate(readFixture(t, "testdata/bricks-color.tga"), 2, 0x00)},
		{"mutated bricks-nodither type", mutate(readFixture(t, "testdata/bricks-nodither.tga"), 2, 0x00)},
		{"mutated bricks-gray type", mutate(readFixture(t, "testdata/bricks-gray.tga"), 2, 0x00)},
		{"mutated bricks-color width", mutate(readFixture(t, "testdata/bricks-color.tga"), 12, 0x01)},
		{"mutated bricks-color depth", mutate(readFixture(t, "testdata/bricks-color.tga"), 16, 0x07)},
		{"mutated bricks-color descriptor", mutate(readFixture(t, "testdata/bricks-color.tga"), 17, 0x10)},
		{"18 zero bytes", make([]byte, 18)},
		{"bytes 0x00 through 0x11", seq},
		{"ELF bytes padded to 18", append([]byte{0x7F, 'E', 'L', 'F'}, make([]byte, 14)...)},
		{"empty input", []byte{}},
		{"truncated header", readFixture(t, "testdata/bricks-color.tga")[:17]},
	}
	for _, tc := range unselected {
		wantErrFormat(t, tc.name, tc.src)
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
		src := readFixture(t, tc.file)
		img, format, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			t.Fatalf("Decode(%s): %v", tc.file, err)
		}
		if format != tc.format {
			t.Fatalf("Decode(%s) format = %q, want %q (TGA must not shadow)", tc.file, format, tc.format)
		}
		if _, ok := img.(*image.RGBA); !ok {
			t.Fatalf("Decode(%s) type = %T", tc.file, img)
		}
	}

	full := readFixture(t, "testdata/bricks-color.tga")
	corrupt := full[:18+48]
	img, format, err := image.Decode(bytes.NewReader(corrupt))
	if img != nil {
		t.Fatalf("truncated payload returned %T, want nil", img)
	}
	if format != "tga" {
		t.Fatalf("truncated payload format = %q, want tga", format)
	}
	if !errors.Is(err, wuffs.ErrDecode) {
		t.Fatalf("truncated payload error = %v, want wuffs.ErrDecode", err)
	}
	if errors.Is(err, image.ErrFormat) {
		t.Fatal("truncated payload must not be image.ErrFormat")
	}

	unlisted := append([]byte{0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0x00, 0x01, 0x00, 0x18, 0x00}, bytes.Repeat([]byte{0x10, 0x20, 0x30}, 3)...)
	img, format, err = image.Decode(bytes.NewReader(unlisted))
	if !errors.Is(err, image.ErrFormat) {
		t.Fatalf("unlisted 3x1: error = %v (format %q), want image.ErrFormat", err, format)
	}
	if format != "" || img != nil {
		t.Fatalf("unlisted 3x1: format=%q img=%T", format, img)
	}
	if _, _, err := wuffs.Decode(unlisted); err != nil {
		t.Fatalf("wuffs.Decode(unlisted 3x1): %v", err)
	}
	if _, _, err := wuffs.DecodeReader(bytes.NewReader(unlisted)); err != nil {
		t.Fatalf("wuffs.DecodeReader(unlisted 3x1): %v", err)
	}
}
