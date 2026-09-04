package wuffs

import "testing"

// TestUnitFormatTHDeclaration verifies that the package source declares the
// exported TH format FourCC constant with an explicit uint32 type and the
// exact WUFFS_BASE__FOURCC__TH value 0x54482020 ("TH  "). It inspects only
// non-test .go files via go/parser and go/types (never regular expressions and
// never the wasm guest), and it looks the name up by string so that this test
// compiles even before the constant exists.
func TestUnitFormatTHDeclaration(t *testing.T) {
	assertFormatFourCC(t, "FormatTH", 0x54482020)
}
