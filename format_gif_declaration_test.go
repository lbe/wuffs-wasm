package wuffs

import "testing"

// TestUnitFormatGIFDeclaration verifies that the package source declares the
// exported GIF format FourCC constant with an explicit uint32 type and the
// exact WUFFS_BASE__FOURCC__GIF value 0x47494620 ("GIF "). It inspects only
// non-test .go files via go/parser and go/types (never regular expressions and
// never the wasm guest), and it looks the name up by string so that this test
// compiles even before the constant exists.
func TestUnitFormatGIFDeclaration(t *testing.T) {
	assertFormatFourCC(t, "FormatGIF", 0x47494620)
}
