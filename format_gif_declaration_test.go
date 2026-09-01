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

// TestUnitFormatGIFDeclaration verifies that the package source declares the
// exported GIF format FourCC constant with an explicit uint32 type and the
// exact WUFFS_BASE__FOURCC__GIF value 0x47494620 ("GIF "). It inspects only
// non-test .go files via go/parser and go/types (never regular expressions and
// never the wasm guest), and it looks the name up by string so that this test
// compiles even before the constant exists.
func TestUnitFormatGIFDeclaration(t *testing.T) {
	// lookup type-checks the parsed non-test package sources and returns the
	// declared object for name, or nil when the package has no such
	// declaration.
	lookup := func(name string) (types.Object, error) {
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

	obj, err := lookup("FormatGIF")
	if err != nil {
		t.Fatalf("inspect package source: %v", err)
	}
	if obj == nil {
		t.Fatalf("expected exported constant FormatGIF with explicit type uint32 and value 0x47494620 in the package source, but no such declaration exists")
	}
	c, ok := obj.(*types.Const)
	if !ok {
		t.Fatalf("FormatGIF is declared as %T, want constant", obj)
	}
	if !obj.Exported() {
		t.Errorf("FormatGIF is not exported")
	}
	if !types.Identical(c.Type(), types.Typ[types.Uint32]) {
		t.Errorf("FormatGIF type = %v, want explicit type uint32", c.Type())
	}
	got, exact := constant.Int64Val(c.Val())
	if !exact || got != 1195984416 {
		t.Errorf("FormatGIF value = %v, want 0x47494620 (1195984416)", c.Val())
	}
}
