package wuffs

import "testing"

// TestUnitFormatNIEDeclaration verifies that the package source declares the
// exported NIE format FourCC constant with an explicit uint32 type and the
// exact WUFFS_BASE__FOURCC__NIE value 0x4E494520 ("NIE "). It inspects only
// non-test .go files via go/parser and go/types (never regular expressions and
// never the wasm guest), and it looks the name up by string so that this test
// compiles even before the constant exists.
func TestUnitFormatNIEDeclaration(t *testing.T) {
	assertFormatFourCC(t, "FormatNIE", 0x4E494520)
}
