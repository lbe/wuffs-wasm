package wuffs

import (
	"image"
	"image/color"
	"math"
)

const maxWuffsDimension = 0xFFFFFF // 16,777,215

// decoderOperations is the private seam used by the package-level allocating
// helpers. *Decoder satisfies it; same-package tests can substitute a fake.
type decoderOperations interface {
	Reserve(dstBytes, srcBytes int) error
	Probe(src []byte) (*Meta, error)
	DecodeRGBA(dst *image.RGBA, src []byte) (*Meta, error)
	DecodeNRGBA(dst *image.NRGBA, src []byte) (*Meta, error)
	DecodeGray(dst *image.Gray, src []byte) (*Meta, error)
}

// DecodeConfig returns the image configuration (color model and dimensions)
// from src without decoding pixels. It creates a temporary decoder via
// decodeConfigWithDecoder, automatically reserves the source slot to fit src,
// and maps the result to image.Config{ColorModel: color.RGBAModel} for
// compatible input. No destination image is allocated.
func DecodeConfig(src []byte) (image.Config, error) {
	return decodeConfigWithDecoder(src, New())
}

// decodeConfigWithDecoder is the unexported implementation of DecodeConfig. It
// accepts a decoder seam so that same-package tests can substitute a fake
// without shared mutable state. The common preparation (Reserve, Probe) is
// delegated to probeWithDecoder.
func decodeConfigWithDecoder(src []byte, ops decoderOperations) (image.Config, error) {
	meta, err := probeWithDecoder(src, ops)
	if err != nil {
		return image.Config{}, err
	}
	return image.Config{
		ColorModel: color.RGBAModel,
		Width:      int(meta.Width),
		Height:     int(meta.Height),
	}, nil
}

// Probe reports image dimensions and format from src without decoding pixels.
// It constructs a temporary Decoder, grows the source slot to len(src) with
// Reserve(0, len(src)), delegates to (*Decoder).Probe, and returns a detached
// Meta pointer that does not alias the Decoder's internal state.
func Probe(src []byte) (*Meta, error) {
	return probeWithDecoder(src, New())
}

// Decode allocates and decodes src into a tightly packed *image.RGBA and
// returns a detached *Meta that does not alias the internal Decoder state.
// It creates a temporary Decoder, reserves source and destination memory
// automatically, allocates the host image, and decodes pixels into it.
// Performance-sensitive callers should reuse a Decoder and destination via
// the (*Decoder).Probe, Reserve, and DecodeRGBA methods.
func Decode(src []byte) (*image.RGBA, *Meta, error) {
	return decodeWithAllocator(src, New(), nil)
}

// DecodeNRGBA allocates and decodes src into a tightly packed *image.NRGBA
// and returns a detached *Meta that does not alias the internal Decoder state.
// It creates a temporary Decoder, reserves source and destination memory
// automatically, allocates the host image, and decodes pixels into it.
// Performance-sensitive callers should reuse a Decoder and destination via
// the (*Decoder).Probe, Reserve, and DecodeNRGBA methods.
func DecodeNRGBA(src []byte) (*image.NRGBA, *Meta, error) {
	return decodeNRGBAWithAllocator(src, New(), nil)
}

// DecodeGray allocates and decodes src into a tightly packed *image.Gray and
// returns a detached *Meta that does not alias the internal Decoder state.
// It creates a temporary Decoder, reserves source and destination memory
// automatically, allocates the host image, and decodes pixels into it.
// The wasm destination slot remains four-byte-per-pixel BGRA scratch; the
// host converts it into one-byte grayscale pixels. Consequently, Meta.Stride
// retains the guest stride (Width*4), while Meta.BytesWritten reports the
// host bytes written (Width*Height).
// Performance-sensitive callers should reuse a Decoder and destination via
// the (*Decoder).Probe, Reserve, and DecodeGray methods.
func DecodeGray(src []byte) (*image.Gray, *Meta, error) {
	return decodeGrayWithAllocator(src, New(), image.NewGray)
}

// probeWithDecoder reserves a source slot sized to len(src), probes the source
// through ops, and returns a detached Meta pointer so repeated calls do not
// share state. Same-package tests can substitute a fake decoderOperations.
func probeWithDecoder(src []byte, ops decoderOperations) (*Meta, error) {
	if err := ops.Reserve(0, len(src)); err != nil {
		return nil, err
	}
	meta, err := ops.Probe(src)
	if err != nil {
		return nil, err
	}
	detached := *meta
	return &detached, nil
}

// decodePrep handles the common setup shared by decode-with-allocator paths:
// source Reserve, Probe, geometry validation (for 1 or 4 bytes-per-pixel
// host formats), and destination Reserve. It returns the prepared decoder
// operations, decoded dimensions, and the computed guest scratch length, or
// an error on any step.
func decodePrep(src []byte, ops decoderOperations, bytesPerPixel int) (decoderOperations, int, int, int, error) {
	if err := ops.Reserve(0, len(src)); err != nil {
		return nil, 0, 0, 0, err
	}

	meta, err := ops.Probe(src)
	if err != nil {
		return nil, 0, 0, 0, err
	}

	width := int(meta.Width)
	height := int(meta.Height)
	_, _, guestLen, err := validateAllocatingGeometry(width, height, bytesPerPixel)
	if err != nil {
		return nil, 0, 0, 0, err
	}

	if err = ops.Reserve(guestLen, len(src)); err != nil {
		return nil, 0, 0, 0, err
	}

	return ops, width, height, guestLen, nil
}

// decodeWithAllocator is the unexported implementation of Decode. It accepts
// a decoder seam and an RGBA allocator so that same-package tests can inject
// fakes without shared mutable state. The common preparation (Reserve, Probe,
// geometry validation) is delegated to decodePrep.
func decodeWithAllocator(src []byte, ops decoderOperations, newRGBA func(image.Rectangle) *image.RGBA) (*image.RGBA, *Meta, error) {
	d, width, height, _, err := decodePrep(src, ops, 4)
	if err != nil {
		return nil, nil, err
	}

	var dst *image.RGBA
	if newRGBA != nil {
		dst = newRGBA(image.Rect(0, 0, width, height))
	} else {
		dst = image.NewRGBA(image.Rect(0, 0, width, height))
	}

	decMeta, err := d.DecodeRGBA(dst, src)
	if err != nil {
		return nil, nil, err
	}

	detached := *decMeta
	return dst, &detached, nil
}

// decodeNRGBAWithAllocator is the unexported implementation of DecodeNRGBA. It accepts
// a decoder seam and an NRGBA allocator so that same-package tests can inject
// fakes without shared mutable state. The common preparation (Reserve, Probe,
// geometry validation) is delegated to decodePrep.
func decodeNRGBAWithAllocator(src []byte, ops decoderOperations, newNRGBA func(image.Rectangle) *image.NRGBA) (*image.NRGBA, *Meta, error) {
	d, width, height, _, err := decodePrep(src, ops, 4)
	if err != nil {
		return nil, nil, err
	}

	var dst *image.NRGBA
	if newNRGBA != nil {
		dst = newNRGBA(image.Rect(0, 0, width, height))
	} else {
		dst = image.NewNRGBA(image.Rect(0, 0, width, height))
	}

	decMeta, err := d.DecodeNRGBA(dst, src)
	if err != nil {
		return nil, nil, err
	}

	detached := *decMeta
	return dst, &detached, nil
}

// decodeGrayWithAllocator is the unexported implementation of DecodeGray. It
// accepts a decoder seam and a Gray allocator so that same-package tests can
// inject fakes without shared mutable state. The common preparation (Reserve,
// Probe, geometry validation) is delegated to decodePrep.
func decodeGrayWithAllocator(src []byte, ops decoderOperations, newGray func(image.Rectangle) *image.Gray) (*image.Gray, *Meta, error) {
	d, width, height, _, err := decodePrep(src, ops, 1)
	if err != nil {
		return nil, nil, err
	}

	var dst *image.Gray
	if newGray != nil {
		dst = newGray(image.Rect(0, 0, width, height))
	} else {
		dst = image.NewGray(image.Rect(0, 0, width, height))
	}

	decMeta, err := d.DecodeGray(dst, src)
	if err != nil {
		return nil, nil, err
	}

	detached := *decMeta
	return dst, &detached, nil
}

// validateAllocatingGeometry checks decoded image dimensions and derives the
// host stride, host buffer length, and four-byte-per-pixel guest buffer length.
// It returns those three values and a nil error on success; otherwise it
// returns zero values and ErrBadImage. Zero, overflowing, or otherwise
// unrepresentable dimensions are rejected.
func validateAllocatingGeometry(width, height, bytesPerPixel int) (int, int, int, error) {
	if width <= 0 || height <= 0 {
		return 0, 0, 0, ErrBadImage
	}
	if width > maxWuffsDimension || height > maxWuffsDimension {
		return 0, 0, 0, ErrBadImage
	}
	if bytesPerPixel != 1 && bytesPerPixel != 4 {
		return 0, 0, 0, ErrBadImage
	}

	rowBytes := uint64(width) * uint64(bytesPerPixel)
	if rowBytes > uint64(math.MaxInt) {
		return 0, 0, 0, ErrBadImage
	}
	tightBytes := rowBytes * uint64(height)
	if tightBytes > uint64(math.MaxInt) {
		return 0, 0, 0, ErrBadImage
	}

	decodedBytes := uint64(width) * uint64(height) * 4
	if decodedBytes > math.MaxUint32 {
		return 0, 0, 0, ErrBadImage
	}
	if decodedBytes > uint64(math.MaxInt) {
		return 0, 0, 0, ErrBadImage
	}

	return int(rowBytes), int(tightBytes), int(decodedBytes), nil
}
