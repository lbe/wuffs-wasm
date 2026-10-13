package registrationtest

import (
	"bytes"
	"errors"
	"image"
	"testing"

	wuffs "github.com/lbe/wuffs-wasm"
)

func runLiteralSignatures(t *testing.T) {
	t.Helper()

	fixtures := []struct{ file, format string }{
		{"testdata/bricks-color.png", "png"},
		{"testdata/bricks-color.lossless.webp", "webp"},
		{"testdata/bricks-color.bmp", "bmp"},
		{"testdata/bricks-nodither.gif", "gif"},
		{"testdata/hat.jpeg", "jpeg"},
		{"testdata/bricks-color.qoi", "qoi"},
		{"testdata/bricks-color.etc2.pkm", "etc2"},
		{"testdata/crude-flag.nie", "nie"},
		{"testdata/mona-lisa.21x32.th", "th"},
	}
	for _, tc := range fixtures {
		src := readFixture(t, tc.file)
		img, format, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			t.Fatalf("Decode(%s): %v, want format %q", tc.file, err, tc.format)
		}
		if format != tc.format {
			t.Fatalf("Decode(%s) format = %q, want %q", tc.file, format, tc.format)
		}
		rgba, ok := img.(*image.RGBA)
		if !ok {
			t.Fatalf("Decode(%s) type = %T, want *image.RGBA", tc.file, img)
		}
		ref, _, err := wuffs.Decode(src)
		if err != nil {
			t.Fatalf("wuffs.Decode(%s): %v", tc.file, err)
		}
		if !rgba.Bounds().Eq(ref.Bounds()) || rgba.Stride != ref.Stride || !bytes.Equal(rgba.Pix, ref.Pix) {
			t.Fatalf("Decode(%s) pixels differ from wuffs.Decode", tc.file)
		}
	}

	sizes := [][]byte{{0x00, 0x00, 0x00, 0x00}, {0x24, 0x10, 0x00, 0x00}}
	for i, sz := range sizes {
		hdr := append(append([]byte("RIFF"), sz...), []byte("WEBP")...)
		img, format, err := image.Decode(bytes.NewReader(hdr))
		if img != nil {
			t.Fatalf("synthetic webp header %d returned %T, want nil image", i, img)
		}
		if format != "webp" {
			t.Fatalf("synthetic webp header %d format = %q, want webp", i, format)
		}
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Fatalf("synthetic webp header %d error = %v, want wuffs.ErrDecode", i, err)
		}
		if errors.Is(err, image.ErrFormat) {
			t.Fatalf("synthetic webp header %d must not be image.ErrFormat", i)
		}
	}

	seq := make([]byte, 18)
	for i := range seq {
		seq[i] = byte(i)
	}
	controls := []struct {
		name string
		src  []byte
	}{
		{"RIFF without WEBP", append(append([]byte("RIFF"), 0x00, 0x00, 0x00, 0x00), []byte("XXXX")...)},
		{"RIFX with WEBP", append(append([]byte("RIFX"), 0x00, 0x00, 0x00, 0x00), []byte("WEBP")...)},
		{"non-RIFF with WEBP", append(append([]byte("XXXX"), 0x00, 0x00, 0x00, 0x00), []byte("WEBP")...)},
		{"mutated PNG signature", mutate(readFixture(t, "testdata/bricks-color.png"), 0, 0x88)},
		{"mutated WebP WEBP tag", mutate(readFixture(t, "testdata/bricks-color.lossless.webp"), 8, 'X')},
		{"mutated BMP signature", mutate(readFixture(t, "testdata/bricks-color.bmp"), 0, 'X')},
		{"mutated GIF signature", mutate(readFixture(t, "testdata/bricks-nodither.gif"), 0, 'H')},
		{"mutated JPEG SOI", mutate(readFixture(t, "testdata/hat.jpeg"), 0, 0x00)},
		{"mutated QOI magic", mutate(readFixture(t, "testdata/bricks-color.qoi"), 3, 'g')},
		{"mutated PKM magic", mutate(readFixture(t, "testdata/bricks-color.etc2.pkm"), 3, '!')},
		{"mutated NIE magic", mutate(readFixture(t, "testdata/crude-flag.nie"), 0, 0x00)},
		{"mutated TH magic", mutate(readFixture(t, "testdata/mona-lisa.21x32.th"), 0, 0x00)},
		{"18 zero bytes", make([]byte, 18)},
		{"bytes 0x00 through 0x11", seq},
		{"ELF bytes padded to 18", append([]byte{0x7F, 'E', 'L', 'F'}, make([]byte, 14)...)},
	}
	for _, tc := range controls {
		wantErrFormat(t, tc.name, tc.src)
	}
}

func wantErrFormat(t *testing.T, name string, src []byte) {
	t.Helper()
	img, format, err := image.Decode(bytes.NewReader(src))
	if !errors.Is(err, image.ErrFormat) {
		t.Fatalf("%s: error = %v (format %q image %T), want image.ErrFormat", name, err, format, img)
	}
	if format != "" {
		t.Fatalf("%s: format = %q, want empty with image.ErrFormat", name, format)
	}
	if img != nil {
		t.Fatalf("%s: image = %T, want nil with image.ErrFormat", name, img)
	}
}
