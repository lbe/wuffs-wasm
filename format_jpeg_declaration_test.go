package wuffs

import "testing"

// TestUnitFormatJPEGDeclaration verifies that the package source declares the
// exported JPEG format FourCC constant with an explicit uint32 type and the
// exact WUFFS_BASE__FOURCC__JPEG value 0x4A504547 ("JPEG"). It inspects only
// non-test .go files via go/parser and go/types (never regular expressions and
// never the wasm guest), and it looks the name up by string so that this test
// compiles even before the constant exists.
func TestUnitFormatJPEGDeclaration(t *testing.T) {
	assertFormatFourCC(t, "FormatJPEG", 0x4A504547)
}
