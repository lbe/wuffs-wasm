package wuffs_test

import (
	"errors"
	"image"
	"testing"

	wuffs "github.com/lbe/wuffs-wasm"
)

// newClosedDecoder returns a Decoder that has already been closed.
func newClosedDecoder(t *testing.T) *wuffs.Decoder {
	t.Helper()
	d := wuffs.New()
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return d
}

// TestUnitDecoderCloseTwiceReturnsNil requires a second Close to succeed as a no-op.
func TestUnitDecoderCloseTwiceReturnsNil(t *testing.T) {
	d := wuffs.New()
	if err := d.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("second Close: %v, want nil", err)
	}
}

// TestUnitDecoderMethodsAfterCloseReturnErrClosed requires every error-returning
// method on a closed Decoder to return ErrClosed and a zero non-error result.
func TestUnitDecoderMethodsAfterCloseReturnErrClosed(t *testing.T) {
	src := loadFixture(t, "bricks-color.png")
	// bricks-color.png is 160x120; the destinations only need to be well-formed
	// because the closed check runs before any dst validation.
	rect := image.Rect(0, 0, 160, 120)

	tests := []struct {
		name string
		call func(d *wuffs.Decoder) (zero bool, err error)
	}{
		{"Reserve", func(d *wuffs.Decoder) (bool, error) {
			return true, d.Reserve(0, len(src))
		}},
		{"Probe", func(d *wuffs.Decoder) (bool, error) {
			m, err := d.Probe(src)
			return m == nil, err
		}},
		{"DecodeRGBA", func(d *wuffs.Decoder) (bool, error) {
			m, err := d.DecodeRGBA(image.NewRGBA(rect), src)
			return m == nil, err
		}},
		{"DecodeNRGBA", func(d *wuffs.Decoder) (bool, error) {
			m, err := d.DecodeNRGBA(image.NewNRGBA(rect), src)
			return m == nil, err
		}},
		{"DecodeGray", func(d *wuffs.Decoder) (bool, error) {
			m, err := d.DecodeGray(image.NewGray(rect), src)
			return m == nil, err
		}},
		{"FrameCount", func(d *wuffs.Decoder) (bool, error) {
			n, err := d.FrameCount(src)
			return n == 0, err
		}},
		{"LoopCount", func(d *wuffs.Decoder) (bool, error) {
			n, err := d.LoopCount(src)
			return n == 0, err
		}},
		{"DecodeFrame", func(d *wuffs.Decoder) (bool, error) {
			f, err := d.DecodeFrame(image.NewRGBA(rect), src, 0)
			return f == nil, err
		}},
		{"Metadata", func(d *wuffs.Decoder) (bool, error) {
			m, err := d.Metadata(src)
			return m == nil, err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newClosedDecoder(t)
			zero, err := tc.call(d)
			if !errors.Is(err, wuffs.ErrClosed) {
				t.Fatalf("%s after Close: err = %v, want ErrClosed", tc.name, err)
			}
			if !zero {
				t.Fatalf("%s after Close: non-error result is not the zero value", tc.name)
			}
		})
	}
}

// TestUnitDecoderVersionAfterClose requires Version and VersionNum to keep
// reporting the same values after Close.
func TestUnitDecoderVersionAfterClose(t *testing.T) {
	d := wuffs.New()
	wantVersion := d.Version()
	wantNum := d.VersionNum()
	if wantVersion == "" {
		t.Fatal("Version before Close is empty")
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := d.Version(); got != wantVersion {
		t.Errorf("Version after Close = %q, want %q", got, wantVersion)
	}
	if got := d.VersionNum(); got != wantNum {
		t.Errorf("VersionNum after Close = %d, want %d", got, wantNum)
	}
}

// TestUnitDecoderTrimAfterCloseIsNoop requires Trim on a closed Decoder to
// return without panicking and leave the Decoder closed.
func TestUnitDecoderTrimAfterCloseIsNoop(t *testing.T) {
	src := loadFixture(t, "bricks-color.png")
	d := newClosedDecoder(t)
	d.Trim()
	if _, err := d.Probe(src); !errors.Is(err, wuffs.ErrClosed) {
		t.Fatalf("Probe after Close and Trim: err = %v, want ErrClosed", err)
	}
}
