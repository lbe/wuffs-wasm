package wuffs

import (
	"errors"
	"testing"
)

// TestDecodeRGBARejectsNilDestination verifies that a nil destination is
// rejected before guest decoding or any wasm memory mutation.
func TestDecodeRGBARejectsNilDestination(t *testing.T) {
	d := New()
	src := mustReadFixture(t, "bricks-color.png")
	pre := captureMemState(t, d)

	var (
		meta    *Meta
		err     error
		paniced bool
	)
	func() {
		defer func() {
			if recover() != nil {
				paniced = true
			}
		}()
		meta, err = d.DecodeRGBA(nil, src)
	}()

	if paniced {
		t.Fatal("DecodeRGBA(nil, valid source) panicked")
	}
	if meta != nil {
		t.Fatalf("DecodeRGBA(nil, valid source) returned Meta = %+v, want nil", meta)
	}
	if !errors.Is(err, ErrBadImage) {
		t.Fatalf("DecodeRGBA(nil, valid source) error = %v, want ErrBadImage", err)
	}
	assertMemUnchanged(t, d, pre, "nil destination")
}

// TestDecodeNRGBARejectsNilDestination verifies that a nil NRGBA destination
// is rejected before guest decoding or any wasm memory mutation.
func TestDecodeNRGBARejectsNilDestination(t *testing.T) {
	d := New()
	src := mustReadFixture(t, "bricks-color.png")
	pre := captureMemState(t, d)

	var (
		meta    *Meta
		err     error
		paniced bool
	)
	func() {
		defer func() {
			if recover() != nil {
				paniced = true
			}
		}()
		meta, err = d.DecodeNRGBA(nil, src)
	}()

	if paniced {
		t.Fatal("DecodeNRGBA(nil, valid source) panicked")
	}
	if meta != nil {
		t.Fatalf("DecodeNRGBA(nil, valid source) returned Meta = %+v, want nil", meta)
	}
	if !errors.Is(err, ErrBadImage) {
		t.Fatalf("DecodeNRGBA(nil, valid source) error = %v, want ErrBadImage", err)
	}
	assertMemUnchanged(t, d, pre, "nil NRGBA destination")
}
