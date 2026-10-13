# FUZZ-01 — Native Go fuzzers (host invariants)

**Execution protocol:** `plans/pi-runbook.md`

**Execution branch:** `fuzz-01-native-fuzzers`

**Progress file:** `tmp/pi_progress.md`

**Commit authorized:** Yes — one reviewed commit per committing task

**Roadmap:** None — maintainer/hygiene only; do **not** edit `plans/api-roadmap.md`.

**Purpose:** Add native `testing.F` fuzzers that hunt **host** bugs (panic, wasm
OOB, `Pix` aliasing, implicit `memory.Grow`, non-public errors). They are not a
search for Wuffs decoder defects. Seed-only execution rides along with
`go test` / CI. Mutation (`-fuzz`) is local or nightly; it is **not** a PR CI
gate.

This plan does **not** add oss-fuzz, DecodeNRGBA fuzzers, allocating
`Decode`/`DecodeReader` fuzzers, concurrent use of one `Decoder`, guest/shim
changes, or public API.

---

## Sources of truth

Read before acting:

1. `AGENTS.md`
2. The user's latest instruction
3. `plans/pi-runbook.md`
4. This plan
5. `API.md` (error sentinels, caller-owned `Pix`, explicit `Reserve`)
6. `export_test.go` (`CaptureGuestMemoryState`, `ShrunkMaxSrc`)

If sources conflict, stop and report the conflict.

---

## Product invariants (must hold after every task)

- No CGO. No edits to `wasm/shim.c`, `wasm/wuffs.wasm`, or
  `internal/wuffswasm/wuffs.go`.
- No public API additions. Fuzzers live in `package wuffs_test`.
- Do not call `Reserve` inside a fuzz iteration. Default slots only:
  src ≤ 64 KiB (`ShrunkMaxSrc()`), dst 128 KiB → skip decode when
  `Width*Height > 32768`.
- `New()` once per fuzz iteration. Do not reuse a `Decoder` across inputs
  (`Reserve` is monotonic; reuse would grow toward 256 MiB).
- Do **not** add `go test -fuzz` to `.github/workflows/ci.yml`. Seed mode is
  enough: `go test` already executes `Fuzz*` functions with `f.Add` values.
- Do not commit `testdata/fuzz/` (generated corpus). Do not `git add -f`.
- If a fuzzer finds a crash: **STOP**. Do not weaken assertions. Do not
  convert the crasher into an expected-failure. Report the input.

---

## Target fuzzers

| Name              | Input                     | After size gates                                       |
| ----------------- | ------------------------- | ------------------------------------------------------ |
| `FuzzDecodeRGBA`  | `src []byte`              | `Probe` then `DecodeRGBA` into a tight `image.RGBA`    |
| `FuzzDecodeGray`  | `src []byte`              | `Probe` then `DecodeGray` into a tight `image.Gray`    |
| `FuzzDecodeFrame` | `src []byte`, `index int` | `Probe` then `DecodeFrame` (index may be out of range) |
| `FuzzMetadata`    | `src []byte`              | `Metadata` only                                        |

Shared properties on every call:

1. No panic.
2. Any error is `errors.Is` one of: `ErrUnknownFormat`, `ErrSrcTooLarge`,
   `ErrDecode`, `ErrBadImage`, `ErrDstTooSmall`.
3. `CaptureGuestMemoryState` is unchanged (no grow, no relocate, no slot
   layout change).
4. For decode methods: `Pix` pointer/len/cap, `Rect`, and `Stride` are
   unchanged. On error, `Pix` **bytes** are also unchanged.

---

## Seed corpus

`seedFuzzSources` walks `testdata/` and `f.Add`s every file whose extension is
in the set below, length in `(0, ShrunkMaxSrc()]`. Skip directories `bench`
and `fuzz`. Also `f.Add` empty, `{0x00}`, `"XXXX"`, and a RIFF/WEBP-shaped
junk header.

Extensions: `.png`, `.webp`, `.gif`, `.jpeg`, `.jpg`, `.bmp`, `.qoi`, `.tga`,
`.pgm`, `.ppm`, `.wbmp`, `.pkm`, `.nie`, `.nia`, `.th`, `.handsum`.

If fewer than 10 files are added from the walk, `f.Fatal` (fixtures missing).

---

## Execution control

- Use only Pi subagents `worker` and `reviewer` per `plans/pi-runbook.md`.
- Branch `fuzz-01-native-fuzzers`: create with
  `git switch -c fuzz-01-native-fuzzers` only when on `main`, branch absent,
  working tree clean.
- Each committing task: worker implements → Verify → `tmp/commit_message.txt` →
  reviewer → `git commit -F tmp/commit_message.txt` → append hash to
  `tmp/pi_progress.md`.

---

## Executor STOP gates (mandatory)

On any trigger below (or `plans/pi-runbook.md` standing constraint 13): **STOP**,
message the user (task id, evidence), **no commit**, **no next task** until written
user direction.

| # | Trigger                                                                | Executor action |
| - | ---------------------------------------------------------------------- | --------------- |
| 1 | Working tree not clean at preflight                                    | STOP            |
| 2 | Plan step contradicts runtime or tests                                 | STOP            |
| 3 | Verify fails for reason not explained by current task                  | STOP            |
| 4 | Edit outside task allowed files                                        | STOP            |
| 5 | Fuzzer crash, hang, or unexpected error type (not a public sentinel)   | STOP            |
| 6 | Worker ran `go test -fuzz` on a non-final task, or added `-fuzz` to CI | STOP            |
| 7 | Worker staged `testdata/fuzz/` or used `git add -f`                    | STOP            |

---

## Preflight

Branch hygiene only.

### Worker instructions

1. If `git status --short` is non-empty → STOP.
2. If current branch is `main` → `git switch -c fuzz-01-native-fuzzers`.
3. If current branch is `fuzz-01-native-fuzzers` → continue.
4. If current branch is any other name → STOP.
5. Create `tmp/pi_progress.md` with `BASELINE_HEAD=$(git rev-parse HEAD)`.

### Allowed files

- `tmp/pi_progress.md`
- `tmp/fuzz-01-native-fuzzers/` (logs — do not stage)

### Verify

```bash
git branch --show-current
git status --short
git diff --name-only
test -f plans/fuzz-01-native-fuzzers.md
```

Expected branch: `fuzz-01-native-fuzzers`. Expected `git status --short`: empty.

Commit authorized: **no**.

---

## Task 1 — Ignore generated fuzz corpus

**Prerequisite:** Preflight complete; `plans/fuzz-01-native-fuzzers.md` present
on the branch.

### Allowed files

- `.gitignore`

### Worker instructions

Append exactly these lines at the end of `.gitignore` (blank line before the
comment, matching the BENCH-02 block style):

```gitignore
# FUZZ-01 generated Go fuzz corpus (never commit)
testdata/fuzz/
```

Do not gitignore `fuzz_test.go`.

### Verify

```bash
grep -F 'testdata/fuzz/' .gitignore
make format-check
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`, `go test -fuzz`.

Commit authorized: **yes**.

Commit message:

```text
chore: gitignore generated Go fuzz corpus
```

---

## Task 2 — `FuzzDecodeRGBA` and shared helpers

**Prerequisite:** Task 1 committed.

### Allowed files

- `fuzz_test.go` (new, `package wuffs_test`)

### Worker instructions

Create `fuzz_test.go` with **exactly** this behavior (identifiers and constants
fixed; comments may be tightened to match nearby test style, not removed):

```go
package wuffs_test

import (
	"bytes"
	"errors"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"github.com/lbe/wuffs-wasm"
)

const fuzzMaxPixels = 32768

var fuzzSeedExt = map[string]bool{
	".png": true, ".webp": true, ".gif": true, ".jpeg": true, ".jpg": true,
	".bmp": true, ".qoi": true, ".tga": true, ".pgm": true, ".ppm": true,
	".wbmp": true, ".pkm": true, ".nie": true, ".nia": true, ".th": true,
	".handsum": true,
}

func isPublicErr(err error) bool {
	return err != nil && (errors.Is(err, wuffs.ErrUnknownFormat) ||
		errors.Is(err, wuffs.ErrSrcTooLarge) ||
		errors.Is(err, wuffs.ErrDecode) ||
		errors.Is(err, wuffs.ErrBadImage) ||
		errors.Is(err, wuffs.ErrDstTooSmall))
}

func loadFuzzSeeds(f *testing.F) [][]byte {
	seeds := [][]byte{
		{},
		{0x00},
		[]byte("XXXX"),
		[]byte("RIFF\x00\x00\x00\x00WEBP"),
	}
	added := 0
	err := filepath.WalkDir("testdata", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case "bench", "fuzz":
				return fs.SkipDir
			default:
				return nil
			}
		}
		if !fuzzSeedExt[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if len(b) == 0 || len(b) > wuffs.ShrunkMaxSrc() {
			return nil
		}
		seeds = append(seeds, b)
		added++
		return nil
	})
	if err != nil {
		f.Fatal(err)
	}
	if added < 10 {
		f.Fatalf("loadFuzzSeeds added %d testdata files, want at least 10", added)
	}
	return seeds
}

func seedFuzzSources(f *testing.F) {
	for _, src := range loadFuzzSeeds(f) {
		f.Add(src)
	}
}

func assertGuestUnchanged(t *testing.T, d *wuffs.Decoder, before wuffs.GuestMemoryState, where string) {
	t.Helper()
	if got := wuffs.CaptureGuestMemoryState(d); got != before {
		t.Fatalf("%s mutated guest memory layout: got %+v want %+v", where, got, before)
	}
}

func FuzzDecodeRGBA(f *testing.F) {
	seedFuzzSources(f)
	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) > wuffs.ShrunkMaxSrc() {
			return
		}
		d := wuffs.New()
		pre := wuffs.CaptureGuestMemoryState(d)
		meta, err := d.Probe(src)
		if err != nil {
			if !isPublicErr(err) {
				t.Fatalf("Probe: unexpected error %v", err)
			}
			assertGuestUnchanged(t, d, pre, "Probe")
			dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
			fuzzDecodeRGBA(t, d, dst, src)
			return
		}
		if meta == nil {
			t.Fatal("Probe returned nil Meta without error")
		}
		assertGuestUnchanged(t, d, pre, "Probe")
		w, h := int(meta.Width), int(meta.Height)
		if w <= 0 || h <= 0 || uint64(w)*uint64(h) > fuzzMaxPixels {
			return
		}
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		fuzzDecodeRGBA(t, d, dst, src)
	})
}

func fuzzDecodeRGBA(t *testing.T, d *wuffs.Decoder, dst *image.RGBA, src []byte) {
	t.Helper()
	pre := wuffs.CaptureGuestMemoryState(d)
	ptr := unsafe.SliceData(dst.Pix)
	ln, cp := len(dst.Pix), cap(dst.Pix)
	rect, stride := dst.Rect, dst.Stride
	pixCopy := append([]byte(nil), dst.Pix...)

	_, err := d.DecodeRGBA(dst, src)
	if err != nil && !isPublicErr(err) {
		t.Fatalf("DecodeRGBA: unexpected error %v", err)
	}
	if unsafe.SliceData(dst.Pix) != ptr || len(dst.Pix) != ln || cap(dst.Pix) != cp || dst.Rect != rect || dst.Stride != stride {
		t.Fatal("DecodeRGBA mutated dst Pix/Rect/Stride header")
	}
	if err != nil && !bytes.Equal(dst.Pix, pixCopy) {
		t.Fatal("failed DecodeRGBA mutated dst.Pix bytes")
	}
	assertGuestUnchanged(t, d, pre, "DecodeRGBA")
}
```

Do not add the other `Fuzz*` functions in this task.

### Verify

```bash
go test -run='^FuzzDecodeRGBA$' -count=1 -v .
make format-check
```

Expected: PASS. Seeds include empty/junk (Probe error path) and real fixtures
(decode success path). FORBIDDEN before commit: `make test`, `make test-race`,
`make cover`, `go test ./...`, `go test -fuzz`.

Commit authorized: **yes**.

Commit message:

```text
test(fuzz): add FuzzDecodeRGBA for host decode invariants
```

---

## Task 3 — Gray, frame, and metadata fuzzers

**Prerequisite:** Task 2 committed.

### Allowed files

- `fuzz_test.go`

### Worker instructions

Append these three fuzzers and their helpers to `fuzz_test.go`. Keep Task 2
helpers and `FuzzDecodeRGBA` unchanged.

Required function names: `FuzzDecodeGray`, `FuzzDecodeFrame`, `FuzzMetadata`,
`fuzzDecodeGray`, `fuzzDecodeFrame`.

```go
func FuzzDecodeGray(f *testing.F) {
	seedFuzzSources(f)
	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) > wuffs.ShrunkMaxSrc() {
			return
		}
		d := wuffs.New()
		pre := wuffs.CaptureGuestMemoryState(d)
		meta, err := d.Probe(src)
		if err != nil {
			if !isPublicErr(err) {
				t.Fatalf("Probe: unexpected error %v", err)
			}
			assertGuestUnchanged(t, d, pre, "Probe")
			dst := image.NewGray(image.Rect(0, 0, 1, 1))
			fuzzDecodeGray(t, d, dst, src)
			return
		}
		if meta == nil {
			t.Fatal("Probe returned nil Meta without error")
		}
		assertGuestUnchanged(t, d, pre, "Probe")
		w, h := int(meta.Width), int(meta.Height)
		if w <= 0 || h <= 0 || uint64(w)*uint64(h) > fuzzMaxPixels {
			return
		}
		dst := image.NewGray(image.Rect(0, 0, w, h))
		fuzzDecodeGray(t, d, dst, src)
	})
}

func seedFuzzSourcesIndexed(f *testing.F) {
	for _, src := range loadFuzzSeeds(f) {
		f.Add(src, 0)
	}
	f.Add([]byte("XXXX"), -1)
	f.Add([]byte("XXXX"), 1<<30)
}

func FuzzDecodeFrame(f *testing.F) {
	seedFuzzSourcesIndexed(f)
	f.Fuzz(func(t *testing.T, src []byte, index int) {
		if len(src) > wuffs.ShrunkMaxSrc() {
			return
		}
		d := wuffs.New()
		pre := wuffs.CaptureGuestMemoryState(d)
		meta, err := d.Probe(src)
		if err != nil {
			if !isPublicErr(err) {
				t.Fatalf("Probe: unexpected error %v", err)
			}
			assertGuestUnchanged(t, d, pre, "Probe")
			dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
			fuzzDecodeFrame(t, d, dst, src, index)
			return
		}
		if meta == nil {
			t.Fatal("Probe returned nil Meta without error")
		}
		assertGuestUnchanged(t, d, pre, "Probe")
		w, h := int(meta.Width), int(meta.Height)
		if w <= 0 || h <= 0 || uint64(w)*uint64(h) > fuzzMaxPixels {
			return
		}
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		fuzzDecodeFrame(t, d, dst, src, index)
	})
}
```

Gray helper (byte-identical contract, Gray types):

```go
func fuzzDecodeGray(t *testing.T, d *wuffs.Decoder, dst *image.Gray, src []byte) {
	t.Helper()
	pre := wuffs.CaptureGuestMemoryState(d)
	ptr := unsafe.SliceData(dst.Pix)
	ln, cp := len(dst.Pix), cap(dst.Pix)
	rect, stride := dst.Rect, dst.Stride
	pixCopy := append([]byte(nil), dst.Pix...)

	_, err := d.DecodeGray(dst, src)
	if err != nil && !isPublicErr(err) {
		t.Fatalf("DecodeGray: unexpected error %v", err)
	}
	if unsafe.SliceData(dst.Pix) != ptr || len(dst.Pix) != ln || cap(dst.Pix) != cp || dst.Rect != rect || dst.Stride != stride {
		t.Fatal("DecodeGray mutated dst Pix/Rect/Stride header")
	}
	if err != nil && !bytes.Equal(dst.Pix, pixCopy) {
		t.Fatal("failed DecodeGray mutated dst.Pix bytes")
	}
	assertGuestUnchanged(t, d, pre, "DecodeGray")
}
```

```go
func fuzzDecodeFrame(t *testing.T, d *wuffs.Decoder, dst *image.RGBA, src []byte, index int) {
	t.Helper()
	pre := wuffs.CaptureGuestMemoryState(d)
	ptr := unsafe.SliceData(dst.Pix)
	ln, cp := len(dst.Pix), cap(dst.Pix)
	rect, stride := dst.Rect, dst.Stride
	pixCopy := append([]byte(nil), dst.Pix...)

	_, err := d.DecodeFrame(dst, src, index)
	if err != nil && !isPublicErr(err) {
		t.Fatalf("DecodeFrame: unexpected error %v", err)
	}
	if unsafe.SliceData(dst.Pix) != ptr || len(dst.Pix) != ln || cap(dst.Pix) != cp || dst.Rect != rect || dst.Stride != stride {
		t.Fatal("DecodeFrame mutated dst Pix/Rect/Stride header")
	}
	if err != nil && !bytes.Equal(dst.Pix, pixCopy) {
		t.Fatal("failed DecodeFrame mutated dst.Pix bytes")
	}
	assertGuestUnchanged(t, d, pre, "DecodeFrame")
}
```

`FuzzMetadata`:

```go
func FuzzMetadata(f *testing.F) {
	seedFuzzSources(f)
	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) > wuffs.ShrunkMaxSrc() {
			return
		}
		d := wuffs.New()
		pre := wuffs.CaptureGuestMemoryState(d)
		_, err := d.Metadata(src)
		if err != nil && !isPublicErr(err) {
			t.Fatalf("Metadata: unexpected error %v", err)
		}
		assertGuestUnchanged(t, d, pre, "Metadata")
	})
}
```

### Verify

```bash
go test -run='^Fuzz' -count=1 .
make format-check
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`, `go test -fuzz`.

Commit authorized: **yes**.

Commit message:

```text
test(fuzz): add Gray, DecodeFrame, and Metadata fuzzers
```

---

## Task 4 — Developer docs and `make fuzz`

**Prerequisite:** Task 3 committed.

### Allowed files

- `Makefile`
- `docs/DEVELOPMENT.md`
- `testdata/README`

### Worker instructions

1. `Makefile` `help` target: add `make fuzz` after `make test-race`. Add
   `FUZZ` / `FUZZTIME` to the environment-variables list. Add:

```makefile
# Native Go fuzz (mutation). Seed-only Fuzz* tests already run under make test.
FUZZ ?= FuzzDecodeRGBA
FUZZTIME ?= 30s

.PHONY: fuzz
fuzz:
	go test -fuzz=$(FUZZ) -fuzztime=$(FUZZTIME) .
```

Place `.PHONY: fuzz` with the other phony recipes (not inside `help`).

2. `docs/DEVELOPMENT.md`: after the **Build workflow** test-suite commands,
   add a **Fuzzing** section that states:

   - Target is host invariants, not Wuffs decoder bugs.
   - `go test` / CI run `Fuzz*` in seed-only mode.
   - Mutation: `make fuzz` (defaults `FuzzDecodeRGBA`, 30s) or
     `make fuzz FUZZ=FuzzDecodeFrame FUZZTIME=2m`.
   - Names: `FuzzDecodeRGBA`, `FuzzDecodeGray`, `FuzzDecodeFrame`,
     `FuzzMetadata`.
   - Caps: no `Reserve`; src ≤ 64 KiB; skip decode above 32768 pixels.
   - Generated corpus is `testdata/fuzz/` (gitignored). Do not commit it.
   - A crash is a product bug: keep the failing input, do not weaken the
     fuzzer.

3. `testdata/README`: add a short **Fuzz corpus** note after the opening
   M/L-benchmark paragraph: `testdata/fuzz/` is created by `go test -fuzz`
   and is gitignored; do not vendor it. Do **not** change the
   thirteen-format inventory sentence (documentation-contract test).

### Verify

```bash
make help
make format-check
go test -run='^TestIntegrationVerifiedFormatDocumentationInventory$' -count=1 .
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`, `go test -fuzz`.

Commit authorized: **yes**.

Commit message:

```text
docs(fuzz): document native fuzzers and make fuzz
```

---

## Task 5 — Final verification

**Prerequisite:** Task 4 committed.

### Worker instructions

1. Run every command in **Verify**; save combined output to
   `tmp/fuzz-01-native-fuzzers/final-gates.log`.
2. Then run a bounded mutation smoke (not a pass/fail of “found zero
   interesting inputs”). Each iteration calls `wuffs.New()` (wasm guest);
   unbounded `-parallel` (default GOMAXPROCS) OOMs this process. Use four
   workers:

```bash
go test -fuzz=FuzzDecodeRGBA -fuzztime=10s -run=^$ -parallel=4 .
```

If this crashes, panics, or fails an assertion: **STOP** (gate 5). Worker
SIGKILL/EOF with a corpus input that replays PASS under
`go test -run=FuzzDecodeRGBA/<id>` is harness OOM, not a product crasher —
re-run this same `-parallel=4` command; do not treat that as gate 5.
If it exits 0 after 10s, continue.
3. Confirm `git status --porcelain --untracked-files=all` shows no
`testdata/fuzz/` staged files. Untracked `testdata/fuzz/` is expected and
must remain untracked.
4. Commit authorized: **no**.

### Verify

```bash
make format-check
make lint
make test
make test-race
go build ./...
git status --porcelain --untracked-files=all
```

FORBIDDEN before commit: N/A (no commit).

---

## Completion report

1. `git rev-parse HEAD` on `fuzz-01-native-fuzzers`.
2. Confirmation that seed-mode `go test -run='^Fuzz'` passed.
3. `final-gates.log` path.
4. Result of the 10s `FuzzDecodeRGBA` mutation smoke with `-parallel=4`
   (exit 0 or STOP).
5. Confirmation CI workflow was not edited.

---

## Execution prompt (for Cursor Agent)

```text
Execute plans/fuzz-01-native-fuzzers.md in order:
Preflight → Task 1 → Task 2 → Task 3 → Task 4 → Task 5.
Follow plans/pi-runbook.md (test budget, commits, format).
Obey this plan's ## Executor STOP gates (mandatory) and pi-runbook standing constraint 13.
Progress file: tmp/pi_progress.md.
Do not edit plans/api-roadmap.md.
Do not add go test -fuzz to CI. Run -fuzz only in Task 5, with -parallel=4.
Do not commit testdata/fuzz/. Do not git add -f.
Do not push unless I instruct.
Begin with Preflight only on a clean main; ensure this plan file is committed before Task 1.
```
