package wuffs

import (
	"errors"
	"testing"
)

// TestUnitDstTooSmallError_Is verifies that *DstTooSmallError matches the
// ErrDstTooSmall sentinel but not ErrBadImage or other sentinels.
func TestUnitDstTooSmallError_Is(t *testing.T) {
	err := &DstTooSmallError{}

	if !errors.Is(err, ErrDstTooSmall) {
		t.Fatalf("errors.Is(err, ErrDstTooSmall) = false, want true")
	}

	if errors.Is(err, ErrBadImage) {
		t.Fatalf("errors.Is(err, ErrBadImage) = true, want false")
	}

	if errors.Is(err, ErrDecode) {
		t.Fatalf("errors.Is(err, ErrDecode) = true, want false")
	}
}

// TestUnitDstTooSmallError_As verifies that errors.As still extracts the
// structured error and that its fields are preserved.
func TestUnitDstTooSmallError_As(t *testing.T) {
	want := &DstTooSmallError{
		MinBytes: 4096,
		Width:    32,
		Height:   32,
		Stride:   128,
	}

	var got *DstTooSmallError
	if !errors.As(want, &got) {
		t.Fatalf("errors.As did not extract *DstTooSmallError")
	}

	if *got != *want {
		t.Fatalf("extracted fields = %+v, want %+v", *got, *want)
	}

	// The sentinel identity must survive even when the error is wrapped.
	wrapped := &DstTooSmallError{MinBytes: 1, Width: 1, Height: 1, Stride: 4}
	if !errors.Is(wrapped, ErrDstTooSmall) {
		t.Fatalf("errors.Is(wrapped, ErrDstTooSmall) = false, want true")
	}
}
