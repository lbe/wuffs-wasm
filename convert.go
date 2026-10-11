package wuffs

import "image"

// unpremultiply converts an alpha-premultiplied color channel to straight
// alpha. It must not be called with a == 0.
func unpremultiply(c, a uint8) uint8 {
	return uint8((uint16(c) * 255) / uint16(a))
}

// copyScratchToRGBARegion copies the Frame.Bounds sub-rectangle of a decoded
// full-canvas RGBA-premultiplied scratch image into dst. The guest emits RGBA
// byte order, so each row is a plain copy with no channel swap. Only pixels
// inside bounds are written; pixels outside bounds in dst are untouched.
//
// bounds is the frame rectangle inside the overall width*height canvas
// (half-open, canvas coordinates). dstStride is the number of bytes per row
// in dst (may differ from canvas width*4). The source stride is always
// canvas width*4 bytes.
func copyScratchToRGBARegion(dst []byte, dstStride int, src []byte, canvasWidth int, bounds image.Rectangle) {
	srcStride := canvasWidth * 4
	rowLen := bounds.Dx() * 4
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		s := y*srcStride + bounds.Min.X*4
		d := y*dstStride + bounds.Min.X*4
		copy(dst[d:d+rowLen], src[s:s+rowLen])
	}
}

// copyScratchToNRGBA copies width*height decoded RGBA-non-premultiplied
// pixels from src into dst. The guest emits RGBA byte order, so each row is
// a plain copy with no channel swap.
func copyScratchToNRGBA(dst []byte, dstStride int, src []byte, width, height int) {
	srcStride := width * 4
	rowLen := width * 4
	for y := 0; y < height; y++ {
		s := y * srcStride
		d := y * dstStride
		copy(dst[d:d+rowLen], src[s:s+rowLen])
	}
}

// convertBGRAToGray copies width*height decoded BGRA-premultiplied pixels from
// src into dst, producing one-byte grayscale values matching color.GrayModel.
//
// dstStride is the number of bytes per row in dst (may differ from width).
// The source stride is always width*4 bytes.
func convertBGRAToGray(dst []byte, dstStride int, src []byte, width, height int) {
	srcStride := width * 4
	for y := 0; y < height; y++ {
		s := y * srcStride
		d := y * dstStride
		for x := 0; x < width; x++ {
			si := s + x*4
			di := d + x
			b := src[si+0]
			g := src[si+1]
			r := src[si+2]
			a := src[si+3]
			if a == 0 {
				dst[di] = 0
				continue
			}
			if a != 255 {
				r = unpremultiply(r, a)
				g = unpremultiply(g, a)
				b = unpremultiply(b, a)
			}

			// Match color.GrayModel's conversion from NRGBA without creating
			// a color interface value for every pixel. NRGBA.RGBA expands each
			// channel to 16 bits and applies alpha before grayModel applies the
			// JFIF coefficients and rounds to eight bits.
			r16 := uint32(r)
			r16 |= r16 << 8
			r16 *= uint32(a)
			r16 /= 0xff
			g16 := uint32(g)
			g16 |= g16 << 8
			g16 *= uint32(a)
			g16 /= 0xff
			b16 := uint32(b)
			b16 |= b16 << 8
			b16 *= uint32(a)
			b16 /= 0xff
			dst[di] = uint8((19595*r16 + 38470*g16 + 7471*b16 + 1<<15) >> 24)
		}
	}
}
