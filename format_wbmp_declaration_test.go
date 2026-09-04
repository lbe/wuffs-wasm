package wuffs

import "testing"

// TestUnitFormatWBMPDeclaration verifies that the package source declares the
// exported WBMP format FourCC constant with an explicit uint32 type and the
// exact WUFFS_BASE__FOURCC__WBMP value 0x57424D50 ("WBMP"). It inspects only
// non-test .go files via go/parser and go/types (never regular expressions and
// never the wasm guest), and it looks the name up by string so that this test
// compiles even before the constant exists.
func TestUnitFormatWBMPDeclaration(t *testing.T) {
	assertFormatFourCC(t, "FormatWBMP", 0x57424D50)
}
