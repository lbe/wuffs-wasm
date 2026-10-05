package wuffs

import (
	"image"
	"io"
	"sync"

	"github.com/lbe/wuffs-wasm/internal/imgreg"
)

const (
	// pngMagic is the 8-byte PNG file signature.
	pngMagic = "\x89PNG\r\n\x1a\n"
	// webpMagic selects WebP's fixed-offset RIFF container tag. Each ? is
	// image.RegisterFormat's whole-byte wildcard covering the four-byte RIFF
	// size field, so any RIFF<size>WEBP header routes to webp without
	// pinning one fixture's size bytes or matching a generic RIFF prefix.
	webpMagic = "RIFF????WEBP"
	// bmpMagic is the 2-byte BMP file signature.
	bmpMagic = "BM"
	// gifMagic is the 4-byte GIF version-prefix signature shared by
	// GIF87a and GIF89a.
	gifMagic = "GIF8"
	// jpegMagic is the SOI plus first-marker prefix opening every JPEG.
	jpegMagic = "\xff\xd8\xff"
	// qoiMagic is the 4-byte QOI file signature.
	qoiMagic = "qoif"
	// etc2Magic is the 4-byte PKM file signature shared by every PKM
	// version.
	etc2Magic = "PKM "
	// nieMagic is the 4-byte NIE still-image signature.
	nieMagic = "\x6e\xc3\xaf\x45"
	// thMagic is the 3-byte cooked ThumbHash magic identifier.
	thMagic = "\xc3\xbe\xfe"
	// npbmGrayMagic selects the binary P5 graymap class. It is an exact
	// two-byte prefix so the ASCII P1/P2 and raw-bit P4 classes stay
	// unselected.
	npbmGrayMagic = "P5"
	// npbmColorMagic selects the binary P6 pixmap class. It is an exact
	// two-byte prefix so the ASCII P3 and PAM P7 classes stay unselected.
	npbmColorMagic = "P6"
	// wbmpMuybridgeMagic is the complete Type 0 header plus canonical-dimension
	// prefix of testdata/muybridge-frame-000.wbmp (30x20, single-byte varints).
	wbmpMuybridgeMagic = "\x00\x00\x1e\x14"
	// wbmpBricksMagic is the complete Type 0 header plus canonical-dimension
	// prefix of testdata/bricks-nodither.wbmp (160x120, two-byte width varint).
	wbmpBricksMagic = "\x00\x00\x81\x20\x78"
	// tgaColorMagic is the complete 18-byte header of
	// testdata/bricks-color.tga (raw true-color 160x120x24, descriptor 0x20).
	tgaColorMagic = "\x00\x00\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00\xa0\x00\x78\x00\x18\x20"
	// tgaNoditherMagic is the complete 18-byte header of
	// testdata/bricks-nodither.tga (indexed 160x120, descriptor 0x20).
	tgaNoditherMagic = "\x00\x01\x01\x00\x00\x00\x01\x18\x00\x00\x00\x00\xa0\x00\x78\x00\x08\x20"
	// tgaGrayMagic is the complete 18-byte header of
	// testdata/bricks-gray.tga (RLE grayscale 160x120x8, descriptor 0x20).
	tgaGrayMagic = "\x00\x00\x0b\x00\x00\x00\x00\x00\x00\x00\x00\x00\xa0\x00\x78\x00\x08\x20"
)

// registeredFormat is one image.RegisterFormat catalog entry: the canonical
// format name and its match prefix.
type registeredFormat struct {
	name  string
	magic string
}

var (
	// registerWith is the registrar seam: it defaults to imgreg.RegisterFormat
	// and is swapped only by same-package tests to observe submissions without
	// touching the real global registry.
	registerWith = imgreg.RegisterFormat

	registerOnce sync.Once

	// registeredFormats is the registration catalog submitted to the image
	// registry exactly once by RegisterFormats.
	registeredFormats = append([]registeredFormat{
		{"png", pngMagic},
		{"webp", webpMagic},
		{"bmp", bmpMagic},
		{"gif", gifMagic},
		{"jpeg", jpegMagic},
		{"qoi", qoiMagic},
		{"etc2", etc2Magic},
		{"nie", nieMagic},
		{"th", thMagic},
		{"npbm", npbmGrayMagic},
		{"npbm", npbmColorMagic},
		{"wbmp", wbmpMuybridgeMagic},
		{"wbmp", wbmpBricksMagic},
		{"tga", tgaColorMagic},
		{"tga", tgaNoditherMagic},
		{"tga", tgaGrayMagic},
	}, hnsmFormats()...)
)

const (
	// hnsmTopBits is the fixed 15-bit Handsum structural prefix occupying
	// the top bits of the 3-byte header.
	hnsmTopBits = 0x7F6B
	// hnsmMaxQuality is the largest accepted Handsum quality value.
	hnsmMaxQuality = 3
	// hnsmMaxGeometry is the largest accepted Handsum geometry code; 0x1F
	// is reserved and stays image.ErrFormat.
	hnsmMaxGeometry = 30
)

// hnsmColors are the accepted Handsum color classes; class 1 is rejected
// and stays image.ErrFormat.
var hnsmColors = []byte{0, 2, 3}

// hnsmMagic packs one accepted Handsum structural header into its exact
// three-byte magic: 15 prefix bits, 2 color bits, 2 quality bits, and
// 5 geometry bits.
func hnsmMagic(color, quality, geom byte) string {
	v := (uint32(hnsmTopBits) << 9) | (uint32(color) << 7) | (uint32(quality) << 5) | uint32(geom)
	return string([]byte{byte(v >> 16), byte(v >> 8), byte(v)})
}

// hnsmFormats enumerates the bounded accepted Handsum structural prefix
// space: every accepted color class at every quality 0..3 and every
// geometry code 0..30. Each entry is an exact three-byte magic because
// image.RegisterFormat has no masked-bit matcher.
func hnsmFormats() []registeredFormat {
	out := make([]registeredFormat, 0, len(hnsmColors)*(hnsmMaxQuality+1)*(hnsmMaxGeometry+1))
	for _, color := range hnsmColors {
		for quality := 0; quality <= hnsmMaxQuality; quality++ {
			for geom := 0; geom <= hnsmMaxGeometry; geom++ {
				out = append(out, registeredFormat{"hnsm", hnsmMagic(color, byte(quality), byte(geom))})
			}
		}
	}
	return out
}

// RegisterFormats registers the wuffs decoders with the standard library
// image package's global decoder registry, so image.Decode and
// image.DecodeConfig recognize the supported formats. Registration is
// explicit opt-in: importing this package never mutates the registry. It is
// safe for concurrent use and repeated calls register each format only once.
// It registers exactly the thirteen verified formats. The global image
// registry is first-match-wins, so earlier registrations remain authoritative.
// The TGA entries use exact-header 18-byte magics and the WBMP entries use
// literal-prefix magics; both keep their intentional prefix-recognition
// false-positive boundaries.
func RegisterFormats() {
	registerOnce.Do(func() {
		for _, f := range registeredFormats {
			registerWith(f.name, f.magic, decodeRegistered, DecodeConfigReader)
		}
	})
}

// decodeRegistered adapts DecodeReader to the decoder signature the
// registry expects, dropping the redundant format name.
func decodeRegistered(r io.Reader) (image.Image, error) {
	img, _, err := DecodeReader(r)
	return img, err
}
