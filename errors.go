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

	// ErrDstTooSmall is the sentinel matched by (*DstTooSmallError).Is.
	// It identifies a destination that is too small to hold the decoded
	// image, whether the host layout (Stride/Pix) or the guest scratch
	// (dst slot) is the limiting factor.
	ErrDstTooSmall = fmt.Errorf("wuffs: destination buffer too small")

	// ErrBadImage is returned when the caller's dst Rect, Stride, or Pix
	// does not match the decoded image (wrong size or shape, not merely
	// too small to hold the pixels).
	ErrBadImage = fmt.Errorf("wuffs: dst Rect/Stride/Pix do not match decoded image")
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
//
// It is produced for two distinct conditions, both identified by
// errors.Is(err, ErrDstTooSmall):
//
//   - Host layout errors: the caller's dst.Stride or dst.Pix cannot hold the
//     decoded image. Stride then describes the required host layout (the
//     tight row stride width*4 for a too-small stride, or the caller's own
//     dst.Stride for short Pix), and MinBytes is Stride*Height. Stride is NOT
//     necessarily the guest meta value in this case.
//   - Guest scratch errors: the guest destination slot (dst slot) was
//     exhausted. Stride is the guest metadata requirement
//     (guest-reported stride), MinBytes is the guest-reported stride*height,
//     and Width/Height come from the guest meta slot.
//
// In both cases Width and Height are the decoded image dimensions in pixels.
type DstTooSmallError struct {
	// MinBytes is the minimum number of bytes required for the destination buffer.
	// For host layout errors it is the required Stride*Height in host bytes.
	// For guest scratch errors it is the guest-reported stride*height.
	MinBytes uint32

	// Width is the decoded image width in pixels, as reported by the guest
	// meta slot when the destination buffer is too small.
	Width uint32

	// Height is the decoded image height in pixels, as reported by the guest
	// meta slot when the destination buffer is too small.
	Height uint32

	// Stride is the required row stride in bytes.
	//
	// For host layout errors (short Pix or insufficient stride) it describes
	// the required host layout: width*4 for a too-small stride, or the
	// caller's dst.Stride for short Pix. It is NOT always the guest meta value.
	//
	// For guest scratch (dst slot) exhaustion it is the guest metadata
	// requirement (the guest-reported stride).
	Stride uint32
}

// Is reports whether this error matches the given target. It returns true only
// when target is ErrDstTooSmall, so a *DstTooSmallError is never mistaken for
// ErrBadImage or any other sentinel, while its structured fields remain
// extractable via errors.As.
func (e *DstTooSmallError) Is(target error) bool {
	return target == ErrDstTooSmall
}

func (e *DstTooSmallError) Error() string {
	return fmt.Sprintf("wuffs: destination buffer too small, need %d bytes", e.MinBytes)
}
