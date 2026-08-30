package wuffs_test

import (
	"bytes"
	"errors"
	"image"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

const (
	alphaFixtureWidth  = 3
	alphaFixtureHeight = 1
)

// TestIntegrationDecodeNRGBA exercises the caller-owned NRGBA contract using
// a fixture with opaque, translucent, and fully transparent pixels.
func TestIntegrationDecodeNRGBA(t *testing.T) {
	src := loadFixture(t, "alpha-pixels.png")
	probeDecoder := wuffs.New()
	probe, err := probeDecoder.Probe(src)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if probe.Width != alphaFixtureWidth || probe.Height != alphaFixtureHeight {
		t.Fatalf("Probe dimensions = (%d,%d), want (%d,%d)", probe.Width, probe.Height, alphaFixtureWidth, alphaFixtureHeight)
	}

	const padding = 5
	stride := alphaFixtureWidth*4 + padding
	pix := bytes.Repeat([]byte{0xA5}, stride*alphaFixtureHeight)
	dst := &image.NRGBA{
		Rect:   image.Rect(0, 0, alphaFixtureWidth, alphaFixtureHeight),
		Stride: stride,
		Pix:    pix,
	}
	pixPtr := &dst.Pix[0]
	pixLen, pixCap := len(dst.Pix), cap(dst.Pix)
	rect, dstStride := dst.Rect, dst.Stride

	d := wuffs.New()
	beforeSlot := wuffs.CurrentDstSlotLen(d)
	meta, err := d.DecodeNRGBA(dst, src)
	if err != nil {
		t.Fatalf("DecodeNRGBA: %v", err)
	}
	if meta.Width != probe.Width || meta.Height != probe.Height || meta.Stride != probe.Stride || meta.Format != probe.Format {
		t.Errorf("DecodeNRGBA Meta = %+v, want Probe dimensions/stride/format %+v", *meta, *probe)
	}
	if meta.Stride != alphaFixtureWidth*4 || meta.BytesWritten != alphaFixtureWidth*alphaFixtureHeight*4 {
		t.Errorf("Meta stride/bytes = (%d,%d), want (%d,%d)", meta.Stride, meta.BytesWritten, alphaFixtureWidth*4, alphaFixtureWidth*alphaFixtureHeight*4)
	}
	if got := wuffs.CurrentDstSlotLen(d); got != beforeSlot {
		t.Errorf("DecodeNRGBA changed guest destination slot length from %d to %d", beforeSlot, got)
	}
	if &dst.Pix[0] != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != dstStride {
		t.Fatal("DecodeNRGBA changed the caller-owned destination layout")
	}
	want := []byte{
		0x12, 0x34, 0x56, 0xFF,
		0xFF, 0x00, 0xFF, 0x80,
		0x00, 0x00, 0x00, 0x00,
	}
	if got := dst.Pix[:alphaFixtureWidth*4]; !bytes.Equal(got, want) {
		t.Errorf("DecodeNRGBA pixels = % X, want % X", got, want)
	}
	for i, got := range dst.Pix[alphaFixtureWidth*4:] {
		if got != 0xA5 {
			t.Fatalf("padding byte %d = 0x%02X, want 0xA5", i, got)
		}
	}

	pixels := append([]byte(nil), dst.Pix...)
	if _, err := d.Probe(src); err != nil {
		t.Fatalf("Probe after DecodeNRGBA: %v", err)
	}
	if err := wuffs.RequiredReserve(d, alphaFixtureWidth*alphaFixtureHeight*4, len(src)); err != nil {
		t.Fatalf("Reserve after DecodeNRGBA: %v", err)
	}
	if _, err := d.DecodeRGBA(image.NewRGBA(image.Rect(0, 0, alphaFixtureWidth, alphaFixtureHeight)), src); err != nil {
		t.Fatalf("DecodeRGBA after DecodeNRGBA: %v", err)
	}
	if !bytes.Equal(dst.Pix, pixels) {
		t.Fatal("later Probe, Reserve, or decode mutated returned NRGBA pixels")
	}
}

func TestIntegrationDecodeNRGBAWEBP(t *testing.T) {
	src := loadFixture(t, "bricks-color.lossless.webp")
	dst := image.NewNRGBA(image.Rect(0, 0, 160, 120))
	meta, err := wuffs.New().DecodeNRGBA(dst, src)
	if err != nil {
		t.Fatalf("DecodeNRGBA WEBP: %v", err)
	}
	if meta.Width != 160 || meta.Height != 120 || meta.Format != wuffs.FormatWEBP {
		t.Errorf("WEBP Meta = %+v, want 160x120 FormatWEBP", *meta)
	}
}

func TestIntegrationDecodeNRGBADestinationErrors(t *testing.T) {
	src := loadFixture(t, "alpha-pixels.png")
	validRect := image.Rect(0, 0, alphaFixtureWidth, alphaFixtureHeight)

	t.Run("nil", func(t *testing.T) {
		if _, err := wuffs.New().DecodeNRGBA(nil, src); !errors.Is(err, wuffs.ErrBadImage) {
			t.Fatalf("DecodeNRGBA(nil) error = %v, want ErrBadImage", err)
		}
	})
	for _, tc := range []struct {
		name string
		dst  *image.NRGBA
	}{
		{"empty Rect", &image.NRGBA{}},
		{"nil Pix", &image.NRGBA{Rect: validRect, Stride: alphaFixtureWidth * 4}},
		{"non-zero Rect.Min", &image.NRGBA{Rect: image.Rect(1, 1, 4, 2), Stride: alphaFixtureWidth * 4, Pix: make([]byte, alphaFixtureWidth*4)}},
		{"dimension mismatch", &image.NRGBA{Rect: image.Rect(0, 0, 2, 1), Stride: 8, Pix: make([]byte, 8)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := wuffs.New().DecodeNRGBA(tc.dst, src); !errors.Is(err, wuffs.ErrBadImage) {
				t.Fatalf("DecodeNRGBA error = %v, want ErrBadImage", err)
			}
		})
	}

	for _, tc := range []struct {
		name           string
		dst            *image.NRGBA
		wantMin, wantS uint32
	}{
		{"short Stride", &image.NRGBA{Rect: validRect, Stride: alphaFixtureWidth*4 - 1, Pix: make([]byte, alphaFixtureWidth*4-1)}, alphaFixtureWidth * alphaFixtureHeight * 4, alphaFixtureWidth * 4},
		{"short padded Pix", &image.NRGBA{Rect: validRect, Stride: alphaFixtureWidth*4 + 4, Pix: make([]byte, alphaFixtureWidth*4+3)}, alphaFixtureWidth*4 + 4, alphaFixtureWidth*4 + 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := wuffs.New().DecodeNRGBA(tc.dst, src)
			var small *wuffs.DstTooSmallError
			if !errors.As(err, &small) || !errors.Is(err, wuffs.ErrDstTooSmall) {
				t.Fatalf("DecodeNRGBA error = %v, want DstTooSmallError", err)
			}
			if small.MinBytes != tc.wantMin || small.Stride != tc.wantS || small.Width != alphaFixtureWidth || small.Height != alphaFixtureHeight {
				t.Errorf("DstTooSmallError = %+v, want MinBytes=%d Stride=%d Width=%d Height=%d", *small, tc.wantMin, tc.wantS, alphaFixtureWidth, alphaFixtureHeight)
			}
		})
	}
}

func TestIntegrationDecodeNRGBAReserveAndSentinels(t *testing.T) {
	src := loadFixture(t, "alpha-pixels.png")
	restore := wuffs.SetInitialDstSlotBytes(1)
	defer restore()
	d := wuffs.New()
	dst := image.NewNRGBA(image.Rect(0, 0, alphaFixtureWidth, alphaFixtureHeight))
	beforeSlot := wuffs.CurrentDstSlotLen(d)
	_, err := d.DecodeNRGBA(dst, src)
	var small *wuffs.DstTooSmallError
	if !errors.As(err, &small) || small.MinBytes != alphaFixtureWidth*alphaFixtureHeight*4 || small.Stride != alphaFixtureWidth*4 {
		t.Fatalf("guest shortage = %v, want four-byte scratch DstTooSmallError", err)
	}
	if got := wuffs.CurrentDstSlotLen(d); got != beforeSlot {
		t.Fatalf("guest shortage grew destination slot from %d to %d", beforeSlot, got)
	}
	if err := wuffs.RequiredReserve(d, alphaFixtureWidth*alphaFixtureHeight*4, len(src)); err != nil {
		t.Fatalf("RequiredReserve: %v", err)
	}
	if _, err := d.DecodeNRGBA(dst, src); err != nil {
		t.Fatalf("DecodeNRGBA after Reserve: %v", err)
	}

	for _, tc := range []struct {
		name string
		src  []byte
		want error
	}{
		{"empty", nil, wuffs.ErrDecode},
		{"unknown", []byte("not an image"), wuffs.ErrUnknownFormat},
		{"truncated", src[:20], wuffs.ErrDecode},
		{"over capacity", loadFixture(t, "harvesters.png"), wuffs.ErrSrcTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := wuffs.New().DecodeNRGBA(image.NewNRGBA(image.Rect(0, 0, alphaFixtureWidth, alphaFixtureHeight)), tc.src)
			if !errors.Is(err, tc.want) {
				t.Fatalf("DecodeNRGBA error = %v, want errors.Is(..., %v)", err, tc.want)
			}
		})
	}
}

func TestIntegrationDecodeNRGBANoAllocs(t *testing.T) {
	src := loadFixture(t, "alpha-pixels.png")
	d := wuffs.New()
	if err := wuffs.RequiredReserve(d, alphaFixtureWidth*alphaFixtureHeight*4, len(src)); err != nil {
		t.Fatalf("RequiredReserve: %v", err)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, alphaFixtureWidth, alphaFixtureHeight))
	if _, err := d.DecodeNRGBA(dst, src); err != nil {
		t.Fatalf("warmup DecodeNRGBA: %v", err)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if _, err := d.DecodeNRGBA(dst, src); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("DecodeNRGBA allocations = %v, want 0", allocs)
	}
}
