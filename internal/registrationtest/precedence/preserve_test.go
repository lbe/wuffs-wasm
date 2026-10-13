package precedence

import (
	"bytes"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	wuffs "github.com/lbe/wuffs-wasm"
)

// TestIntegrationRegisterFormatsPreservesEarlierDecoder runs in an isolated
// test binary so a BM sentinel registered before RegisterFormats keeps
// precedence over the Wuffs BMP decoder.
func TestIntegrationRegisterFormatsPreservesEarlierDecoder(t *testing.T) {
	image.RegisterFormat("sentinel-bmp", "BM", sentinelDecode, sentinelDecodeConfig)
	wuffs.RegisterFormats()

	payload := append([]byte("BM"), bytes.Repeat([]byte("x"), 32)...)
	img, format, err := image.Decode(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("Decode(sentinel BM): %v", err)
	}
	if format != "sentinel-bmp" {
		t.Fatalf("Decode format = %q, want sentinel-bmp", format)
	}
	rgba, ok := img.(*image.RGBA)
	if !ok || !rgba.Bounds().Eq(image.Rect(0, 0, 3, 2)) {
		t.Fatalf("Decode sentinel bounds/type: %T %v", img, img)
	}
	if len(rgba.Pix) < 4 || rgba.Pix[0] != 0x11 || rgba.Pix[1] != 0x22 || rgba.Pix[2] != 0x33 || rgba.Pix[3] != 0xFF {
		t.Fatalf("Decode sentinel pixel = %v, want 11 22 33 FF prefix", rgba.Pix[:4])
	}

	cfg, cfgFormat, err := image.DecodeConfig(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("DecodeConfig(sentinel BM): %v", err)
	}
	if cfgFormat != "sentinel-bmp" {
		t.Fatalf("DecodeConfig format = %q, want sentinel-bmp", cfgFormat)
	}
	wantCfg := image.Config{ColorModel: color.RGBAModel, Width: 3, Height: 2}
	if cfg != wantCfg {
		t.Fatalf("DecodeConfig = %+v, want %+v", cfg, wantCfg)
	}

	png := readFixture(t, "testdata/bricks-color.png")
	img, format, err = image.Decode(bytes.NewReader(png))
	if err != nil {
		t.Fatalf("image.Decode after RegisterFormats: %v", err)
	}
	if format != "png" {
		t.Fatalf("image.Decode format = %q, want png", format)
	}
	rgba, ok = img.(*image.RGBA)
	if !ok {
		t.Fatalf("image.Decode returned %T, want *image.RGBA", img)
	}
	ref, _, err := wuffs.Decode(png)
	if err != nil {
		t.Fatalf("wuffs.Decode: %v", err)
	}
	if !rgba.Bounds().Eq(ref.Bounds()) || rgba.Stride != ref.Stride || !bytes.Equal(rgba.Pix, ref.Pix) {
		t.Fatal("decoded pixels differ from wuffs.Decode output")
	}
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

func readFixture(t *testing.T, rel string) []byte {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	path := filepath.Join(root, rel)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return b
}
