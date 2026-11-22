//go:build unix

package wuffs

import (
	"strconv"

	"golang.org/x/sys/unix"
)

// newGuestMemory reserves maxMemoryBytes of inaccessible address space. On a
// 32-bit platform, or when the reservation fails, it falls back to a Go slice.
func newGuestMemory() guestMemory {
	if strconv.IntSize == 32 {
		return &sliceMemory{}
	}
	res, err := unix.Mmap(-1, 0, maxMemoryBytes, unix.PROT_NONE, unix.MAP_PRIVATE|unix.MAP_ANON)
	if err != nil {
		return &sliceMemory{}
	}
	return &mmapMemory{res: res, buf: res[:0]}
}

// mmapMemory is linear memory inside one anonymous private mapping. Grow
// makes the next pages readable and writable in place; nothing moves.
type mmapMemory struct {
	res []byte // the whole reservation, as returned by unix.Mmap
	buf []byte // res[:current size]
}

func (m *mmapMemory) Slice() *[]byte { return &m.buf }

func (m *mmapMemory) Grow(delta, max int64) int64 {
	old, newLen, ok := growLen(len(m.buf), delta, max)
	if !ok || newLen > len(m.res) {
		return -1
	}
	if newLen > len(m.buf) {
		if err := unix.Mprotect(m.res[len(m.buf):newLen], unix.PROT_READ|unix.PROT_WRITE); err != nil {
			return -1
		}
		m.buf = m.res[:newLen]
	}
	return old
}

func (m *mmapMemory) release() error {
	if m.res == nil {
		return nil
	}
	err := unix.Munmap(m.res)
	m.res, m.buf = nil, nil
	return err
}

func (m *mmapMemory) trim(from int) {
	if from < len(m.buf) {
		trimPages(m.buf[from:])
	}
}

// guestMemoryMapped reports that 64-bit targets map guest memory outside the
// Go heap.
const guestMemoryMapped = true
