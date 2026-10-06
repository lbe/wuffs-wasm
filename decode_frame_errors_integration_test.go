package wuffs_test

// Task 9 RED test: error, dst, and concurrency characterization for
// DecodeFrame. Fails (or characterizes) until host-side error paths satisfy
// the API.md dst and error contracts.

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"sync"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// TestIntegrationDecodeFrameErrors verifies the DecodeFrame error contract:
//
//   - Truncated muybridge.gif: Probe succeeds on the truncated source while
//     DecodeFrame of the last frame fails with errors.Is(err, ErrDecode) and
//     leaves every dst byte untouched.
//   - Dst validation mirrors the DecodeRGBA table tests: empty Rect,
//     nil/empty Pix, non-zero Rect.Min, and dimension mismatch return
//     ErrBadImage; Stride < width*4 and len(Pix) < Stride*Dy return
//     *DstTooSmallError matching ErrDstTooSmall.
//   - Concurrency: two goroutines with two separate Decoder instances decode
//     different frame indices concurrently with no panic and per-index CRCs
//     matching testdata/gif.animation.golden.manifest.
func TestIntegrationDecodeFrameErrors(t *testing.T) {
	t.Run("truncated src probes but DecodeFrame fails with dst unchanged", func(t *testing.T) {
		src := loadFixture(t, "muybridge.gif")
		trunc := src[:len(src)-128]

		d := wuffs.New()
		probe, err := d.Probe(trunc)
		if err != nil {
			t.Fatalf("Probe(truncated muybridge.gif) error = %v, want nil", err)
		}
		if probe == nil {
			t.Fatal("Probe(truncated muybridge.gif) returned nil Meta, want non-nil")
		}
		w, h := int(probe.Width), int(probe.Height)
		if reserveErr := wuffs.RequiredReserve(d, int(probe.Stride)*h, len(trunc)); reserveErr != nil {
			t.Fatalf("RequiredReserve: %v", reserveErr)
		}

		n, err := d.FrameCount(src)
		if err != nil {
			t.Fatalf("FrameCount(muybridge.gif) error = %v, want nil", err)
		}
		last := n - 1

		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		for i := range dst.Pix {
			dst.Pix[i] = 0xA5
		}
		prefill := append([]byte(nil), dst.Pix...)

		frame, err := d.DecodeFrame(dst, trunc, last)
		if err == nil {
			t.Fatalf("DecodeFrame(truncated muybridge.gif, %d) error = nil, want ErrDecode", last)
		}
		if !errors.Is(err, wuffs.ErrDecode) {
			t.Errorf("DecodeFrame(truncated muybridge.gif, %d) error = %v, want errors.Is(err, ErrDecode)", last, err)
		}
		if frame != nil {
			t.Errorf("DecodeFrame(truncated muybridge.gif, %d) returned non-nil Frame %+v, want nil", last, frame)
		}
		if !bytes.Equal(dst.Pix, prefill) {
			t.Error("failed DecodeFrame mutated the caller-owned destination pixels")
		}
	})

	t.Run("dst validation mirrors DecodeRGBA", func(t *testing.T) {
		src := loadFixture(t, "bricks-nodither.gif")

		const (
			wantW = 160
			wantH = 120
		)

		newReservedDecoder := func(t *testing.T) *wuffs.Decoder {
			t.Helper()
			d := wuffs.New()
			if err := wuffs.RequiredReserve(d, wantW*wantH*4, len(src)); err != nil {
				t.Fatalf("RequiredReserve: %v", err)
			}
			return d
		}

		t.Run("nil dst returns ErrBadImage", func(t *testing.T) {
			d := newReservedDecoder(t)
			if _, err := d.DecodeFrame(nil, src, 0); !errors.Is(err, wuffs.ErrBadImage) {
				t.Errorf("DecodeFrame(nil, src, 0) error = %v, want errors.Is(err, ErrBadImage)", err)
			}
		})

		t.Run("empty Rect returns ErrBadImage", func(t *testing.T) {
			d := newReservedDecoder(t)
			dst := &image.RGBA{Rect: image.Rect(0, 0, 0, 0)}
			if _, err := d.DecodeFrame(dst, src, 0); !errors.Is(err, wuffs.ErrBadImage) {
				t.Errorf("DecodeFrame error = %v, want errors.Is(err, ErrBadImage)", err)
			}
		})

		t.Run("nil Pix returns ErrBadImage", func(t *testing.T) {
			d := newReservedDecoder(t)
			dst := &image.RGBA{Rect: image.Rect(0, 0, wantW, wantH), Stride: wantW * 4, Pix: nil}
			if _, err := d.DecodeFrame(dst, src, 0); !errors.Is(err, wuffs.ErrBadImage) {
				t.Errorf("DecodeFrame error = %v, want errors.Is(err, ErrBadImage)", err)
			}
		})

		t.Run("empty Pix returns ErrBadImage", func(t *testing.T) {
			d := newReservedDecoder(t)
			dst := &image.RGBA{Rect: image.Rect(0, 0, wantW, wantH), Stride: wantW * 4, Pix: make([]byte, 0)}
			if _, err := d.DecodeFrame(dst, src, 0); !errors.Is(err, wuffs.ErrBadImage) {
				t.Errorf("DecodeFrame error = %v, want errors.Is(err, ErrBadImage)", err)
			}
		})

		t.Run("non-zero Rect.Min returns ErrBadImage", func(t *testing.T) {
			d := newReservedDecoder(t)
			dst := &image.RGBA{
				Rect:   image.Rect(2, 3, 2+wantW, 3+wantH),
				Stride: wantW * 4,
				Pix:    make([]byte, wantW*wantH*4),
			}
			if _, err := d.DecodeFrame(dst, src, 0); !errors.Is(err, wuffs.ErrBadImage) {
				t.Errorf("DecodeFrame error = %v, want errors.Is(err, ErrBadImage)", err)
			}
		})

		t.Run("wrong dimensions return ErrBadImage", func(t *testing.T) {
			d := newReservedDecoder(t)
			dst := &image.RGBA{
				Rect:   image.Rect(0, 0, 100, wantH),
				Stride: 100 * 4,
				Pix:    make([]byte, 100*wantH*4),
			}
			if _, err := d.DecodeFrame(dst, src, 0); !errors.Is(err, wuffs.ErrBadImage) {
				t.Errorf("DecodeFrame error = %v, want errors.Is(err, ErrBadImage)", err)
			}
		})

		t.Run("too-small Stride returns DstTooSmallError", func(t *testing.T) {
			d := newReservedDecoder(t)
			dst := &image.RGBA{
				Rect:   image.Rect(0, 0, wantW, wantH),
				Stride: 1,
				Pix:    make([]byte, wantW*wantH*4),
			}
			_, err := d.DecodeFrame(dst, src, 0)
			if err == nil {
				t.Fatal("DecodeFrame error = nil, want *DstTooSmallError")
			}
			var dstErr *wuffs.DstTooSmallError
			if !errors.As(err, &dstErr) {
				t.Fatalf("DecodeFrame error = %T %v, want *DstTooSmallError", err, err)
			}
			if !errors.Is(err, wuffs.ErrDstTooSmall) {
				t.Errorf("errors.Is(err, ErrDstTooSmall) = false, want true (err = %v)", err)
			}
			if dstErr.MinBytes != uint32(wantW*wantH*4) {
				t.Errorf("DstTooSmallError.MinBytes = %d, want %d", dstErr.MinBytes, wantW*wantH*4)
			}
			if dstErr.Width != wantW {
				t.Errorf("DstTooSmallError.Width = %d, want %d", dstErr.Width, wantW)
			}
			if dstErr.Height != wantH {
				t.Errorf("DstTooSmallError.Height = %d, want %d", dstErr.Height, wantH)
			}
			if dstErr.Stride != uint32(wantW*4) {
				t.Errorf("DstTooSmallError.Stride = %d, want %d", dstErr.Stride, wantW*4)
			}
		})

		t.Run("short Pix returns DstTooSmallError", func(t *testing.T) {
			d := newReservedDecoder(t)
			stride := wantW*4 + 8
			dst := &image.RGBA{
				Rect:   image.Rect(0, 0, wantW, wantH),
				Stride: stride,
				Pix:    make([]byte, stride*wantH-1),
			}
			_, err := d.DecodeFrame(dst, src, 0)
			if err == nil {
				t.Fatal("DecodeFrame error = nil, want *DstTooSmallError")
			}
			var dstErr *wuffs.DstTooSmallError
			if !errors.As(err, &dstErr) {
				t.Fatalf("DecodeFrame error = %T %v, want *DstTooSmallError", err, err)
			}
			if !errors.Is(err, wuffs.ErrDstTooSmall) {
				t.Errorf("errors.Is(err, ErrDstTooSmall) = false, want true (err = %v)", err)
			}
			if dstErr.MinBytes != uint32(stride*wantH) {
				t.Errorf("DstTooSmallError.MinBytes = %d, want %d", dstErr.MinBytes, stride*wantH)
			}
			if dstErr.Width != wantW {
				t.Errorf("DstTooSmallError.Width = %d, want %d", dstErr.Width, wantW)
			}
			if dstErr.Height != wantH {
				t.Errorf("DstTooSmallError.Height = %d, want %d", dstErr.Height, wantH)
			}
			if dstErr.Stride != uint32(stride) {
				t.Errorf("DstTooSmallError.Stride = %d, want %d", dstErr.Stride, stride)
			}
		})
	})

	t.Run("concurrent decoders decode different frames", func(t *testing.T) {
		src := loadFixture(t, "muybridge.gif")
		manifest := loadAnimationManifest(t, "gif.animation.golden.manifest")

		probe, err := wuffs.New().Probe(src)
		if err != nil {
			t.Fatalf("Probe(muybridge.gif) error = %v, want nil", err)
		}
		w, h := int(probe.Width), int(probe.Height)
		dstBytes := int(probe.Stride) * h

		indices := []int{3, 11}
		errs := make([]error, len(indices))
		crcs := make([]uint32, len(indices))

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(len(indices))
		for i, index := range indices {
			go func(i, index int) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						errs[i] = fmt.Errorf("decoder %d (frame %d) panicked: %v", i, index, r)
					}
				}()
				<-start
				d := wuffs.New()
				if resErr := wuffs.RequiredReserve(d, dstBytes, len(src)); resErr != nil {
					errs[i] = fmt.Errorf("decoder %d RequiredReserve: %w", i, resErr)
					return
				}
				dst := image.NewRGBA(image.Rect(0, 0, w, h))
				frame, decErr := d.DecodeFrame(dst, src, index)
				if decErr != nil {
					errs[i] = fmt.Errorf("decoder %d DecodeFrame(src, %d): %w", i, index, decErr)
					return
				}
				if frame == nil {
					errs[i] = fmt.Errorf("decoder %d DecodeFrame(src, %d) returned nil Frame", i, index)
					return
				}
				if frame.Index != index {
					errs[i] = fmt.Errorf("decoder %d Frame.Index = %d, want %d", i, frame.Index, index)
					return
				}
				crcs[i] = crc32.ChecksumIEEE(dst.Pix)
			}(i, index)
		}
		close(start)
		wg.Wait()

		for i, index := range indices {
			if errs[i] != nil {
				t.Errorf("%v", errs[i])
				continue
			}
			want, ok := manifest["muybridge.gif"][index]
			if !ok {
				t.Fatalf("frame %d: no manifest entry for muybridge.gif", index)
			}
			if crcs[i] != want {
				t.Errorf("frame %d: concurrent full-canvas CRC = 0x%08X, want manifest 0x%08X", index, crcs[i], want)
			}
		}
	})
}
