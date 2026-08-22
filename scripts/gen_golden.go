// gen_golden.go generates the CRC32 golden file for testdata/bricks-color.png.
//
// Run from the project root:
//
//	go run scripts/gen_golden.go
package main

import (
	"fmt"
	"hash/crc32"
	"image"
	"os"
	"path/filepath"

	"github.com/lbe/wuffs-wasm"
)

func main() {
	pngPath := filepath.Join("testdata", "bricks-color.png")
	pngSrc, err := os.ReadFile(pngPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading %s: %v\n", pngPath, err)
		os.Exit(1)
	}

	dst := image.NewRGBA(image.Rect(0, 0, 0, 0))
	d := wuffs.New()

	meta, err := d.DecodeRGBA(dst, pngSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DecodeRGBA: %v\n", err)
		os.Exit(1)
	}
	if meta == nil {
		fmt.Fprintln(os.Stderr, "DecodeRGBA returned nil Meta")
		os.Exit(1)
	}

	crc := crc32.ChecksumIEEE(dst.Pix)

	goldenPath := filepath.Join("testdata", "bricks-color.golden.crc32")
	if err := os.WriteFile(goldenPath, []byte(fmt.Sprintf("%d\n", crc)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "writing %s: %v\n", goldenPath, err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s (CRC32: %08X)\n", goldenPath, crc)
}
