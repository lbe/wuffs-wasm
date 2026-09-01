package wuffs

import (
	"errors"
	"image"
	"io"
	"testing"
)

type partialErrorReader struct {
	data []byte
	err  error
}

func (r *partialErrorReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		if r.err == nil {
			return 0, io.EOF
		}
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if r.err == nil {
		return n, nil
	}
	return n, r.err
}

func TestReaderFormatNames(t *testing.T) {
	// Keep the expected error in each case so supported formats also assert
	// that the mapper does not unexpectedly return an error.
	tests := []struct {
		name    string
		fourCC  uint32
		want    string
		wantErr error
	}{
		{name: "PNG", fourCC: FormatPNG, want: "png"},
		{name: "WebP", fourCC: FormatWEBP, want: "webp"},
		{name: "unknown", fourCC: 0x12345678, wantErr: ErrUnknownFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatName(tt.fourCC)
			if got != tt.want {
				t.Errorf("formatName(0x%08X) name = %q, want %q", tt.fourCC, got, tt.want)
			}
			//nolint:errorlint // exact sentinel identity is part of the adapter contract
			if err != tt.wantErr {
				t.Errorf("formatName(0x%08X) error = %v, want exact %v", tt.fourCC, err, tt.wantErr)
			}
		})
	}
}

func TestUnitDecodeReaderFailureSemantics(t *testing.T) {
	readErr := errors.New("reader failed")
	decodeErr := errors.New("decode failed")

	t.Run("reader error skips decode", func(t *testing.T) {
		called := false
		gotImage, gotFormat, gotErr := decodeReaderWithDecoder(
			&partialErrorReader{data: []byte("partial"), err: readErr},
			func([]byte) (*image.RGBA, *Meta, error) {
				called = true
				return image.NewRGBA(image.Rect(0, 0, 1, 1)), &Meta{Format: FormatPNG}, nil
			},
		)
		if called {
			t.Fatal("decode function was called after reader failure")
		}
		if gotImage != nil || gotFormat != "" {
			t.Errorf("reader failure outputs = (%v, %q), want (nil, empty)", gotImage, gotFormat)
		}
		//nolint:errorlint // exact sentinel identity is part of the adapter contract
		if gotErr != readErr {
			t.Errorf("reader failure = %v, want exact %v", gotErr, readErr)
		}
	})

	t.Run("decode error", func(t *testing.T) {
		gotImage, gotFormat, gotErr := decodeReaderWithDecoder(
			&partialErrorReader{data: []byte("input")},
			func([]byte) (*image.RGBA, *Meta, error) { return nil, nil, decodeErr },
		)
		if gotImage != nil || gotFormat != "" {
			t.Errorf("decode failure outputs = (%v, %q), want (nil, empty)", gotImage, gotFormat)
		}
		//nolint:errorlint // exact sentinel identity is part of the adapter contract
		if gotErr != decodeErr {
			t.Errorf("decode failure = %v, want exact %v", gotErr, decodeErr)
		}
	})

	t.Run("unknown decoded format", func(t *testing.T) {
		decoded := image.NewRGBA(image.Rect(0, 0, 1, 1))
		gotImage, gotFormat, gotErr := decodeReaderWithDecoder(
			&partialErrorReader{data: []byte("input")},
			func([]byte) (*image.RGBA, *Meta, error) {
				return decoded, &Meta{Format: 0x12345678}, nil
			},
		)
		if gotImage != nil || gotFormat != "" {
			t.Errorf("unknown format outputs = (%v, %q), want (nil, empty)", gotImage, gotFormat)
		}
		//nolint:errorlint // exact sentinel identity is part of the adapter contract
		if gotErr != ErrUnknownFormat {
			t.Errorf("unknown format error = %v, want exact %v", gotErr, ErrUnknownFormat)
		}
	})
}

func TestUnitDecodeConfigReaderFailureSemantics(t *testing.T) {
	readErr := errors.New("reader failed")
	decodeErr := errors.New("decode failed")

	t.Run("reader error skips DecodeConfig", func(t *testing.T) {
		called := false
		gotConfig, gotErr := decodeConfigReaderWithDecoder(
			&partialErrorReader{data: []byte("partial"), err: readErr},
			func([]byte) (image.Config, error) {
				called = true
				return image.Config{Width: 1, Height: 1}, nil
			},
		)
		if called {
			t.Fatal("DecodeConfig function was called after reader failure")
		}
		if gotConfig != (image.Config{}) {
			t.Errorf("reader failure Config = %v, want zero image.Config", gotConfig)
		}
		//nolint:errorlint // exact sentinel identity is part of the adapter contract
		if gotErr != readErr {
			t.Errorf("reader failure = %v, want exact %v", gotErr, readErr)
		}
	})

	t.Run("DecodeConfig error", func(t *testing.T) {
		gotConfig, gotErr := decodeConfigReaderWithDecoder(
			&partialErrorReader{data: []byte("input")},
			func([]byte) (image.Config, error) { return image.Config{}, decodeErr },
		)
		if gotConfig != (image.Config{}) {
			t.Errorf("DecodeConfig failure Config = %v, want zero image.Config", gotConfig)
		}
		//nolint:errorlint // exact sentinel identity is part of the adapter contract
		if gotErr != decodeErr {
			t.Errorf("DecodeConfig failure = %v, want exact %v", gotErr, decodeErr)
		}
	})
}
