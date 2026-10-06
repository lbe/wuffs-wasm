package wuffs

// Task 5 RED test: DecodeRGBA must equal DecodeFrame(dst, src, 0) in pixels
// and returned *Meta semantics, without returning a *Frame.

import (
	"hash/crc32"
	"image"
	"testing"
)

// decodeRGBASignature pins the DecodeRGBA API at compile time: it takes a
// caller-owned *image.RGBA and src, and returns (*Meta, error) — never a
// *Frame. Any signature change breaks this assertion.
var _ func(*Decoder, *image.RGBA, []byte) (*Meta, error) = (*Decoder).DecodeRGBA

// TestIntegrationDecodeRGBAFrameZeroEquivalence verifies the Task 5 product
// invariant: DecodeRGBA(dst, src) equals DecodeFrame(dst, src, 0) for pixels
// and returned *Meta semantics. For each PNG/GIF still fixture, both calls run
// on the same *Decoder into fresh zeroed canvases; the dst.Pix CRCs must match
// and d.lastMeta must be identical field-for-field after each call.
func TestIntegrationDecodeRGBAFrameZeroEquivalence(t *testing.T) {
	fixtures := []string{
		"bricks-nodither.png",
		"bricks-nodither.gif",
		"bricks-color.png",
	}

	for _, file := range fixtures {
		t.Run(file, func(t *testing.T) {
			src := mustReadFixture(t, file)

			d := New()
			probe, err := d.Probe(src)
			if err != nil {
				t.Fatalf("Probe(%s) error = %v, want nil", file, err)
			}
			if probe == nil || probe.Width == 0 || probe.Height == 0 {
				t.Fatalf("Probe(%s) Meta = %+v, want non-zero dimensions", file, probe)
			}
			w, h := int(probe.Width), int(probe.Height)
			if reserveErr := d.Reserve(int(probe.Stride)*h, len(src)); reserveErr != nil {
				t.Fatalf("Reserve(%s) error = %v, want nil", file, reserveErr)
			}

			dstA := image.NewRGBA(image.Rect(0, 0, w, h))
			metaA, err := d.DecodeRGBA(dstA, src)
			if err != nil {
				t.Fatalf("DecodeRGBA(%s) error = %v, want nil", file, err)
			}
			if metaA == nil {
				t.Fatalf("DecodeRGBA(%s) returned nil Meta, want non-nil", file)
			}
			crcA := crc32.ChecksumIEEE(dstA.Pix)
			lastMetaA := d.lastMeta

			dstB := image.NewRGBA(image.Rect(0, 0, w, h))
			frame, err := d.DecodeFrame(dstB, src, 0)
			if err != nil {
				t.Fatalf("DecodeFrame(%s, 0) error = %v, want nil", file, err)
			}
			if frame == nil {
				t.Fatalf("DecodeFrame(%s, 0) returned nil Frame, want non-nil", file)
			}
			if frame.Index != 0 {
				t.Errorf("DecodeFrame(%s, 0) Frame.Index = %d, want 0", file, frame.Index)
			}
			crcB := crc32.ChecksumIEEE(dstB.Pix)
			lastMetaB := d.lastMeta

			if crcA != crcB {
				t.Errorf("Pix CRC mismatch for %s: DecodeRGBA = 0x%08X, DecodeFrame(0) = 0x%08X",
					file, crcA, crcB)
			}
			if lastMetaA.Err != lastMetaB.Err {
				t.Errorf("lastMeta.Err differs for %s: DecodeRGBA = %d, DecodeFrame(0) = %d",
					file, lastMetaA.Err, lastMetaB.Err)
			}
			if lastMetaA.Width != lastMetaB.Width {
				t.Errorf("lastMeta.Width differs for %s: DecodeRGBA = %d, DecodeFrame(0) = %d",
					file, lastMetaA.Width, lastMetaB.Width)
			}
			if lastMetaA.Height != lastMetaB.Height {
				t.Errorf("lastMeta.Height differs for %s: DecodeRGBA = %d, DecodeFrame(0) = %d",
					file, lastMetaA.Height, lastMetaB.Height)
			}
			if lastMetaA.Stride != lastMetaB.Stride {
				t.Errorf("lastMeta.Stride differs for %s: DecodeRGBA = %d, DecodeFrame(0) = %d",
					file, lastMetaA.Stride, lastMetaB.Stride)
			}
			if lastMetaA.BytesWritten != lastMetaB.BytesWritten {
				t.Errorf("lastMeta.BytesWritten differs for %s: DecodeRGBA = %d, DecodeFrame(0) = %d",
					file, lastMetaA.BytesWritten, lastMetaB.BytesWritten)
			}
			if lastMetaA.Format != lastMetaB.Format {
				t.Errorf("lastMeta.Format differs for %s: DecodeRGBA = 0x%08X, DecodeFrame(0) = 0x%08X",
					file, lastMetaA.Format, lastMetaB.Format)
			}
		})
	}
}
