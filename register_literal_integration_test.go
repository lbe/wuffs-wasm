package wuffs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegrationRegisterFormatsLiteralSignatures verifies the literal-signature
// registration matrix in an isolated registry subprocess: after a single
// RegisterFormats call, image.Decode accepts the checked-in fixtures for PNG,
// WebP, BMP, GIF, JPEG, QOI, ETC2 PKM, NIE, and cooked TH with the exact
// canonical lowercase name and *image.RGBA pixels identical to wuffs.Decode;
// the fixed-offset RIFF????WEBP wildcard selects truncated 12-byte synthetic
// headers (two differing size fields) by name and reaches the decode callback
// with the established decode error; RIFF inputs lacking WEBP at bytes 8-11,
// non-RIFF inputs carrying WEBP at bytes 8-11, near-signature mutations, and
// the cross-format collision corpus stay image.ErrFormat.
func TestIntegrationRegisterFormatsLiteralSignatures(t *testing.T) {
	for _, f := range []string{"registration.go", filepath.Join("internal", "imgreg", "imgreg.go")} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		for _, banned := range []string{`"image/png"`, `"image/jpeg"`, `"image/gif"`} {
			if strings.Contains(string(src), banned) {
				t.Errorf("%s must not import %s: the registry callback decodes through wuffs, not another codec", f, banned)
			}
		}
	}

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
		src, err := os.ReadFile(tc.file)
		if err != nil {
			fail("read %s: %v", tc.file, err)
		}
		img, format, err := image.Decode(bytes.NewReader(src))
		if err != nil {
			fail("Decode(%s): %v, want nil error with format %q", tc.file, err, tc.format)
		}
		if format != tc.format {
			fail("Decode(%s) format = %q, want %q", tc.file, format, tc.format)
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

	sizes := [][]byte{{0x00, 0x00, 0x00, 0x00}, {0x24, 0x10, 0x00, 0x00}}
	for i, sz := range sizes {
		hdr := append(append([]byte("RIFF"), sz...), []byte("WEBP")...)
		img, format, err := image.Decode(bytes.NewReader(hdr))
		if img != nil {
			fail("synthetic webp header %d returned %T, want nil image", i, img)
		}
		if format != "webp" {
			fail("synthetic webp header %d format = %q, want %q (RIFF????WEBP wildcard)", i, format, "webp")
		}
		if !errors.Is(err, wuffs.ErrDecode) {
			fail("synthetic webp header %d error = %v, want wuffs.ErrDecode", i, err)
		}
		if errors.Is(err, image.ErrFormat) {
			fail("synthetic webp header %d error = %v, must not be image.ErrFormat", i, err)
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
	controls := []struct {
		name string
		src  []byte
	}{
		{"RIFF without WEBP", append(append([]byte("RIFF"), 0x00, 0x00, 0x00, 0x00), []byte("XXXX")...)},
		{"RIFX with WEBP", append(append([]byte("RIFX"), 0x00, 0x00, 0x00, 0x00), []byte("WEBP")...)},
		{"non-RIFF with WEBP", append(append([]byte("XXXX"), 0x00, 0x00, 0x00, 0x00), []byte("WEBP")...)},
		{"mutated PNG signature", mutate(read("testdata/bricks-color.png"), 0, 0x88)},
		{"mutated WebP WEBP tag", mutate(read("testdata/bricks-color.lossless.webp"), 8, 'X')},
		{"mutated BMP signature", mutate(read("testdata/bricks-color.bmp"), 0, 'X')},
		{"mutated GIF signature", mutate(read("testdata/bricks-nodither.gif"), 0, 'H')},
		{"mutated JPEG SOI", mutate(read("testdata/hat.jpeg"), 0, 0x00)},
		{"mutated QOI magic", mutate(read("testdata/bricks-color.qoi"), 3, 'g')},
		{"mutated PKM magic", mutate(read("testdata/bricks-color.etc2.pkm"), 3, '!')},
		{"mutated NIE magic", mutate(read("testdata/crude-flag.nie"), 0, 0x00)},
		{"mutated TH magic", mutate(read("testdata/mona-lisa.21x32.th"), 0, 0x00)},
		{"18 zero bytes", make([]byte, 18)},
		{"bytes 0x00 through 0x11", func() []byte { s := make([]byte, 18); for i := range s { s[i] = byte(i) }; return s }()},
		{"ELF bytes padded to 18", append([]byte{0x7F, 'E', 'L', 'F'}, make([]byte, 14)...)},
	}
	for _, tc := range controls {
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
		t.Fatalf("isolated literal-signature probe failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "probe: ok") {
		t.Fatalf("isolated literal-signature probe output = %q, want it to report ok", string(out))
	}
}
