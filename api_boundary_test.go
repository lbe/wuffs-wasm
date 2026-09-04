package wuffs

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestUnitPublicAPIBoundary enforces the exact public declaration inventory of
// the root package. It inspects only non-test .go files via go/parser (never
// regular expressions) so that test-only exports do not become product API.
//
// The required inventory is the portion of API.md currently owned by the
// baseline roadmap workstreams through CORE-02 plus the FORMAT-01 workstream.
// Future roadmap tasks update this inventory in their own plans.
func TestUnitPublicAPIBoundary(t *testing.T) {
	fset := token.NewFileSet()

	// Collect non-test .go files in the package directory.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, rerr := os.ReadFile(filepath.Join(".", name))
		if rerr != nil {
			t.Fatalf("read %s: %v", name, rerr)
		}
		f, perr := parser.ParseFile(fset, name, src, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", name, perr)
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		t.Fatal("no non-test package files found")
	}

	// Inventory collectors.
	consts := map[string]bool{}
	vars := map[string]bool{}
	types := map[string]bool{}
	funcs := map[string]bool{}
	typeMethods := map[string]map[string]bool{} // type -> method set
	typeFields := map[string]map[string]bool{}  // type -> field set

	for _, f := range files {
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if !name.IsExported() {
								continue
							}
							switch d.Tok {
							case token.CONST:
								consts[name.Name] = true
							case token.VAR:
								vars[name.Name] = true
							}
						}
					case *ast.TypeSpec:
						if !s.Name.IsExported() {
							continue
						}
						types[s.Name.Name] = true
						typeMethods[s.Name.Name] = map[string]bool{}
						typeFields[s.Name.Name] = map[string]bool{}
						if st, ok := s.Type.(*ast.StructType); ok {
							for _, field := range st.Fields.List {
								for _, n := range field.Names {
									if n.IsExported() {
										typeFields[s.Name.Name][n.Name] = true
									}
								}
							}
						}
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil {
					if d.Name.IsExported() {
						funcs[d.Name.Name] = true
					}
					continue
				}
				recvTypeName := recvBaseName(d.Recv)
				if recvTypeName == "" || !types[recvTypeName] {
					continue
				}
				if !d.Name.IsExported() {
					continue
				}
				if typeMethods[recvTypeName] == nil {
					typeMethods[recvTypeName] = map[string]bool{}
				}
				typeMethods[recvTypeName][d.Name.Name] = true
			}
		}
	}

	// Required inventory.
	wantConsts := []string{"FormatPNG", "FormatWEBP", "FormatBMP", "FormatETC2", "FormatGIF", "FormatHNSM", "FormatJPEG", "FormatNIE", "FormatNPBM", "FormatQOI", "FormatTH", "FormatTGA", "FormatWBMP"}
	wantVars := []string{"ErrBadImage", "ErrDecode", "ErrDstTooSmall", "ErrSrcTooLarge", "ErrUnknownFormat"}
	wantTypes := []string{"Decoder", "DstTooSmallError", "Meta"}
	wantFuncs := []string{"New", "Probe", "Decode", "DecodeConfig", "DecodeGray", "DecodeNRGBA", "DecodeReader", "DecodeConfigReader"}
	wantMethods := map[string][]string{
		"Decoder":          {"DecodeGray", "DecodeNRGBA", "DecodeRGBA", "Probe", "Reserve", "Version", "VersionNum"},
		"DstTooSmallError": {"Error", "Is"},
	}
	wantFields := map[string][]string{
		"Meta":             {"Err", "Width", "Height", "Stride", "BytesWritten", "Format"},
		"DstTooSmallError": {"MinBytes", "Width", "Height", "Stride"},
		"Decoder":          {}, // none exported
	}

	// Report MISSING and UNEXPECTED clearly.
	var missing, unexpected []string

	for _, name := range wantConsts {
		if !consts[name] {
			missing = append(missing, "constant: "+name)
		}
	}
	for _, name := range keys(consts) {
		if !contains(wantConsts, name) {
			unexpected = append(unexpected, "constant: "+name)
		}
	}
	for _, name := range wantVars {
		if !vars[name] {
			missing = append(missing, "var: "+name)
		}
	}
	for _, name := range keys(vars) {
		if !contains(wantVars, name) {
			unexpected = append(unexpected, "var: "+name)
		}
	}
	for _, name := range wantTypes {
		if !types[name] {
			missing = append(missing, "type: "+name)
		}
	}
	for _, name := range keys(types) {
		if !contains(wantTypes, name) {
			unexpected = append(unexpected, "type: "+name)
		}
	}
	for _, name := range wantFuncs {
		if !funcs[name] {
			missing = append(missing, "func: "+name)
		}
	}
	for _, name := range keys(funcs) {
		if !contains(wantFuncs, name) {
			unexpected = append(unexpected, "func: "+name)
		}
	}
	for typ, want := range wantMethods {
		for _, name := range want {
			if !typeMethods[typ][name] {
				missing = append(missing, "method "+typ+"."+name)
			}
		}
		for _, name := range keys(typeMethods[typ]) {
			if !contains(want, name) {
				unexpected = append(unexpected, "method "+typ+"."+name)
			}
		}
	}
	for typ, want := range wantFields {
		for _, name := range want {
			if !typeFields[typ][name] {
				missing = append(missing, "field "+typ+"."+name)
			}
		}
		for _, name := range keys(typeFields[typ]) {
			if !contains(want, name) {
				unexpected = append(unexpected, "field "+typ+"."+name)
			}
		}
	}

	sort.Strings(missing)
	sort.Strings(unexpected)

	if len(missing) > 0 {
		t.Errorf("MISSING public declarations:\n%s", joinLines(missing))
	}
	if len(unexpected) > 0 {
		t.Errorf("UNEXPECTED public declarations (outside CORE-02 inventory):\n%s", joinLines(unexpected))
	}
}

// TestUnitNoImplicitRegistration keeps reader adapters independent of the
// standard library's global image decoder registry. The check resolves uses
// through go/types, so aliases, dot imports, function values, and indirection
// are covered without matching source text.
func TestUnitNoImplicitRegistration(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var files []*ast.File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, readErr := os.ReadFile(e.Name())
		if readErr != nil {
			t.Fatalf("read %s: %v", e.Name(), readErr)
		}
		f, parseErr := parser.ParseFile(fset, e.Name(), src, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", e.Name(), parseErr)
		}
		files = append(files, f)
	}
	if got := registrationRefs(t, fset, files); len(got) != 0 {
		t.Errorf("production source references image.RegisterFormat: %v", got)
	}

	tests := []struct {
		name string
		src  string
		bad  bool
	}{
		{name: "ordinary image use", src: `package p; import "image"; var _ = image.NewRGBA`, bad: false},
		{name: "self registering import", src: `package p; import _ "image/png"`, bad: true},
		{name: "direct selector", src: `package p; import "image"; func f(){ image.RegisterFormat("x", "x", nil, nil) }`, bad: true},
		{name: "aliased selector", src: `package p; import im "image"; var _ = im.RegisterFormat`, bad: true},
		{name: "dot import", src: `package p; import . "image"; var _ = RegisterFormat`, bad: true},
		{name: "function value", src: `package p; import "image"; var register = image.RegisterFormat`, bad: true},
		{name: "indirect call", src: `package p; import "image"; func f(){ register := image.RegisterFormat; register("x", "x", nil, nil) }`, bad: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			syntheticSet := token.NewFileSet()
			f, parseErr := parser.ParseFile(syntheticSet, "snippet.go", tc.src, 0)
			if parseErr != nil {
				t.Fatalf("parse snippet: %v", parseErr)
			}
			got := registrationRefs(t, syntheticSet, []*ast.File{f})
			if (len(got) != 0) != tc.bad {
				t.Errorf("registration references = %v, want bad=%t", got, tc.bad)
			}
		})
	}
}

func registrationRefs(t *testing.T, fset *token.FileSet, files []*ast.File) []string {
	t.Helper()
	imports := map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true}
	var refs []string
	for _, f := range files {
		for _, imp := range f.Imports {
			path, unquoteErr := strconv.Unquote(imp.Path.Value)
			if unquoteErr == nil && imports[path] {
				refs = append(refs, "import "+path)
			}
		}
	}
	info := &types.Info{Uses: make(map[*ast.Ident]types.Object)}
	conf := types.Config{Importer: importer.Default(), Error: func(error) {}}
	_, _ = conf.Check("synthetic", fset, files, info)
	// The production package is expected to type-check. Synthetic snippets
	// may intentionally omit irrelevant details; resolved uses remain valid.
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			obj := info.Uses[id]
			if fn, ok := obj.(*types.Func); ok && fn.Name() == "RegisterFormat" && fn.Pkg() != nil && fn.Pkg().Path() == "image" {
				refs = append(refs, "reference image.RegisterFormat")
			}
			return true
		})
	}
	return refs
}

// recvBaseName returns the base type name of a method receiver, stripping any
// pointer or array indirection.
func recvBaseName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	return typeBaseName(recv.List[0].Type)
}

// typeBaseName returns the identifier name of a (possibly pointer) type
// expression.
func typeBaseName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return typeBaseName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return typeBaseName(t.Sel)
	case *ast.ArrayType:
		return typeBaseName(t.Elt)
	case *ast.IndexExpr:
		return typeBaseName(t.X)
	}
	return ""
}

// keys returns the sorted keys of a string set.
func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// contains reports whether slice contains s.
func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// joinLines joins strings with a leading "  - " bullet per line.
func joinLines(lines []string) string {
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("  - ")
		b.WriteString(l)
	}
	return b.String()
}
