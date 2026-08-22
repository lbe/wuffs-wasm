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
// testdata/bricks-color.png (160×120) into a pre-allocated image.RGBA
// with correct dimensions, that an undersized destination triggers
// DstTooSmallError, and that the same Decoder can decode twice.
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

		// Caller sets a non-zero Rect (Dx>0, Dy>0) but provides Pix that is
		// far too small. The host pre-check must reject before guest call.
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
	})

	t.Run("repeated decode succeeds", func(t *testing.T) {
		d := wuffs.New()

		for i := 0; i < 2; i++ {
			dst := image.NewRGBA(image.Rect(0, 0, 0, 0))
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
}

// TestIntegrationReserveRetry verifies that a guest DstTooSmallError surfaces
// the decoded image dimensions from the guest meta slot, and that the caller
// can reserve a larger destination slot and retry successfully.
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
	dst := image.NewRGBA(image.Rect(0, 0, 0, 0))

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
	if got := dst.Rect.Dx(); got != wantW {
		t.Errorf("retry dst.Rect.Dx() = %d, want %d", got, wantW)
	}
	if got := dst.Rect.Dy(); got != wantH {
		t.Errorf("retry dst.Rect.Dy() = %d, want %d", got, wantH)
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
	const dstBytes = 1165 * 859 * 4
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
	for i := 0; i < iterations; i++ {
		dst := image.NewRGBA(image.Rect(0, 0, 0, 0))
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

// TestIntegrationDecodeRGBA_PNGGolden verifies that the CRC32 of the decoded
// RGBA pixel data for testdata/bricks-color.png matches the checked-in golden
// value in testdata/bricks-color.golden.crc32. The golden file is generated by
// scripts/gen_golden.go using the integer unpremultiply semantics documented in
// convert.go. Regenerate via: go run scripts/gen_golden.go
func TestIntegrationDecodeRGBA_PNGGolden(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	dst := image.NewRGBA(image.Rect(0, 0, 0, 0))
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
			dst := image.NewRGBA(image.Rect(0, 0, 0, 0))
			_, err := d.DecodeRGBA(dst, tc.src)
			if !errors.Is(err, tc.want) {
				t.Errorf("DecodeRGBA() error = %v, want errors.Is(err, %v) to be true", err, tc.want)
			}
		})
	}
}

// TestIntegrationDecodeRGBA_AllocsPerRun measures the number of heap allocations
// per DecodeRGBA call on bricks-color.png after RequiredReserve has sized the
// wasm memory slots. The target is zero heap allocations on the hot path.
// Exceeding the documented ceiling is a gate failure.
//
// wasm2go ceiling: update the const below on first green run with the observed
// baseline alloc count from testing.AllocsPerRun.
func TestIntegrationDecodeRGBA_AllocsPerRun(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")

	d := wuffs.New()

	// Size reserve for the bricks-color.png fixture: 160×120 RGBA.
	const dstBytes = 160 * 120 * 4
	srcBytes := len(pngSrc)
	if resErr := wuffs.RequiredReserve(d, dstBytes, srcBytes); resErr != nil {
		t.Fatalf("RequiredReserve: %v", resErr)
	}

	// wasm2go ceiling: 1 alloc per decode after Reserve. The single allocation
	// is the image.NewRGBA struct itself; DecodeRGBA no longer allocates Pix or
	// Meta on the hot path. Observed baseline on first green run.
	const allocCeiling = 1

	allocs := testing.AllocsPerRun(5, func() {
		dst := image.NewRGBA(image.Rect(0, 0, 0, 0))
		if _, err := d.DecodeRGBA(dst, pngSrc); err != nil {
			t.Errorf("DecodeRGBA: %v", err)
		}
	})

	t.Logf("allocs per DecodeRGBA: %.0f (ceiling: %d)", allocs, allocCeiling)

	if allocs > allocCeiling {
		t.Errorf("DecodeRGBA allocated %.0f heap objects per run, want ≤ %d", allocs, allocCeiling)
	}
}

// decodePreallocatedRGBA decodes src into a pre-allocated image.RGBA with zero
// bounds (empty Rect), asserting that the guest fills the dimensions to
// wantW×wantH and that decode produced at least one non-zero pixel. It returns
// the decoder's Meta on success.
func decodePreallocatedRGBA(t *testing.T, d *wuffs.Decoder, src []byte, wantW, wantH int) *wuffs.Meta {
	t.Helper()

	dst := image.NewRGBA(image.Rect(0, 0, 0, 0))

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

	// After guest decode, dst.Rect should reflect the decoded dimensions.
	if got := dst.Rect.Dx(); got != wantW {
		t.Errorf("dst.Rect.Dx() = %d, want %d", got, wantW)
	}
	if got := dst.Rect.Dy(); got != wantH {
		t.Errorf("dst.Rect.Dy() = %d, want %d", got, wantH)
	}

	// At least one pixel must be non-zero, proving decode produced data.
	anyNonZero := false
	for y := 0; y < dst.Rect.Dy() && !anyNonZero; y++ {
		for x := 0; x < dst.Rect.Dx(); x++ {
			if c := dst.RGBAAt(x, y); c != (color.RGBA{}) {
				anyNonZero = true
				break
			}
		}
	}
	if !anyNonZero {
		t.Error("decoded image is entirely zero; expected non-zero pixels")
	}

	return meta
}

// TestIntegrationConcurrentDecoders verifies that two separate *Decoder
// instances, each owning their own wasm2go module and WASI host state, can be
// created and decode PNG images concurrently without interfering with one
// another. Each decoder must produce the correct image dimensions.
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
					err1 = fmt.Errorf("iteration %d: decoder 1 creation/decode panicked: %v", iter, r)
				}
			}()
			<-start
			d1 = wuffs.New()
			dst := image.NewRGBA(image.Rect(0, 0, 0, 0))
			meta1, err1 = d1.DecodeRGBA(dst, pngSrc)
		}(i)

		go func(iter int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					err2 = fmt.Errorf("iteration %d: decoder 2 creation/decode panicked: %v", iter, r)
				}
			}()
			<-start
			d2 = wuffs.New()
			dst := image.NewRGBA(image.Rect(0, 0, 0, 0))
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
