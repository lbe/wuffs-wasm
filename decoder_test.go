package wuffs_test

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/lbe/wuffs-wasm"
)

// loadFixture reads a file from the testdata directory and fails the test on error.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading testdata/%s: %v", name, err)
	}
	return data
}

// anyNonZeroPixels reports whether dst contains at least one non-zero pixel.
func anyNonZeroPixels(t *testing.T, dst *image.RGBA) bool {
	t.Helper()
	for y := 0; y < dst.Rect.Dy(); y++ {
		for x := 0; x < dst.Rect.Dx(); x++ {
			if c := dst.RGBAAt(x, y); c != (color.RGBA{}) {
				return true
			}
		}
	}
	return false
}

// TestIntegrationVersion verifies that the decoder reports the embedded
// Wuffs library version numerically as 0x00040000 (Wuffs 0.4).
func TestIntegrationVersion(t *testing.T) {
	d := wuffs.New()
	if d == nil {
		t.Fatal("New() returned nil decoder")
	}

	// The embedded Wuffs library version must be 0x00040000 (Wuffs 0.4).
	const expectedVersion = 0x00040000
	if got := d.VersionNum(); got != expectedVersion {
		t.Errorf("VersionNum() = 0x%08X, want 0x%08X", got, expectedVersion)
	}
}

// TestIntegrationFixturesPresent verifies that the vendored test fixture
// files are present in the testdata/ directory.
func TestIntegrationFixturesPresent(t *testing.T) {
	fixtures := []string{
		"bricks-color.png",
		"harvesters.png",
		"bricks-color.lossless.webp",
	}

	for _, fixture := range fixtures {
		path := filepath.Join("testdata", fixture)
		t.Run(fixture, func(t *testing.T) {
			if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
				t.Errorf("missing test fixture: %s", path)
			}
		})
	}
}

// TestIntegrationDecodeRGBA_PNG verifies that DecodeRGBA decodes
// testdata/bricks-color.png (160×120) into a caller-owned, correctly sized
// image.RGBA, that an undersized destination triggers DstTooSmallError, that
// the caller's Pix/Rect/Stride are never mutated, and that the same Decoder
// can decode into the same destination twice.
func TestIntegrationDecodeRGBA_PNG(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	const (
		wantW = 160
		wantH = 120
	)

	t.Run("success pre-allocated RGBA", func(t *testing.T) {
		d := wuffs.New()
		decodePreallocatedRGBA(t, d, pngSrc, wantW, wantH)
	})

	t.Run("undersized Pix returns DstTooSmallError", func(t *testing.T) {
		d := wuffs.New()

		// Caller sets a correctly sized Rect but provides Pix that is far too
		// small. The host validation must reject it after decode.
		dst := &image.RGBA{
			Rect:   image.Rect(0, 0, wantW, wantH),
			Stride: wantW * 4,
			Pix:    make([]byte, 4), // Way too small for 160×120.
		}

		_, err := d.DecodeRGBA(dst, pngSrc)
		if err == nil {
			t.Fatal("expected DstTooSmallError for undersized Pix, got nil")
		}
		var dstErr *wuffs.DstTooSmallError
		if !errors.As(err, &dstErr) {
			t.Fatalf("expected *DstTooSmallError, got %T: %v", err, err)
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

	t.Run("repeated decode succeeds", func(t *testing.T) {
		d := wuffs.New()

		// One correctly sized destination reused across both iterations.
		dst := image.NewRGBA(image.Rect(0, 0, wantW, wantH))
		for i := 0; i < 2; i++ {
			meta, err := d.DecodeRGBA(dst, pngSrc)
			if err != nil {
				t.Fatalf("iteration %d: DecodeRGBA: %v", i, err)
			}
			if meta == nil {
				t.Fatalf("iteration %d: nil Meta", i)
			}
			if got := int(meta.Width); got != wantW {
				t.Errorf("iteration %d: Meta.Width = %d, want %d", i, got, wantW)
			}
			if got := int(meta.Height); got != wantH {
				t.Errorf("iteration %d: Meta.Height = %d, want %d", i, got, wantH)
			}
		}
	})

	t.Run("Pix/Rect/Stride identity preserved", func(t *testing.T) {
		d := wuffs.New()
		if resErr := wuffs.RequiredReserve(d, wantW*wantH*4, len(pngSrc)); resErr != nil {
			t.Fatalf("RequiredReserve: %v", resErr)
		}

		dst := image.NewRGBA(image.Rect(0, 0, wantW, wantH))

		// Capture destination identity before decode.
		ptr := unsafe.SliceData(dst.Pix)
		ln := len(dst.Pix)
		cp := cap(dst.Pix)
		rect := dst.Rect
		stride := dst.Stride

		meta, err := d.DecodeRGBA(dst, pngSrc)
		if err != nil {
			t.Fatalf("DecodeRGBA: %v", err)
		}
		if meta == nil {
			t.Fatal("DecodeRGBA returned nil Meta")
		}

		// Assert identity after the first decode.
		if unsafe.SliceData(dst.Pix) != ptr {
			t.Error("dst.Pix data pointer changed after decode")
		}
		if len(dst.Pix) != ln {
			t.Errorf("len(dst.Pix) = %d, want %d", len(dst.Pix), ln)
		}
		if cap(dst.Pix) != cp {
			t.Errorf("cap(dst.Pix) = %d, want %d", cap(dst.Pix), cp)
		}
		if dst.Rect != rect {
			t.Errorf("dst.Rect = %v, want %v", dst.Rect, rect)
		}
		if dst.Stride != stride {
			t.Errorf("dst.Stride = %d, want %d", dst.Stride, stride)
		}
		// Decode must have produced pixels in the caller's buffer.
		if !anyNonZeroPixels(t, dst) {
			t.Error("decoded image is entirely zero; expected non-zero pixels")
		}

		// Decode a second time into the same destination.
		meta2, err2 := d.DecodeRGBA(dst, pngSrc)
		if err2 != nil {
			t.Fatalf("second DecodeRGBA: %v", err2)
		}
		if meta2 == nil {
			t.Fatal("second DecodeRGBA returned nil Meta")
		}
		if unsafe.SliceData(dst.Pix) != ptr {
			t.Error("dst.Pix data pointer changed after second decode")
		}
		if len(dst.Pix) != ln {
			t.Errorf("len(dst.Pix) after second decode = %d, want %d", len(dst.Pix), ln)
		}
		if cap(dst.Pix) != cp {
			t.Errorf("cap(dst.Pix) after second decode = %d, want %d", cap(dst.Pix), cp)
		}
		if dst.Rect != rect {
			t.Errorf("dst.Rect after second decode = %v, want %v", dst.Rect, rect)
		}
		if dst.Stride != stride {
			t.Errorf("dst.Stride after second decode = %d, want %d", dst.Stride, stride)
		}
	})
}

// TestIntegrationDecodeRGBA_PaddedStride verifies that a destination with
// Stride > width*4 places every decoded row at dst.Stride, preserves the
// padding bytes, and leaves the caller's Pix, Rect, and Stride unchanged. It
// must fail if convert advances destination rows by width*4 instead of
// dst.Stride.
func TestIntegrationDecodeRGBA_PaddedStride(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	const (
		wantW  = 160
		wantH  = 120
		pad    = 16
		stride = wantW*4 + pad
	)

	d := wuffs.New()
	if resErr := wuffs.RequiredReserve(d, stride*wantH, len(pngSrc)); resErr != nil {
		t.Fatalf("RequiredReserve: %v", resErr)
	}

	dst := &image.RGBA{
		Rect:   image.Rect(0, 0, wantW, wantH),
		Stride: stride,
		Pix:    make([]byte, stride*wantH),
	}

	// Save the padding tail of every row so we can prove it is untouched.
	savedPad := make([][]byte, wantH)
	for r := 0; r < wantH; r++ {
		off := r*stride + wantW*4
		savedPad[r] = append([]byte(nil), dst.Pix[off:off+pad]...)
	}

	// Capture identity before decode.
	ptr := unsafe.SliceData(dst.Pix)
	ln := len(dst.Pix)
	cp := cap(dst.Pix)
	rect := dst.Rect
	st := dst.Stride

	meta, err := d.DecodeRGBA(dst, pngSrc)
	if err != nil {
		t.Fatalf("DecodeRGBA: %v", err)
	}
	if meta == nil {
		t.Fatal("DecodeRGBA returned nil Meta")
	}
	if got := int(meta.Width); got != wantW {
		t.Errorf("Meta.Width = %d, want %d", got, wantW)
	}
	if got := int(meta.Height); got != wantH {
		t.Errorf("Meta.Height = %d, want %d", got, wantH)
	}

	// Decode a tight (no padding) destination to obtain the expected pixels.
	tight := image.NewRGBA(image.Rect(0, 0, wantW, wantH))
	if resErr := wuffs.RequiredReserve(d, wantW*wantH*4, len(pngSrc)); resErr != nil {
		t.Fatalf("RequiredReserve (tight): %v", resErr)
	}
	tMeta, tErr := d.DecodeRGBA(tight, pngSrc)
	if tErr != nil {
		t.Fatalf("tight DecodeRGBA: %v", tErr)
	}
	if tMeta == nil {
		t.Fatal("tight DecodeRGBA returned nil Meta")
	}

	for r := 0; r < wantH; r++ {
		got := dst.Pix[r*stride : r*stride+wantW*4]
		want := tight.Pix[r*tight.Stride : r*tight.Stride+wantW*4]
		if !bytes.Equal(got, want) {
			t.Errorf("row %d: padded decode differs from tight decode", r)
		}
		// Padding tail must be unchanged.
		off := r*stride + wantW*4
		if !bytes.Equal(dst.Pix[off:off+pad], savedPad[r]) {
			t.Errorf("row %d: padding bytes changed after decode", r)
		}
	}

	// Identity must be preserved.
	if unsafe.SliceData(dst.Pix) != ptr {
		t.Error("dst.Pix data pointer changed after decode")
	}
	if len(dst.Pix) != ln {
		t.Errorf("len(dst.Pix) = %d, want %d", len(dst.Pix), ln)
	}
	if cap(dst.Pix) != cp {
		t.Errorf("cap(dst.Pix) = %d, want %d", cap(dst.Pix), cp)
	}
	if dst.Rect != rect {
		t.Errorf("dst.Rect = %v, want %v", dst.Rect, rect)
	}
	if dst.Stride != st {
		t.Errorf("dst.Stride = %d, want %d", dst.Stride, st)
	}
}

// TestIntegrationDecodeRGBA_UnrepresentableHostLayout proves that a valid
// decoded rectangle with a very large positive Stride whose required byte
// count cannot be represented by DstTooSmallError.MinBytes (uint32) returns
// ErrBadImage without panicking, overflowing, or truncating a structured error.
func TestIntegrationDecodeRGBA_UnrepresentableHostLayout(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	const (
		wantW = 160
		wantH = 120
	)

	// A stride large enough that Stride*Height exceeds uint32. Skip only when
	// int cannot represent such a stride on this platform.
	const hugeStride = int(1) << 30
	if hugeStride <= 0 || hugeStride*wantH < 0 {
		t.Skipf("int cannot represent stride %d", hugeStride)
	}

	// Non-empty Pix so the empty/nil checks do not fire first.
	dst := &image.RGBA{
		Rect:   image.Rect(0, 0, wantW, wantH),
		Stride: hugeStride,
		Pix:    make([]byte, 4),
	}

	d := wuffs.New()
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("DecodeRGBA panicked: %v", r)
			}
		}()
		_, err = d.DecodeRGBA(dst, pngSrc)
	}()
	if err == nil {
		t.Fatal("expected error for unrepresentable host layout, got nil")
	}
	if !errors.Is(err, wuffs.ErrBadImage) {
		t.Errorf("errors.Is(err, ErrBadImage) = false, want true (err = %v)", err)
	}
	if errors.Is(err, wuffs.ErrDstTooSmall) {
		t.Errorf("errors.Is(err, ErrDstTooSmall) = true, want false (err = %v)", err)
	}
	var dstErr *wuffs.DstTooSmallError
	if errors.As(err, &dstErr) {
		t.Errorf("errors.As extracted *DstTooSmallError, want false (err = %v)", err)
	}
}

// TestIntegrationDecodeRGBA_DstValidation covers each host validation branch:
// ErrBadImage for empty/nil/empty Pix, non-zero Rect.Min, and dimension
// mismatch; *DstTooSmallError for too-small Stride or short Pix.
func TestIntegrationDecodeRGBA_DstValidation(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	const (
		wantW = 160
		wantH = 120
	)

	t.Run("empty Rect returns ErrBadImage", func(t *testing.T) {
		d := wuffs.New()
		dst := &image.RGBA{Rect: image.Rect(0, 0, 0, 0)}
		_, err := d.DecodeRGBA(dst, pngSrc)
		if !errors.Is(err, wuffs.ErrBadImage) {
			t.Fatalf("errors.Is(err, ErrBadImage) = false, want true (err = %v)", err)
		}
	})

	t.Run("nil Pix returns ErrBadImage", func(t *testing.T) {
		d := wuffs.New()
		dst := &image.RGBA{Rect: image.Rect(0, 0, wantW, wantH), Stride: wantW * 4, Pix: nil}
		_, err := d.DecodeRGBA(dst, pngSrc)
		if !errors.Is(err, wuffs.ErrBadImage) {
			t.Fatalf("errors.Is(err, ErrBadImage) = false, want true (err = %v)", err)
		}
	})

	t.Run("empty Pix returns ErrBadImage", func(t *testing.T) {
		d := wuffs.New()
		dst := &image.RGBA{Rect: image.Rect(0, 0, wantW, wantH), Stride: wantW * 4, Pix: make([]byte, 0)}
		_, err := d.DecodeRGBA(dst, pngSrc)
		if !errors.Is(err, wuffs.ErrBadImage) {
			t.Fatalf("errors.Is(err, ErrBadImage) = false, want true (err = %v)", err)
		}
	})

	t.Run("non-zero Rect.Min returns ErrBadImage", func(t *testing.T) {
		d := wuffs.New()
		dst := &image.RGBA{
			Rect:   image.Rect(2, 3, 2+wantW, 3+wantH),
			Stride: wantW * 4,
			Pix:    make([]byte, wantW*wantH*4),
		}
		_, err := d.DecodeRGBA(dst, pngSrc)
		if !errors.Is(err, wuffs.ErrBadImage) {
			t.Fatalf("errors.Is(err, ErrBadImage) = false, want true (err = %v)", err)
		}
	})

	t.Run("wrong dimensions return ErrBadImage", func(t *testing.T) {
		d := wuffs.New()
		dst := &image.RGBA{
			Rect:   image.Rect(0, 0, 100, wantH),
			Stride: 100 * 4,
			Pix:    make([]byte, 100*wantH*4),
		}
		_, err := d.DecodeRGBA(dst, pngSrc)
		if !errors.Is(err, wuffs.ErrBadImage) {
			t.Fatalf("errors.Is(err, ErrBadImage) = false, want true (err = %v)", err)
		}
	})

	t.Run("too-small Stride returns DstTooSmallError", func(t *testing.T) {
		d := wuffs.New()
		dst := &image.RGBA{
			Rect:   image.Rect(0, 0, wantW, wantH),
			Stride: 1,
			Pix:    make([]byte, wantW*wantH*4),
		}
		_, err := d.DecodeRGBA(dst, pngSrc)
		if err == nil {
			t.Fatal("expected *DstTooSmallError for too-small Stride, got nil")
		}
		var dstErr *wuffs.DstTooSmallError
		if !errors.As(err, &dstErr) {
			t.Fatalf("expected *DstTooSmallError, got %T: %v", err, err)
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
		d := wuffs.New()
		// Padded stride: rows are laid out Dx*4 bytes each but the destination
		// stride is wider. Allocate Pix one byte short of the padded requirement
		// so len(Pix) < Stride*Dy while the tight width*4 row still fits.
		stride := wantW*4 + 8
		dst := &image.RGBA{
			Rect:   image.Rect(0, 0, wantW, wantH),
			Stride: stride,
			Pix:    make([]byte, stride*wantH-1),
		}
		_, err := d.DecodeRGBA(dst, pngSrc)
		if err == nil {
			t.Fatal("expected *DstTooSmallError for short Pix, got nil")
		}
		var dstErr *wuffs.DstTooSmallError
		if !errors.As(err, &dstErr) {
			t.Fatalf("expected *DstTooSmallError, got %T: %v", err, err)
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
}

// TestIntegrationReserveRetry verifies that a guest DstTooSmallError surfaces
// the decoded image dimensions from the guest meta slot, and that the caller
// can reserve a larger destination slot and retry successfully into a
// correctly sized host destination.
func TestIntegrationReserveRetry(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	const (
		wantW = 160
		wantH = 120
	)
	wantStride := uint32(wantW * 4)
	wantMinBytes := wantStride * wantH

	// Shrink the initial destination slot so the guest cannot fit the decoded
	// image and must return DstTooSmallError. Restore the default after the
	// test so other tests are not affected.
	restore := wuffs.SetInitialDstSlotBytes(1024)
	defer restore()

	d := wuffs.New()
	// Correctly sized host destination; the guest dst slot is what is too small.
	dst := image.NewRGBA(image.Rect(0, 0, wantW, wantH))

	// Capture caller-owned destination properties before the failing decode.
	wantPixPtr := unsafe.SliceData(dst.Pix)
	wantPixLen := len(dst.Pix)
	wantPixCap := cap(dst.Pix)
	wantRect := dst.Rect
	wantStrideInt := dst.Stride

	// First decode attempt: the guest destination slot is too small.
	_, err := d.DecodeRGBA(dst, pngSrc)
	if err == nil {
		t.Fatal("expected DstTooSmallError on first decode, got nil")
	}
	var dstErr *wuffs.DstTooSmallError
	if !errors.As(err, &dstErr) {
		t.Fatalf("expected *DstTooSmallError, got %T: %v", err, err)
	}
	if dstErr.MinBytes < wantMinBytes {
		t.Errorf("DstTooSmallError.MinBytes = %d, want >= %d", dstErr.MinBytes, wantMinBytes)
	}
	if dstErr.Width != wantW {
		t.Errorf("DstTooSmallError.Width = %d, want %d", dstErr.Width, wantW)
	}
	if dstErr.Height != wantH {
		t.Errorf("DstTooSmallError.Height = %d, want %d", dstErr.Height, wantH)
	}
	if dstErr.Stride != wantStride {
		t.Errorf("DstTooSmallError.Stride = %d, want %d", dstErr.Stride, wantStride)
	}
	// The failed decode must not have perturbed the caller-owned destination.
	if got := unsafe.SliceData(dst.Pix); got != wantPixPtr {
		t.Errorf("Pix pointer changed after failure: %p -> %p", wantPixPtr, got)
	}
	if len(dst.Pix) != wantPixLen || cap(dst.Pix) != wantPixCap {
		t.Errorf("Pix len/cap changed after failure: len %d->%d cap %d->%d", wantPixLen, len(dst.Pix), wantPixCap, cap(dst.Pix))
	}
	if dst.Rect != wantRect || dst.Stride != wantStrideInt {
		t.Errorf("Rect/Stride changed after failure: %v/%d -> %v/%d", wantRect, wantStrideInt, dst.Rect, dst.Stride)
	}

	// Reserve a destination slot large enough for the decoded image and retry.
	if resErr := wuffs.RequiredReserve(d, wantW*wantH*4, len(pngSrc)); resErr != nil {
		t.Fatalf("RequiredReserve: %v", resErr)
	}
	meta, err := d.DecodeRGBA(dst, pngSrc)
	if err != nil {
		t.Fatalf("retry DecodeRGBA: %v", err)
	}
	if meta == nil {
		t.Fatal("retry DecodeRGBA returned nil Meta")
	}
	if meta.Width != wantW {
		t.Errorf("retry Meta.Width = %d, want %d", meta.Width, wantW)
	}
	if meta.Height != wantH {
		t.Errorf("retry Meta.Height = %d, want %d", meta.Height, wantH)
	}
	// Pixel identity: the caller's Pix must now hold non-zero decoded data.
	if !anyNonZeroPixels(t, dst) {
		t.Error("retry decode produced zero pixels")
	}
	// The successful retry must reuse the very same destination: pointer, len,
	// cap, Rect, and Stride are all unchanged by the decode (only pixels differ).
	if got := unsafe.SliceData(dst.Pix); got != wantPixPtr {
		t.Errorf("Pix pointer changed after retry: %p -> %p", wantPixPtr, got)
	}
	if len(dst.Pix) != wantPixLen || cap(dst.Pix) != wantPixCap {
		t.Errorf("Pix len/cap changed after retry: len %d->%d cap %d->%d", wantPixLen, len(dst.Pix), wantPixCap, cap(dst.Pix))
	}
	if dst.Rect != wantRect || dst.Stride != wantStrideInt {
		t.Errorf("Rect/Stride changed after retry: %v/%d -> %v/%d", wantRect, wantStrideInt, dst.Rect, dst.Stride)
	}
}

// TestBenchmarkPNGBaselineRecord measures decode throughput for a realistic
// PNG (harvesters.png 1165×859 RGBA, ~1.99 MB src) and logs MB/s and ns/op.
// It uses the RequiredReserve test helper to explicitly size the dst slot
// (~4 MiB) before the timed decode loop. The test passes if decode succeeds;
// no throughput assertion is made — human go/no-go after Plan 1.
//
// A successful decode implicitly verifies that the guest bump allocator
// region is not exceeded for this large PNG fixture.
func TestBenchmarkPNGBaselineRecord(t *testing.T) {
	pngSrc := loadFixture(t, "harvesters.png")

	d := wuffs.New()

	// harvesters.png is 1165×859 RGBA → ~4 MiB decoded.
	const (
		wantW    = 1165
		wantH    = 859
		dstBytes = wantW * wantH * 4
	)
	srcBytes := len(pngSrc)

	// Reserve memory using the test helper with explicit sizes.
	if resErr := wuffs.RequiredReserve(d, dstBytes, srcBytes); resErr != nil {
		t.Fatalf("RequiredReserve: %v", resErr)
	}

	// Timed decode loop.
	const iterations = 10
	start := time.Now()
	var (
		meta *wuffs.Meta
		err  error
	)
	dst := image.NewRGBA(image.Rect(0, 0, wantW, wantH))
	for i := 0; i < iterations; i++ {
		meta, err = d.DecodeRGBA(dst, pngSrc)
		if err != nil {
			t.Fatalf("DecodeRGBA iteration %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	nsPerOp := elapsed.Nanoseconds() / iterations
	throughputMB := float64(len(pngSrc)) * float64(iterations) / elapsed.Seconds() / (1024 * 1024)

	t.Logf("decode throughput: %.2f MB/s, %d ns/op", throughputMB, nsPerOp)

	// --- stdlib comparison ---
	start2 := time.Now()
	for i := 0; i < iterations; i++ {
		if _, err := png.Decode(bytes.NewReader(pngSrc)); err != nil {
			t.Fatalf("stdlib png.Decode iteration %d: %v", i, err)
		}
	}
	elapsed2 := time.Since(start2)

	nsPerOp2 := elapsed2.Nanoseconds() / iterations
	throughputMB2 := float64(len(pngSrc)) * float64(iterations) / elapsed2.Seconds() / (1024 * 1024)

	t.Logf("stdlib decode throughput: %.2f MB/s, %d ns/op", throughputMB2, nsPerOp2)
	t.Logf("comparison harvesters: wuffs-go %.2f MB/s (%d ns/op) vs stdlib %.2f MB/s (%d ns/op)",
		throughputMB, nsPerOp, throughputMB2, nsPerOp2)

	if meta == nil {
		t.Fatal("DecodeRGBA returned nil Meta")
	}
}

// TestIntegrationProbe_PNG verifies that Probe reports the PNG image
// dimensions (160×120), stride (640), and format (FormatPNG) for
// testdata/bricks-color.png without decoding pixels (BytesWritten == 0, err
// == nil). It also verifies that Probe is repeatable on the same Decoder, and
// that a Decoder which has already decoded via DecodeRGBA can still Probe.
func TestIntegrationProbe_PNG(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	const (
		wantW     = 160
		wantH     = 120
		wantStrid = 640
		wantFmt   = wuffs.FormatPNG
	)

	d := wuffs.New()

	meta, err := d.Probe(pngSrc)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if meta == nil {
		t.Fatal("Probe returned nil Meta")
	}
	if meta.Width != wantW {
		t.Errorf("Probe Meta.Width = %d, want %d", meta.Width, wantW)
	}
	if meta.Height != wantH {
		t.Errorf("Probe Meta.Height = %d, want %d", meta.Height, wantH)
	}
	if meta.Stride != wantStrid {
		t.Errorf("Probe Meta.Stride = %d, want %d", meta.Stride, wantStrid)
	}
	if meta.Format != wantFmt {
		t.Errorf("Probe Meta.Format = 0x%08X, want 0x%08X", meta.Format, wantFmt)
	}
	if meta.BytesWritten != 0 {
		t.Errorf("Probe Meta.BytesWritten = %d, want 0", meta.BytesWritten)
	}

	// Probe twice on the same Decoder; both must return the same Meta.
	meta2, err2 := d.Probe(pngSrc)
	if err2 != nil {
		t.Fatalf("Probe (second call): %v", err2)
	}
	if meta2 == nil {
		t.Fatal("Probe (second call) returned nil Meta")
	}
	if meta2.Width != wantW {
		t.Errorf("Probe (second call) Meta.Width = %d, want %d", meta2.Width, wantW)
	}
	if meta2.Height != wantH {
		t.Errorf("Probe (second call) Meta.Height = %d, want %d", meta2.Height, wantH)
	}
	if meta2.Stride != wantStrid {
		t.Errorf("Probe (second call) Meta.Stride = %d, want %d", meta2.Stride, wantStrid)
	}
	if meta2.Format != wantFmt {
		t.Errorf("Probe (second call) Meta.Format = 0x%08X, want 0x%08X", meta2.Format, wantFmt)
	}
	if meta2.BytesWritten != 0 {
		t.Errorf("Probe (second call) Meta.BytesWritten = %d, want 0", meta2.BytesWritten)
	}

	// DecodeRGBA then Probe again on the same Decoder.
	dst := image.NewRGBA(image.Rect(0, 0, wantW, wantH))
	if _, decErr := d.DecodeRGBA(dst, pngSrc); decErr != nil {
		t.Fatalf("DecodeRGBA: %v", decErr)
	}
	meta3, err3 := d.Probe(pngSrc)
	if err3 != nil {
		t.Fatalf("Probe (after DecodeRGBA): %v", err3)
	}
	if meta3 == nil {
		t.Fatal("Probe (after DecodeRGBA) returned nil Meta")
	}
	if meta3.Width != wantW {
		t.Errorf("Probe (after DecodeRGBA) Meta.Width = %d, want %d", meta3.Width, wantW)
	}
	if meta3.Height != wantH {
		t.Errorf("Probe (after DecodeRGBA) Meta.Height = %d, want %d", meta3.Height, wantH)
	}
	if meta3.Stride != wantStrid {
		t.Errorf("Probe (after DecodeRGBA) Meta.Stride = %d, want %d", meta3.Stride, wantStrid)
	}
	if meta3.Format != wantFmt {
		t.Errorf("Probe (after DecodeRGBA) Meta.Format = 0x%08X, want 0x%08X", meta3.Format, wantFmt)
	}
	if meta3.BytesWritten != 0 {
		t.Errorf("Probe (after DecodeRGBA) Meta.BytesWritten = %d, want 0", meta3.BytesWritten)
	}
}

// TestIntegrationProbe_SentinelErrors verifies that Probe maps the
// guest-returned sentinel paths to the expected host sentinel errors, and that
// an incomplete-but-config-readable PNG still succeeds. It covers: an
// unrecognized source format (ErrUnknownFormat), zero-length source
// (ErrDecode), a truncated PNG whose IHDR still yields a valid config (success,
// not ErrDecode), source exceeding the shrunk src-slot capacity
// (ErrSrcTooLarge), and a shrunk dst slot where Probe does not grow dst while
// a subsequent DecodeRGBA still surfaces DstTooSmallError.
func TestIntegrationProbe_SentinelErrors(t *testing.T) {
	garbage := []byte("\x00garbage: this is not any supported image format\xff\xfe\x00\x01")

	pngSrc := loadFixture(t, "bricks-color.png")
	harvest := loadFixture(t, "harvesters.png")

	tests := []struct {
		name string
		src  []byte
		want error // nil means expect success (no error)
	}{
		{
			name: "unknown format returns ErrUnknownFormat",
			src:  garbage,
			want: wuffs.ErrUnknownFormat,
		},
		{
			name: "empty src returns ErrDecode",
			src:  nil,
			want: wuffs.ErrDecode,
		},
		{
			name: "truncated PNG config succeeds",
			src:  pngSrc[:50],
			want: nil,
		},
		{
			name: "src too large returns ErrSrcTooLarge",
			src:  harvest,
			want: wuffs.ErrSrcTooLarge,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if errors.Is(tc.want, wuffs.ErrSrcTooLarge) {
				if got := wuffs.ShrunkMaxSrc(); len(tc.src) <= got {
					t.Skipf("src (%d bytes) does not exceed shrunk src slot (%d bytes)", len(tc.src), got)
				}
			}

			d := wuffs.New()
			meta, err := d.Probe(tc.src)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("Probe() error = %v, want nil", err)
				}
				if meta == nil {
					t.Fatal("Probe() returned nil Meta, want non-nil")
				}
				if meta.Width != 160 || meta.Height != 120 || meta.Stride != 640 {
					t.Errorf("Probe Meta = {W:%d H:%d S:%d}, want {W:160 H:120 S:640}", meta.Width, meta.Height, meta.Stride)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("Probe() error = %v, want errors.Is(err, %v) to be true", err, tc.want)
			}
		})
	}

	// Shrunk-dst block: Probe must not grow the dst slot, and a subsequent
	// DecodeRGBA with a correctly sized host destination must still surface
	// DstTooSmallError from the guest scratch path.
	t.Run("shrunk dst slot Probe does not grow dst", func(t *testing.T) {
		restore := wuffs.SetInitialDstSlotBytes(1024)
		defer restore()

		d := wuffs.New()

		meta, err := d.Probe(pngSrc)
		if err != nil {
			t.Fatalf("Probe() error = %v, want nil", err)
		}
		if meta == nil {
			t.Fatal("Probe() returned nil Meta, want non-nil")
		}
		if meta.Width != 160 || meta.Height != 120 || meta.Stride != 640 {
			t.Errorf("Probe Meta = {W:%d H:%d S:%d}, want {W:160 H:120 S:640}", meta.Width, meta.Height, meta.Stride)
		}
		var dstErr *wuffs.DstTooSmallError
		if errors.As(err, &dstErr) {
			t.Error("Probe() returned *DstTooSmallError, want success")
		}

		// Probe did not grow the dst slot.
		if got := wuffs.CurrentDstSlotLen(d); got != 1024 {
			t.Errorf("CurrentDstSlotLen(d) = %d, want 1024", got)
		}

		// Same Decoder, correctly sized host dst, no Reserve: DecodeRGBA must
		// return *DstTooSmallError (1024-byte guest dst slot is still too small).
		dst := image.NewRGBA(image.Rect(0, 0, 160, 120))
		_, decErr := d.DecodeRGBA(dst, pngSrc)
		if decErr == nil {
			t.Fatal("DecodeRGBA() after Probe error = nil, want *DstTooSmallError")
		}
		if !errors.As(decErr, &dstErr) {
			t.Fatalf("DecodeRGBA() error = %v, want *DstTooSmallError", decErr)
		}
	})
}

// TestIntegrationProbeThenDecodeRGBA_PNG verifies the composed workflow of
// probing a PNG for its dimensions/stride and then decoding it via DecodeRGBA
// using the stride reported by Probe to size the destination slot. It asserts
// the decoded dimensions match the probed dimensions, that the decoded pixel
// CRC32 matches the checked-in golden value, and that later Probe and Reserve
// calls cannot mutate the already-decoded host pixels.
func TestIntegrationProbeThenDecodeRGBA_PNG(t *testing.T) {
	restore := wuffs.SetInitialDstSlotBytes(1024)
	defer restore()

	d := wuffs.New()

	pngSrc := loadFixture(t, "bricks-color.png")

	meta, err := d.Probe(pngSrc)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if meta == nil {
		t.Fatal("Probe returned nil Meta")
	}
	if meta.Width != 160 {
		t.Errorf("Probe Meta.Width = %d, want 160", meta.Width)
	}
	if meta.Height != 120 {
		t.Errorf("Probe Meta.Height = %d, want 120", meta.Height)
	}
	if meta.Stride != 640 {
		t.Errorf("Probe Meta.Stride = %d, want 640", meta.Stride)
	}
	if meta.BytesWritten != 0 {
		t.Errorf("Probe Meta.BytesWritten = %d, want 0", meta.BytesWritten)
	}

	// Size the dst slot using the Stride from Probe (same bytes as 160*120*4).
	wuffs.RequiredReserve(d, int(meta.Stride)*int(meta.Height), len(pngSrc))

	dst := image.NewRGBA(image.Rect(0, 0, 160, 120))
	meta2, err := d.DecodeRGBA(dst, pngSrc)
	if err != nil {
		t.Fatalf("DecodeRGBA: %v", err)
	}
	if meta2 == nil {
		t.Fatal("DecodeRGBA returned nil Meta")
	}
	if dst.Rect.Dx() != 160 || dst.Rect.Dy() != 120 {
		t.Errorf("dst.Rect = %v, want 160x120", dst.Rect)
	}

	// Compute CRC32 of the decoded pixel data.
	gotCRC := crc32.ChecksumIEEE(dst.Pix)

	// Read expected CRC32 from golden file.
	raw := loadFixture(t, "bricks-color.golden.crc32")

	wantCRC, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 32)
	if err != nil {
		t.Fatalf("parsing golden CRC32 from testdata/bricks-color.golden.crc32: %v", err)
	}

	if gotCRC != uint32(wantCRC) {
		t.Errorf("CRC32 of decoded Pix = 0x%08X, want 0x%08X", gotCRC, uint32(wantCRC))
	}

	// Persistence: copy decoded host pixels and prove later Probe/Reserve
	// cannot mutate them.
	cp := append([]byte(nil), dst.Pix...)

	if _, err := d.Probe(pngSrc); err != nil {
		t.Fatalf("Probe after decode: %v", err)
	}
	if !bytes.Equal(cp, dst.Pix) {
		t.Error("Probe mutated decoded host pixels")
	}

	if err := wuffs.RequiredReserve(d, 160*120*4, len(pngSrc)); err != nil {
		t.Fatalf("RequiredReserve after decode: %v", err)
	}
	if !bytes.Equal(cp, dst.Pix) {
		t.Error("Reserve mutated decoded host pixels")
	}
}

// TestIntegrationProbe_WEBP verifies that Probe reports the WebP image
// dimensions (160×120), stride (640), and format (FormatWEBP) for
// testdata/bricks-color.lossless.webp without decoding pixels
// (BytesWritten == 0, err == nil).
func TestIntegrationProbe_WEBP(t *testing.T) {
	webpSrc := loadFixture(t, "bricks-color.lossless.webp")

	const (
		wantW     = 160
		wantH     = 120
		wantStrid = 640
		wantFmt   = wuffs.FormatWEBP
	)

	d := wuffs.New()

	meta, err := d.Probe(webpSrc)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if meta == nil {
		t.Fatal("Probe returned nil Meta")
	}
	if meta.Width != wantW {
		t.Errorf("Probe Meta.Width = %d, want %d", meta.Width, wantW)
	}
	if meta.Height != wantH {
		t.Errorf("Probe Meta.Height = %d, want %d", meta.Height, wantH)
	}
	if meta.Stride != wantStrid {
		t.Errorf("Probe Meta.Stride = %d, want %d", meta.Stride, wantStrid)
	}
	if meta.Format != wantFmt {
		t.Errorf("Probe Meta.Format = 0x%08X, want 0x%08X", meta.Format, wantFmt)
	}
	if meta.BytesWritten != 0 {
		t.Errorf("Probe Meta.BytesWritten = %d, want 0", meta.BytesWritten)
	}
}

// TestIntegrationDecodeRGBA_PNGGolden verifies that the CRC32 of the decoded
// RGBA pixel data for testdata/bricks-color.png matches the checked-in golden
// value in testdata/bricks-color.golden.crc32. The golden file is generated by
// scripts/gen_golden.go using the integer unpremultiply semantics documented in
// convert.go. Regenerate via: go run scripts/gen_golden.go
func TestIntegrationDecodeRGBA_PNGGolden(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	dst := image.NewRGBA(image.Rect(0, 0, 160, 120))
	d := wuffs.New()

	meta, err := d.DecodeRGBA(dst, pngSrc)
	if err != nil {
		t.Fatalf("DecodeRGBA: %v", err)
	}
	if meta == nil {
		t.Fatal("DecodeRGBA returned nil Meta")
	}

	// Compute CRC32 of the decoded pixel data.
	gotCRC := crc32.ChecksumIEEE(dst.Pix)

	// Read expected CRC32 from golden file.
	raw := loadFixture(t, "bricks-color.golden.crc32")

	wantCRC, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 32)
	if err != nil {
		t.Fatalf("parsing golden CRC32 from testdata/bricks-color.golden.crc32: %v", err)
	}

	if gotCRC != uint32(wantCRC) {
		t.Errorf("CRC32 of decoded Pix = 0x%08X, want 0x%08X\n\tdst.Rect = %v\n\tdst.Stride = %d\n\tlen(dst.Pix) = %d", gotCRC, uint32(wantCRC), dst.Rect, dst.Stride, len(dst.Pix))
	}
}

// TestIntegrationDecodeRGBA_WEBP verifies that DecodeRGBA decodes
// testdata/bricks-color.lossless.webp (160×120) into a pre-allocated
// image.RGBA with correct dimensions and at least one non-zero pixel.
func TestIntegrationDecodeRGBA_WEBP(t *testing.T) {
	webpSrc := loadFixture(t, "bricks-color.lossless.webp")

	d := wuffs.New()
	decodePreallocatedRGBA(t, d, webpSrc, 160, 120)
}

// TestIntegrationDecodeRGBA_SentinelErrors verifies that guest-returned decode
// errors surface to the caller as the expected sentinel errors (asserted via
// errors.Is) without panicking. It drives the four sentinel paths: an
// unrecognized source format (ErrUnknownFormat), truncated/corrupt input
// (ErrDecode), zero-length source (ErrDecode), and source exceeding the shrunk
// src-slot capacity (ErrSrcTooLarge).
func TestIntegrationDecodeRGBA_SentinelErrors(t *testing.T) {
	garbage := []byte("\x00garbage: this is not any supported image format\xff\xfe\x00\x01")

	pngSrc := loadFixture(t, "bricks-color.png")
	harvSrc := loadFixture(t, "harvesters.png")

	tests := []struct {
		name string
		src  []byte
		want error
	}{
		{
			name: "unknown format returns ErrUnknownFormat",
			src:  garbage,
			want: wuffs.ErrUnknownFormat,
		},
		{
			name: "corrupt input returns ErrDecode",
			src:  pngSrc[:50],
			want: wuffs.ErrDecode,
		},
		{
			name: "empty src returns ErrDecode",
			src:  nil,
			want: wuffs.ErrDecode,
		},
		{
			name: "src too large returns ErrSrcTooLarge",
			src:  harvSrc,
			want: wuffs.ErrSrcTooLarge,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if errors.Is(tc.want, wuffs.ErrSrcTooLarge) {
				if got := wuffs.ShrunkMaxSrc(); len(tc.src) <= got {
					t.Skipf("src (%d bytes) does not exceed shrunk src slot (%d bytes)", len(tc.src), got)
				}
			}

			d := wuffs.New()
			dst := image.NewRGBA(image.Rect(0, 0, 160, 120))
			_, err := d.DecodeRGBA(dst, tc.src)
			if !errors.Is(err, tc.want) {
				t.Errorf("DecodeRGBA() error = %v, want errors.Is(err, %v) to be true", err, tc.want)
			}
		})
	}
}

// TestIntegrationDecodeRGBA_AllocsPerRun measures the number of heap allocations
// per DecodeRGBA call on bricks-color.png after RequiredReserve has sized the
// wasm memory slots and a warm-up decode has been performed. The target is
// exactly zero heap allocations on the hot path: the destination is allocated
// once outside the timed loop and reused.
func TestIntegrationDecodeRGBA_AllocsPerRun(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	d := wuffs.New()

	// Size reserve for the bricks-color.png fixture: 160×120 RGBA.
	const (
		wantW    = 160
		wantH    = 120
		dstBytes = wantW * wantH * 4
	)
	srcBytes := len(pngSrc)
	if resErr := wuffs.RequiredReserve(d, dstBytes, srcBytes); resErr != nil {
		t.Fatalf("RequiredReserve: %v", resErr)
	}

	// One correctly sized destination, allocated outside the timed loop.
	dst := image.NewRGBA(image.Rect(0, 0, wantW, wantH))

	// Warm up so any one-time setup (e.g. d.lastMeta aliasing) is complete
	// before measurement.
	if _, err := d.DecodeRGBA(dst, pngSrc); err != nil {
		t.Fatalf("warm-up DecodeRGBA: %v", err)
	}

	allocs := testing.AllocsPerRun(5, func() {
		if _, err := d.DecodeRGBA(dst, pngSrc); err != nil {
			t.Errorf("DecodeRGBA: %v", err)
		}
	})

	t.Logf("allocs per DecodeRGBA: %.0f (target: 0)", allocs)

	if allocs != 0 {
		t.Errorf("DecodeRGBA allocated %.0f heap objects per run, want 0", allocs)
	}
}

// decodePreallocatedRGBA decodes src into a caller-owned image.RGBA sized to
// wantW×wantH, reserving guest scratch, asserting that decode produced the
// expected dimensions and at least one non-zero pixel. It returns the
// decoder's Meta on success.
func decodePreallocatedRGBA(t *testing.T, d *wuffs.Decoder, src []byte, wantW, wantH int) *wuffs.Meta {
	t.Helper()

	dst := image.NewRGBA(image.Rect(0, 0, wantW, wantH))
	if resErr := wuffs.RequiredReserve(d, wantW*wantH*4, len(src)); resErr != nil {
		t.Fatalf("RequiredReserve: %v", resErr)
	}

	meta, err := d.DecodeRGBA(dst, src)
	if err != nil {
		t.Fatalf("DecodeRGBA: %v", err)
	}
	if meta == nil {
		t.Fatal("DecodeRGBA returned nil Meta")
	}

	if got := int(meta.Width); got != wantW {
		t.Errorf("Meta.Width = %d, want %d", got, wantW)
	}
	if got := int(meta.Height); got != wantH {
		t.Errorf("Meta.Height = %d, want %d", got, wantH)
	}

	// At least one pixel must be non-zero, proving decode produced data.
	if !anyNonZeroPixels(t, dst) {
		t.Error("decoded image is entirely zero; expected non-zero pixels")
	}

	return meta
}

// TestIntegrationDecodeRGBA_Format verifies that DecodeRGBA populates the
// Meta.Format field with the sniffed image format FourCC for each decoded
// image type. PNG decodes must yield FormatPNG and WEBP decodes must yield
// FormatWEBP, alongside the correct Width/Height/Stride.
func TestIntegrationDecodeRGBA_Format(t *testing.T) {
	tests := []struct {
		name    string
		srcFile string
		wantFmt uint32
	}{
		{
			name:    "BRICK_COLOR_PNG",
			srcFile: "bricks-color.png",
			wantFmt: wuffs.FormatPNG,
		},
		{
			name:    "BRICK_COLOR_LOSSLESS_WEBP",
			srcFile: "bricks-color.lossless.webp",
			wantFmt: wuffs.FormatWEBP,
		},
	}

	const (
		wantW     = 160
		wantH     = 120
		wantStrid = 640
	)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc := tc
			t.Parallel()

			src := loadFixture(t, tc.srcFile)
			d := wuffs.New()
			dst := image.NewRGBA(image.Rect(0, 0, wantW, wantH))

			meta, err := d.DecodeRGBA(dst, src)
			if err != nil {
				t.Fatalf("DecodeRGBA: %v", err)
			}
			if meta == nil {
				t.Fatal("DecodeRGBA returned nil Meta")
			}

			if got := int(meta.Width); got != wantW {
				t.Errorf("Meta.Width = %d, want %d", got, wantW)
			}
			if got := int(meta.Height); got != wantH {
				t.Errorf("Meta.Height = %d, want %d", got, wantH)
			}
			if got := int(meta.Stride); got != wantStrid {
				t.Errorf("Meta.Stride = %d, want %d", got, wantStrid)
			}
			if meta.Format != tc.wantFmt {
				t.Errorf("Meta.Format = 0x%08X, want 0x%08X", meta.Format, tc.wantFmt)
			}
		})
	}
}

// TestIntegrationConcurrentDecoders verifies that two separate *Decoder
// instances, each owning their own wasm2go module and WASI host state, can be
// created and decode PNG images concurrently without interfering with one
// another. Each decoder owns its own correctly sized destination.
func TestIntegrationConcurrentDecoders(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	const (
		wantW      = 160
		wantH      = 120
		iterations = 50
	)

	for i := 0; i < iterations; i++ {
		var d1, d2 *wuffs.Decoder
		var err1, err2 error
		var meta1, meta2 *wuffs.Meta

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)

		go func(iter int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					err1 = fmt.Errorf("decoder 1 panicked: %v", r)
				}
			}()
			<-start
			d1 = wuffs.New()
			dst := image.NewRGBA(image.Rect(0, 0, wantW, wantH))
			meta1, err1 = d1.DecodeRGBA(dst, pngSrc)
		}(i)

		go func(iter int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					err2 = fmt.Errorf("decoder 2 panicked: %v", r)
				}
			}()
			<-start
			d2 = wuffs.New()
			dst := image.NewRGBA(image.Rect(0, 0, wantW, wantH))
			meta2, err2 = d2.DecodeRGBA(dst, pngSrc)
		}(i)

		// Release both goroutines at the same time to maximize overlap.
		close(start)
		wg.Wait()

		if err1 != nil {
			t.Fatalf("%v", err1)
		}
		if err2 != nil {
			t.Fatalf("%v", err2)
		}
		if d1 == nil {
			t.Fatalf("iteration %d: decoder 1 is nil", i)
		}
		if d2 == nil {
			t.Fatalf("iteration %d: decoder 2 is nil", i)
		}

		for j, meta := range []*wuffs.Meta{meta1, meta2} {
			if meta == nil {
				t.Fatalf("iteration %d: decoder %d returned nil Meta", i, j+1)
			}
			if got := int(meta.Width); got != wantW {
				t.Errorf("iteration %d: decoder %d Meta.Width = %d, want %d", i, j+1, got, wantW)
			}
			if got := int(meta.Height); got != wantH {
				t.Errorf("iteration %d: decoder %d Meta.Height = %d, want %d", i, j+1, got, wantH)
			}
		}
	}
}
