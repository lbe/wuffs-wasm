package wuffs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegrationRegisterFormatsPreservesEarlierDecoder verifies that a
// decoder registered before RegisterFormats keeps precedence: the earlier
// entry also matches a Wuffs-owned prefix (BM) and must stay selected by
// image.Decode and image.DecodeConfig after RegisterFormats.
func TestIntegrationRegisterFormatsPreservesEarlierDecoder(t *testing.T) {
	probe := `package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"

	wuffs "github.com/lbe/wuffs-wasm"
)

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "probe: "+format+"\n", args...)
	os.Exit(1)
}

func sentinelDecode(io.Reader) (image.Image, error) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Pix[0] = 0x11
	img.Pix[1] = 0x22
	img.Pix[2] = 0x33
	img.Pix[3] = 0xFF
	return img, nil
}

func sentinelDecodeConfig(io.Reader) (image.Config, error) {
	return image.Config{ColorModel: color.RGBAModel, Width: 3, Height: 2}, nil
}

func main() {
	image.RegisterFormat("sentinel-bmp", "BM", sentinelDecode, sentinelDecodeConfig)
	wuffs.RegisterFormats()

	payload := append([]byte("BM"), bytes.Repeat([]byte("x"), 32)...)

	img, format, err := image.Decode(bytes.NewReader(payload))
	if err != nil {
		fail("Decode: %v, want sentinel result", err)
	}
	if format != "sentinel-bmp" {
		fail("Decode format = %q, want %q", format, "sentinel-bmp")
	}
	rgba, ok := img.(*image.RGBA)
	if !ok {
		fail("Decode type = %T, want *image.RGBA sentinel", img)
	}
	if !rgba.Bounds().Eq(image.Rect(0, 0, 3, 2)) {
		fail("Decode bounds = %v, want (0,0)-(3,2)", rgba.Bounds())
	}
	if len(rgba.Pix) < 4 || rgba.Pix[0] != 0x11 || rgba.Pix[1] != 0x22 || rgba.Pix[2] != 0x33 || rgba.Pix[3] != 0xFF {
		fail("Decode sentinel pixel = %v, want 11 22 33 FF prefix", rgba.Pix[:4])
	}

	cfg, cfgFormat, err := image.DecodeConfig(bytes.NewReader(payload))
	if err != nil {
		fail("DecodeConfig: %v, want sentinel result", err)
	}
	if cfgFormat != "sentinel-bmp" {
		fail("DecodeConfig format = %q, want %q", cfgFormat, "sentinel-bmp")
	}
	wantCfg := image.Config{ColorModel: color.RGBAModel, Width: 3, Height: 2}
	if cfg != wantCfg {
		fail("DecodeConfig = %+v, want %+v", cfg, wantCfg)
	}

	png, err := os.ReadFile("testdata/bricks-color.png")
	if err != nil {
		fail("read png fixture: %v", err)
	}
	if _, format, err := image.Decode(bytes.NewReader(png)); err != nil || format != "png" {
		fail("Decode(png) = (%q, %v), want (png, nil)", format, err)
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
		t.Fatalf("isolated precedence probe failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "probe: ok") {
		t.Fatalf("isolated precedence probe output = %q, want it to report ok", string(out))
	}
}
