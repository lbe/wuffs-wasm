package wuffs

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"hash/crc32"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

// TestDecodeRGBAHonorsReservedSourceCapacity encodes the acceptance criteria
// for the DecodeRGBA source-capacity contract:
//
//   - DecodeRGBA must not size slots internally: a go/parser inspection of
//     decoder.go must find no call to (*Decoder).Reserve within the
//     (*Decoder).DecodeRGBA function body.
//   - After Reserve grows the src slot beyond the default 64 KiB, a source
//     extended to exactly the reserved SrcLen decodes successfully to the
//     golden pixels (CRC32 of dst.Pix matches the checked-in golden value),
//     while the same source extended to SrcLen+1 returns ErrSrcTooLarge before
//     invoking the guest.
//   - The captured wasm memory backing pointer (unsafe.SliceData), wasm byte
//     length, and complete SlotLayout stay identical around both calls,
//     including the destination-slot capacity.
//   - The successful decode must preserve the caller's dst.Pix slice identity
//     (the library never allocates or replaces Pix, nor points it at wasm
//     memory).
func TestDecodeRGBAHonorsReservedSourceCapacity(t *testing.T) {
	// --- AST contract: DecodeRGBA must not call (*Decoder).Reserve ---
	t.Run("DecodeRGBA contains no call to (*Decoder).Reserve", func(t *testing.T) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "decoder.go", nil, 0)
		if err != nil {
			t.Fatalf("parse decoder.go: %v", err)
		}

		// Locate the (*Decoder).DecodeRGBA function declaration.
		var decodeBody *ast.BlockStmt
		ast.Inspect(f, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok {
				if fn.Recv != nil && len(fn.Recv.List) == 1 &&
					fn.Name.Name == "DecodeRGBA" {
					if star, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
						if id, ok := star.X.(*ast.Ident); ok && id.Name == "Decoder" {
							decodeBody = fn.Body
						}
					}
				}
			}
			return true
		})
		if decodeBody == nil {
			t.Fatalf("(*Decoder).DecodeRGBA not found in decoder.go")
		}

		// Reject any d.Reserve( call expression within DecodeRGBA's body.
		ast.Inspect(decodeBody, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if x, ok := sel.X.(*ast.Ident); ok &&
						x.Name == "d" && sel.Sel.Name == "Reserve" {
						t.Errorf("(*Decoder).DecodeRGBA must not call (*Decoder).Reserve, found d.Reserve( at %s",
							fset.Position(call.Pos()))
					}
				}
			}
			return true
		})
	})

	// The remaining behavioral assertions use a reserved src slot larger than
	// the default 64 KiB and a format fixture that stays valid with trailing
	// bytes (a PNG with trailing zeros still decodes as a valid 160×120 image).
	const reserved = 256 * 1024 // 256 KiB, well above defaultSrcCap (64 KiB)
	base := mustReadFixture(t, "bricks-color.png")
	wantCRC := readGoldenCRC32(t, "bricks-color.golden.crc32")

	// --- Success: source exactly at the reserved SrcLen decodes to golden ---
	t.Run("reserved-length source decodes to golden pixels", func(t *testing.T) {
		d := New()
		if err := d.Reserve(0, reserved); err != nil {
			t.Fatalf("Reserve(0,%d): %v", reserved, err)
		}
		if got := d.MemoryLayout().SrcLen; got != uint32(reserved) {
			t.Fatalf("reserved src slot SrcLen = %d, want %d", got, reserved)
		}

		// Exactly the reserved SrcLen: valid PNG prefix plus trailing zeros.
		src := make([]byte, reserved)
		copy(src, base)

		dst := image.NewRGBA(image.Rect(0, 0, 160, 120))
		prePixPtr := unsafe.SliceData(dst.Pix)

		pre := captureMemState(t, d)

		meta, err := d.DecodeRGBA(dst, src)
		if err != nil {
			t.Fatalf("DecodeRGBA(reserved source) error = %v, want nil", err)
		}
		if meta == nil {
			t.Fatal("DecodeRGBA(reserved source) returned nil Meta, want non-nil")
		}
		if meta.Width != 160 || meta.Height != 120 || meta.Stride != 640 {
			t.Errorf("DecodeRGBA(reserved source) Meta = {W:%d H:%d S:%d}, want {W:160 H:120 S:640}",
				meta.Width, meta.Height, meta.Stride)
		}

		// Pixels must match the checked-in golden CRC32.
		gotCRC := crc32.ChecksumIEEE(dst.Pix)
		if gotCRC != wantCRC {
			t.Errorf("CRC32 of decoded Pix = 0x%08X, want 0x%08X", gotCRC, wantCRC)
		}

		// The caller's dst.Pix slice must be preserved (identity + length).
		postPixPtr := unsafe.SliceData(dst.Pix)
		if postPixPtr != prePixPtr {
			t.Errorf("dst.Pix backing pointer changed: pre=%p post=%p", prePixPtr, postPixPtr)
		}
		if len(dst.Pix) != 160*120*4 {
			t.Errorf("dst.Pix length = %d, want %d", len(dst.Pix), 160*120*4)
		}

		// wasm memory identity must be preserved: DecodeRGBA must not size
		// slots internally and the reserved layout must survive the call.
		assertMemUnchanged(t, d, pre, "reserved")
	})

	// --- Failure: source at SrcLen+1 is rejected before the guest ---
	t.Run("reserved-length plus one returns ErrSrcTooLarge before guest", func(t *testing.T) {
		d := New()
		if err := d.Reserve(0, reserved); err != nil {
			t.Fatalf("Reserve(0,%d): %v", reserved, err)
		}

		src := make([]byte, reserved+1)
		copy(src, base)

		dst := image.NewRGBA(image.Rect(0, 0, 160, 120))
		prePix := append([]byte(nil), dst.Pix...)

		pre := captureMemState(t, d)

		meta, err := d.DecodeRGBA(dst, src)
		if !errors.Is(err, ErrSrcTooLarge) {
			t.Fatalf("DecodeRGBA(reserved+1) error = %v, want errors.Is(err, ErrSrcTooLarge)", err)
		}
		if meta != nil {
			t.Errorf("DecodeRGBA(reserved+1) Meta = %v, want nil", meta)
		}

		// The rejected call must not touch wasm memory, slot layout, or the
		// caller's pixels.
		if !bytes.Equal(prePix, dst.Pix) {
			t.Errorf("dst.Pix mutated by rejected DecodeRGBA:\n pre = %v\n post = %v", prePix, dst.Pix)
		}

		assertMemUnchanged(t, d, pre, "rejected")
	})
}

// memState captures the caller-visible wasm memory identity immediately
// before a DecodeRGBA call so a test can assert it was not disturbed.
type memState struct {
	ptr    unsafe.Pointer
	length int
	layout SlotLayout
}

// captureMemState records the current wasm backing pointer, byte length, and
// slot layout for later comparison in assertMemUnchanged.
func captureMemState(t *testing.T, d *Decoder) memState {
	t.Helper()
	slice := d.module.Xmemory().Slice()
	return memState{
		ptr:    unsafe.Pointer(unsafe.SliceData(*slice)),
		length: len(*slice),
		layout: d.currentLayout,
	}
}

// assertMemUnchanged fails if the wasm backing pointer, byte length, or slot
// layout differ from the captured memState, confirming the call did not resize
// slots internally. tag labels the failure (e.g. "reserved" or "rejected").
func assertMemUnchanged(t *testing.T, d *Decoder, pre memState, tag string) {
	t.Helper()
	post := captureMemState(t, d)
	if post.layout != pre.layout {
		t.Errorf("SlotLayout changed after %s DecodeRGBA:\n pre = %+v\n post = %+v", tag, pre.layout, post.layout)
	}
	if post.length != pre.length {
		t.Errorf("wasm byte length after %s DecodeRGBA = %d, want %d", tag, post.length, pre.length)
	}
	if post.ptr != pre.ptr {
		t.Errorf("wasm backing pointer after %s DecodeRGBA = %p, want %p", tag, post.ptr, pre.ptr)
	}
}

// readGoldenCRC32 reads the checked-in golden CRC32 value from testdata.
func readGoldenCRC32(t *testing.T, name string) uint32 {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading testdata/%s: %v", name, err)
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 32)
	if err != nil {
		t.Fatalf("parsing golden CRC32 from testdata/%s: %v", name, err)
	}
	return uint32(v)
}
