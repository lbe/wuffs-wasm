package wuffs_test

// FORMAT-03 contract helpers, shared by the ETC2, HNSM, NIE, and TH
// characterization tests and the TGA/WBMP negative corpora. They encode the
// invariants the per-format files would otherwise repeat verbatim:
//
//   - allocation: after Reserve and one warm-up DecodeRGBA, every repeated
//     decode into a correctly sized caller-owned destination performs zero Go
//     heap allocations;
//   - rejected-input: a near-signature or stripped-prefix input stays
//     ErrUnknownFormat - never ErrDecode - through both Probe and DecodeRGBA
//     with complete guest memory preserved; a rejected DecodeRGBA also leaves
//     the caller destination layout and every prefilled pixel byte untouched
//     and returns nil metadata;
//   - recognizer priority: one canonical fixture per established recognizer
//     keeps its own FourCC - the newest sniff never shadows it - across a
//     single Probe that also proves complete guest-memory preservation;
//   - negative corpus: arbitrary bytes stay ErrUnknownFormat through Probe
//     with a nil Meta.
//
// These are helpers only; they deliberately add no top-level Test* function.

import (
	"bytes"
	"errors"
	"image"
	"testing"

	"github.com/lbe/wuffs-wasm"
)

// assertZeroAllocsPerRun runs one warm-up DecodeRGBA of src into dst, then
// requires that every repeated DecodeRGBA performs exactly zero Go heap
// allocations. d must already be reserved and dst correctly sized; name
// labels the fixture in every diagnostic.
func assertZeroAllocsPerRun(t *testing.T, d *wuffs.Decoder, dst *image.RGBA, src []byte, name string) {
	t.Helper()
	if _, err := d.DecodeRGBA(dst, src); err != nil {
		t.Fatalf("warm-up DecodeRGBA(%s): %v", name, err)
	}

	allocs := testing.AllocsPerRun(100, func() {
		if _, err := d.DecodeRGBA(dst, src); err != nil {
			t.Errorf("DecodeRGBA(%s): %v", name, err)
		}
	})
	if allocs != 0 {
		t.Errorf("DecodeRGBA(%s) allocated %.0f heap objects per run, want 0", name, allocs)
	}
}

// assertRejectedPerOp asserts that op ("Probe" or "DecodeRGBA") rejects src
// as ErrUnknownFormat - never ErrDecode - while preserving the complete guest
// memory state. name labels the case in every diagnostic. It backs the
// FORMAT-03 near-signature and stripped-prefix rejection subtests, which all
// expect the same untouched-memory ErrUnknownFormat outcome.
//
// The DecodeRGBA branch additionally proves the caller-destination contract:
// the destination is prefilled with a nonzero sentinel; the complete layout
// (Pix backing pointer, length, capacity, Rect, and Stride) and a byte copy of
// the prefilled pixels are captured before the call; and after the call both
// the layout and every sentinel byte are unchanged. Both branches require the
// returned metadata to be nil.
func assertRejectedPerOp(t *testing.T, op, name string, src []byte) {
	t.Helper()
	d := wuffs.New()
	if err := wuffs.RequiredReserve(d, 4, len(src)); err != nil {
		t.Fatalf("RequiredReserve(%s): %v", name, err)
	}
	guestBefore := wuffs.CaptureGuestMemoryState(d)

	var err error
	if op == "Probe" {
		var meta *wuffs.Meta
		meta, err = d.Probe(src)
		assertGuestMemoryPreserved(t, op, name, d, guestBefore)
		if meta != nil {
			t.Errorf("Probe(%s) returned non-nil Meta %+v, want nil", name, meta)
		}
	} else {
		dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
		for i := range dst.Pix {
			dst.Pix[i] = 0xA5
		}
		prefill := append([]byte(nil), dst.Pix...)
		dstBefore := captureRGBAState(dst)

		var meta *wuffs.Meta
		meta, err = d.DecodeRGBA(dst, src)
		assertGuestMemoryPreserved(t, op, name, d, guestBefore)
		assertDestinationPreserved(t, op, name, dst, dstBefore)
		if meta != nil {
			t.Errorf("DecodeRGBA(%s) returned non-nil Meta %+v, want nil", name, meta)
		}
		if !bytes.Equal(dst.Pix, prefill) {
			t.Errorf("DecodeRGBA(%s) mutated the sentinel destination pixels", name)
		}
	}
	if err == nil {
		t.Errorf("%s(%s) error = nil, want ErrUnknownFormat", op, name)
	}
	if !errors.Is(err, wuffs.ErrUnknownFormat) {
		t.Errorf("%s(%s) error = %v, want errors.Is(err, ErrUnknownFormat)", op, name, err)
	}
	if errors.Is(err, wuffs.ErrDecode) {
		t.Errorf("%s(%s) error = %v, must not be ErrDecode", op, name, err)
	}
}

// recognizerCase names one canonical fixture and the established FourCC it
// must keep after a new FORMAT-03 sniff is added.
type recognizerCase struct {
	file string
	want uint32
}

// assertRecognizerPriority probes every canonical fixture in cases and
// asserts that it still classifies as its established FourCC - never as the
// new format identified by fourCC. formatName and sniffLabel label the new
// format in diagnostics (they coincide for HNSM, NIE, and TH; ETC2 is
// "ETC2" while its "PKM " sniff is the literal check). Each case gets a
// fresh decoder reserved to the four bytes the probe needs, captures its
// complete guest-memory state immediately before the single Probe call, and
// asserts that state is preserved immediately after it, before any fatal
// metadata assertion.
func assertRecognizerPriority(t *testing.T, cases []recognizerCase, fourCC uint32, formatName, sniffLabel string) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			src := loadFixture(t, tc.file)

			d := wuffs.New()
			if err := wuffs.RequiredReserve(d, 4, len(src)); err != nil {
				t.Fatalf("RequiredReserve(%s): %v", tc.file, err)
			}
			guestBefore := wuffs.CaptureGuestMemoryState(d)
			meta, err := d.Probe(src)
			assertGuestMemoryPreserved(t, "Probe", tc.file, d, guestBefore)
			if err != nil {
				t.Fatalf("Probe(%s) error = %v, want nil", tc.file, err)
			}
			if meta == nil {
				t.Fatalf("Probe(%s) returned nil Meta, want non-nil", tc.file)
			}
			if meta.Format != tc.want {
				t.Errorf("Probe(%s) Format = 0x%08X, want 0x%08X", tc.file, meta.Format, tc.want)
			}
			if meta.Format == fourCC {
				t.Errorf("Probe(%s) Format = %s 0x%08X, the %s sniff must not shadow established recognizers", tc.file, formatName, meta.Format, sniffLabel)
			}
		})
	}
}

// expectUnknownProbe probes src and asserts ErrUnknownFormat with a nil Meta.
func expectUnknownProbe(t *testing.T, d *wuffs.Decoder, name string, src []byte) {
	t.Helper()
	meta, err := d.Probe(src)
	if !errors.Is(err, wuffs.ErrUnknownFormat) {
		t.Errorf("Probe(%s) error = %v, want ErrUnknownFormat", name, err)
	}
	if meta != nil {
		t.Errorf("Probe(%s) returned non-nil Meta %+v, want nil", name, meta)
	}
}
