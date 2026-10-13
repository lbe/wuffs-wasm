package registrationtest

import (
	"bytes"
	"errors"
	"image"
	"testing"

	wuffs "github.com/lbe/wuffs-wasm"
)

func runWBMP(t *testing.T) {
	t.Helper()

	for _, file := range []string{"testdata/muybridge-frame-000.wbmp", "testdata/bricks-nodither.wbmp"} {
		src := readFixture(t, file)
		img, format, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			t.Fatalf("Decode(%s): %v", file, err)
		}
		if format != "wbmp" {
			t.Fatalf("Decode(%s) format = %q, want wbmp", file, format)
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
		{"mutated muybridge width", mutate(readFixture(t, "testdata/muybridge-frame-000.wbmp"), 2, 0x1F)},
		{"mutated bricks width tail", mutate(readFixture(t, "testdata/bricks-nodither.wbmp"), 3, 0x21)},
		{"nonzero TypeField", mutate(readFixture(t, "testdata/muybridge-frame-000.wbmp"), 0, 0x01)},
		{"nonzero FixHeaderField", mutate(readFixture(t, "testdata/muybridge-frame-000.wbmp"), 1, 0x01)},
		{"18 zero bytes", make([]byte, 18)},
		{"bytes 0x00 through 0x11", seq},
		{"ELF bytes", []byte{0x7F, 0x45, 0x4C, 0x46}},
		{"empty input", []byte{}},
		{"bare 00 00 prefix", []byte{0x00, 0x00}},
	}
	for _, tc := range unselected {
		wantErrFormat(t, tc.name, tc.src)
	}

	unlisted := []byte{0x00, 0x00, 0x01, 0x01, 0x80}
	img, format, err := image.Decode(bytes.NewReader(unlisted))
	if !errors.Is(err, image.ErrFormat) {
		t.Fatalf("unlisted 1x1: error = %v (format %q), want image.ErrFormat", err, format)
	}
	if format != "" || img != nil {
		t.Fatalf("unlisted 1x1: format=%q img=%T", format, img)
	}
	if _, _, err := wuffs.Decode(unlisted); err != nil {
		t.Fatalf("wuffs.Decode(unlisted 1x1): %v", err)
	}
	if _, _, err := wuffs.DecodeReader(bytes.NewReader(unlisted)); err != nil {
		t.Fatalf("wuffs.DecodeReader(unlisted 1x1): %v", err)
	}
}
