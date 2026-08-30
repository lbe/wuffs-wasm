package wuffs

// unpremultiply converts an alpha-premultiplied color channel to straight
// alpha. It must not be called with a == 0.
func unpremultiply(c, a uint8) uint8 {
	return uint8((uint16(c) * 255) / uint16(a))
}

// convertBGRAToStraight copies width*height decoded BGRA-premultiplied pixels
// from src into dst, writing straight (non-premultiplied) RGBA/NRGBA bytes in
// Go image order.
//
// For each pixel it swaps the B and R channels and straightens the alpha:
//   - a == 0: all channels are set to zero (fully transparent).
//   - a == 255: channels are copied through unchanged (fully opaque).
//   - otherwise: R, G, B are unpremultiplied via integer division (c*255)/a.
//
// dstStride is the number of bytes per row in dst (may differ from width*4).
// The source stride is always width*4 bytes.
func convertBGRAToStraight(dst []byte, dstStride int, src []byte, width, height int) {
	srcStride := width * 4
	for y := 0; y < height; y++ {
		s := y * srcStride
		d := y * dstStride
		for x := 0; x < width; x++ {
			si := s + x*4
			di := d + x*4
			b := src[si+0]
			g := src[si+1]
			r := src[si+2]
			a := src[si+3]
			switch a {
			case 0:
				dst[di+0] = 0
				dst[di+1] = 0
				dst[di+2] = 0
				dst[di+3] = 0
			case 255:
				dst[di+0] = r
				dst[di+1] = g
				dst[di+2] = b
				dst[di+3] = 255
			default:
				dst[di+0] = unpremultiply(r, a)
				dst[di+1] = unpremultiply(g, a)
				dst[di+2] = unpremultiply(b, a)
				dst[di+3] = a
			}
		}
	}
}

// convertBGRAToRGBA copies width*height decoded BGRA pixels from src into
// dst, producing straight (non-premultiplied) RGBA in Go image order.
func convertBGRAToRGBA(dst []byte, dstStride int, src []byte, width, height int) {
	convertBGRAToStraight(dst, dstStride, src, width, height)
}

// convertBGRAToNRGBA copies width*height decoded BGRA pixels from src into
// dst, producing straight (non-premultiplied) NRGBA in Go image order.
func convertBGRAToNRGBA(dst []byte, dstStride int, src []byte, width, height int) {
	convertBGRAToStraight(dst, dstStride, src, width, height)
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
