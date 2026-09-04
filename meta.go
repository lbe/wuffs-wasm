package wuffs

import "encoding/binary"

// Meta holds the decoded image metadata written by the guest into the meta slot.
// The guest writes metadata (err, width, height, stride, bytes_written, format)
// into the meta slot after a successful or partial decode. The host reads it
// back via readMeta.
//
// Memory layout (little-endian uint32):
//
//	[0]  err
//	[4]  width
//	[8]  height
//	[12] stride
//	[16] bytes_written
//	[20] format
type Meta struct {
	Err          int32
	Width        uint32
	Height       uint32
	Stride       uint32
	BytesWritten uint32
	// Format is the decoded image format FourCC (e.g. FormatPNG, FormatWEBP,
	// FormatBMP, FormatGIF, FormatJPEG, FormatNPBM, FormatQOI, FormatTGA,
	// FormatWBMP, FormatETC2, FormatHNSM, FormatNIE, FormatTH), written by
	// the guest into the meta slot. Zero until populated.
	Format uint32
}

const (
	// FormatPNG is the FourCC for PNG images (WUFFS_BASE__FOURCC__PNG).
	FormatPNG uint32 = 0x504E4720
	// FormatWEBP is the FourCC for WebP images (WUFFS_BASE__FOURCC__WEBP).
	FormatWEBP uint32 = 0x57454250
	// FormatBMP is the FourCC for BMP images (WUFFS_BASE__FOURCC__BMP).
	FormatBMP uint32 = 0x424D5020
	// FormatGIF is the FourCC for GIF images (WUFFS_BASE__FOURCC__GIF).
	FormatGIF uint32 = 0x47494620
	// FormatJPEG is the FourCC for JPEG images (WUFFS_BASE__FOURCC__JPEG).
	FormatJPEG uint32 = 0x4A504547
	// FormatNPBM is the FourCC for Netpbm (PBM/PGM/PPM) images
	// (WUFFS_BASE__FOURCC__NPBM).
	FormatNPBM uint32 = 0x4E50424D
	// FormatQOI is the FourCC for QOI images (WUFFS_BASE__FOURCC__QOI).
	FormatQOI uint32 = 0x514F4920
	// FormatTGA is the FourCC for TGA images (WUFFS_BASE__FOURCC__TGA).
	FormatTGA uint32 = 0x54474120
	// FormatWBMP is the FourCC for WBMP images (WUFFS_BASE__FOURCC__WBMP).
	FormatWBMP uint32 = 0x57424D50
	// FormatETC2 is the FourCC for ETC2 images (WUFFS_BASE__FOURCC__ETC2).
	FormatETC2 uint32 = 0x45544332
	// FormatHNSM is the FourCC for Handsum images (WUFFS_BASE__FOURCC__HNSM).
	FormatHNSM uint32 = 0x484E534D
	// FormatNIE is the FourCC for NIE images (WUFFS_BASE__FOURCC__NIE).
	FormatNIE uint32 = 0x4E494520
	// FormatTH is the FourCC for ThumbHash images (WUFFS_BASE__FOURCC__TH).
	FormatTH uint32 = 0x54482020
)

// readMeta reads the Meta struct from the guest meta slot at the given offset
// in wasm linear memory. The meta slot is 24 bytes (6 uint32 fields):
// err, width, height, stride, bytes_written, format.
//
// It is an internal implementation detail of guest scratch management, not
// part of the public API.
func readMeta(mem []byte, metaOff uint32) Meta {
	if int(metaOff)+24 > len(mem) {
		return Meta{}
	}
	slot := mem[metaOff : metaOff+24]
	return Meta{
		Err:          int32(binary.LittleEndian.Uint32(slot[0:4])),
		Width:        binary.LittleEndian.Uint32(slot[4:8]),
		Height:       binary.LittleEndian.Uint32(slot[8:12]),
		Stride:       binary.LittleEndian.Uint32(slot[12:16]),
		BytesWritten: binary.LittleEndian.Uint32(slot[16:20]),
		Format:       binary.LittleEndian.Uint32(slot[20:24]),
	}
}
