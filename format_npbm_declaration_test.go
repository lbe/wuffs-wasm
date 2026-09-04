package wuffs

import "testing"

// TestUnitFormatNPBMDeclaration verifies that the package source declares the
// exported NPBM format FourCC constant with an explicit uint32 type and the
// exact WUFFS_BASE__FOURCC__NPBM value 0x4E50424D ("NPBM"). It inspects only
// non-test .go files via go/parser and go/types (never regular expressions and
// never the wasm guest), and it looks the name up by string so that this test
// compiles even before the constant exists.
func TestUnitFormatNPBMDeclaration(t *testing.T) {
	assertFormatFourCC(t, "FormatNPBM", 0x4E50424D)
}
