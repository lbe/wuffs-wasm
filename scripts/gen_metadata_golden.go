//go:build ignore

// gen_metadata_golden.go generates testdata/metadata-full.gif.metadata.golden,
// pinning the GIF ICCP and XMP application-extension blobs reported by
// (*Decoder).Metadata on testdata/artificial-gif/metadata-full.gif.
//
// Run from the project root:
//
//	go run ./scripts/gen_metadata_golden.go
//
// Golden format (binary): uint32 little-endian len(ICC), uint32
// little-endian len(XMP), then ICC bytes, then XMP bytes.
//
// Oracle: ../wuffs/test/c/std/gif.c do_test_wuffs_gif_decode_metadata
// (ICCP 8 bytes, XMP 10 bytes).
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lbe/wuffs-wasm"
)

func main() {
	src, err := os.ReadFile(filepath.Join("testdata", "artificial-gif", "metadata-full.gif"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen_metadata_golden: reading metadata-full.gif: %v\n", err)
		os.Exit(1)
	}
	d := wuffs.New()
	if err := d.Reserve(65536, len(src)); err != nil {
		fmt.Fprintf(os.Stderr, "gen_metadata_golden: Reserve: %v\n", err)
		os.Exit(1)
	}
	md, err := d.Metadata(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen_metadata_golden: Metadata: %v\n", err)
		os.Exit(1)
	}
	out := make([]byte, 0, 8+len(md.ICC)+len(md.XMP))
	var hdr [8]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(len(md.ICC)))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(md.XMP)))
	out = append(out, hdr[:]...)
	out = append(out, md.ICC...)
	out = append(out, md.XMP...)
	path := filepath.Join("testdata", "metadata-full.gif.metadata.golden")
	if err := os.WriteFile(path, out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "gen_metadata_golden: writing %s: %v\n", path, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (icc %d bytes, xmp %d bytes)\n", path, len(md.ICC), len(md.XMP))
}
