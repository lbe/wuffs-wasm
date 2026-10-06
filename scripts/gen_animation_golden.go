//go:build ignore

// gen_animation_golden.go generates animation golden manifests recording
// the CRC32-IEEE of each decoded animation frame's full-canvas straight RGBA
// pixels.
//
// Run from the project root:
//
//	go run ./scripts/gen_animation_golden.go
//
// Manifest line format (stable): "<file> <index> <crc32-hex, 8 digits>".
// Each CRC covers the full dst.Pix canvas of a fresh zeroed image.RGBA
// (Probe geometry) after DecodeFrame(dst, src, index).
//
// The writer is structured for extension: writeManifest takes a fixture
// name and frame count. The GIF manifest (Task 6) and the NIE manifest
// (Task 8) are both emitted.
package main

import (
	"fmt"
	"hash/crc32"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/lbe/wuffs-wasm"
)

// animationFixture selects one manifest's source file and frame count.
type animationFixture struct {
	file   string
	frames int
}

// gifFixtures is the Task 6 verified GIF multi-frame set.
var gifFixtures = []animationFixture{
	{file: "muybridge.gif", frames: 15},
	{file: "animated-red-blue.gif", frames: 4},
}

// nieFixtures is the Task 8 verified NIE nïA multi-frame set.
var nieFixtures = []animationFixture{
	{file: "animated-red-blue.nia", frames: 4},
}

func main() {
	if err := writeManifest("gif.animation.golden.manifest", gifFixtures); err != nil {
		fmt.Fprintf(os.Stderr, "gen_animation_golden: %v\n", err)
		os.Exit(1)
	}
	if err := writeManifest("nie.animation.golden.manifest", nieFixtures); err != nil {
		fmt.Fprintf(os.Stderr, "gen_animation_golden: %v\n", err)
		os.Exit(1)
	}
}

// writeManifest decodes every frame of each fixture with a fresh Decoder,
// a Probe-sized fresh zeroed image.RGBA per frame, and records the
// full-canvas CRC32-IEEE in the stable line format.
func writeManifest(manifest string, fixtures []animationFixture) error {
	var sb strings.Builder
	sb.WriteString("# Animation golden manifest. Each line records the CRC-32 (IEEE)\n")
	sb.WriteString("# of the full-canvas straight RGBA bytes in the caller's image.RGBA\n")
	sb.WriteString("# Pix after DecodeFrame(dst, src, index) on a fresh zeroed canvas\n")
	sb.WriteString("# sized from Probe geometry. Format: \"<file> <index> <crc32-hex>\".\n")
	for _, fx := range fixtures {
		src, err := os.ReadFile(filepath.Join("testdata", fx.file))
		if err != nil {
			return fmt.Errorf("reading testdata/%s: %w", fx.file, err)
		}
		d := wuffs.New()
		meta, err := d.Probe(src)
		if err != nil {
			return fmt.Errorf("Probe(%s): %w", fx.file, err)
		}
		if meta == nil || meta.Width == 0 || meta.Height == 0 {
			return fmt.Errorf("Probe(%s): nil or empty Meta", fx.file)
		}
		if n, err := d.FrameCount(src); err != nil || n != fx.frames {
			return fmt.Errorf("FrameCount(%s) = %d, %v; want %d", fx.file, n, err, fx.frames)
		}
		if err := d.Reserve(int(meta.Stride)*int(meta.Height), len(src)); err != nil {
			return fmt.Errorf("Reserve(%s): %w", fx.file, err)
		}
		w, h := int(meta.Width), int(meta.Height)
		for i := 0; i < fx.frames; i++ {
			dst := image.NewRGBA(image.Rect(0, 0, w, h))
			if _, err := d.DecodeFrame(dst, src, i); err != nil {
				return fmt.Errorf("DecodeFrame(%s, %d): %w", fx.file, i, err)
			}
			fmt.Fprintf(&sb, "%s %d %08X\n", fx.file, i, crc32.ChecksumIEEE(dst.Pix))
		}
	}
	path := filepath.Join("testdata", manifest)
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Printf("wrote %s\n", path)
	return nil
}
