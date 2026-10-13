package registrationtest

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"testing"

	wuffs "github.com/lbe/wuffs-wasm"
)

func runDecodeConfig(t *testing.T) {
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
		{"testdata/hippopotamus.pgm", "npbm"},
		{"testdata/hippopotamus.ppm", "npbm"},
		{"testdata/muybridge-frame-000.wbmp", "wbmp"},
		{"testdata/bricks-color.tga", "tga"},
		{"testdata/bricks-gray.c1q1.handsum", "hnsm"},
	}
	for _, tc := range fixtures {
		src := readFixture(t, tc.file)
		cfg, format, err := image.DecodeConfig(bytes.NewReader(src))
		if err != nil {
			t.Fatalf("DecodeConfig(%s): %v, want format %q", tc.file, err, tc.format)
		}
		if format != tc.format {
			t.Fatalf("DecodeConfig(%s) format = %q, want %q", tc.file, format, tc.format)
		}
		ref, err := wuffs.DecodeConfig(src)
		if err != nil {
			t.Fatalf("wuffs.DecodeConfig(%s): %v", tc.file, err)
		}
		if cfg != ref {
			t.Fatalf("DecodeConfig(%s) = %+v, want %+v", tc.file, cfg, ref)
		}
		if cfg.ColorModel != color.RGBAModel {
			t.Fatalf("DecodeConfig(%s) ColorModel = %v, want color.RGBAModel", tc.file, cfg.ColorModel)
		}
	}

	truncated := []struct {
		name string
		src  []byte
	}{
		{"truncated png signature", readFixture(t, "testdata/bricks-color.png")[:8]},
		{"truncated webp header", readFixture(t, "testdata/bricks-color.lossless.webp")[:12]},
		{"truncated bmp signature", readFixture(t, "testdata/bricks-color.bmp")[:2]},
		{"truncated gif prefix", readFixture(t, "testdata/bricks-nodither.gif")[:4]},
		{"truncated jpeg prefix", readFixture(t, "testdata/hat.jpeg")[:3]},
		{"truncated qoi magic", readFixture(t, "testdata/bricks-color.qoi")[:4]},
		{"truncated etc2 magic", readFixture(t, "testdata/bricks-color.etc2.pkm")[:4]},
		{"truncated nie magic", readFixture(t, "testdata/crude-flag.nie")[:4]},
		{"truncated th magic", readFixture(t, "testdata/mona-lisa.21x32.th")[:3]},
		{"truncated npbm prefix", readFixture(t, "testdata/hippopotamus.pgm")[:2]},
	}
	for _, tc := range truncated {
		cfg, format, err := image.DecodeConfig(bytes.NewReader(tc.src))
		wantCfg, wantErr := wuffs.DecodeConfigReader(bytes.NewReader(tc.src))
		if err == nil || wantErr == nil {
			t.Fatalf("%s: errors = (%v, %v), want both non-nil", tc.name, err, wantErr)
		}
		if fmt.Sprint(err) != fmt.Sprint(wantErr) {
			t.Fatalf("%s: error = %v, want %v", tc.name, err, wantErr)
		}
		if errors.Is(err, image.ErrFormat) {
			t.Fatalf("%s: must not be image.ErrFormat after registry selection", tc.name)
		}
		if cfg != wantCfg {
			t.Fatalf("%s: config = %+v, want %+v", tc.name, cfg, wantCfg)
		}
		if format == "" {
			t.Fatalf("%s: format empty, want registry-selected name", tc.name)
		}
	}

	unselected := []struct {
		name string
		src  []byte
	}{
		{"empty", []byte{}},
		{"unknown text", []byte("not an image at all........")},
		{"18 zero bytes", make([]byte, 18)},
		{"elf padded", append([]byte{0x7F, 'E', 'L', 'F'}, make([]byte, 14)...)},
	}
	for _, tc := range unselected {
		cfg, format, err := image.DecodeConfig(bytes.NewReader(tc.src))
		if !errors.Is(err, image.ErrFormat) {
			t.Fatalf("%s: error = %v (format %q), want image.ErrFormat", tc.name, err, format)
		}
		if format != "" {
			t.Fatalf("%s: format = %q, want empty", tc.name, format)
		}
		if cfg != (image.Config{}) {
			t.Fatalf("%s: config = %+v, want zero", tc.name, cfg)
		}
	}
}
