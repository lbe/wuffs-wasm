package wuffs

import (
	"errors"
	"image"
	"image/color"
	"testing"
)

// TestUnitDecodeConfigRecordsExactSequence proves that every successful call
// through the seam records exactly Reserve(0, len(src)) followed by Probe(src)
// and no other decoder or allocator operation (req 1).
func TestUnitDecodeConfigRecordsExactSequence(t *testing.T) {
	png := mustReadFixture(t, "bricks-color.png")
	webp := mustReadFixture(t, "bricks-color.lossless.webp")

	tests := []struct {
		name string
		src  []byte
	}{
		{"valid PNG", png},
		{"valid lossless WebP", webp},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := &decoderRecorder{}
			cfg, err := decodeConfigWithDecoder(tc.src, rec)
			if err != nil {
				t.Fatalf("decodeConfigWithDecoder: %v", err)
			}
			if cfg.ColorModel == nil {
				t.Fatal("decodeConfigWithDecoder returned zero-color-model Config")
			}

			// Exact operation sequence: Reserve then Probe, no other ops.
			if len(rec.ops) != 2 {
				t.Fatalf("recorded %d ops, want 2: %v", len(rec.ops), rec.ops)
			}
			if rec.ops[0] != "Reserve" {
				t.Errorf("op[0] = %q, want %q", rec.ops[0], "Reserve")
			}
			if rec.ops[1] != "Probe" {
				t.Errorf("op[1] = %q, want %q", rec.ops[1], "Probe")
			}

			// Exact Reserve arguments: (0, len(src)).
			if len(rec.reservations) != 1 {
				t.Fatalf("recorded %d reserves, want 1", len(rec.reservations))
			}
			wantCall := reserveCall{dstBytes: 0, srcBytes: len(tc.src)}
			if rec.reservations[0] != wantCall {
				t.Errorf("reservations[0] = %+v, want %+v", rec.reservations[0], wantCall)
			}
		})
	}
}

// TestUnitDecodeConfigNoDestinationWork proves that successful DecodeConfig
// performs no destination Reserve, host allocation, or pixel decode: the
// recorder records only Reserve(0, len(src)) and Probe(src), and a real
// decoder retains its initial destination slot length (req 2).
func TestUnitDecodeConfigNoDestinationWork(t *testing.T) {
	t.Run("recorder proves no destination op", func(t *testing.T) {
		src := mustReadFixture(t, "bricks-color.png")
		rec := &decoderRecorder{}
		cfg, err := decodeConfigWithDecoder(src, rec)
		if err != nil {
			t.Fatalf("decodeConfigWithDecoder: %v", err)
		}
		if cfg.ColorModel == nil {
			t.Fatal("decodeConfigWithDecoder returned zero-color-model Config")
		}

		// Only Reserve and Probe; no destination Reserve, no typed decode.
		if len(rec.ops) != 2 {
			t.Fatalf("ops = %v, want [Reserve, Probe]", rec.ops)
		}
		for _, op := range rec.ops {
			if op == "DecodeRGBA" || op == "DecodeNRGBA" || op == "DecodeGray" {
				t.Errorf("unexpected decode op: %s", op)
			}
		}
		_ = cfg

		// The only Reserve has dstBytes=0 (source-only).
		if len(rec.reservations) != 1 {
			t.Fatalf("recorded %d reserves, want 1", len(rec.reservations))
		}
		if rec.reservations[0].dstBytes != 0 {
			t.Errorf("Reserve dstBytes = %d, want 0 (source-only)", rec.reservations[0].dstBytes)
		}
	})

	t.Run("real decoder dstLen unchanged", func(t *testing.T) {
		src := mustReadFixture(t, "bricks-color.png")
		d := New()
		preDstLen := d.currentLayout.dstLen

		cfg, err := decodeConfigWithDecoder(src, d)
		if err != nil {
			t.Fatalf("decodeConfigWithDecoder: %v", err)
		}
		if cfg.ColorModel == nil {
			t.Fatal("decodeConfigWithDecoder returned zero-color-model Config")
		}

		if d.currentLayout.dstLen != preDstLen {
			t.Errorf("dstLen changed from %d to %d; DecodeConfig must not grow destination", preDstLen, d.currentLayout.dstLen)
		}
	})
}

// TestUnitDecodeConfigSourceReserveFailure proves that an injected
// source-Reserve error returns zero image.Config and the exact injected
// error, and records only Reserve(0, len(src)) — Probe and every later
// operation are skipped (req 3).
func TestUnitDecodeConfigSourceReserveFailure(t *testing.T) {
	sentinelErr := errors.New("injected source reserve failure in DecodeConfig")
	src := mustReadFixture(t, "bricks-color.png")

	rec := &decoderRecorder{srcReserveErr: sentinelErr}
	cfg, err := decodeConfigWithDecoder(src, rec)

	// Exact error identity.
	//nolint:errorlint // exact identity required by contract; the injected sentinel must match
	if err != sentinelErr {
		t.Fatalf("decodeConfigWithDecoder err = %v, want exact %v", err, sentinelErr)
	}
	if cfg != (image.Config{}) {
		t.Errorf("decodeConfigWithDecoder returned Config %v, want zero image.Config", cfg)
	}

	// Only the source Reserve call is recorded.
	if len(rec.ops) != 1 {
		t.Fatalf("recorded %d ops, want 1 (only Reserve): %v", len(rec.ops), rec.ops)
	}
	if rec.ops[0] != "Reserve" {
		t.Errorf("op[0] = %q, want %q", rec.ops[0], "Reserve")
	}
	if len(rec.reservations) != 1 {
		t.Fatalf("recorded %d reserves, want 1", len(rec.reservations))
	}
	wantCall := reserveCall{dstBytes: 0, srcBytes: len(src)}
	if rec.reservations[0] != wantCall {
		t.Errorf("reservations[0] = %+v, want %+v", rec.reservations[0], wantCall)
	}
}

// TestUnitDecodeConfigProbeFailure proves that an injected Probe error
// returns zero image.Config and the exact injected error, and records
// only Reserve(0, len(src)) followed by Probe(src) — no destination
// Reserve, allocation, or typed decode (req 4).
func TestUnitDecodeConfigProbeFailure(t *testing.T) {
	sentinelErr := errors.New("injected probe failure in DecodeConfig")
	src := mustReadFixture(t, "bricks-color.png")

	rec := &decoderRecorder{
		srcReserveErr: nil, // source Reserve succeeds
		probeErr:      sentinelErr,
	}
	cfg, err := decodeConfigWithDecoder(src, rec)

	// Exact error identity.
	//nolint:errorlint // exact identity required by contract; the injected sentinel must match
	if err != sentinelErr {
		t.Fatalf("decodeConfigWithDecoder err = %v, want exact %v", err, sentinelErr)
	}
	if cfg != (image.Config{}) {
		t.Errorf("decodeConfigWithDecoder returned Config %v, want zero image.Config", cfg)
	}

	// Reserve and Probe are recorded, but no destination or decode ops.
	if len(rec.ops) != 2 {
		t.Fatalf("recorded %d ops, want 2 (Reserve, Probe): %v", len(rec.ops), rec.ops)
	}
	if rec.ops[0] != "Reserve" {
		t.Errorf("op[0] = %q, want %q", rec.ops[0], "Reserve")
	}
	if rec.ops[1] != "Probe" {
		t.Errorf("op[1] = %q, want %q", rec.ops[1], "Probe")
	}
}

// TestUnitDecodeConfigConfigValues proves that the returned successful
// config is image.Config{ColorModel: color.RGBAModel, Width, Height}
// when the underlying probe succeeds (req 7).
func TestUnitDecodeConfigConfigValues(t *testing.T) {
	png := mustReadFixture(t, "bricks-color.png")
	webp := mustReadFixture(t, "bricks-color.lossless.webp")

	const (
		wantW = 160
		wantH = 120
	)

	// Large input (> 64 KiB default source slot).
	large := make([]byte, 128*1024)
	copy(large, png)

	tests := []struct {
		name  string
		src   []byte
		wantW int
		wantH int
	}{
		{"valid PNG", png, wantW, wantH},
		{"valid lossless WebP", webp, wantW, wantH},
		{"large input", large, wantW, wantH},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := DecodeConfig(tc.src)
			if err != nil {
				t.Fatalf("DecodeConfig: %v", err)
			}

			want := image.Config{
				ColorModel: color.RGBAModel,
				Width:      tc.wantW,
				Height:     tc.wantH,
			}
			if cfg != want {
				t.Errorf("DecodeConfig = %+v, want %+v", cfg, want)
			}
		})
	}
}

// TestUnitDecodeConfigErrorPaths proves that empty, unknown-format, and
// non-config-readable truncated input return zero config and the exact
// error emitted by the underlying decoder Probe (req 6, exact identity).
func TestUnitDecodeConfigErrorPaths(t *testing.T) {
	t.Run("empty input returns zero config and exact ErrDecode", func(t *testing.T) {
		cfg, err := DecodeConfig(nil)
		//nolint:errorlint // exact identity required by contract; primitive sentinel must match
		if err != ErrDecode {
			t.Fatalf("DecodeConfig(nil) err = %v, want exact %v", err, ErrDecode)
		}
		if cfg != (image.Config{}) {
			t.Errorf("DecodeConfig(nil) Config = %v, want zero image.Config", cfg)
		}
	})

	t.Run("unknown format returns zero config and exact ErrUnknownFormat", func(t *testing.T) {
		cfg, err := DecodeConfig([]byte("not a known image format header"))
		//nolint:errorlint // exact identity required by contract; primitive sentinel must match
		if err != ErrUnknownFormat {
			t.Fatalf("DecodeConfig(unknown) err = %v, want exact %v", err, ErrUnknownFormat)
		}
		if cfg != (image.Config{}) {
			t.Errorf("DecodeConfig(unknown) Config = %v, want zero image.Config", cfg)
		}
	})

	t.Run("non-config-readable truncated returns zero config and exact error", func(t *testing.T) {
		// A one-byte PNG "header" is too short for format sniffing. Wuffs
		// cannot match the full 8-byte PNG signature or any other format.
		tooShort := []byte{0x89}

		cfg, err := DecodeConfig(tooShort)
		//nolint:errorlint // exact identity required by contract; primitive sentinel must match
		if err != ErrUnknownFormat {
			t.Fatalf("DecodeConfig(tooShort) err = %v, want exact %v", err, ErrUnknownFormat)
		}
		if cfg != (image.Config{}) {
			t.Errorf("DecodeConfig(tooShort) Config = %v, want zero image.Config", cfg)
		}
	})
}
