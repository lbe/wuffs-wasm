package wuffs

import (
	"errors"
	"image"
	"image/color"
	"reflect"
	"testing"
)

// typedHelper bundles one allocating decode helper with everything the shared
// typed contract matrix needs: a name, the operation strings its recording
// allocator and decoder post, an adapter that runs the helper against an
// arbitrary decoderOperations seam, and the concrete image type and color
// model the helper produces.
type typedHelper struct {
	name           string
	allocOp        string // operation recorded by the injected allocator
	decodeOp       string // operation recorded by the typed decode method
	decode         func(ops decoderOperations, rec *decoderRecorder, src []byte) (interface{}, *Meta, error)
	setDecodeErr   func(rec *decoderRecorder, err error)
	wantType       reflect.Type
	wantColorModel color.Model
}

// typedHelpers builds the RGBA, NRGBA, and Gray helper table crossed by the
// shared contract matrix. Each adapter runs its allocating helper through the
// decoderOperations seam; when rec is non-nil the adapter injects an
// allocator that records the helper's allocOp into the recorder, and when rec
// is nil it uses the helper's production default image constructor (needed
// for the real-decoder Meta detachment case).
func typedHelpers() []typedHelper {
	const (
		allocRGBA  = "typed-alloc-RGBA"
		allocNRGBA = "typed-alloc-NRGBA"
		allocGray  = "typed-alloc-Gray"
	)
	return []typedHelper{
		{
			name:           "RGBA",
			allocOp:        allocRGBA,
			decodeOp:       "DecodeRGBA",
			wantType:       reflect.TypeOf((*image.RGBA)(nil)),
			wantColorModel: color.RGBAModel,
			decode: func(ops decoderOperations, rec *decoderRecorder, src []byte) (interface{}, *Meta, error) {
				var alloc func(image.Rectangle) *image.RGBA
				if rec != nil {
					alloc = func(r image.Rectangle) *image.RGBA {
						rec.recordAlloc(allocRGBA)
						return image.NewRGBA(r)
					}
				}
				img, meta, err := decodeWithAllocator(src, ops, alloc)
				return interface{}(img), meta, err
			},
			setDecodeErr: func(rec *decoderRecorder, err error) { rec.rgbaDecodeErr = err },
		},
		{
			name:           "NRGBA",
			allocOp:        allocNRGBA,
			decodeOp:       "DecodeNRGBA",
			wantType:       reflect.TypeOf((*image.NRGBA)(nil)),
			wantColorModel: color.NRGBAModel,
			decode: func(ops decoderOperations, rec *decoderRecorder, src []byte) (interface{}, *Meta, error) {
				var alloc func(image.Rectangle) *image.NRGBA
				if rec != nil {
					alloc = func(r image.Rectangle) *image.NRGBA {
						rec.recordAlloc(allocNRGBA)
						return image.NewNRGBA(r)
					}
				}
				img, meta, err := decodeNRGBAWithAllocator(src, ops, alloc)
				return interface{}(img), meta, err
			},
			setDecodeErr: func(rec *decoderRecorder, err error) { rec.nrgbaDecodeErr = err },
		},
		{
			name:           "Gray",
			allocOp:        allocGray,
			decodeOp:       "DecodeGray",
			wantType:       reflect.TypeOf((*image.Gray)(nil)),
			wantColorModel: color.GrayModel,
			decode: func(ops decoderOperations, rec *decoderRecorder, src []byte) (interface{}, *Meta, error) {
				var alloc func(image.Rectangle) *image.Gray
				if rec != nil {
					alloc = func(r image.Rectangle) *image.Gray {
						rec.recordAlloc(allocGray)
						return image.NewGray(r)
					}
				}
				img, meta, err := decodeGrayWithAllocator(src, ops, alloc)
				return interface{}(img), meta, err
			},
			setDecodeErr: func(rec *decoderRecorder, err error) { rec.grayDecodeErr = err },
		},
	}
}

// caseEntry is one common contract case executed for every typed helper in
// the shared matrix.
type caseEntry struct {
	name string
	run  func(t *testing.T, h typedHelper)
}

// commonCases is the single shared case table for the typed decode contract.
// Because it is crossed with every typed helper, each entry proves its
// contract for Decode, DecodeNRGBA, and DecodeGray through the same matrix.
func commonCases() []caseEntry {
	const (
		matrixW = 160
		matrixH = 120
	)
	// src is opaque to the recorder; only its length feeds the Reserve calls.
	src := []byte("wuffs PNG typed matrix fixture")
	validMeta := &Meta{Width: matrixW, Height: matrixH}

	return []caseEntry{
		{
			// Requirement 1 (success order), 2 (guest size), and 9
			// (allocator order): exactly source Reserve, Probe, destination
			// Reserve with the checked guest bytes, typed host allocation,
			// then typed decode.
			name: "success order, guest size, and allocation",
			run: func(t *testing.T, h typedHelper) {
				rec := &decoderRecorder{probeMeta: validMeta}
				img, meta, err := h.decode(rec, rec, src)
				if err != nil {
					t.Fatalf("%s decode err = %v, want nil", h.name, err)
				}
				if img == nil {
					t.Fatalf("%s decode returned nil image, want non-nil", h.name)
				}
				if meta == nil {
					t.Fatalf("%s decode returned nil Meta, want non-nil", h.name)
				}

				// Exact call sequence and no extra operations.
				assertMatrixOps(t, h.name, rec.ops,
					[]string{"Reserve", "Probe", "Reserve", h.allocOp, h.decodeOp})

				// Exact Reserve arguments: source Reserve(0, len(src)) then
				// destination Reserve(Width*Height*4, len(src)). The guest
				// size is four bytes per pixel for every helper, including
				// Gray's one-byte host output.
				assertMatrixReserveArgs(t, h.name, rec,
					reserveCall{dstBytes: 0, srcBytes: len(src)},
					reserveCall{dstBytes: matrixW * matrixH * 4, srcBytes: len(src)})

				// The returned Meta mirrors the metadata exposed by Probe.
				if meta.Width != matrixW || meta.Height != matrixH {
					t.Errorf("%s Meta dims = (%d,%d), want (%d,%d)", h.name, meta.Width, meta.Height, matrixW, matrixH)
				}
			},
		},
		{
			// Requirement 3: the first (source) Reserve fails.
			name: "source Reserve failure",
			run: func(t *testing.T, h typedHelper) {
				sentinelErr := errors.New("injected " + h.name + " source Reserve failure")
				rec := &decoderRecorder{srcReserveErr: sentinelErr}
				img, meta, err := h.decode(rec, rec, src)

				assertMatrixExactError(t, h.name, err, sentinelErr)
				assertMatrixNilOutputs(t, h.name, img, meta)

				// Only the source Reserve runs; Probe and every later
				// operation, including the allocation, are skipped.
				assertMatrixOps(t, h.name, rec.ops, []string{"Reserve"})
				assertMatrixReserveArgs(t, h.name, rec, reserveCall{dstBytes: 0, srcBytes: len(src)})
			},
		},
		{
			// Requirement 4: Probe fails after the source Reserve.
			name: "Probe failure",
			run: func(t *testing.T, h typedHelper) {
				sentinelErr := errors.New("injected " + h.name + " Probe failure")
				rec := &decoderRecorder{probeErr: sentinelErr}
				img, meta, err := h.decode(rec, rec, src)

				assertMatrixExactError(t, h.name, err, sentinelErr)
				assertMatrixNilOutputs(t, h.name, img, meta)

				// Source Reserve and Probe run; destination Reserve,
				// allocation, and typed decode are skipped.
				assertMatrixOps(t, h.name, rec.ops, []string{"Reserve", "Probe"})
				assertMatrixReserveArgs(t, h.name, rec, reserveCall{dstBytes: 0, srcBytes: len(src)})
			},
		},
		{
			// Requirement 5: zero, overflowing, or otherwise unrepresentable
			// geometry fails with exactly ErrBadImage before destination
			// Reserve, allocation, or typed decode.
			name: "unsafe metadata returns ErrBadImage",
			run: func(t *testing.T, h typedHelper) {
				variants := []struct {
					name string
					meta Meta
				}{
					{"zero geometry", Meta{Width: 0, Height: 0}},
					{"dimension exceeds maxWuffsDimension", Meta{Width: uint32(maxWuffsDimension) + 1, Height: 1}},
					{"guest bytes overflow uint32", Meta{Width: 0xFFFF, Height: 0xFFFF}},
				}
				for _, v := range variants {
					t.Run(v.name, func(t *testing.T) {
						rec := &decoderRecorder{probeMeta: &v.meta}
						img, meta, err := h.decode(rec, rec, src)

						assertMatrixExactError(t, h.name, err, ErrBadImage)
						assertMatrixNilOutputs(t, h.name, img, meta)

						// Geometry is rejected after Probe and before
						// destination Reserve, allocation, or typed decode.
						assertMatrixOps(t, h.name, rec.ops, []string{"Reserve", "Probe"})
						assertMatrixReserveArgs(t, h.name, rec, reserveCall{dstBytes: 0, srcBytes: len(src)})
					})
				}
			},
		},
		{
			// Requirement 6: the second (destination) Reserve fails with the
			// exact injected error after the source Reserve and Probe
			// succeed, before allocation or typed decode.
			name: "destination Reserve failure",
			run: func(t *testing.T, h typedHelper) {
				sentinelErr := errors.New("injected " + h.name + " destination Reserve failure")
				rec := &decoderRecorder{probeMeta: validMeta, dstReserveErr: sentinelErr}
				img, meta, err := h.decode(rec, rec, src)

				assertMatrixExactError(t, h.name, err, sentinelErr)
				assertMatrixNilOutputs(t, h.name, img, meta)

				// The destination Reserve demonstrably is the second call:
				// both Reserve invocations are recorded and the failing one
				// requests the checked guest bytes.
				assertMatrixOps(t, h.name, rec.ops, []string{"Reserve", "Probe", "Reserve"})
				assertMatrixReserveArgs(t, h.name, rec,
					reserveCall{dstBytes: 0, srcBytes: len(src)},
					reserveCall{dstBytes: matrixW * matrixH * 4, srcBytes: len(src)})
			},
		},
		{
			// Requirement 7: after both successful Reserve calls and the
			// allocation, the typed decode fails with a unique injected
			// error; the image and Meta outputs are nil.
			name: "typed decode failure",
			run: func(t *testing.T, h typedHelper) {
				sentinelErr := errors.New("injected " + h.name + " typed decode failure")
				rec := &decoderRecorder{probeMeta: validMeta}
				h.setDecodeErr(rec, sentinelErr)
				img, meta, err := h.decode(rec, rec, src)

				assertMatrixExactError(t, h.name, err, sentinelErr)
				assertMatrixNilOutputs(t, h.name, img, meta)

				// Both Reserve calls and the allocation run, then the typed
				// decode fails with the injected error.
				assertMatrixOps(t, h.name, rec.ops,
					[]string{"Reserve", "Probe", "Reserve", h.allocOp, h.decodeOp})
			},
		},
		{
			// Requirement 8: the returned Meta is value-equal to, but
			// pointer-distinct from, the real Decoder's lastMeta, and
			// survives mutation of lastMeta and the wasm metadata memory.
			name: "Meta detachment",
			run: func(t *testing.T, h typedHelper) {
				fixture := mustReadFixture(t, "bricks-color.png")
				d := New()
				img, meta, err := h.decode(d, nil, fixture)
				if err != nil {
					t.Fatalf("%s decode err = %v, want nil", h.name, err)
				}
				if img == nil {
					t.Fatalf("%s decode returned nil image, want non-nil", h.name)
				}
				if meta == nil {
					t.Fatalf("%s decode returned nil Meta, want non-nil", h.name)
				}

				// Pointer-distinct from Decoder.lastMeta.
				if meta == &d.lastMeta {
					t.Errorf("%s returned Meta pointer == &d.lastMeta, want distinct pointer", h.name)
				}
				// Value-equal to Decoder.lastMeta before mutation.
				if *meta != d.lastMeta {
					t.Errorf("%s returned Meta %+v != d.lastMeta %+v", h.name, *meta, d.lastMeta)
				}

				captured := *meta

				// Mutate d.lastMeta.
				d.lastMeta.Width = 0xDEADBEEF
				d.lastMeta.Format = 0xCAFEBABE

				// Mutate wasm memory in the meta slot region.
				lay := d.currentLayout
				memSlice := d.module.Xmemory().Slice()
				memBytes := *memSlice
				if lay.metaOff+24 <= uint32(len(memBytes)) {
					zeroSl := memBytes[lay.metaOff : lay.metaOff+24]
					for i := range zeroSl {
						zeroSl[i] = 0xFF
					}
				}

				// After both mutations, the returned Meta value is unchanged.
				if *meta != captured {
					t.Errorf("%s returned Meta mutated by decoder/memory change: got %+v, want %+v", h.name, *meta, captured)
				}
			},
		},
	}
}

// TestUnitTypedMatrix is the shared typed decode contract matrix. It crosses
// the three typed helpers (RGBA, NRGBA, Gray) with the common case table so
// every common success, failure, ordering, reservation, allocator, and
// nil-output contract runs identically for all three helpers.
func TestUnitTypedMatrix(t *testing.T) {
	for _, h := range typedHelpers() {
		t.Run(h.name, func(t *testing.T) {
			for _, c := range commonCases() {
				t.Run(c.name, func(t *testing.T) {
					c.run(t, h)
				})
			}
		})
	}
}

// TestUnitTypedMatrixTypeChecks proves the type-specific output contract of
// each typed helper: the concrete image type, the color model, and the
// returned dimensions. These assertions live outside the common case table
// because the output types and color models genuinely differ per helper.
func TestUnitTypedMatrixTypeChecks(t *testing.T) {
	const (
		w = 160
		h = 120
	)
	src := []byte("wuffs PNG typed matrix type checks")
	for _, th := range typedHelpers() {
		t.Run(th.name, func(t *testing.T) {
			rec := &decoderRecorder{probeMeta: &Meta{Width: w, Height: h}}
			img, meta, err := th.decode(rec, rec, src)
			if err != nil {
				t.Fatalf("%s decode err = %v, want nil", th.name, err)
			}
			if img == nil {
				t.Fatalf("%s decode returned nil image, want non-nil", th.name)
			}
			if meta == nil {
				t.Fatalf("%s decode returned nil Meta, want non-nil", th.name)
			}

			// The output is the helper's concrete image type.
			if reflect.TypeOf(img) != th.wantType {
				t.Errorf("%s image type = %v, want %v", th.name, reflect.TypeOf(img), th.wantType)
			}

			// The output uses the helper's color model: RGBAModel for RGBA,
			// NRGBAModel for NRGBA, GrayModel for Gray.
			if got := img.(image.Image).ColorModel(); got != th.wantColorModel {
				t.Errorf("%s color model = %v, want %v", th.name, got, th.wantColorModel)
			}

			// The output dimensions match the probed geometry.
			b := img.(image.Image).Bounds()
			if b.Dx() != w || b.Dy() != h {
				t.Errorf("%s bounds = %v, want %dx%d", th.name, b, w, h)
			}
		})
	}
}

// assertMatrixExactError asserts err is exactly wantErr (identity, not
// errors.Is), as required by the contract.
func assertMatrixExactError(t *testing.T, name string, err, wantErr error) {
	t.Helper()
	//nolint:errorlint // exact identity required by contract; the injected sentinel must match
	if err != wantErr {
		t.Fatalf("%s decode err = %v (%T), want exact %v (%T)", name, err, err, wantErr, wantErr)
	}
}

// assertMatrixNilOutputs asserts a failed decode returned nil image and nil
// Meta. The helpers return typed nil image pointers, which box as non-nil
// interface{} values, so the image is unwrapped via reflection before the
// nil check.
func assertMatrixNilOutputs(t *testing.T, name string, img interface{}, meta *Meta) {
	t.Helper()
	if img != nil {
		if v := reflect.ValueOf(img); v.Kind() != reflect.Pointer || !v.IsNil() {
			t.Errorf("%s decode returned non-nil image %v, want nil", name, img)
		}
	}
	if meta != nil {
		t.Errorf("%s decode returned non-nil Meta %v, want nil", name, meta)
	}
}

// assertMatrixOps asserts the recorded operation log equals want exactly.
func assertMatrixOps(t *testing.T, name string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s ops = %v, want %v", name, got, want)
	}
}

// assertMatrixReserveArgs asserts the recorded Reserve invocations equal want
// exactly.
func assertMatrixReserveArgs(t *testing.T, name string, rec *decoderRecorder, want ...reserveCall) {
	t.Helper()
	if !reflect.DeepEqual(rec.reservations, want) {
		t.Fatalf("%s reservations = %+v, want %+v", name, rec.reservations, want)
	}
}
