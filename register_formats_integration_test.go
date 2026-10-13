package wuffs

import (
	"go/types"
	"testing"
)

// TestIntegrationRegisterFormatsExplicitOptIn verifies RegisterFormats is
// declared with the expected exported signature. Registry behavior is covered
// in internal/registrationtest (isolated image import graph).
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
}
