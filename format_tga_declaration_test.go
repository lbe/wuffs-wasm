package wuffs

import "testing"

// TestUnitFormatTGADeclaration verifies that the package source declares the
// exported TGA format FourCC constant with an explicit uint32 type and the
// exact WUFFS_BASE__FOURCC__TGA value 0x54474120 ("TGA "). It inspects only
// non-test .go files via go/parser and go/types (never regular expressions and
// never the wasm guest), and it looks the name up by string so that this test
// compiles even before the constant exists.
func TestUnitFormatTGADeclaration(t *testing.T) {
	assertFormatFourCC(t, "FormatTGA", 0x54474120)
}
