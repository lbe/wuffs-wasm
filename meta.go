package wuffs

import "encoding/binary"

// Meta holds the decoded image metadata written by the guest into the meta slot.
// The guest writes metadata (err, width, height, stride, bytes_written) into
// the meta slot after a successful or partial decode. The host reads it back
// via ReadMeta.
//
// Memory layout (little-endian uint32):
//
//	[0]  err
//	[4]  width
//	[8]  height
//	[12] stride
//	[16] bytes_written
type Meta struct {
	Err          int32
	Width        uint32
	Height       uint32
	Stride       uint32
	BytesWritten uint32
}

// ReadMeta reads the Meta struct from the guest meta slot at the given offset
// in wasm linear memory. The meta slot is 20 bytes (5 uint32 fields).
func ReadMeta(mem []byte, metaOff uint32) Meta {
	if int(metaOff)+20 > len(mem) {
		return Meta{}
	}
	slot := mem[metaOff : metaOff+20]
	return Meta{
		Err:          int32(binary.LittleEndian.Uint32(slot[0:4])),
		Width:        binary.LittleEndian.Uint32(slot[4:8]),
		Height:       binary.LittleEndian.Uint32(slot[8:12]),
		Stride:       binary.LittleEndian.Uint32(slot[12:16]),
		BytesWritten: binary.LittleEndian.Uint32(slot[16:20]),
	}
}
