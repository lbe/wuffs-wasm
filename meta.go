package wuffs

import "encoding/binary"

// Meta holds the decoded image metadata written by the guest into the meta slot.
// The guest writes metadata (err, width, height, stride, bytes_written, format)
// into the meta slot after a successful or partial decode. The host reads it
// back via ReadMeta.
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
	// Format is the decoded image format FourCC (e.g. FormatPNG, FormatWEBP),
	// written by the guest into the meta slot. Zero until populated.
	Format uint32
}

const (
	// FormatPNG is the FourCC for PNG images (WUFFS_BASE__FOURCC__PNG).
	FormatPNG uint32 = 0x504E4720
	// FormatWEBP is the FourCC for WebP images (WUFFS_BASE__FOURCC__WEBP).
	FormatWEBP uint32 = 0x57454250
)

// ReadMeta reads the Meta struct from the guest meta slot at the given offset
// in wasm linear memory. The meta slot is 24 bytes (6 uint32 fields):
// err, width, height, stride, bytes_written, format.
func ReadMeta(mem []byte, metaOff uint32) Meta {
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
