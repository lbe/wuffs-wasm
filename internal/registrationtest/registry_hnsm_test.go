package registrationtest

import (
	"bytes"
	"errors"
	"image"
	"testing"

	wuffs "github.com/lbe/wuffs-wasm"
)

func runHandsum(t *testing.T) {
	t.Helper()

	for _, file := range []string{
		"testdata/bricks-gray.c1q1.handsum",
		"testdata/bricks-color.c3q4.handsum",
		"testdata/mona-lisa.21x32.c3q4.handsum",
		"testdata/49.c4q4.handsum",
		"testdata/49.c3q2.handsum",
	} {
		src := readFixture(t, file)
		img, format, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			t.Fatalf("Decode(%s): %v", file, err)
		}
		if format != "hnsm" {
			t.Fatalf("Decode(%s) format = %q, want hnsm", file, format)
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

	for _, color := range []byte{0, 2, 3} {
		for quality := byte(0); quality <= 3; quality++ {
			for geom := byte(0); geom <= 30; geom++ {
				src := hnsmHeader(color, quality, geom)
				img, format, err := image.Decode(bytes.NewReader(src))
				if img != nil {
					t.Fatalf("accepted color %d quality %d geometry 0x%02X returned %T", color, quality, geom, img)
				}
				if format != "hnsm" {
					t.Fatalf("accepted color %d quality %d geometry 0x%02X format = %q, want hnsm", color, quality, geom, format)
				}
				if !errors.Is(err, wuffs.ErrDecode) {
					t.Fatalf("accepted color %d quality %d geometry 0x%02X error = %v, want wuffs.ErrDecode", color, quality, geom, err)
				}
				if errors.Is(err, image.ErrFormat) {
					t.Fatalf("accepted color %d quality %d geometry 0x%02X must not be image.ErrFormat", color, quality, geom)
				}
			}
		}
	}

	for quality := byte(0); quality <= 3; quality++ {
		for geom := byte(0); geom <= 30; geom++ {
			wantErrFormat(t, "rejected color class 1", hnsmHeader(1, quality, geom))
		}
	}
	for _, color := range []byte{0, 2, 3} {
		for quality := byte(0); quality <= 3; quality++ {
			wantErrFormat(t, "rejected reserved geometry 0x1F", hnsmHeader(color, quality, 0x1F))
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
		wantErrFormat(t, tc.name, tc.src)
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
		src := readFixture(t, tc.file)
		img, format, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			t.Fatalf("Decode(%s): %v", tc.file, err)
		}
		if format != tc.format {
			t.Fatalf("Decode(%s) format = %q, want %q", tc.file, format, tc.format)
		}
		if _, ok := img.(*image.RGBA); !ok {
			t.Fatalf("Decode(%s) type = %T", tc.file, img)
		}
	}
}

func hnsmHeader(color, quality, geom byte) []byte {
	v := (uint32(0x7F6B) << 9) | (uint32(color) << 7) | (uint32(quality) << 5) | uint32(geom)
	return []byte{byte(v >> 16), byte(v >> 8), byte(v)}
}
