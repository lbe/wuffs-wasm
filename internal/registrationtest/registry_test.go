package registrationtest

import (
	"testing"

	wuffs "github.com/lbe/wuffs-wasm"
)

// TestIntegrationImageRegistry exercises image.RegisterFormat behavior in a
// test binary that imports only wuffs and image (no image/png/jpeg/gif). One
// ordered pass: empty registry, RegisterFormats, then catalog checks.
// RegisterFormats uses sync.Once; do not split across top-level tests. BM
// sentinel precedence lives in ./precedence (separate test binary).
func TestIntegrationImageRegistry(t *testing.T) {
	runBeforeRegister(t)
	wuffs.RegisterFormats()
	runLiteralSignatures(t)
	runDecodeConfig(t)
	runTGA(t)
	runWBMP(t)
	runNetpbm(t)
	runHandsum(t)
}
