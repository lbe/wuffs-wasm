package wuffs

import (
	"image"
	"io"
)

// readerDecoder is the decode operation used by the reader adapter. Keeping
// the operation as a function type makes the adapter's staged contract clear
// at its call site and keeps the helper straightforward to unit test.
type readerDecoder func([]byte) (*image.RGBA, *Meta, error)

// configReaderDecoder is the configuration-only operation used by the reader
// adapter. Naming the seam keeps its staged contract explicit at call sites
// and lets package tests substitute a decoder without shared state.
type configReaderDecoder func([]byte) (image.Config, error)

// formatName returns the canonical name for a decoded image FourCC.
func formatName(fourCC uint32) (string, error) {
	switch fourCC {
	case FormatPNG:
		return "png", nil
	case FormatWEBP:
		return "webp", nil
	case FormatBMP:
		return "bmp", nil
	case FormatGIF:
		return "gif", nil
	case FormatJPEG:
		return "jpeg", nil
	case FormatNPBM:
		return "npbm", nil
	case FormatQOI:
		return "qoi", nil
	case FormatTGA:
		return "tga", nil
	case FormatWBMP:
		return "wbmp", nil
	default:
		return "", ErrUnknownFormat
	}
}

// decodeReaderWithDecoder reads r in full before invoking decode, then maps
// the decoded format to its canonical standard-library adapter name. A failed
// stage returns no partial result and prevents subsequent stages from running.
func decodeReaderWithDecoder(r io.Reader, decode readerDecoder) (image.Image, string, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}

	img, meta, err := decode(src)
	if err != nil {
		return nil, "", err
	}

	canonicalFormat, err := formatName(meta.Format)
	if err != nil {
		return nil, "", err
	}
	return img, canonicalFormat, nil
}

// decodeConfigReaderWithDecoder buffers r before invoking decode. A reader
// failure skips decode; either failure returns a zero image.Config and the
// original error.
func decodeConfigReaderWithDecoder(r io.Reader, decode configReaderDecoder) (image.Config, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return image.Config{}, err
	}

	cfg, err := decode(src)
	if err != nil {
		return image.Config{}, err
	}
	return cfg, nil
}

// DecodeReader reads r to EOF and decodes the buffered bytes into a newly
// allocated *image.RGBA. The returned format is the canonical name for the
// decoded format: png, webp, bmp, gif, jpeg, npbm, qoi, tga, or wbmp. It
// does not register formats with the standard library image package.
func DecodeReader(r io.Reader) (image.Image, string, error) {
	return decodeReaderWithDecoder(r, Decode)
}

// DecodeConfigReader reads r to EOF and returns the image configuration
// without decoding pixels. Reader and decode errors are returned unchanged;
// failures return a zero image.Config. It does not register formats with the
// standard library's global image decoder registry.
func DecodeConfigReader(r io.Reader) (image.Config, error) {
	return decodeConfigReaderWithDecoder(r, DecodeConfig)
}
