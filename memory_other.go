//go:build !unix && !windows

package wuffs

// newGuestMemory returns the Go-slice fallback.
func newGuestMemory() guestMemory { return &sliceMemory{} }

// guestMemoryMapped reports that this platform keeps guest memory in a Go
// slice.
const guestMemoryMapped = false
