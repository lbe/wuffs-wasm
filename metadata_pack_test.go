package wuffs

import (
	"testing"
	"unsafe"
)

// TestUnitMetadataPackHeaderSize88 pins the Go mirror of the guest metadata
// pack header to 88 bytes, matching wuffs_wasm_metadata_pack_header in
// wasm/shim.c (enforced there by static_assert).
func TestUnitMetadataPackHeaderSize88(t *testing.T) {
	if got := unsafe.Sizeof(metadataPackHeader{}); got != 88 {
		t.Errorf("unsafe.Sizeof(metadataPackHeader{}) = %d, want 88", got)
	}
}
