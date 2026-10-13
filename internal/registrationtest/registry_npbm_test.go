package registrationtest

import (
	"bytes"
	"image"
	"testing"

	wuffs "github.com/lbe/wuffs-wasm"
)

func runNetpbm(t *testing.T) {
	t.Helper()

	for _, file := range []string{
		"testdata/hippopotamus.pgm",
		"testdata/hippopotamus.ppm",
		"testdata/npbm-maxval65535.pgm",
		"testdata/npbm-maxval65535.ppm",
	} {
		src := readFixture(t, file)
		img, format, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			t.Fatalf("Decode(%s): %v", file, err)
		}
		if format != "npbm" {
			t.Fatalf("Decode(%s) format = %q, want npbm", file, format)
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
		wantErrFormat(t, tc.name, tc.src)
	}

	nearPrefix := []struct {
		name string
		src  []byte
	}{
		{"mutated PGM magic", mutate(readFixture(t, "testdata/hippopotamus.pgm"), 0, 'X')},
		{"mutated PPM magic", mutate(readFixture(t, "testdata/hippopotamus.ppm"), 0, 'X')},
		{"mutated PGM class", mutate(readFixture(t, "testdata/hippopotamus.pgm"), 1, '1')},
		{"mutated PPM class", mutate(readFixture(t, "testdata/hippopotamus.ppm"), 1, '7')},
	}
	for _, tc := range nearPrefix {
		wantErrFormat(t, tc.name, tc.src)
	}

	shadowed := []struct{ file, format string }{
		{"testdata/bricks-color.png", "png"},
		{"testdata/bricks-color.bmp", "bmp"},
		{"testdata/bricks-nodither.gif", "gif"},
		{"testdata/hat.jpeg", "jpeg"},
		{"testdata/bricks-color.qoi", "qoi"},
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
