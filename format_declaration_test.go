package wuffs

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"strings"
	"testing"
)

// lookupDecl type-checks the parsed non-test package sources and returns the
// declared object for name, or nil when the package has no such declaration.
// It inspects only non-test .go files via go/parser and go/types (never
// regular expressions and never the wasm guest), and it looks the name up by
// string so that the TestUnitFormat*Declaration tests compile even before the
// constant exists.
func lookupDecl(name string) (types.Object, error) {
	fset := token.NewFileSet()
	var files []*ast.File
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, readErr := os.ReadFile(e.Name())
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), readErr)
		}
		f, parseErr := parser.ParseFile(fset, e.Name(), src, 0)
		if parseErr != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), parseErr)
		}
		files = append(files, f)
	}
	conf := types.Config{Importer: importer.Default(), Error: func(error) {}}
	// The Error handler consumes type-check diagnostics; the reported first
	// error (e.g. an unresolved module import in decoder.go) must not abort
	// inspection, and the package scope is still populated for all own
	// declarations.
	pkg, _ := conf.Check("wuffs", fset, files, nil)
	if pkg == nil {
		return nil, fmt.Errorf("type check package: returned nil package")
	}
	return pkg.Scope().Lookup(name), nil
}

// assertFormatFourCC fails unless the package source declares the exported
// constant name with an explicit uint32 type and the exact WUFFS FourCC value
// want. name is looked up by string via lookupDecl so the callers compile even
// before the constant exists.
func assertFormatFourCC(t *testing.T, name string, want uint32) {
	t.Helper()
	obj, err := lookupDecl(name)
	if err != nil {
		t.Fatalf("inspect package source: %v", err)
	}
	if obj == nil {
		t.Fatalf("expected exported constant %s with explicit type uint32 and value %s in the package source, but no such declaration exists", name, fourCCHex(want))
	}
	c, ok := obj.(*types.Const)
	if !ok {
		t.Fatalf("%s is declared as %T, want constant", name, obj)
	}
	if !obj.Exported() {
		t.Errorf("%s is not exported", name)
	}
	if !types.Identical(c.Type(), types.Typ[types.Uint32]) {
		t.Errorf("%s type = %v, want explicit type uint32", name, c.Type())
	}
	got, exact := constant.Int64Val(c.Val())
	if !exact || got != int64(want) {
		t.Errorf("%s value = %v, want %s (%d)", name, c.Val(), fourCCHex(want), want)
	}
}

// fourCCHex formats v as the upper-case 0x-prefixed hexadecimal literal used
// in declaration test diagnostics, e.g. 0x57424D50 for WBMP.
func fourCCHex(v uint32) string {
	return fmt.Sprintf("0x%X", v)
}
