package wuffs_test

import (
	"errors"
	"image"
	"reflect"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// TestDecodeTypedDestinationsValidateLayouts verifies that each typed decoder
// accepts tight and padded layouts and rejects malformed or unrepresentable
// caller-owned destinations with the documented error shape.
func TestDecodeTypedDestinationsValidateLayouts(t *testing.T) {
	src := loadFixture(t, "bricks-color.png")
	const (
		width  = 160
		height = 120
	)

	types := []struct {
		name   string
		method string
		bpp    int
	}{
		{name: "RGBA", method: "DecodeRGBA", bpp: 4},
		{name: "NRGBA", method: "DecodeNRGBA", bpp: 4},
		{name: "Gray", method: "DecodeGray", bpp: 1},
	}
	cases := []struct {
		name         string
		makeDst      func(kind, bpp int) any
		wantError    error
		wantSmall    bool
		wantMinBytes func(bpp int) int
		wantStride   func(bpp int) int
	}{
		{name: "exact layout", makeDst: func(kind, bpp int) any {
			stride := width * bpp
			return func() any {
				switch kind {
				case 0:
					return &image.RGBA{Rect: image.Rect(0, 0, width, height), Stride: stride, Pix: make([]byte, stride*height)}
				case 1:
					return &image.NRGBA{Rect: image.Rect(0, 0, width, height), Stride: stride, Pix: make([]byte, stride*height)}
				default:
					return &image.Gray{Rect: image.Rect(0, 0, width, height), Stride: stride, Pix: make([]byte, stride*height)}
				}
			}()
		}},
		{name: "padded layout", makeDst: func(kind, bpp int) any {
			stride := width*bpp + 8
			return func() any {
				switch kind {
				case 0:
					return &image.RGBA{Rect: image.Rect(0, 0, width, height), Stride: stride, Pix: make([]byte, stride*height)}
				case 1:
					return &image.NRGBA{Rect: image.Rect(0, 0, width, height), Stride: stride, Pix: make([]byte, stride*height)}
				default:
					return &image.Gray{Rect: image.Rect(0, 0, width, height), Stride: stride, Pix: make([]byte, stride*height)}
				}
			}()
		}},
		{name: "empty rectangle", makeDst: func(kind, bpp int) any {
			return func() any {
				switch kind {
				case 0:
					return &image.RGBA{Rect: image.Rectangle{}, Stride: width * bpp, Pix: make([]byte, width*bpp)}
				case 1:
					return &image.NRGBA{Rect: image.Rectangle{}, Stride: width * bpp, Pix: make([]byte, width*bpp)}
				default:
					return &image.Gray{Rect: image.Rectangle{}, Stride: width * bpp, Pix: make([]byte, width*bpp)}
				}
			}()
		}, wantError: wuffs.ErrBadImage},
		{name: "empty Pix", makeDst: func(kind, bpp int) any {
			return func() any {
				r := image.Rect(0, 0, width, height)
				switch kind {
				case 0:
					return &image.RGBA{Rect: r, Stride: width * bpp, Pix: []byte{}}
				case 1:
					return &image.NRGBA{Rect: r, Stride: width * bpp, Pix: []byte{}}
				default:
					return &image.Gray{Rect: r, Stride: width * bpp, Pix: []byte{}}
				}
			}()
		}, wantError: wuffs.ErrBadImage},
		{name: "non-zero Rect.Min", makeDst: func(kind, bpp int) any {
			return func() any {
				r := image.Rect(1, 2, width+1, height+2)
				switch kind {
				case 0:
					return &image.RGBA{Rect: r, Stride: width * bpp, Pix: make([]byte, width*height*bpp)}
				case 1:
					return &image.NRGBA{Rect: r, Stride: width * bpp, Pix: make([]byte, width*height*bpp)}
				default:
					return &image.Gray{Rect: r, Stride: width * bpp, Pix: make([]byte, width*height*bpp)}
				}
			}()
		}, wantError: wuffs.ErrBadImage},
		{name: "dimension mismatch", makeDst: func(kind, bpp int) any {
			return func() any {
				r := image.Rect(0, 0, width-1, height)
				switch kind {
				case 0:
					return &image.RGBA{Rect: r, Stride: (width - 1) * bpp, Pix: make([]byte, (width-1)*height*bpp)}
				case 1:
					return &image.NRGBA{Rect: r, Stride: (width - 1) * bpp, Pix: make([]byte, (width-1)*height*bpp)}
				default:
					return &image.Gray{Rect: r, Stride: (width - 1) * bpp, Pix: make([]byte, (width-1)*height*bpp)}
				}
			}()
		}, wantError: wuffs.ErrBadImage},
		{name: "short Stride", makeDst: func(kind, bpp int) any {
			stride := width*bpp - 1
			return func() any {
				r := image.Rect(0, 0, width, height)
				pix := make([]byte, width*height*bpp)
				switch kind {
				case 0:
					return &image.RGBA{Rect: r, Stride: stride, Pix: pix}
				case 1:
					return &image.NRGBA{Rect: r, Stride: stride, Pix: pix}
				default:
					return &image.Gray{Rect: r, Stride: stride, Pix: pix}
				}
			}()
		}, wantSmall: true,
			wantMinBytes: func(bpp int) int { return width * height * bpp },
			wantStride:   func(bpp int) int { return width * bpp },
		},
		{name: "short Pix", makeDst: func(kind, bpp int) any {
			stride := width*bpp + 8
			return func() any {
				r := image.Rect(0, 0, width, height)
				pix := make([]byte, stride*height-1)
				switch kind {
				case 0:
					return &image.RGBA{Rect: r, Stride: stride, Pix: pix}
				case 1:
					return &image.NRGBA{Rect: r, Stride: stride, Pix: pix}
				default:
					return &image.Gray{Rect: r, Stride: stride, Pix: pix}
				}
			}()
		}, wantSmall: true,
			wantMinBytes: func(bpp int) int { return (width*bpp + 8) * height },
			wantStride:   func(bpp int) int { return width*bpp + 8 },
		},
		{name: "unrepresentable layout", makeDst: func(kind, bpp int) any {
			stride := int(1) << 30
			return func() any {
				r := image.Rect(0, 0, width, height)
				switch kind {
				case 0:
					return &image.RGBA{Rect: r, Stride: stride, Pix: make([]byte, 1)}
				case 1:
					return &image.NRGBA{Rect: r, Stride: stride, Pix: make([]byte, 1)}
				default:
					return &image.Gray{Rect: r, Stride: stride, Pix: make([]byte, 1)}
				}
			}()
		}, wantError: wuffs.ErrBadImage},
	}

	for kind, typ := range types {
		for _, tc := range cases {
			t.Run(typ.name+"/"+tc.name, func(t *testing.T) {
				dst := tc.makeDst(kind, typ.bpp)
				method := reflect.ValueOf(wuffs.New()).MethodByName(typ.method)
				if !method.IsValid() {
					t.Errorf("Decoder.%s method is missing", typ.method)
					return
				}
				out := method.Call([]reflect.Value{reflect.ValueOf(dst), reflect.ValueOf(src)})
				var err error
				if !out[1].IsNil() {
					err = out[1].Interface().(error)
				}
				switch {
				case tc.wantError != nil:
					if !errors.Is(err, tc.wantError) {
						t.Errorf("error = %v, want errors.Is(..., %v)", err, tc.wantError)
					}
				case tc.wantSmall:
					var got *wuffs.DstTooSmallError
					if !errors.As(err, &got) {
						t.Fatalf("error = %v, want *DstTooSmallError", err)
					}
					if got.Width != uint32(width) || got.Height != uint32(height) {
						t.Errorf("DstTooSmallError dimensions = (%d,%d), want (%d,%d)", got.Width, got.Height, width, height)
					}
					if got.MinBytes != uint32(tc.wantMinBytes(typ.bpp)) {
						t.Errorf("DstTooSmallError.MinBytes = %d, want %d", got.MinBytes, tc.wantMinBytes(typ.bpp))
					}
					if got.Stride != uint32(tc.wantStride(typ.bpp)) {
						t.Errorf("DstTooSmallError.Stride = %d, want %d", got.Stride, tc.wantStride(typ.bpp))
					}
					if !errors.Is(err, wuffs.ErrDstTooSmall) {
						t.Errorf("error = %v, want errors.Is(..., ErrDstTooSmall)", err)
					}
				case err != nil:
					t.Errorf("error = %v, want nil", err)
				}
			})
		}
	}
}
