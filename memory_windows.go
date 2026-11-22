package wuffs

import (
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

// newGuestMemory reserves maxMemoryBytes of address space. On a 32-bit
// platform, or when the reservation fails, it falls back to a Go slice.
func newGuestMemory() guestMemory {
	if strconv.IntSize == 32 {
		return &sliceMemory{}
	}
	addr, err := windows.VirtualAlloc(0, maxMemoryBytes, windows.MEM_RESERVE, windows.PAGE_NOACCESS)
	if err != nil {
		return &sliceMemory{}
	}
	return &virtualMemory{addr: addr}
}

// virtualMemory is linear memory inside one reserved region. Grow commits the
// next pages in place; nothing moves.
type virtualMemory struct {
	addr uintptr // base of the reservation
	buf  []byte  // committed prefix
}

func (m *virtualMemory) Slice() *[]byte { return &m.buf }

func (m *virtualMemory) Grow(delta, max int64) int64 {
	old, newLen, ok := growLen(len(m.buf), delta, max)
	if !ok || m.addr == 0 {
		return -1
	}
	if newLen > len(m.buf) {
		cur := uintptr(len(m.buf))
		if _, err := windows.VirtualAlloc(m.addr+cur, uintptr(newLen)-cur, windows.MEM_COMMIT, windows.PAGE_READWRITE); err != nil {
			return -1
		}
		// The region is outside the Go heap and never moves.
		m.buf = unsafe.Slice((*byte)(unsafe.Add(nil, m.addr)), newLen)
	}
	return old
}

func (m *virtualMemory) release() error {
	if m.addr == 0 {
		return nil
	}
	err := windows.VirtualFree(m.addr, 0, windows.MEM_RELEASE)
	m.addr, m.buf = 0, nil
	return err
}

func (m *virtualMemory) trim(from int) {
	if from >= len(m.buf) {
		return
	}
	start, size := m.addr+uintptr(from), uintptr(len(m.buf)-from)
	if err := windows.VirtualFree(start, size, windows.MEM_DECOMMIT); err != nil {
		return
	}
	if _, err := windows.VirtualAlloc(start, size, windows.MEM_COMMIT, windows.PAGE_READWRITE); err != nil {
		// The guest would fault on its next access to these pages.
		panic("wuffs: recommitting trimmed guest memory failed: " + err.Error())
	}
}

// guestMemoryMapped reports that 64-bit targets map guest memory outside the
// Go heap.
const guestMemoryMapped = true
