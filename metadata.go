package wuffs

import (
	"encoding/binary"
	"errors"
	"time"
	"unsafe"
)

// metadataPackHeaderSize is the guest metadata pack header size in bytes.
// It mirrors wuffs_wasm_metadata_pack_header in wasm/shim.c; the size
// contract is pinned by TestUnitMetadataPackHeaderSize88.
const metadataPackHeaderSize = 88

// metadataPackHeader mirrors wuffs_wasm_metadata_pack_header in wasm/shim.c
// field-for-field so unsafe.Sizeof pins the 88-byte guest layout. Host
// decodes the little-endian pack into this struct via
// metadataPackHeaderFromLE (see Metadata).
type metadataPackHeader struct {
	err         int32
	format      uint32
	exifLen     uint32
	iccLen      uint32
	xmpLen      uint32
	hasGamma    uint8
	hasChrm     uint8
	hasSRGB     uint8
	hasModTime  uint8
	_           [3]uint8
	gamaScaled  uint32
	chrm        [8]int32
	srgbIntent  uint32
	_           [4]uint8
	modTimeSec  int64
	modTimeNsec int32
	_           [4]uint8
}

// metadataPackHeaderSizeCheck keeps unsafe referenced when tests are not
// compiled; the real size pin is TestUnitMetadataPackHeaderSize88.
var _ = unsafe.Sizeof(metadataPackHeader{})

// metadataPackHeaderFromLE decodes the 88-byte little-endian guest pack
// header at the offsets in wasm/shim.c (err @0, format @4, exifLen @8,
// iccLen @12, xmpLen @16, has flags @20..23, gamaScaled @28, chrm @32..64,
// srgbIntent @64, modTimeSec @72, modTimeNsec @80).
func metadataPackHeaderFromLE(hdr []byte) (metadataPackHeader, error) {
	var pack metadataPackHeader
	if len(hdr) < metadataPackHeaderSize {
		return pack, errors.New("wuffs: metadata pack header too short")
	}
	pack.err = int32(binary.LittleEndian.Uint32(hdr[0:4]))
	pack.format = binary.LittleEndian.Uint32(hdr[4:8])
	pack.exifLen = binary.LittleEndian.Uint32(hdr[8:12])
	pack.iccLen = binary.LittleEndian.Uint32(hdr[12:16])
	pack.xmpLen = binary.LittleEndian.Uint32(hdr[16:20])
	pack.hasGamma = hdr[20]
	pack.hasChrm = hdr[21]
	pack.hasSRGB = hdr[22]
	pack.hasModTime = hdr[23]
	pack.gamaScaled = binary.LittleEndian.Uint32(hdr[28:32])
	for i := range pack.chrm {
		pack.chrm[i] = int32(binary.LittleEndian.Uint32(hdr[32+4*i : 36+4*i]))
	}
	pack.srgbIntent = binary.LittleEndian.Uint32(hdr[64:68])
	pack.modTimeSec = int64(binary.LittleEndian.Uint64(hdr[72:80]))
	pack.modTimeNsec = int32(binary.LittleEndian.Uint32(hdr[80:84]))
	return pack, nil
}

// Chromaticities holds CIE xy chromaticity coordinates reported by a cHRM
// chunk or equivalent metadata. Zero when absent (see HasChromaticities).
type Chromaticities struct {
	WhiteX, WhiteY float64
	RedX, RedY     float64
	GreenX, GreenY float64
	BlueX, BlueY   float64
}

// Metadata holds opt-in ancillary metadata (EXIF, ICC, XMP, gamma,
// chromaticities, sRGB, modification time) decoded without pixels.
// Absent fields are zero / nil.
type Metadata struct {
	// Format is the image FourCC of the file these blobs came from.
	Format uint32
	// EXIF is the raw TIFF/EXIF payload; nil if absent.
	EXIF []byte
	// ICC is the raw ICC profile; nil if absent.
	ICC []byte
	// XMP is the raw XMP; nil if absent.
	XMP []byte
	// HasGamma reports whether Gamma holds the file gamma.
	HasGamma bool
	// Gamma is the file gamma when HasGamma.
	Gamma float64
	// HasChromaticities reports whether Chromaticities holds parsed values.
	HasChromaticities bool
	// Chromaticities holds parsed chromaticities when HasChromaticities.
	Chromaticities Chromaticities
	// HasSRGB reports whether SRGB holds the rendering intent.
	HasSRGB bool
	// SRGB is the Wuffs SRGB rendering-intent FourCC payload as reported.
	SRGB uint32
	// HasModTime reports whether ModTime holds the modification time.
	// Many inputs never deliver MTIM; false and zero time mean absent.
	HasModTime bool
	// ModTime is the modification time from MTIM when HasModTime (UTC).
	ModTime time.Time
}

const (
	// MetaEXIF is the FourCC for EXIF metadata ("EXIF").
	MetaEXIF uint32 = 0x45584946
	// MetaICCP is the FourCC for ICC profile metadata ("ICCP").
	MetaICCP uint32 = 0x49434350
	// MetaXMP is the FourCC for XMP metadata ("XMP ").
	MetaXMP uint32 = 0x584D5020
	// MetaGAMA is the FourCC for gamma metadata ("GAMA").
	MetaGAMA uint32 = 0x47414D41
	// MetaCHRM is the FourCC for chromaticity metadata ("CHRM").
	MetaCHRM uint32 = 0x4348524D
	// MetaSRGB is the FourCC for sRGB metadata ("SRGB").
	MetaSRGB uint32 = 0x53524742
	// MetaMTIM is the FourCC for modification-time metadata ("MTIM").
	MetaMTIM uint32 = 0x4D54494D
)

// minimumMetadataDstSlotBytes is the guest destination scratch capacity
// Metadata requires for its pack buffer. It never auto-Reserves.
const minimumMetadataDstSlotBytes = 64 * 1024 // 64 KiB

// Metadata is an opt-in read of EXIF, ICC, XMP, gamma, chromaticities,
// sRGB, and modification time. Absent fields are zero / nil. It does not
// decode pixels and follows the same src cap rules as Probe
// (checkSrcCapacity: oversized src yields ErrSrcTooLarge, empty src yields
// ErrDecode, unrecognized header yields ErrUnknownFormat).
//
// It follows the Probe call pattern: checkSrcCapacity, copySrcToSlot, then
// the guest wuffs_read_image_metadata export writes a pack into the guest
// destination scratch slot (header + EXIF/ICC/XMP blobs, copied into fresh Go
// heap slices). Metadata never calls Reserve or memory.Grow: the dst slot
// must already hold at least 64 KiB, else ErrDecode. Gamma uses
// 100000/gama_scaled; chromaticities divide Wuffs int32×100000 scalars by
// 100000; SRGB carries the Wuffs intent uint32 as reported.
//
// On success the returned *Metadata aliases the Decoder's lastMetadata until
// the next successful Metadata call. EXIF, ICC, and XMP slices are fresh heap
// copies safe to retain.
func (d *Decoder) Metadata(src []byte) (*Metadata, error) {
	if err := d.checkSrcCapacity(src); err != nil {
		return nil, err
	}

	lay := d.currentLayout
	d.copySrcToSlot(lay, src)

	if lay.dstLen < minimumMetadataDstSlotBytes {
		return nil, ErrDecode
	}

	// Zero the pack header region at the dst scratch slot before the export.
	mem := d.module.Xmemory()
	memBytes := *mem.Slice()
	if uint64(lay.dstOff)+metadataPackHeaderSize > uint64(len(memBytes)) {
		return nil, ErrDecode
	}
	for i := uint32(0); i < metadataPackHeaderSize; i++ {
		memBytes[lay.dstOff+i] = 0
	}

	ret := d.module.Xwuffs_read_image_metadata(
		int32(lay.srcOff), int32(len(src)),
		int32(lay.dstOff), int32(lay.dstLen),
	)
	if ret != 0 {
		return nil, errFromGuestReturn(ret)
	}

	// Refresh memory view after guest call (guest may have grown memory).
	memBytes = *d.module.Xmemory().Slice()
	if uint64(lay.dstOff)+metadataPackHeaderSize > uint64(len(memBytes)) {
		return nil, ErrDecode
	}
	hdr := memBytes[lay.dstOff : lay.dstOff+metadataPackHeaderSize]
	pack, err := metadataPackHeaderFromLE(hdr)
	if err != nil {
		return nil, ErrDecode
	}
	if pack.err != 0 {
		return nil, ErrDecode
	}
	exifLen := pack.exifLen
	iccLen := pack.iccLen
	xmpLen := pack.xmpLen
	total := uint64(metadataPackHeaderSize) + uint64(exifLen) + uint64(iccLen) + uint64(xmpLen)
	if uint64(lay.dstOff)+total > uint64(len(memBytes)) {
		return nil, ErrDecode
	}

	// Blobs follow the header in fixed order: EXIF, then ICC, then XMP.
	// Each non-empty blob is copied into a fresh Go heap slice.
	off := lay.dstOff + metadataPackHeaderSize
	var exif, icc, xmp []byte
	if exifLen > 0 {
		exif = make([]byte, exifLen)
		copy(exif, memBytes[off:off+exifLen])
		off += exifLen
	}
	if iccLen > 0 {
		icc = make([]byte, iccLen)
		copy(icc, memBytes[off:off+iccLen])
		off += iccLen
	}
	if xmpLen > 0 {
		xmp = make([]byte, xmpLen)
		copy(xmp, memBytes[off:off+xmpLen])
	}

	md := Metadata{
		Format:            pack.format,
		EXIF:              exif,
		ICC:               icc,
		XMP:               xmp,
		HasGamma:          pack.hasGamma != 0,
		HasChromaticities: pack.hasChrm != 0,
		HasSRGB:           pack.hasSRGB != 0,
		HasModTime:        pack.hasModTime != 0,
	}
	if md.HasGamma {
		md.Gamma = 100000.0 / float64(pack.gamaScaled)
	}
	if md.HasChromaticities {
		v := [8]float64{}
		for i := range v {
			v[i] = float64(pack.chrm[i]) / 100000.0
		}
		md.Chromaticities = Chromaticities{
			WhiteX: v[0], WhiteY: v[1],
			RedX: v[2], RedY: v[3],
			GreenX: v[4], GreenY: v[5],
			BlueX: v[6], BlueY: v[7],
		}
	}
	if md.HasSRGB {
		md.SRGB = pack.srgbIntent
	}
	if md.HasModTime {
		md.ModTime = time.Unix(pack.modTimeSec, int64(pack.modTimeNsec)).UTC()
	}
	d.lastMetadata = md
	return &d.lastMetadata, nil
}
