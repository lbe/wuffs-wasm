package wuffs_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// TestIntegrationDecodeGrayRejectsNilDestination verifies that DecodeGray
// rejects a nil destination without panicking or decoding the source.
func TestIntegrationDecodeGrayRejectsNilDestination(t *testing.T) {
	d := wuffs.New()
	src := loadFixture(t, "bricks-color.png")

	var (
		meta    *wuffs.Meta
		err     error
		paniced bool
	)
	func() {
		defer func() {
			if recover() != nil {
				paniced = true
			}
		}()
		meta, err = d.DecodeGray(nil, src)
	}()

	if paniced {
		t.Fatal("DecodeGray(nil, valid source) panicked")
	}
	if meta != nil {
		t.Fatalf("DecodeGray(nil, valid source) returned Meta = %+v, want nil", meta)
	}
	if !errors.Is(err, wuffs.ErrBadImage) {
		t.Fatalf("DecodeGray(nil, valid source) error = %v, want ErrBadImage", err)
	}
}

const (
	grayFixtureWidth  = 160
	grayFixtureHeight = 120
)

// TestIntegrationDecodeGray verifies the caller-owned Gray contract against
// the checked-in PNG fixture, including color.GrayModel conversion and layout
// preservation for a padded destination.
func TestIntegrationDecodeGray(t *testing.T) {
	src := loadFixture(t, "bricks-color.png")
	decoded, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		t.Fatalf("png.Decode oracle: %v", err)
	}

	const padding = 7
	stride := grayFixtureWidth + padding
	pix := bytes.Repeat([]byte{0xA5}, stride*grayFixtureHeight)
	dst := &image.Gray{
		Rect:   image.Rect(0, 0, grayFixtureWidth, grayFixtureHeight),
		Stride: stride,
		Pix:    pix,
	}
	pixPtr := &dst.Pix[0]
	pixLen, pixCap := len(dst.Pix), cap(dst.Pix)
	rect, dstStride := dst.Rect, dst.Stride

	d := wuffs.New()
	initialSlot := wuffs.CurrentDstSlotLen(d)
	meta, err := d.DecodeGray(dst, src)
	if err != nil {
		t.Fatalf("DecodeGray PNG: %v", err)
	}
	if meta == nil {
		t.Fatal("DecodeGray PNG returned nil Meta")
	}
	if meta.Width != grayFixtureWidth || meta.Height != grayFixtureHeight {
		t.Errorf("Meta dimensions = (%d,%d), want (%d,%d)", meta.Width, meta.Height, grayFixtureWidth, grayFixtureHeight)
	}
	if meta.Stride != grayFixtureWidth*4 {
		t.Errorf("Meta.Stride = %d, want guest stride %d", meta.Stride, grayFixtureWidth*4)
	}
	if meta.BytesWritten != grayFixtureWidth*grayFixtureHeight {
		t.Errorf("Meta.BytesWritten = %d, want host bytes %d", meta.BytesWritten, grayFixtureWidth*grayFixtureHeight)
	}
	if meta.Format != wuffs.FormatPNG {
		t.Errorf("Meta.Format = 0x%08X, want FormatPNG", meta.Format)
	}
	if got := wuffs.CurrentDstSlotLen(d); got != initialSlot {
		t.Errorf("DecodeGray changed guest dst slot length from %d to %d", initialSlot, got)
	}
	if &dst.Pix[0] != pixPtr {
		t.Fatal("DecodeGray relocated destination Pix backing array")
	}
	if len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap {
		t.Errorf("destination Pix shape changed: len/cap = %d/%d, want %d/%d", len(dst.Pix), cap(dst.Pix), pixLen, pixCap)
	}
	if dst.Rect != rect || dst.Stride != dstStride {
		t.Errorf("destination layout changed: Rect=%v Stride=%d, want Rect=%v Stride=%d", dst.Rect, dst.Stride, rect, dstStride)
	}

	for y := 0; y < grayFixtureHeight; y++ {
		for x := 0; x < grayFixtureWidth; x++ {
			want := color.GrayModel.Convert(decoded.At(x, y)).(color.Gray).Y
			if got := dst.Pix[y*stride+x]; got != want {
				t.Fatalf("Gray pixel (%d,%d) = %d, want color.GrayModel value %d", x, y, got, want)
			}
		}
		for x := grayFixtureWidth; x < stride; x++ {
			if dst.Pix[y*stride+x] != 0xA5 {
				t.Fatalf("padding byte (%d,%d) changed to 0x%02X", x, y, dst.Pix[y*stride+x])
			}
		}
	}

	grayBeforeLaterCalls := append([]byte(nil), dst.Pix...)
	if _, err := d.Probe(src); err != nil {
		t.Fatalf("Probe after DecodeGray: %v", err)
	}
	if err := wuffs.RequiredReserve(d, grayFixtureWidth*grayFixtureHeight*4, len(src)); err != nil {
		t.Fatalf("Reserve after DecodeGray: %v", err)
	}
	rgba := image.NewRGBA(image.Rect(0, 0, grayFixtureWidth, grayFixtureHeight))
	if _, err := d.DecodeRGBA(rgba, src); err != nil {
		t.Fatalf("DecodeRGBA after DecodeGray: %v", err)
	}
	if !bytes.Equal(dst.Pix, grayBeforeLaterCalls) {
		t.Fatal("Probe, Reserve, or later decode mutated returned Gray pixels")
	}
}

// TestIntegrationDecodeGrayNoAllocs verifies that a reserved and warmed-up
// DecodeGray reuses its decoder and destination storage.
func TestIntegrationDecodeGrayNoAllocs(t *testing.T) {
	src := loadFixture(t, "bricks-color.png")
	dst := image.NewGray(image.Rect(0, 0, grayFixtureWidth, grayFixtureHeight))
	d := wuffs.New()
	if err := wuffs.RequiredReserve(d, grayFixtureWidth*grayFixtureHeight*4, len(src)); err != nil {
		t.Fatalf("RequiredReserve: %v", err)
	}
	if _, err := d.DecodeGray(dst, src); err != nil {
		t.Fatalf("warmup DecodeGray: %v", err)
	}

	allocs := testing.AllocsPerRun(100, func() {
		if _, err := d.DecodeGray(dst, src); err != nil {
			panic(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("DecodeGray allocations = %v, want 0", allocs)
	}
}

// TestIntegrationDecodeGrayWEBP verifies dimensions, format metadata, and
// one-byte output for the checked-in lossless WebP fixture.
func TestIntegrationDecodeGrayWEBP(t *testing.T) {
	src := loadFixture(t, "bricks-color.lossless.webp")
	dst := image.NewGray(image.Rect(0, 0, grayFixtureWidth, grayFixtureHeight))

	meta, err := wuffs.New().DecodeGray(dst, src)
	if err != nil {
		t.Fatalf("DecodeGray WEBP: %v", err)
	}
	if meta.Width != grayFixtureWidth || meta.Height != grayFixtureHeight {
		t.Errorf("Meta dimensions = (%d,%d), want (%d,%d)", meta.Width, meta.Height, grayFixtureWidth, grayFixtureHeight)
	}
	if meta.Format != wuffs.FormatWEBP {
		t.Errorf("Meta.Format = 0x%08X, want FormatWEBP", meta.Format)
	}
	if meta.Stride != grayFixtureWidth*4 || meta.BytesWritten != grayFixtureWidth*grayFixtureHeight {
		t.Errorf("Meta stride/bytes = (%d,%d), want (%d,%d)", meta.Stride, meta.BytesWritten, grayFixtureWidth*4, grayFixtureWidth*grayFixtureHeight)
	}
}

func TestIntegrationDecodeGrayDestinationErrors(t *testing.T) {
	src := loadFixture(t, "bricks-color.png")
	validRect := image.Rect(0, 0, grayFixtureWidth, grayFixtureHeight)

	t.Run("nil destination is ErrBadImage", func(t *testing.T) {
		_, err := wuffs.New().DecodeGray(nil, src)
		if !errors.Is(err, wuffs.ErrBadImage) {
			t.Fatalf("DecodeGray(nil) error = %v, want ErrBadImage", err)
		}
	})

	malformed := []struct {
		name string
		dst  *image.Gray
	}{
		{name: "empty Rect", dst: &image.Gray{Rect: image.Rectangle{}, Stride: grayFixtureWidth, Pix: make([]byte, grayFixtureWidth)}},
		{name: "nil Pix", dst: &image.Gray{Rect: validRect, Stride: grayFixtureWidth}},
		{name: "empty Pix", dst: &image.Gray{Rect: validRect, Stride: grayFixtureWidth, Pix: []byte{}}},
		{name: "non-zero Rect.Min", dst: &image.Gray{Rect: image.Rect(1, 1, grayFixtureWidth+1, grayFixtureHeight+1), Stride: grayFixtureWidth, Pix: make([]byte, grayFixtureWidth*grayFixtureHeight)}},
		{name: "dimension mismatch", dst: &image.Gray{Rect: image.Rect(0, 0, grayFixtureWidth-1, grayFixtureHeight), Stride: grayFixtureWidth - 1, Pix: make([]byte, (grayFixtureWidth-1)*grayFixtureHeight)}},
	}
	for _, tc := range malformed {
		t.Run(tc.name, func(t *testing.T) {
			_, err := wuffs.New().DecodeGray(tc.dst, src)
			if !errors.Is(err, wuffs.ErrBadImage) {
				t.Fatalf("DecodeGray error = %v, want ErrBadImage", err)
			}
		})
	}

	t.Run("short stride reports one-byte host minimum", func(t *testing.T) {
		stride := grayFixtureWidth - 1
		dst := &image.Gray{Rect: validRect, Stride: stride, Pix: make([]byte, stride*grayFixtureHeight)}
		_, err := wuffs.New().DecodeGray(dst, src)
		var small *wuffs.DstTooSmallError
		if !errors.As(err, &small) || !errors.Is(err, wuffs.ErrDstTooSmall) {
			t.Fatalf("DecodeGray error = %v, want DstTooSmallError", err)
		}
		if small.MinBytes != grayFixtureWidth*grayFixtureHeight || small.Stride != grayFixtureWidth {
			t.Errorf("host shortage = MinBytes %d Stride %d, want %d %d", small.MinBytes, small.Stride, grayFixtureWidth*grayFixtureHeight, grayFixtureWidth)
		}
	})

	t.Run("short Pix reports padded one-byte host minimum", func(t *testing.T) {
		stride := grayFixtureWidth + 8
		dst := &image.Gray{Rect: validRect, Stride: stride, Pix: make([]byte, stride*grayFixtureHeight-1)}
		_, err := wuffs.New().DecodeGray(dst, src)
		var small *wuffs.DstTooSmallError
		if !errors.As(err, &small) || !errors.Is(err, wuffs.ErrDstTooSmall) {
			t.Fatalf("DecodeGray error = %v, want DstTooSmallError", err)
		}
		if small.MinBytes != uint32(stride*grayFixtureHeight) || small.Stride != uint32(stride) {
			t.Errorf("host shortage = MinBytes %d Stride %d, want %d %d", small.MinBytes, small.Stride, stride*grayFixtureHeight, stride)
		}
	})
}

func TestIntegrationDecodeGrayGuestReserveRetry(t *testing.T) {
	src := loadFixture(t, "bricks-color.png")
	restore := wuffs.SetInitialDstSlotBytes(1024)
	defer restore()

	d := wuffs.New()
	dst := image.NewGray(image.Rect(0, 0, grayFixtureWidth, grayFixtureHeight))
	initialSlot := wuffs.CurrentDstSlotLen(d)
	_, err := d.DecodeGray(dst, src)
	var small *wuffs.DstTooSmallError
	if !errors.As(err, &small) || !errors.Is(err, wuffs.ErrDstTooSmall) {
		t.Fatalf("guest-short DecodeGray error = %v, want DstTooSmallError", err)
	}
	if small.MinBytes != grayFixtureWidth*grayFixtureHeight*4 || small.Stride != grayFixtureWidth*4 {
		t.Errorf("guest shortage = MinBytes %d Stride %d, want four-byte scratch %d %d", small.MinBytes, small.Stride, grayFixtureWidth*4, grayFixtureWidth*4)
	}
	if got := wuffs.CurrentDstSlotLen(d); got != initialSlot {
		t.Errorf("failed decode changed guest dst slot length from %d to %d", initialSlot, got)
	}

	if err = wuffs.RequiredReserve(d, grayFixtureWidth*grayFixtureHeight*4, len(src)); err != nil {
		t.Fatalf("Reserve for guest retry: %v", err)
	}
	meta, err := d.DecodeGray(dst, src)
	if err != nil {
		t.Fatalf("DecodeGray after Reserve: %v", err)
	}
	if meta.BytesWritten != grayFixtureWidth*grayFixtureHeight {
		t.Errorf("retry BytesWritten = %d, want %d", meta.BytesWritten, grayFixtureWidth*grayFixtureHeight)
	}
}

func TestIntegrationDecodeGraySentinelErrors(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")
	harvesters := loadFixture(t, "harvesters.png")
	garbage := []byte("not an image")
	dst := image.NewGray(image.Rect(0, 0, grayFixtureWidth, grayFixtureHeight))

	tests := []struct {
		name string
		src  []byte
		want error
	}{
		{name: "unknown", src: garbage, want: wuffs.ErrUnknownFormat},
		{name: "truncated", src: pngSrc[:50], want: wuffs.ErrDecode},
		{name: "empty", src: nil, want: wuffs.ErrDecode},
		{name: "over capacity", src: harvesters, want: wuffs.ErrSrcTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := wuffs.New().DecodeGray(dst, tc.src)
			if !errors.Is(err, tc.want) {
				t.Fatalf("DecodeGray error = %v, want errors.Is(..., %v)", err, tc.want)
			}
		})
	}
}
