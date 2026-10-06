package wuffs_test

// GIF multi-frame characterization against
// testdata/gif.animation.golden.manifest: pins FrameCount/LoopCount and
// animated-red-blue.gif metadata.

import (
	"bufio"
	"hash/crc32"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/lbe/wuffs-wasm"
)

// Animation metadata for testdata/animated-red-blue.gif, recorded from the
// first successful GREEN run (2026-10-06). Canvas is 64x48. All four frames
// report DisposalNone; Overwrite is false; durations increase per frame.
const (
	redBlueFrame0Duration = 100 * time.Millisecond
	redBlueFrame1Duration = 200 * time.Millisecond
	redBlueFrame2Duration = 300 * time.Millisecond
	redBlueFrame3Duration = 400 * time.Millisecond
)

// loadAnimationManifest parses a "<file> <index> <crc32-hex>" manifest into a
// map keyed by file then frame index.
func loadAnimationManifest(t *testing.T, name string) map[string]map[int]uint32 {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("opening testdata/%s: %v", name, err)
	}
	defer f.Close()
	out := map[string]map[int]uint32{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Fatalf("malformed manifest line %q in %s: want 3 fields", line, name)
		}
		idx, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatalf("malformed frame index in line %q: %v", line, err)
		}
		crc, err := strconv.ParseUint(fields[2], 16, 32)
		if err != nil {
			t.Fatalf("malformed crc in line %q: %v", line, err)
		}
		if out[fields[0]] == nil {
			out[fields[0]] = map[int]uint32{}
		}
		out[fields[0]][idx] = uint32(crc)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanning %s: %v", name, err)
	}
	return out
}

// TestIntegrationGIFAnimationCharacterization verifies GIF multi-frame
// behavior against the golden manifest: muybridge.gif reports 15 frames and
// loops forever (0, the Task 3 pinned literal), every frame decodes on a
// fresh zeroed canvas to its pinned CRC, and animated-red-blue.gif reports 4
// frames with the table-driven disposal/overwrite/duration metadata below.
// Every successful decode preserves caller Pix identity and guest memory.
func TestIntegrationGIFAnimationCharacterization(t *testing.T) {
	manifest := loadAnimationManifest(t, "gif.animation.golden.manifest")

	t.Run("muybridge.gif has 15 frames and loops forever", func(t *testing.T) {
		d := wuffs.New()
		src, err := os.ReadFile(filepath.Join("testdata", "muybridge.gif"))
		if err != nil {
			t.Fatalf("reading testdata/muybridge.gif: %v", err)
		}
		before := wuffs.CaptureGuestMemoryState(d)
		n, err := d.FrameCount(src)
		if err != nil {
			t.Fatalf("FrameCount(muybridge.gif) error = %v, want nil", err)
		}
		if n != 15 {
			t.Errorf("FrameCount(muybridge.gif) = %d, want 15", n)
		}
		assertGuestMemoryPreserved(t, "FrameCount", "muybridge.gif", d, before)

		before = wuffs.CaptureGuestMemoryState(d)
		loops, err := d.LoopCount(src)
		if err != nil {
			t.Fatalf("LoopCount(muybridge.gif) error = %v, want nil", err)
		}
		if loops != 0 {
			t.Errorf("LoopCount(muybridge.gif) = %d, want 0 (Task 3 pinned literal)", loops)
		}
		assertGuestMemoryPreserved(t, "LoopCount", "muybridge.gif", d, before)
	})

	t.Run("muybridge.gif per-frame CRCs match manifest", func(t *testing.T) {
		src, err := os.ReadFile(filepath.Join("testdata", "muybridge.gif"))
		if err != nil {
			t.Fatalf("reading testdata/muybridge.gif: %v", err)
		}
		d := wuffs.New()
		probe, err := d.Probe(src)
		if err != nil {
			t.Fatalf("Probe(muybridge.gif) error = %v, want nil", err)
		}
		w, h := int(probe.Width), int(probe.Height)
		if w != 30 || h != 20 {
			t.Fatalf("Probe(muybridge.gif) = %dx%d, want 30x20", w, h)
		}
		if err := d.Reserve(int(probe.Stride)*h, len(src)); err != nil {
			t.Fatalf("Reserve error = %v, want nil", err)
		}
		for i := 0; i < 15; i++ {
			dst := image.NewRGBA(image.Rect(0, 0, w, h))
			pixPtr := unsafe.SliceData(dst.Pix)
			pixLen, pixCap := len(dst.Pix), cap(dst.Pix)
			rect, stride := dst.Rect, dst.Stride
			before := wuffs.CaptureGuestMemoryState(d)
			frame, err := d.DecodeFrame(dst, src, i)
			if err != nil {
				t.Fatalf("DecodeFrame(muybridge.gif, %d) error = %v, want nil", i, err)
			}
			if frame == nil {
				t.Fatalf("DecodeFrame(muybridge.gif, %d) returned nil Frame", i)
			}
			if frame.Index != i {
				t.Errorf("frame %d: Frame.Index = %d, want %d", i, frame.Index, i)
			}
			want, ok := manifest["muybridge.gif"][i]
			if !ok {
				t.Fatalf("frame %d: no manifest entry for muybridge.gif", i)
			}
			if got := crc32.ChecksumIEEE(dst.Pix); got != want {
				t.Errorf("frame %d: full-canvas CRC = 0x%08X, want manifest 0x%08X", i, got, want)
			}
			if unsafe.SliceData(dst.Pix) != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != stride {
				t.Errorf("frame %d: DecodeFrame changed the caller-owned destination layout", i)
			}
			assertGuestMemoryPreserved(t, "DecodeFrame", "muybridge.gif", d, before)
		}
	})

	t.Run("animated-red-blue.gif metadata table", func(t *testing.T) {
		src, err := os.ReadFile(filepath.Join("testdata", "animated-red-blue.gif"))
		if err != nil {
			t.Fatalf("reading testdata/animated-red-blue.gif: %v", err)
		}
		d := wuffs.New()
		n, err := d.FrameCount(src)
		if err != nil {
			t.Fatalf("FrameCount(animated-red-blue.gif) error = %v, want nil", err)
		}
		if n != 4 {
			t.Fatalf("FrameCount(animated-red-blue.gif) = %d, want 4", n)
		}
		probe, err := d.Probe(src)
		if err != nil {
			t.Fatalf("Probe(animated-red-blue.gif) error = %v, want nil", err)
		}
		w, h := int(probe.Width), int(probe.Height)
		if err := d.Reserve(int(probe.Stride)*h, len(src)); err != nil {
			t.Fatalf("Reserve error = %v, want nil", err)
		}
		full := image.Rect(0, 0, w, h)
		want := []struct {
			bounds    image.Rectangle
			duration  time.Duration
			disposal  wuffs.Disposal
			overwrite bool
		}{
			{image.Rect(0, 0, 64, 48), redBlueFrame0Duration, wuffs.DisposalNone, false},
			{image.Rect(15, 31, 52, 40), redBlueFrame1Duration, wuffs.DisposalNone, false},
			{image.Rect(15, 0, 64, 40), redBlueFrame2Duration, wuffs.DisposalNone, false},
			{image.Rect(15, 0, 64, 40), redBlueFrame3Duration, wuffs.DisposalNone, false},
		}
		for i, wnt := range want {
			dst := image.NewRGBA(image.Rect(0, 0, w, h))
			pixPtr := unsafe.SliceData(dst.Pix)
			pixLen, pixCap := len(dst.Pix), cap(dst.Pix)
			rect, stride := dst.Rect, dst.Stride
			before := wuffs.CaptureGuestMemoryState(d)
			frame, err := d.DecodeFrame(dst, src, i)
			if err != nil {
				t.Fatalf("DecodeFrame(animated-red-blue.gif, %d) error = %v, want nil", i, err)
			}
			if frame == nil {
				t.Fatalf("DecodeFrame(animated-red-blue.gif, %d) returned nil Frame", i)
			}
			if frame.Index != i {
				t.Errorf("frame %d: Frame.Index = %d, want %d", i, frame.Index, i)
			}
			if frame.Bounds != wnt.bounds {
				t.Errorf("frame %d: Frame.Bounds = %v, want %v", i, frame.Bounds, wnt.bounds)
			}
			if i == 1 && frame.Bounds == full {
				t.Errorf("frame 1: Frame.Bounds = %v, want non-full-canvas bounds", frame.Bounds)
			}
			if frame.Duration != wnt.duration {
				t.Errorf("frame %d: Frame.Duration = %v, want %v", i, frame.Duration, wnt.duration)
			}
			if frame.Disposal != wnt.disposal {
				t.Errorf("frame %d: Frame.Disposal = %d, want %d", i, frame.Disposal, wnt.disposal)
			}
			if frame.Overwrite != wnt.overwrite {
				t.Errorf("frame %d: Frame.Overwrite = %v, want %v", i, frame.Overwrite, wnt.overwrite)
			}
			if frame.Duration == 0 {
				t.Errorf("frame %d: Frame.Duration = 0, want non-zero", i)
			}
			mcrc, ok := manifest["animated-red-blue.gif"][i]
			if !ok {
				t.Fatalf("frame %d: no manifest entry for animated-red-blue.gif", i)
			}
			if got := crc32.ChecksumIEEE(dst.Pix); got != mcrc {
				t.Errorf("frame %d: full-canvas CRC = 0x%08X, want manifest 0x%08X", i, got, mcrc)
			}
			if unsafe.SliceData(dst.Pix) != pixPtr || len(dst.Pix) != pixLen || cap(dst.Pix) != pixCap || dst.Rect != rect || dst.Stride != stride {
				t.Errorf("frame %d: DecodeFrame changed the caller-owned destination layout", i)
			}
			assertGuestMemoryPreserved(t, "DecodeFrame", "animated-red-blue.gif", d, before)
		}
	})
}
