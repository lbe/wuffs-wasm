package wuffs

import (
	"errors"
	"testing"
)

// TestUnitPackageProbeRecordsExactSequence proves that every successful
// call through the seam records exactly Reserve(0, len(src)) followed by
// Probe(src) and no other decoder or allocator operation (req 5).
func TestUnitPackageProbeRecordsExactSequence(t *testing.T) {
	png := mustReadFixture(t, "bricks-color.png")
	webp := mustReadFixture(t, "bricks-color.lossless.webp")

	// Build a source larger than the default 64 KiB source slot.
	large := make([]byte, 128*1024)
	copy(large, png)

	tests := []struct {
		name string
		src  []byte
	}{
		{"valid PNG", png},
		{"valid lossless WebP", webp},
		{"large valid input (exceeds default slot)", large},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := &decoderRecorder{}
			meta, err := probeWithDecoder(tc.src, rec)
			if err != nil {
				t.Fatalf("probeWithDecoder: %v", err)
			}
			if meta == nil {
				t.Fatal("probeWithDecoder returned nil Meta")
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

// TestUnitPackageProbeSourceReserveFailure proves that an injected
// source-Reserve error returns the exact injected error, records only
// Reserve(0, len(src)), and skips Probe and every
// destination/decode/allocation operation (req 6).
func TestUnitPackageProbeSourceReserveFailure(t *testing.T) {
	sentinelErr := errors.New("injected source reserve failure")
	src := mustReadFixture(t, "bricks-color.png")

	rec := &decoderRecorder{srcReserveErr: sentinelErr}
	meta, err := probeWithDecoder(src, rec)

	// Exact error identity.
	//nolint:errorlint // exact identity required by contract; the injected sentinel must match
	if err != sentinelErr {
		t.Fatalf("probeWithDecoder err = %v, want exact %v", err, sentinelErr)
	}
	if meta != nil {
		t.Errorf("probeWithDecoder returned non-nil Meta %v, want nil", meta)
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

// TestUnitPackageProbeMetaDetached proves that the Meta returned by
// probeWithDecoder is value-equal to the decoder's lastMeta but
// pointer-distinct, and that mutating the decoder's lastMeta and wasm
// metadata memory does not change the returned value (req 7).
func TestUnitPackageProbeMetaDetached(t *testing.T) {
	src := mustReadFixture(t, "bricks-color.png")
	d := New()

	// probeWithDecoder internally calls Reserve(0, len(src)).
	meta, err := probeWithDecoder(src, d)
	if err != nil {
		t.Fatalf("probeWithDecoder: %v", err)
	}
	if meta == nil {
		t.Fatal("probeWithDecoder returned nil Meta")
	}

	// Pointer-distinct from Decoder.lastMeta.
	if meta == &d.lastMeta {
		t.Error("returned Meta pointer == &d.lastMeta, want distinct pointer")
	}

	// Value equality before mutation.
	if *meta != d.lastMeta {
		t.Errorf("returned Meta %+v != d.lastMeta %+v", *meta, d.lastMeta)
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

	// After mutations, the captured value must remain unchanged.
	if *meta != captured {
		t.Errorf("returned Meta was mutated by decoder/memory change: got %+v, want %+v", *meta, captured)
	}
}

// TestUnitPackageProbeNoDestinationWork proves that probeWithDecoder
// performs no destination slot growth or pixel decode: the recorder
// records only Reserve(0, len(src)) and Probe(src), and a real decoder
// retains its initial destination slot length (req 9).
func TestUnitPackageProbeNoDestinationWork(t *testing.T) {
	t.Run("recorder proves no destination op", func(t *testing.T) {
		src := mustReadFixture(t, "bricks-color.png")
		rec := &decoderRecorder{}
		meta, err := probeWithDecoder(src, rec)
		if err != nil {
			t.Fatalf("probeWithDecoder: %v", err)
		}
		if meta == nil {
			t.Fatal("probeWithDecoder returned nil Meta")
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

		// The only Reserve has dstBytes=0 (source-only).
		if len(rec.reservations) != 1 {
			t.Fatalf("recorded %d reserves, want 1", len(rec.reservations))
		}
		if rec.reservations[0].dstBytes != 0 {
			t.Errorf("Reserve dstBytes = %d, want 0 (source-only)", rec.reservations[0].dstBytes)
		}
		_ = meta
	})

	t.Run("real decoder dstLen unchanged", func(t *testing.T) {
		src := mustReadFixture(t, "bricks-color.png")
		d := New()
		preDstLen := d.currentLayout.dstLen

		meta, err := probeWithDecoder(src, d)
		if err != nil {
			t.Fatalf("probeWithDecoder: %v", err)
		}
		if meta == nil {
			t.Fatal("probeWithDecoder returned nil Meta")
		}

		if d.currentLayout.dstLen != preDstLen {
			t.Errorf("dstLen changed from %d to %d; probe must not grow destination", preDstLen, d.currentLayout.dstLen)
		}
	})
}
