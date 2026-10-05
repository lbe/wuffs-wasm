package wuffs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegrationRegisterFormatsDecodeConfig verifies that the registration
// catalog routes configuration decoding through image.DecodeConfig with the
// canonical lowercase name and an image.Config exactly equal to
// wuffs.DecodeConfig on the same bytes, without decoding pixels.
func TestIntegrationRegisterFormatsDecodeConfig(t *testing.T) {
	probe := `package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"

	wuffs "github.com/lbe/wuffs-wasm"
)

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "probe: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	wuffs.RegisterFormats()

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
		src, err := os.ReadFile(tc.file)
		if err != nil {
			fail("read %s: %v", tc.file, err)
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(src))
		if err != nil {
			fail("DecodeConfig(%s): %v, want nil error with format %q", tc.file, err, tc.format)
		}
		if format != tc.format {
			fail("DecodeConfig(%s) format = %q, want %q", tc.file, format, tc.format)
		}
		ref, err := wuffs.DecodeConfig(src)
		if err != nil {
			fail("wuffs.DecodeConfig(%s): %v", tc.file, err)
		}
		if cfg != ref {
			fail("DecodeConfig(%s) config = %+v, want exact wuffs.DecodeConfig %+v", tc.file, cfg, ref)
		}
		if cfg.ColorModel != color.RGBAModel {
			fail("DecodeConfig(%s) ColorModel = %v, want color.RGBAModel", tc.file, cfg.ColorModel)
		}
		if cfg.Width != ref.Width || cfg.Height != ref.Height {
			fail("DecodeConfig(%s) dimensions = %dx%d, want %dx%d", tc.file, cfg.Width, cfg.Height, ref.Width, ref.Height)
		}
	}

	// Recognizable truncated inputs must return the same decode error
	// produced through DecodeConfigReader after registry selection.
	truncated := []struct {
		name string
		src  []byte
	}{
		{"truncated png signature", mustRead("testdata/bricks-color.png")[:8]},
		{"truncated webp header", mustRead("testdata/bricks-color.lossless.webp")[:12]},
		{"truncated bmp signature", mustRead("testdata/bricks-color.bmp")[:2]},
		{"truncated gif prefix", mustRead("testdata/bricks-nodither.gif")[:4]},
		{"truncated jpeg prefix", mustRead("testdata/hat.jpeg")[:3]},
		{"truncated qoi magic", mustRead("testdata/bricks-color.qoi")[:4]},
		{"truncated etc2 magic", mustRead("testdata/bricks-color.etc2.pkm")[:4]},
		{"truncated nie magic", mustRead("testdata/crude-flag.nie")[:4]},
		{"truncated th magic", mustRead("testdata/mona-lisa.21x32.th")[:3]},
		{"truncated npbm prefix", mustRead("testdata/hippopotamus.pgm")[:2]},
	}
	for _, tc := range truncated {
		cfg, format, err := image.DecodeConfig(bytes.NewReader(tc.src))
		wantCfg, wantErr := wuffs.DecodeConfigReader(bytes.NewReader(tc.src))
		if err == nil || wantErr == nil {
			fail("%s: errors = (%v, %v), want both non-nil decode errors", tc.name, err, wantErr)
		}
		if fmt.Sprint(err) != fmt.Sprint(wantErr) {
			fail("%s: error = %v, want DecodeConfigReader error %v", tc.name, err, wantErr)
		}
		if errors.Is(err, image.ErrFormat) {
			fail("%s: error = %v, must not be image.ErrFormat after registry selection", tc.name, err)
		}
		if cfg != wantCfg {
			fail("%s: config = %+v, want DecodeConfigReader config %+v", tc.name, cfg, wantCfg)
		}
		if format == "" {
			fail("%s: format empty, want registry-selected name", tc.name)
		}
	}

	// Inputs matching no registered pattern must return image.ErrFormat,
	// an empty name, and zero image.Config.
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
			fail("%s: error = %v (format %q config %+v), want image.ErrFormat", tc.name, err, format, cfg)
		}
		if format != "" {
			fail("%s: format = %q, want empty with image.ErrFormat", tc.name, format)
		}
		if cfg != (image.Config{}) {
			fail("%s: config = %+v, want zero image.Config with image.ErrFormat", tc.name, cfg)
		}
	}

	fmt.Println("probe: ok")
}

func mustRead(file string) []byte {
	src, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "probe: read %s: %v\n", file, err)
		os.Exit(1)
	}
	return src
}
`
	dir := t.TempDir()
	probePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(probePath, []byte(probe), 0o600); err != nil {
		t.Fatalf("writing isolated probe: %v", err)
	}
	out, err := exec.Command("go", "run", probePath).CombinedOutput()
	if err != nil {
		t.Fatalf("isolated DecodeConfig probe failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "probe: ok") {
		t.Fatalf("isolated DecodeConfig probe output = %q, want it to report ok", string(out))
	}
}
