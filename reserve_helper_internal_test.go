package wuffs

import "testing"

// reserveForDecode reserves d's slots for decoding src with the guest reserve
// rule: the source slot first, then the probed canvas plus the Wuffs work
// buffer that the guest places in the dst slot after the pixels. It returns
// the probe metadata and fails t on any error.
func reserveForDecode(t testing.TB, d *Decoder, src []byte) *Meta {
	t.Helper()
	if err := d.Reserve(0, len(src)); err != nil {
		t.Fatalf("Reserve(0, len(src)): %v", err)
	}
	m, err := d.Probe(src)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if err := d.Reserve(int(m.Stride)*int(m.Height)+int(m.WorkbufLen), len(src)); err != nil {
		t.Fatalf("Reserve(canvas+workbuf, len(src)): %v", err)
	}
	return m
}
