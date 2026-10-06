package wuffs

import (
	"go/constant"
	"go/types"
	"testing"
)

// TestUnitFrameAndDisposalDeclaration verifies that the package source declares
// the exported animation Frame and Disposal types with the exact API.md shape:
// Frame carries the eight exported fields Index, Bounds, Duration, Disposal,
// Opaque, Overwrite, Background, and IOPosition; Disposal is a uint8 kind; and
// the DisposalNone, DisposalRestoreBackground, and DisposalRestorePrevious
// constants have values 0, 1, and 2. It inspects only non-test .go files via
// go/parser and go/types (never regular expressions and never the wasm guest),
// and it looks each name up by string so that this test compiles even before
// the declarations exist.
func TestUnitFrameAndDisposalDeclaration(t *testing.T) {
	frameObj, err := lookupDecl("Frame")
	if err != nil {
		t.Fatalf("inspect package source: %v", err)
	}
	if frameObj == nil {
		t.Fatalf("expected exported type Frame with fields Index, Bounds, Duration, Disposal, Opaque, Overwrite, Background, IOPosition in the package source, but no such declaration exists")
	}
	frameName, ok := frameObj.(*types.TypeName)
	if !ok {
		t.Fatalf("Frame is declared as %T, want type", frameObj)
	}
	if !frameObj.Exported() {
		t.Errorf("Frame is not exported")
	}
	frameStruct, ok := frameName.Type().Underlying().(*types.Struct)
	if !ok {
		t.Fatalf("Frame underlying type = %v, want struct", frameName.Type().Underlying())
	}
	wantFields := []struct {
		name string
		want string
	}{
		{"Index", "int"},
		{"Bounds", "image.Rectangle"},
		{"Duration", "time.Duration"},
		{"Disposal", "wuffs.Disposal"},
		{"Opaque", "bool"},
		{"Overwrite", "bool"},
		{"Background", "image/color.RGBA"},
		{"IOPosition", "uint64"},
	}
	for _, want := range wantFields {
		field, found := fieldByName(frameStruct, want.name)
		if !found {
			t.Errorf("Frame is missing exported field %s", want.name)
			continue
		}
		if !field.Exported() {
			t.Errorf("Frame.%s is not exported", want.name)
			continue
		}
		if got := field.Type().String(); got != want.want {
			t.Errorf("Frame.%s type = %s, want %s", want.name, got, want.want)
		}
	}

	disposalObj, err := lookupDecl("Disposal")
	if err != nil {
		t.Fatalf("inspect package source: %v", err)
	}
	if disposalObj == nil {
		t.Fatalf("expected exported type Disposal with underlying type uint8 in the package source, but no such declaration exists")
	}
	disposalName, ok := disposalObj.(*types.TypeName)
	if !ok {
		t.Fatalf("Disposal is declared as %T, want type", disposalObj)
	}
	if !disposalObj.Exported() {
		t.Errorf("Disposal is not exported")
	}
	if basic, ok := disposalName.Type().Underlying().(*types.Basic); !ok || basic.Kind() != types.Uint8 {
		t.Errorf("Disposal underlying type = %v, want uint8", disposalName.Type().Underlying())
	}

	assertDisposalValue(t, "DisposalNone", 0)
	assertDisposalValue(t, "DisposalRestoreBackground", 1)
	assertDisposalValue(t, "DisposalRestorePrevious", 2)
}

// fieldByName returns the struct field with the given name.
func fieldByName(st *types.Struct, name string) (*types.Var, bool) {
	for i := 0; i < st.NumFields(); i++ {
		if f := st.Field(i); f.Name() == name {
			return f, true
		}
	}
	return nil, false
}

// assertDisposalValue fails unless the package source declares the exported
// constant name with type Disposal and the exact value want. name is looked up
// by string via lookupDecl so the callers compile even before the constant
// exists.
func assertDisposalValue(t *testing.T, name string, want int64) {
	t.Helper()
	obj, err := lookupDecl(name)
	if err != nil {
		t.Fatalf("inspect package source: %v", err)
	}
	if obj == nil {
		t.Fatalf("expected exported constant %s with type Disposal and value %d in the package source, but no such declaration exists", name, want)
	}
	c, ok := obj.(*types.Const)
	if !ok {
		t.Fatalf("%s is declared as %T, want constant", name, obj)
	}
	if !obj.Exported() {
		t.Errorf("%s is not exported", name)
	}
	if got := c.Type().String(); got != "wuffs.Disposal" {
		t.Errorf("%s type = %s, want wuffs.Disposal", name, got)
	}
	got, exact := constant.Int64Val(c.Val())
	if !exact || got != want {
		t.Errorf("%s value = %v, want %d", name, c.Val(), want)
	}
}
