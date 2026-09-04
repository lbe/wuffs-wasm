package wuffs

import "testing"

// TestUnitFormatETC2Declaration verifies that the package source declares the
// exported ETC2 format FourCC constant with an explicit uint32 type and the
// exact WUFFS_BASE__FOURCC__ETC2 value 0x45544332 ("ETC2"). It inspects only
// non-test .go files via go/parser and go/types (never regular expressions and
// never the wasm guest), and it looks the name up by string so that this test
// compiles even before the constant exists.
func TestUnitFormatETC2Declaration(t *testing.T) {
	assertFormatFourCC(t, "FormatETC2", 0x45544332)
}
