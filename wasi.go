package wuffs

import (
	"crypto/rand"

	wasihost "github.com/lbe/wasm2go-wasi-host"
	"github.com/lbe/wuffs-wasm/internal/wuffswasm"
)

// newWASIState creates the WASI host state for a single Decoder. The memory
// callback reads through moduleRef so each Decoder owns its own wasm2go module;
// this keeps the WASI host state separate per Decoder and avoids sharing a
// package-level module.
func newWASIState(moduleRef **wuffswasm.Module) *wasihost.State {
	cfg := wasihost.NewModuleConfig().WithRandSource(rand.Reader)
	return wasihost.New(func() []byte { return memoryView(moduleRef) }, cfg)
}

// memoryView returns the current wasm linear memory for the module referenced
// by moduleRef. It is a standalone helper so the WASI callback captures a clear
// name rather than an inline double-dereference.
func memoryView(moduleRef **wuffswasm.Module) []byte {
	return *(*moduleRef).Xmemory().Slice()
}
