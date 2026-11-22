package wuffs

import "golang.org/x/sys/unix"

// trimPages releases b's pages; for a private anonymous mapping Linux
// zero-fills them on next access. Failure leaves them resident, which is safe.
func trimPages(b []byte) {
	_ = unix.Madvise(b, unix.MADV_DONTNEED) //nolint:errcheck // failure leaves pages resident, which is safe
}
