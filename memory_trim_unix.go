//go:build unix && !linux

package wuffs

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// trimPages replaces b's pages with fresh zero-filled anonymous pages at the
// same address and protection. MADV_DONTNEED does not guarantee zero-filled
// pages outside Linux, so a fixed remap is used instead.
func trimPages(b []byte) {
	//nolint:errcheck // failure leaves pages resident, which is safe
	_, _ = unix.MmapPtr(-1, 0, unsafe.Pointer(&b[0]), uintptr(len(b)),
		unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON|unix.MAP_FIXED)
}
