package wuffs

import (
	"bytes"
	"image/color"
	"testing"
)

// poisonActiveBytes copies want into dst, then overwrites every active pixel
// byte with its bitwise complement so that padding sentinel values remain
// observable after conversion.
func poisonActiveBytes(dst, want []byte, width, height, stride, bytesPerPixel int) {
	copy(dst, want)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			di := y*stride + x*bytesPerPixel
			for c := 0; c < bytesPerPixel; c++ {
				dst[di+c] = ^want[di+c]
			}
		}
	}
}

// TestConvertBGRAScratchToNRGBA verifies that convertBGRAToNRGBA turns tightly
// packed BGRA-premultiplied scratch pixels into straight image.NRGBA bytes.
// Expected values are computed directly from the BGRA inputs, not copied from
// any DecodeRGBA output.
func TestConvertBGRAScratchToNRGBA(t *testing.T) {
	cases := []struct {
		name   string
		width  int
		height int
		stride int
		src    []byte
		want   []byte
	}{
		{
			name:   "opaque blue",
			width:  1,
			height: 1,
			stride: 4,
			src:    []byte{0xFF, 0x00, 0x00, 0xFF}, // BGRA_PREMUL: B=255, A=255
			want:   []byte{0x00, 0x00, 0xFF, 0xFF}, // NRGBA straight: R=0, G=0, B=255, A=255
		},
		{
			name:   "translucent red",
			width:  1,
			height: 1,
			stride: 4,
			src:    []byte{0x00, 0x00, 0x80, 0x80}, // BGRA_PREMUL: R=128, A=128
			want:   []byte{0xFF, 0x00, 0x00, 0x80}, // NRGBA straight: R=255, A=128
		},
		{
			name:   "fully transparent",
			width:  1,
			height: 1,
			stride: 4,
			src:    []byte{0x12, 0x34, 0x56, 0x00}, // BGRA_PREMUL: A=0
			want:   []byte{0x00, 0x00, 0x00, 0x00}, // NRGBA: all zero
		},
		{
			name:   "padded stride preserves padding sentinels",
			width:  2,
			height: 2,
			stride: 12, // 2 pixels * 4 bytes + 4 bytes padding
			src: []byte{
				0xFF, 0x00, 0x00, 0xFF, 0x00, 0xFF, 0x00, 0xFF, // row 0: blue, green
				0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, // row 1: red, white
			},
			want: []byte{
				// row 0
				0x00, 0x00, 0xFF, 0xFF, 0x00, 0xFF, 0x00, 0xFF,
				0xAB, 0xCD, 0xEF, 0x01, // padding sentinel
				// row 1
				0xFF, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
				0xAB, 0xCD, 0xEF, 0x01, // padding sentinel
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srcBefore := append([]byte(nil), tc.src...)
			dst := make([]byte, len(tc.want))
			poisonActiveBytes(dst, tc.want, tc.width, tc.height, tc.stride, 4)

			convertBGRAToNRGBA(dst, tc.stride, tc.src, tc.width, tc.height)

			if !bytes.Equal(dst, tc.want) {
				t.Errorf("dst = %v, want %v", dst, tc.want)
			}
			if !bytes.Equal(tc.src, srcBefore) {
				t.Errorf("src mutated: got %v, want %v", tc.src, srcBefore)
			}
		})
	}
}

// bgraPremulToGray returns the grayscale value that color.GrayModel assigns to
// a BGRA-premultiplied pixel after unpremultiplying it to straight alpha.
func bgraPremulToGray(b, g, r, a uint8) uint8 {
	if a == 0 {
		return 0
	}
	c := color.NRGBA{R: r, G: g, B: b, A: a}
	if a != 255 {
		c.R = uint8((uint16(r) * 255) / uint16(a))
		c.G = uint8((uint16(g) * 255) / uint16(a))
		c.B = uint8((uint16(b) * 255) / uint16(a))
	}
	return color.GrayModel.Convert(c).(color.Gray).Y
}

// TestConvertBGRAScratchToGray verifies that convertBGRAToGray turns tightly
// packed BGRA-premultiplied scratch pixels into one-byte image.Gray values.
// Expected values are derived from color.GrayModel so the test acts as an
// independent semantic oracle.
func TestConvertBGRAScratchToGray(t *testing.T) {
	cases := []struct {
		name   string
		width  int
		height int
		stride int
		src    []byte
		want   []byte
	}{
		{
			name:   "opaque blue",
			width:  1,
			height: 1,
			stride: 1,
			src:    []byte{0xFF, 0x00, 0x00, 0xFF}, // BGRA_PREMUL: B=255, G=0, R=0, A=255
			want:   []byte{bgraPremulToGray(0xFF, 0x00, 0x00, 0xFF)},
		},
		{
			name:   "translucent red",
			width:  1,
			height: 1,
			stride: 1,
			src:    []byte{0x00, 0x00, 0x80, 0x80}, // BGRA_PREMUL: R=128, A=128
			want:   []byte{bgraPremulToGray(0x00, 0x00, 0x80, 0x80)},
		},
		{
			name:   "fully transparent",
			width:  1,
			height: 1,
			stride: 1,
			src:    []byte{0x12, 0x34, 0x56, 0x00}, // BGRA_PREMUL: A=0
			want:   []byte{0x00},
		},
		{
			name:   "padded stride preserves padding sentinels",
			width:  2,
			height: 2,
			stride: 4, // 2 pixels + 2 padding bytes per row
			src: []byte{
				0xFF, 0x00, 0x00, 0xFF, 0x00, 0xFF, 0x00, 0xFF, // row 0: blue, green
				0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, // row 1: red, white
			},
			want: []byte{
				// row 0
				bgraPremulToGray(0xFF, 0x00, 0x00, 0xFF),
				bgraPremulToGray(0x00, 0xFF, 0x00, 0xFF),
				0xAB, 0xCD, // padding sentinel
				// row 1
				bgraPremulToGray(0x00, 0x00, 0xFF, 0xFF),
				bgraPremulToGray(0xFF, 0xFF, 0xFF, 0xFF),
				0xAB, 0xCD, // padding sentinel
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srcBefore := append([]byte(nil), tc.src...)
			dst := make([]byte, len(tc.want))
			poisonActiveBytes(dst, tc.want, tc.width, tc.height, tc.stride, 1)

			convertBGRAToGray(dst, tc.stride, tc.src, tc.width, tc.height)

			if !bytes.Equal(dst, tc.want) {
				t.Errorf("dst = %v, want %v", dst, tc.want)
			}
			if !bytes.Equal(tc.src, srcBefore) {
				t.Errorf("src mutated: got %v, want %v", tc.src, srcBefore)
			}
		})
	}
}
