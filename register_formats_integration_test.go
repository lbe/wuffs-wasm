package wuffs

import (
	"go/types"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegrationRegisterFormatsExplicitOptIn verifies the exported
// RegisterFormats entry point: decoding through the standard library image
// registry must fail with image.ErrFormat until RegisterFormats is called,
// and must afterwards decode the PNG fixture identically to wuffs.Decode.
//
// The registry checks run in a subprocess (internal/registrationtest/probe)
// whose imports are limited to wuffs and image, so the global registry state
// is deterministic even though other tests in this package's test binary
// link in self-registering codecs. RegisterFormats itself is resolved through
// the go/types source inspection shared with the declaration tests, so this
// test compiles and fails with a meaningful assertion before the symbol
// exists.
func TestIntegrationRegisterFormatsExplicitOptIn(t *testing.T) {
	obj, err := lookupDecl("RegisterFormats")
	if err != nil {
		t.Fatalf("inspect package source: %v", err)
	}
	if obj == nil {
		t.Fatal("RegisterFormats is not declared in package wuffs: expected an exported func RegisterFormats()")
	}
	if !obj.Exported() {
		t.Error("RegisterFormats is declared but not exported")
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		t.Fatalf("RegisterFormats is declared as %T, want func", obj)
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		t.Fatalf("RegisterFormats type = %T, want *types.Signature", fn.Type())
	}
	if sig.Params().Len() != 0 || sig.Results().Len() != 0 {
		t.Errorf("RegisterFormats signature = %v, want func() with no parameters and no results", sig)
	}

	// The probe source carries an ignore build tag so go build ./... stays
	// green before RegisterFormats exists; naming the file explicitly runs it
	// anyway.
	cmd := exec.Command("go", "run", "./internal/registrationtest/probe/main.go",
		"-fixture", filepath.Join("testdata", "bricks-color.png"))
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("isolated registration probe failed: %v\n%s", runErr, out)
	}
	if !strings.Contains(string(out), "probe: ok") {
		t.Errorf("isolated registration probe output = %q, want it to report ok", string(out))
	}
}
