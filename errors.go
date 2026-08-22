package wuffs

import "fmt"

// Guest decode return codes. These are the non-zero values returned by
// wuffs_decode_image and mapped to host-facing errors below.
const (
	// guestErrUnknownFormat means the source data does not match any
	// recognized image format header.
	guestErrUnknownFormat int32 = -1

	// guestErrDstTooSmall means the destination buffer is too small for the
	// decoded image.
	guestErrDstTooSmall int32 = -2
)

// Sentinel errors returned by the decoder.
//
// Guest code mapping:
//
//	-1 → ErrUnknownFormat  (unrecognized image format)
//	-2 → DstTooSmallError  (destination buffer too small for decoded image)
//	-3 → ErrDecode         (general decode failure)
//	-4 → ErrDecode         (zero src_len, bad meta_off, or general decode failure)
//
// ErrSrcTooLarge is host-side only: it is returned before the guest call when
// the source exceeds the decoder's src-slot capacity.
var (
	// ErrUnknownFormat is returned when the source data does not match any
	// recognized image format header.
	ErrUnknownFormat = fmt.Errorf("wuffs: unknown image format")

	// ErrSrcTooLarge is returned when the source data exceeds the host
	// src-slot capacity configured via Reserve.
	ErrSrcTooLarge = fmt.Errorf("wuffs: source data too large for src slot")

	// ErrDecode is returned on general decode failures, including zero
	// src_len or bad meta_off values.
	ErrDecode = fmt.Errorf("wuffs: decode error")
)

// errFromGuestReturn maps a non-zero guest decode return code to a host error.
func errFromGuestReturn(ret int32) error {
	switch ret {
	case guestErrUnknownFormat:
		return ErrUnknownFormat
	case guestErrDstTooSmall:
		return &DstTooSmallError{}
	default:
		return ErrDecode
	}
}

// DstTooSmallError is a structured error carrying the minimum destination
// buffer size and image metadata required to decode the image.
type DstTooSmallError struct {
	// MinBytes is the minimum number of bytes required for the destination buffer.
	MinBytes uint32

	// Width is the decoded image width in pixels, as reported by the guest
	// meta slot when the destination buffer is too small.
	Width uint32

	// Height is the decoded image height in pixels, as reported by the guest
	// meta slot when the destination buffer is too small.
	Height uint32

	// Stride is the decoded image stride in bytes, as reported by the guest
	// meta slot when the destination buffer is too small.
	Stride uint32
}

func (e *DstTooSmallError) Error() string {
	return fmt.Sprintf("wuffs: destination buffer too small, need %d bytes", e.MinBytes)
}
