package wuffs

import "testing"

// TestUnitFormatHNSMDeclaration verifies that the package source declares the
// exported HNSM format FourCC constant with an explicit uint32 type and the
// exact WUFFS_BASE__FOURCC__HNSM value 0x484E534D ("HNSM"). It inspects only
// non-test .go files via go/parser and go/types (never regular expressions and
// never the wasm guest), and it looks the name up by string so that this test
// compiles even before the constant exists.
func TestUnitFormatHNSMDeclaration(t *testing.T) {
	assertFormatFourCC(t, "FormatHNSM", 0x484E534D)
}
