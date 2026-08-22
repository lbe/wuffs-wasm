package wuffs

// convertBGRAtoRGBA copies width*height decoded BGRA pixels from src into
// dst, producing straight (non-premultiplied) RGBA in Go image order.
//
// For each pixel it swaps the B and R channels and straightens the alpha:
//   - a == 0: all channels are set to zero (fully transparent).
//   - a == 255: channels are copied through unchanged (fully opaque).
//   - otherwise: R, G, B are unpremultiplied via integer division (R*255)/a.
//
// dstStride is the number of bytes per row in dst (may differ from width*4).
func convertBGRAtoRGBA(dst []byte, dstStride int, src []byte, width, height int) {
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
			// Write straight RGBA.
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
				dst[di+0] = uint8((uint16(r) * 255) / uint16(a))
				dst[di+1] = uint8((uint16(g) * 255) / uint16(a))
				dst[di+2] = uint8((uint16(b) * 255) / uint16(a))
				dst[di+3] = a
			}
		}
	}
}
