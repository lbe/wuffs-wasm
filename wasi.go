package wuffs

import (
	"crypto/rand"

	wasihost "github.com/lbe/wasm2go-wasi-host"
)

// newWASIState creates the WASI host state for a single Decoder. The memory
// callback reads the Decoder's own guest memory, so WASI host state is never
// shared between Decoders.
func newWASIState(mem guestMemory) *wasihost.State {
	cfg := wasihost.NewModuleConfig().WithRandSource(rand.Reader)
	return wasihost.New(func() []byte { return *mem.Slice() }, cfg)
}
