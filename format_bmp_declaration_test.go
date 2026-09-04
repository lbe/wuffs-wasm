package wuffs

import "testing"

// TestUnitFormatBMPDeclaration verifies that the package source declares the
// exported BMP format FourCC constant with an explicit uint32 type and the
// exact WUFFS_BASE__FOURCC__BMP value 0x424D5020 ("BMP "). It inspects only
// non-test .go files via go/parser and go/types (never regular expressions and
// never the wasm guest), and it looks the name up by string so that this test
// compiles even before the constant exists.
func TestUnitFormatBMPDeclaration(t *testing.T) {
	assertFormatFourCC(t, "FormatBMP", 0x424D5020)
}
