//go:build linux && rss

package wuffs_test

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	wuffs "github.com/lbe/wuffs-wasm"
)

// The RSS tests run each scenario in a fresh child process of the test binary,
// so memory left by other tests cannot skew the measurement. The child prints
// one line "rss <name> <KiB>" per measurement point. Run with -tags rss
// (make test-all).

const rssChildEnv = "WUFFS_RSS_CHILD"

const miB = 1024 // KiB

// TestIntegrationRSSChild is the child entry point; it does nothing unless
// rssChildEnv names a scenario.
func TestIntegrationRSSChild(t *testing.T) {
	name := os.Getenv(rssChildEnv)
	if name == "" {
		t.Skip("RSS child scenario; run through the TestIntegrationRSS tests")
	}
	scenario, ok := rssScenarios[name]
	if !ok {
		t.Fatalf("unknown RSS scenario %q", name)
	}
	scenario(t)
}

var rssScenarios = map[string]func(t *testing.T){
	"new-decoder": func(t *testing.T) {
		printRSS(t, "before")
		d := wuffs.New()
		printRSS(t, "after")
		_ = d.Close()
	},
	"eight-decoders": func(t *testing.T) {
		src, size := opaquePNG(t, 1024)
		printRSS(t, "before")
		var ds []*wuffs.Decoder
		for range 8 {
			d := wuffs.New()
			reserveForDecode(t, d, src)
			dst := image.NewRGBA(image.Rect(0, 0, size, size))
			if _, err := d.DecodeRGBA(dst, src); err != nil {
				t.Fatalf("DecodeRGBA: %v", err)
			}
			ds = append(ds, d)
		}
		printRSS(t, "after")
		for _, d := range ds {
			_ = d.Close()
		}
	},
	"trim": func(t *testing.T) {
		src, size := opaquePNG(t, 2000)
		d := wuffs.New()
		reserveForDecode(t, d, src)
		dst := image.NewRGBA(image.Rect(0, 0, size, size))
		touch(dst.Pix)
		printRSS(t, "before")
		if _, err := d.DecodeRGBA(dst, src); err != nil {
			t.Fatalf("DecodeRGBA: %v", err)
		}
		printRSS(t, "decoded")
		d.Trim()
		printRSS(t, "after")
		_ = d.Close()
	},
	"close": func(t *testing.T) {
		src, size := opaquePNG(t, 2000)
		d := wuffs.New()
		reserveForDecode(t, d, src)
		dst := image.NewRGBA(image.Rect(0, 0, size, size))
		touch(dst.Pix)
		if _, err := d.DecodeRGBA(dst, src); err != nil {
			t.Fatalf("DecodeRGBA: %v", err)
		}
		printRSS(t, "before")
		if err := d.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		printRSS(t, "after")
	},
}

// opaquePNG encodes an opaque n×n RGB PNG with varied pixels.
func opaquePNG(t *testing.T, n int) ([]byte, int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for i := range img.Pix {
		if i%4 == 3 {
			img.Pix[i] = 0xFF
		} else {
			img.Pix[i] = byte(i*7 + i/4096)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes(), n
}

// touch writes every page of b so it is resident before a measurement.
func touch(b []byte) {
	for i := 0; i < len(b); i += 4096 {
		b[i] = 1
	}
}

func printRSS(t *testing.T, point string) {
	t.Helper()
	fmt.Printf("rss %s %d\n", point, vmRSSKiB(t))
}

func vmRSSKiB(t *testing.T) int {
	t.Helper()
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatalf("reading /proc/self/status: %v", err)
	}
	for line := range strings.SplitSeq(string(status), "\n") {
		if rest, ok := strings.CutPrefix(line, "VmRSS:"); ok {
			kib, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(rest), " kB"))
			if err != nil {
				t.Fatalf("parsing VmRSS %q: %v", line, err)
			}
			return kib
		}
	}
	t.Fatal("VmRSS not found in /proc/self/status")
	return 0
}

// runRSSChild runs scenario in a fresh process and returns its measurements.
func runRSSChild(t *testing.T, scenario string) map[string]int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestIntegrationRSSChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), rssChildEnv+"="+scenario)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("RSS child %q: %v\n%s", scenario, err, out)
	}
	points := map[string]int{}
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "rss" {
			kib, err := strconv.Atoi(fields[2])
			if err != nil {
				t.Fatalf("parsing %q: %v", line, err)
			}
			points[fields[1]] = kib
		}
	}
	t.Logf("%s: %v KiB", scenario, points)
	return points
}

func TestIntegrationRSSNewDecoder(t *testing.T) {
	p := runRSSChild(t, "new-decoder")
	if grew := p["after"] - p["before"]; grew >= 4*miB {
		t.Fatalf("new Decoder added %d KiB RSS, want < 4 MiB", grew)
	}
}

func TestIntegrationRSSEightDecoders(t *testing.T) {
	p := runRSSChild(t, "eight-decoders")
	if p["after"] >= 120*miB {
		t.Fatalf("8 decoders after 1024x1024 decodes: VmRSS %d KiB, want < 120 MiB", p["after"])
	}
}

func TestIntegrationRSSTrim(t *testing.T) {
	p := runRSSChild(t, "trim")
	if diff := p["after"] - p["before"]; diff > 2*miB {
		t.Fatalf("RSS after decode and Trim is %d KiB above its value before the decode, want <= 2 MiB", diff)
	}
}

func TestIntegrationRSSClose(t *testing.T) {
	p := runRSSChild(t, "close")
	// The decode wrote the 2000x2000 canvas and the PNG work buffer, about
	// 30 MiB of guest pages; Close must return them.
	if freed := p["before"] - p["after"]; freed < 24*miB {
		t.Fatalf("Close freed %d KiB RSS, want >= 24 MiB", freed)
	}
}
