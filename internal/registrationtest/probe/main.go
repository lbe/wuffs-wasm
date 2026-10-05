//go:build ignore

// Command probe exercises the wuffs.RegisterFormats entry point in a process
// whose standard library image decoder registry is clean: it imports wuffs
// and image but no self-registering codec (image/png, image/jpeg, image/gif).
//
// The probe runs as a subprocess of TestIntegrationRegisterFormatsExplicitOptIn
// so that the global registry state is deterministic regardless of which other
// packages the parent test binary links in. The ignore build tag keeps this
// file out of ./... builds; the test runs it with an explicit file argument,
// which bypasses the tag.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"os"

	wuffs "github.com/lbe/wuffs-wasm"
)

func main() {
	fixture := flag.String("fixture", "", "path to a PNG fixture")
	flag.Parse()
	if err := run(*fixture); err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
		os.Exit(1)
	}
	fmt.Println("probe: ok")
}

func run(fixture string) error {
	src, err := os.ReadFile(fixture)
	if err != nil {
		return fmt.Errorf("read fixture: %w", err)
	}

	// Before RegisterFormats the registry must not know the fixture format.
	if _, _, err := image.Decode(bytes.NewReader(src)); !errors.Is(err, image.ErrFormat) {
		return fmt.Errorf("image.Decode before RegisterFormats: got %v, want image.ErrFormat", err)
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(src)); !errors.Is(err, image.ErrFormat) {
		return fmt.Errorf("image.DecodeConfig before RegisterFormats: got %v, want image.ErrFormat", err)
	}

	wuffs.RegisterFormats()

	img, format, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return fmt.Errorf("image.Decode after RegisterFormats: %w", err)
	}
	if format != "png" {
		return fmt.Errorf("image.Decode format = %q, want %q", format, "png")
	}
	rgba, ok := img.(*image.RGBA)
	if !ok {
		return fmt.Errorf("image.Decode returned %T, want *image.RGBA", img)
	}

	ref, _, err := wuffs.Decode(src)
	if err != nil {
		return fmt.Errorf("wuffs.Decode: %w", err)
	}
	if !rgba.Bounds().Eq(ref.Bounds()) {
		return fmt.Errorf("decoded bounds = %v, want %v", rgba.Bounds(), ref.Bounds())
	}
	if rgba.Stride != ref.Stride {
		return fmt.Errorf("decoded stride = %d, want %d", rgba.Stride, ref.Stride)
	}
	if !bytes.Equal(rgba.Pix, ref.Pix) {
		return errors.New("decoded pixels differ from wuffs.Decode output")
	}
	return nil
}
