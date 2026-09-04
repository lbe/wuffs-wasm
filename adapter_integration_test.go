package wuffs_test

import (
	"bytes"
	"errors"
	"image"
	"io"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

var _ image.Image = (*image.RGBA)(nil)

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

type errorAfterReader struct {
	data []byte
	err  error
}

func (r *errorAfterReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

// TestIntegrationDecodeReaderFormatMatrix pins every canonical FourCC mapping
// end-to-end: DecodeReader must report the canonical format name and return
// pixels identical to the direct Decode path for each checked-in fixture.
// Netpbm P5 (PGM) and P6 (PPM) fixtures both report "npbm".
func TestIntegrationDecodeReaderFormatMatrix(t *testing.T) {
	for _, tt := range []struct {
		name   string
		file   string
		format string
	}{
		{name: "PNG", file: "bricks-color.png", format: "png"},
		{name: "WebP", file: "bricks-color.lossless.webp", format: "webp"},
		{name: "BMP", file: "bricks-color.bmp", format: "bmp"},
		{name: "GIF", file: "bricks-nodither.gif", format: "gif"},
		{name: "JPEG", file: "hat.jpeg", format: "jpeg"},
		{name: "NPBM PGM", file: "hippopotamus.pgm", format: "npbm"},
		{name: "NPBM PPM", file: "hippopotamus.ppm", format: "npbm"},
		{name: "QOI", file: "bricks-color.qoi", format: "qoi"},
		{name: "TGA", file: "bricks-color.tga", format: "tga"},
		{name: "WBMP", file: "bricks-nodither.wbmp", format: "wbmp"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			src := loadFixture(t, tt.file)
			want, _, err := wuffs.Decode(src)
			if err != nil {
				t.Fatalf("Decode fixture: %v", err)
			}
			gotImage, gotFormat, err := wuffs.DecodeReader(&chunkedReader{data: append([]byte(nil), src...), chunk: 3})
			if err != nil {
				t.Fatalf("DecodeReader: %v", err)
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
}

func TestIntegrationDecodeReaderErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  []byte
	}{
		{name: "empty", src: nil},
		{name: "unknown", src: []byte("not an image")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, wantErr := wuffs.Decode(tc.src)
			gotImage, gotFormat, gotErr := wuffs.DecodeReader(&chunkedReader{data: append([]byte(nil), tc.src...), chunk: 1})
			if gotImage != nil || gotFormat != "" {
				t.Errorf("failure outputs = (%v, %q), want (nil, empty)", gotImage, gotFormat)
			}
			//nolint:errorlint // exact sentinel identity is part of the adapter contract
			if gotErr != wantErr {
				t.Errorf("error = %v, want exact Decode error %v", gotErr, wantErr)
			}
		})
	}

	readErr := errors.New("reader failed")
	src := loadFixture(t, "bricks-color.png")
	gotImage, gotFormat, gotErr := wuffs.DecodeReader(&errorAfterReader{data: append([]byte(nil), src...), err: readErr})
	if gotImage != nil || gotFormat != "" {
		t.Errorf("reader failure outputs = (%v, %q), want (nil, empty)", gotImage, gotFormat)
	}
	//nolint:errorlint // exact sentinel identity is part of the adapter contract
	if gotErr != readErr {
		t.Errorf("reader failure = %v, want exact %v", gotErr, readErr)
	}
}

func TestIntegrationDecodeConfigReaderByteParity(t *testing.T) {
	for _, tc := range []struct {
		name string
		file string
	}{
		{name: "PNG", file: "bricks-color.png"},
		{name: "WebP", file: "bricks-color.lossless.webp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := loadFixture(t, tc.file)
			want, wantErr := wuffs.DecodeConfig(src)
			if wantErr != nil {
				t.Fatalf("DecodeConfig fixture: %v", wantErr)
			}

			got, gotErr := wuffs.DecodeConfigReader(&chunkedReader{
				data:  append([]byte(nil), src...),
				chunk: 3,
			})
			if gotErr != nil {
				t.Fatalf("DecodeConfigReader: %v", gotErr)
			}
			if got != want {
				t.Errorf("DecodeConfigReader config = %+v, want exact DecodeConfig config %+v", got, want)
			}
		})
	}
}

func TestIntegrationDecodeConfigReaderErrors(t *testing.T) {
	pngSrc := loadFixture(t, "bricks-color.png")
	truncated := pngSrc[:50]

	t.Run("truncated config parity", func(t *testing.T) {
		want, wantErr := wuffs.DecodeConfig(truncated)
		got, gotErr := wuffs.DecodeConfigReader(&chunkedReader{
			data:  append([]byte(nil), truncated...),
			chunk: 1,
		})
		if got != want {
			t.Errorf("config = %+v, want exact DecodeConfig config %+v", got, want)
		}
		//nolint:errorlint // exact sentinel identity is part of the adapter contract
		if gotErr != wantErr {
			t.Errorf("error = %v, want exact DecodeConfig error %v", gotErr, wantErr)
		}
	})

	for _, tc := range []struct {
		name string
		src  []byte
	}{
		{name: "empty", src: nil},
		{name: "unknown", src: []byte("not an image")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, wantErr := wuffs.DecodeConfig(tc.src)
			got, gotErr := wuffs.DecodeConfigReader(&chunkedReader{
				data:  append([]byte(nil), tc.src...),
				chunk: 1,
			})
			if got != want {
				t.Errorf("config = %+v, want exact DecodeConfig config %+v", got, want)
			}
			//nolint:errorlint // exact sentinel identity is part of the adapter contract
			if gotErr != wantErr {
				t.Errorf("error = %v, want exact DecodeConfig error %v", gotErr, wantErr)
			}
		})
	}

	t.Run("reader error returns exact sentinel and zero config", func(t *testing.T) {
		readErr := errors.New("reader failed")
		got, gotErr := wuffs.DecodeConfigReader(&errorAfterReader{
			data: append([]byte(nil), pngSrc...),
			err:  readErr,
		})
		if got != (image.Config{}) {
			t.Errorf("reader failure config = %+v, want zero image.Config", got)
		}
		//nolint:errorlint // exact sentinel identity is part of the adapter contract
		if gotErr != readErr {
			t.Errorf("reader failure = %v, want exact %v", gotErr, readErr)
		}
	})
}
