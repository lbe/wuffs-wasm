package registrationtest

import (
	"bytes"
	"errors"
	"image"
	"testing"
)

func runBeforeRegister(t *testing.T) {
	t.Helper()
	png := readFixture(t, "testdata/bricks-color.png")

	if _, _, err := image.Decode(bytes.NewReader(png)); !errors.Is(err, image.ErrFormat) {
		t.Fatalf("image.Decode before RegisterFormats: %v, want image.ErrFormat", err)
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(png)); !errors.Is(err, image.ErrFormat) {
		t.Fatalf("image.DecodeConfig before RegisterFormats: %v, want image.ErrFormat", err)
	}
}
