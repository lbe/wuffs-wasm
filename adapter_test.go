package wuffs

import (
	"bytes"
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

// assertFormatName pins one formatName outcome: the exact canonical name for
// a mapped FourCC and exact error sentinel identity - nil for a mapped
// format, ErrUnknownFormat for an unknown value. Every table-driven
// formatName matrix shares this assertion body.
func assertFormatName(t *testing.T, fourCC uint32, want string, wantErr error) {
	t.Helper()
	got, err := formatName(fourCC)
	if got != want {
		t.Errorf("formatName(0x%08X) name = %q, want %q", fourCC, got, want)
	}
	//nolint:errorlint // exact sentinel identity is part of the adapter contract
	if err != wantErr {
		t.Errorf("formatName(0x%08X) error = %v, want exact %v", fourCC, err, wantErr)
	}
}

func TestUnitReaderFormatNames(t *testing.T) {
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
			assertFormatName(t, tt.fourCC, tt.want, tt.wantErr)
		})
	}
}

func TestUnitFormatNamesFORMAT02(t *testing.T) {
	// FORMAT-02 adds four canonical names to the reader adapter mapper. The
	// table also re-pins the five pre-existing mappings and the unknown-FourCC
	// error contract so the mapper's full behavior stays a single observable
	// outcome.
	tests := []struct {
		name    string
		fourCC  uint32
		want    string
		wantErr error
	}{
		{name: "PNG preserved", fourCC: FormatPNG, want: "png"},
		{name: "WebP preserved", fourCC: FormatWEBP, want: "webp"},
		{name: "BMP preserved", fourCC: FormatBMP, want: "bmp"},
		{name: "GIF preserved", fourCC: FormatGIF, want: "gif"},
		{name: "JPEG preserved", fourCC: FormatJPEG, want: "jpeg"},
		{name: "NPBM", fourCC: FormatNPBM, want: "npbm"},
		{name: "QOI", fourCC: FormatQOI, want: "qoi"},
		{name: "TGA", fourCC: FormatTGA, want: "tga"},
		{name: "WBMP", fourCC: FormatWBMP, want: "wbmp"},
		{name: "unknown", fourCC: 0x12345678, wantErr: ErrUnknownFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertFormatName(t, tt.fourCC, tt.want, tt.wantErr)
		})
	}
}

func TestUnitFormatNameBMP(t *testing.T) {
	// The BMP FourCC must map to the canonical standard-library adapter name
	// "bmp" with no error. The mapper currently lacks a BMP case, so this
	// fails until the mapping exists.
	got, err := formatName(FormatBMP)
	if got != "bmp" {
		t.Errorf("formatName(0x%08X) name = %q, want %q", FormatBMP, got, "bmp")
	}
	//nolint:errorlint // exact sentinel identity is part of the adapter contract
	if err != nil {
		t.Errorf("formatName(0x%08X) error = %v, want nil", FormatBMP, err)
	}
}

func TestUnitFormatNameGIF(t *testing.T) {
	// The GIF FourCC must map to the canonical standard-library adapter name
	// "gif" with no error. The mapper currently lacks a GIF case, so this
	// fails until the mapping exists.
	got, err := formatName(FormatGIF)
	if got != "gif" {
		t.Errorf("formatName(0x%08X) name = %q, want %q", FormatGIF, got, "gif")
	}
	//nolint:errorlint // exact sentinel identity is part of the adapter contract
	if err != nil {
		t.Errorf("formatName(0x%08X) error = %v, want nil", FormatGIF, err)
	}
}

func TestUnitFormatNameJPEG(t *testing.T) {
	// The JPEG FourCC must map to the canonical standard-library adapter name
	// "jpeg" with no error. The mapper currently lacks a JPEG case, so this
	// fails until the mapping exists.
	got, err := formatName(FormatJPEG)
	if got != "jpeg" {
		t.Errorf("formatName(0x%08X) name = %q, want %q", FormatJPEG, got, "jpeg")
	}
	//nolint:errorlint // exact sentinel identity is part of the adapter contract
	if err != nil {
		t.Errorf("formatName(0x%08X) error = %v, want nil", FormatJPEG, err)
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

// chunkedReader yields its data in fixed-size chunks and then io.EOF, so the
// integration matrix below can prove DecodeReader consumes chunked input
// through EOF against the live wasm guest. Package-private to this test file;
// the external wuffs_test package keeps its own copy.
type chunkedReader struct {
	data  []byte
	chunk int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := r.chunk
	if n > len(p) {
		n = len(p)
	}
	if n > len(r.data) {
		n = len(r.data)
	}
	copy(p[:n], r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

// TestIntegrationDecodeReaderFORMAT03Matrix pins the FORMAT-03 reader contract:
// the private formatName mapper returns the canonical names "etc2", "hnsm",
// "nie", and "th" for their exact FourCC values, preserves all nine existing
// mappings, and keeps the ErrUnknownFormat behavior for unknown values; and
// DecodeReader consumes chunked input through EOF and reports the canonical
// name plus pixels identical to Decode for all four FORMAT-03 fixtures.
func TestIntegrationDecodeReaderFORMAT03Matrix(t *testing.T) {
	t.Run("formatName maps FORMAT-03 and preserves the nine existing names", func(t *testing.T) {
		tests := []struct {
			name    string
			fourCC  uint32
			want    string
			wantErr error
		}{
			{name: "PNG preserved", fourCC: FormatPNG, want: "png"},
			{name: "WebP preserved", fourCC: FormatWEBP, want: "webp"},
			{name: "BMP preserved", fourCC: FormatBMP, want: "bmp"},
			{name: "GIF preserved", fourCC: FormatGIF, want: "gif"},
			{name: "JPEG preserved", fourCC: FormatJPEG, want: "jpeg"},
			{name: "NPBM preserved", fourCC: FormatNPBM, want: "npbm"},
			{name: "QOI preserved", fourCC: FormatQOI, want: "qoi"},
			{name: "TGA preserved", fourCC: FormatTGA, want: "tga"},
			{name: "WBMP preserved", fourCC: FormatWBMP, want: "wbmp"},
			{name: "ETC2", fourCC: FormatETC2, want: "etc2"},
			{name: "HNSM", fourCC: FormatHNSM, want: "hnsm"},
			{name: "NIE", fourCC: FormatNIE, want: "nie"},
			{name: "TH", fourCC: FormatTH, want: "th"},
			{name: "unknown", fourCC: 0x12345678, wantErr: ErrUnknownFormat},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assertFormatName(t, tt.fourCC, tt.want, tt.wantErr)
			})
		}
	})

	t.Run("DecodeReader parity through chunked EOF", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			file   string
			format string
		}{
			{name: "ETC2", file: "bricks-color.etc2.pkm", format: "etc2"},
			{name: "HNSM", file: "bricks-color.c3q4.handsum", format: "hnsm"},
			{name: "NIE", file: "crude-flag.nie", format: "nie"},
			{name: "TH", file: "mona-lisa.21x32.th", format: "th"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				src := mustReadFixture(t, tt.file)
				want, _, err := Decode(src)
				if err != nil {
					t.Fatalf("Decode fixture: %v", err)
				}
				gotImage, gotFormat, gotErr := DecodeReader(&chunkedReader{data: append([]byte(nil), src...), chunk: 3})
				if gotErr != nil {
					t.Errorf("DecodeReader(%s) error = %v, want nil", tt.file, gotErr)
					return
				}
				got, ok := gotImage.(*image.RGBA)
				if !ok {
					t.Fatalf("DecodeReader image type = %T, want *image.RGBA", gotImage)
				}
				if gotFormat != tt.format {
					t.Errorf("format = %q, want %q", gotFormat, tt.format)
				}
				if !got.Bounds().Eq(want.Bounds()) || got.Stride != want.Stride || !bytes.Equal(got.Pix, want.Pix) {
					t.Error("DecodeReader image differs from Decode image")
				}
			})
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
