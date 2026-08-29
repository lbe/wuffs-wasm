package wuffs

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

// TestProbeHonorsReservedSourceCapacity encodes the acceptance criteria for
// the Probe source-capacity contract:
//
//   - Probe must not size slots internally: a go/parser inspection of
//     decoder.go must find no call to (*Decoder).Reserve within the
//     (*Decoder).Probe function body.
//   - After Reserve grows the src slot beyond the default 64 KiB, a source
//     extended to exactly the reserved SrcLen probes successfully with correct
//     metadata and BytesWritten == 0, while the same source extended to
//     SrcLen+1 returns ErrSrcTooLarge before invoking the guest.
//   - The captured wasm memory backing pointer (unsafe.SliceData), wasm byte
//     length, and complete slotLayout stay identical around both calls,
//     including the destination-slot capacity.
func TestProbeHonorsReservedSourceCapacity(t *testing.T) {
	// --- AST contract: Probe must not call (*Decoder).Reserve ---
	t.Run("Probe contains no call to (*Decoder).Reserve", func(t *testing.T) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "decoder.go", nil, 0)
		if err != nil {
			t.Fatalf("parse decoder.go: %v", err)
		}

		// Locate the (*Decoder).Probe function declaration.
		var probeBody *ast.BlockStmt
		ast.Inspect(f, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok {
				if fn.Recv != nil && len(fn.Recv.List) == 1 &&
					fn.Name.Name == "Probe" {
					if star, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
						if id, ok := star.X.(*ast.Ident); ok && id.Name == "Decoder" {
							probeBody = fn.Body
						}
					}
				}
			}
			return true
		})
		if probeBody == nil {
			t.Fatalf("(*Decoder).Probe not found in decoder.go")
		}

		// Reject any d.Reserve( call expression within Probe's body.
		ast.Inspect(probeBody, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if x, ok := sel.X.(*ast.Ident); ok &&
						x.Name == "d" && sel.Sel.Name == "Reserve" {
						t.Errorf("(*Decoder).Probe must not call (*Decoder).Reserve, found d.Reserve( at %s",
							fset.Position(call.Pos()))
					}
				}
			}
			return true
		})
	})

	// The remaining behavioral assertions use a reserved src slot larger than
	// the default 64 KiB and a format fixture that stays valid with trailing
	// bytes (a PNG with trailing zeros still sniffs as a valid 160×120 image).
	const reserved = 256 * 1024 // 256 KiB, well above defaultSrcCap (64 KiB)
	base := mustReadFixture(t, "bricks-color.png")

	// --- Success: source exactly at the reserved SrcLen probes cleanly ---
	t.Run("reserved-length source probes with zero bytes written", func(t *testing.T) {
		d := New()
		if err := d.Reserve(0, reserved); err != nil {
			t.Fatalf("Reserve(0,%d): %v", reserved, err)
		}
		if got := d.currentLayout.srcLen; got != uint32(reserved) {
			t.Fatalf("reserved src slot SrcLen = %d, want %d", got, reserved)
		}

		// Exactly the reserved SrcLen: valid PNG prefix plus trailing zeros.
		src := make([]byte, reserved)
		copy(src, base)

		preSlice := d.module.Xmemory().Slice()
		prePtr := unsafe.Pointer(unsafe.SliceData(*preSlice))
		preLen := len(*preSlice)
		preLayout := d.currentLayout

		meta, err := d.Probe(src)
		if err != nil {
			t.Fatalf("Probe(reserved source) error = %v, want nil", err)
		}
		if meta == nil {
			t.Fatal("Probe(reserved source) returned nil Meta, want non-nil")
		}
		if meta.BytesWritten != 0 {
			t.Errorf("Probe(reserved source) BytesWritten = %d, want 0", meta.BytesWritten)
		}
		// Metadata must reflect the decoded PNG (bricks-color.png: 160×120, stride 640).
		if meta.Width != 160 || meta.Height != 120 || meta.Stride != 640 {
			t.Errorf("Probe(reserved source) Meta = {W:%d H:%d S:%d}, want {W:160 H:120 S:640}",
				meta.Width, meta.Height, meta.Stride)
		}

		// wasm memory identity must be preserved: Probe must not size slots
		// internally and no guest must grow memory on the probe path.
		postSlice := d.module.Xmemory().Slice()
		postPtr := unsafe.Pointer(unsafe.SliceData(*postSlice))
		postLen := len(*postSlice)
		postLayout := d.currentLayout
		if postLayout != preLayout {
			t.Errorf("slotLayout changed after reserved Probe:\n pre = %+v\n post = %+v", preLayout, postLayout)
		}
		if postLen != preLen {
			t.Errorf("wasm byte length after reserved Probe = %d, want %d", postLen, preLen)
		}
		if postPtr != prePtr {
			t.Errorf("wasm backing pointer after reserved Probe = %p, want %p", postPtr, prePtr)
		}
	})

	// --- Failure: source at SrcLen+1 is rejected before the guest ---
	t.Run("reserved-length plus one returns ErrSrcTooLarge before guest", func(t *testing.T) {
		d := New()
		if err := d.Reserve(0, reserved); err != nil {
			t.Fatalf("Reserve(0,%d): %v", reserved, err)
		}

		src := make([]byte, reserved+1)
		copy(src, base)

		preSlice := d.module.Xmemory().Slice()
		prePtr := unsafe.Pointer(unsafe.SliceData(*preSlice))
		preLen := len(*preSlice)
		preLayout := d.currentLayout

		meta, err := d.Probe(src)
		if !errors.Is(err, ErrSrcTooLarge) {
			t.Fatalf("Probe(reserved+1) error = %v, want errors.Is(err, ErrSrcTooLarge)", err)
		}
		if meta != nil {
			t.Errorf("Probe(reserved+1) Meta = %v, want nil", meta)
		}

		// The rejected call must not touch wasm memory or slot layout.
		postSlice := d.module.Xmemory().Slice()
		postPtr := unsafe.Pointer(unsafe.SliceData(*postSlice))
		postLen := len(*postSlice)
		postLayout := d.currentLayout
		if postLayout != preLayout {
			t.Errorf("slotLayout changed after rejected Probe:\n pre = %+v\n post = %+v", preLayout, postLayout)
		}
		if postLen != preLen {
			t.Errorf("wasm byte length after rejected Probe = %d, want %d", postLen, preLen)
		}
		if postPtr != prePtr {
			t.Errorf("wasm backing pointer after rejected Probe = %p, want %p", postPtr, prePtr)
		}
	})
}

// mustReadFixture reads a file from the testdata directory and fails the test on error.
func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading testdata/%s: %v", name, err)
	}
	return data
}
