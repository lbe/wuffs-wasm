package wuffs

import "testing"

// TestUnitFormatQOIDeclaration verifies that the package source declares the
// exported QOI format FourCC constant with an explicit uint32 type and the
// exact WUFFS_BASE__FOURCC__QOI value 0x514F4920 ("QOI "). It inspects only
// non-test .go files via go/parser and go/types (never regular expressions and
// never the wasm guest), and it looks the name up by string so that this test
// compiles even before the constant exists.
func TestUnitFormatQOIDeclaration(t *testing.T) {
	assertFormatFourCC(t, "FormatQOI", 0x514F4920)
}
