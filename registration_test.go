package wuffs

import (
	"image"
	"io"
	"sync"
	"testing"
)

// TestUnitRegisterFormatsOnce asserts that repeated and concurrently
// released calls to RegisterFormats submit each (name, magic) registration
// entry to the decoder registry exactly once. A counting registrar is
// injected through the package-level registerWith seam, so the real global
// image registry is never touched.
func TestUnitRegisterFormatsOnce(t *testing.T) {
	orig := registerWith
	t.Cleanup(func() { registerWith = orig })

	var mu sync.Mutex
	counts := map[string]int{}
	registerWith = func(name, magic string, decode func(io.Reader) (image.Image, error), decodeConfig func(io.Reader) (image.Config, error)) {
		mu.Lock()
		counts[name+"\x00"+magic]++
		mu.Unlock()
	}

	// Sequential repeated calls must not duplicate submissions.
	for range 3 {
		RegisterFormats()
	}

	// Goroutines released together must not duplicate submissions.
	const goroutines = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			RegisterFormats()
		}()
	}
	close(start)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(counts) == 0 {
		t.Fatal("no formats submitted through the injected registrar: RegisterFormats bypasses the registerWith seam")
	}
	for entry, n := range counts {
		if n != 1 {
			t.Errorf("registration entry %q submitted %d times, want exactly once", entry, n)
		}
	}
}
