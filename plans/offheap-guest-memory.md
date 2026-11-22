# Off-Heap Guest Memory

## Objective

Move each `Decoder`'s wasm guest linear memory off the Go heap so that address
space is reserved once, pages become resident only when written, and growth
never copies. Add `(*Decoder).Close` to free that memory at once and
`(*Decoder).Trim` to return pages after a large decode while keeping the
decoder.

Source: `/tmp/wuffs-wasm-handover.md` (sfpg-go handover against v0.5.0,
`b457af7`), Option A. In v0.5.0 the generated `memory_grow` grows the guest
memory with `append`, so a grown decoder is resident in full: 8 decoders after
1024×1024 PNG decodes hold 434 MiB RSS (87 MiB in v0.4.0).

## Fixed decisions

- Option A: wuffs-wasm maps the guest memory itself. Option B (caller-supplied
  memory) and pre-sizing a Go slice are rejected.
- Reserve 256 MiB of address space per decoder (the module maximum). This
  raises virtual size only; RSS rises only for pages written.
- `Close() error` matches `io.Closer`: a second `Close` is a no-op returning
  nil. After `Close`, every method with an error result returns `ErrClosed`.
  `Version()` and `VersionNum()` keep working.
- The next release is v0.6.0. Do not tag until the user instructs it.

## Constraints

- Never hand-edit `internal/wuffswasm/wuffs.go`. Change it only through
  `wasm/build.sh` linker switches or wasm2go switches, then regenerate.
- Regenerate with wasm2go c28cf76 only:
  `go run github.com/ncruces/wasm2go@v0.0.0-20261009170336-c28cf769d2bf -unsafe -pkg wuffswasm -o internal/wuffswasm/wuffs.go wasm/wuffs.wasm`.
  Do not use the `wasm2go` on `PATH` (v0.4.22) and do not upgrade the
  generator.
- Touch nothing outside this repository.
- No commit attribution trailer. No push, merge, or tag.

## Execution and cost

Claude executes this plan directly (not the pi-runbook process). To save
quota, work that is mechanical or well specified goes to Haiku subagents;
work where a mistake corrupts memory or breaks the platform contract stays
with the main model.

| Task                                | Model | Reason                                                  |
| ----------------------------------- | ----- | ------------------------------------------------------- |
| 1 Branch                            | main  | One command                                             |
| 2 Import memory, guard, regenerate  | main  | Build/generator correctness; STOP gates 1, 2, 4         |
| 3 Mapped memory, per-platform files | main  | `unsafe`, syscalls, slice aliasing                      |
| 3 Replace 16 `Xmemory()` host sites | Haiku | Mechanical once the `Decoder` field exists              |
| 4 `Close`, `ErrClosed`, cleanup     | main  | Lifecycle and cleanup semantics                         |
| 4 `ErrClosed` guards on each method | Haiku | Same two-line guard per method, given the pattern       |
| 5 `Trim`                            | main  | Releases guest pages; layout safety                     |
| 6 Close/ErrClosed unit tests        | Haiku | Table-driven over methods, given the contract           |
| 6 Trim round-trip tests per format  | Haiku | Repetitive over fixtures, given one worked example      |
| 6 RSS tests, cross builds           | main  | Thresholds and measurement design                       |
| 7 Shim hardening (C changes)        | main  | Memory-safety code in the guest                         |
| 8 C lint in `make lint`             | main  | Tool config, CI, and suppression decisions              |
| 9 Docs                              | Haiku | Given the exact API and facts to state                  |
| 10 Run verification suite           | Haiku | Runs the commands, reports exit codes and failures only |
| Review of every Haiku result        | main  | Targeted commands, not full diffs or transcripts        |

Rules for Haiku work:

- Each prompt is self-contained: exact files it may edit, the pattern or
  worked example, the hard rules (no generated-code edits, nothing outside
  the repo, no git state changes, no Python, no weakened or skipped tests),
  and the report format (files changed, commands run with exit codes,
  explicit uncertainties).
- Haiku runs the relevant checks itself before reporting.
- The main model accepts a Haiku result only after re-running the checks
  that Haiku cannot be trusted to cover (benchmarks, cross builds,
  `make lint`) and grepping its diff for removed assertions.
- The main model reads only targeted excerpts: grep results, failing-test
  lines, and the specific hunks under review.

## Task 1 — Branch

Create `feat/offheap-memory` from `main` with a clean working tree. This plan
file is committed with the implementation.

## Task 2 — Import the guest memory

1. Add `-Wl,--import-memory` to `LDFLAGS` in `wasm/build.sh`. Keep
   `--initial-memory=33554432` and `--max-memory=268435456`.
2. Add a guard that static data ends below `bump_base` (`0x900000`), so `Trim`
   can never release it: compare `__heap_base` against `bump_base` in the shim
   (a startup check that fails every export) or at link time.
3. Rebuild with `bash wasm/build.sh` and regenerate with the c28cf76 command
   above. Expected generated shape: `Module` gains `memImp Memory`; `New` takes
   a host value whose `Xmemory() Memory` supplies the memory; guest
   `memory.grow` calls `memImp.Grow`.

## Task 3 — Mapped memory implementation (non-generated files)

A type implementing the generated `Memory` interface
(`Slice() *[]byte`, `Grow(delta, max int64) int64`), one file per platform:

| Platform       | Reserve                                                       | Grow                                       | Release                    |
| -------------- | ------------------------------------------------------------- | ------------------------------------------ | -------------------------- |
| 64-bit unix    | `unix.Mmap(-1, 0, 256 MiB, PROT_NONE, MAP_PRIVATE\|MAP_ANON)` | `unix.Mprotect` the next range RW, reslice | `unix.Munmap`              |
| 64-bit Windows | `VirtualAlloc(MEM_RESERVE)`                                   | `VirtualAlloc(MEM_COMMIT)`, `unsafe.Slice` | `VirtualFree(MEM_RELEASE)` |
| 32-bit, other  | none                                                          | `append` on a Go slice                     | drop the slice             |

- `New` grows the memory to 32 MiB before calling the generated `New`, which
  writes static data segments at about 8 MiB.
- `Decoder` holds its `Memory` and uses it at all host sites that call
  `d.module.Xmemory()` (16 in non-test files), plus the test hooks.
- `golang.org/x/sys` moves from indirect to direct in `go.mod`.

## Task 4 — Close

- `func (d *Decoder) Close() error` releases the memory and marks the decoder
  closed. A second call returns nil.
- Add exported sentinel `ErrClosed`. Every method with an error result returns
  it after `Close`.
- Register `runtime.AddCleanup` in `New` to release the memory of a decoder
  dropped without `Close`. The cleanup argument must not reference the
  `Decoder`.

## Task 5 — Trim

- `func (d *Decoder) Trim()` releases the pages from `bump_base` (`0x900000`)
  to the end of memory and keeps the stack and static data below it. Memory
  size and slot layout are unchanged, so no new `Reserve` is required.
- Linux: `madvise(MADV_DONTNEED)`. macOS and other unix: `mmap` with
  `MAP_FIXED` over the range at the same protection. Windows:
  `VirtualFree(MEM_DECOMMIT)` then `VirtualAlloc(MEM_COMMIT)`. Fallback: no-op.
- Safe because everything at or above `bump_base` is per call: every export
  calls `bump_rewind()`, the shim calls no `malloc`, its only mutable static
  is `bump_ptr`, the host rewrites its slots on every call, the work buffer is
  per call, and the guest never grows memory itself.
- On a closed decoder, `Trim` is a no-op.

## Task 6 — Tests

Names identify unit versus integration tests.

- Unit: `Close` twice returns nil both times; after `Close`, each
  error-returning method returns `ErrClosed`; `Version`/`VersionNum` still
  work.
- Integration: for each supported format, and for each frame of an animated
  GIF, decode, `Trim`, decode again; pixels match the first decode.
- Integration: existing large-PNG regression tests still pass.
- Integration (Linux, build tag run by `make test-all`, generous margins),
  reading `VmRSS` from `/proc/self/status`:
  - a new decoder adds less than 4 MiB;
  - 8 decoders after 1024×1024 PNG decodes stay under 120 MiB;
  - after a 2000×2000 PNG decode and `Trim`, RSS is within 2 MiB of its value
    before the decode;
  - after `Close`, the decoder's pages are gone.
- Cross builds: `GOOS=darwin`. `GOOS=windows` and `GOARCH=386` builds fail
  inside the `wasm2go-wasi-host` dependency, on `main` before this work as
  well; the user chose to ignore that for now (2026-11-22). Instead, the
  memory files are type-checked on their own (`go vet` in an isolated copy)
  for windows/amd64, windows/386, windows/arm64, linux/386, darwin,
  freebsd, and js/wasm.

## Task 7 — Harden the shim

The shim is hand-written C outside the Wuffs safety guarantee; it must keep
the Wuffs contract (valid, correctly sized, non-overlapping buffers). A review
on 2026-11-22 found no current violation, but two bounds hold only through
indirect invariants. Make each one explicit:

1. **Bound `bump_alloc` itself.** Add a bump-region end (`bump_base` +
   `BUMP_LIMIT`) and make `bump_alloc` return `NULL` when an allocation would
   pass it. Every caller checks for `NULL` and returns `WUFFS_WASM_ERR_DECODE`
   (setting its meta `err` the same way its other error paths do). The
   existing per-caller `BUMP_LIMIT` checks stay. Today safety depends on every
   caller checking `BUMP_LIMIT` before allocating.
2. **Check the ICC buffer against its own capacity.** In
   `read_image_metadata`, the raw-passthrough ICC copy checks
   `icc_len + n > src_len`; check `icc_len + n > WUFFS_WASM_METADATA_ICC_CAP`
   instead (the raw-transform path already does). Today this is safe only
   because the 4 MiB metadata guard limits `src_len` to under 1 MiB.

Regenerate the wasm and binding as in Task 2.

Tests: both checks are defense in depth and unreachable while the
per-caller `BUMP_LIMIT` checks and the 4 MiB metadata guard hold (an ICC
profile over 1 MiB needs a source over 1 MiB, which the guard rejects
first). No behavioral test can reach them, so none is added; every existing
metadata, decode, and animation test must still pass, showing the hardening
changes no result.

Out of scope, recorded for a follow-up: the 4 MiB metadata guard makes
`Metadata` return `ErrDecode` for any source over about 1 MiB (functional
limit, not a safety issue).

## Task 8 — C lint in `make lint`

Added at the user's request on 2026-11-22: lint `wasm/shim.c`, which was never
linted, as part of the single `make lint` target (no separate target).

- `.clang-tidy`: `bugprone-*` and `clang-analyzer-*`, all warnings as errors,
  `HeaderFilterRegex` matching nothing so the included Wuffs release is not
  linted. Disabled, with reasons in the file:
  `bugprone-easily-swappable-parameters` (fixed export ABI),
  `bugprone-suspicious-include` (the `.c` include is by design), and
  `clang-analyzer-security.insecureAPI.DeprecatedOrUnsafeBufferHandling`
  (wasi-libc has no `memset_s`/`memcpy_s`; sizes are checked explicitly).
- `make lint` runs golangci-lint, checks that the sibling
  `wuffs-mirror-release-c` checkout exists, then runs the clang-tidy bundled
  with the pinned wasi-sdk (`wasm/wasi-sdk.sh`, shared with `wasm/build.sh`)
  for the wasm32 target with `-DWUFFS_WASM_LINT`.
- `WUFFS_WASM_LINT` stops `wasm/wuffs_config.h` defining
  `WUFFS_IMPLEMENTATION`, so lint parses only Wuffs declarations and the
  analyzer cannot follow calls into the Wuffs release. The decoder table uses
  the public `sizeof__wuffs_*__decoder` functions instead of `sizeof(TYPE)`,
  because C Wuffs defines decoder structs only with the implementation.
- Why: the first CI run (clang-tidy 21 from apt) reported
  `clang-analyzer-security.ArrayBound` inside the Wuffs vtable loop, reached
  from shim.c calls; local clang-tidy 19 lacked that check. Pinning one
  clang-tidy and parsing declarations only makes lint deterministic and
  limits it to shim.c.
- The one finding, `__heap_base` as a reserved identifier, is required by the
  linker and suppressed inline with `NOLINT(bugprone-reserved-identifier)`.
  Removing the suppression makes `make lint` fail.
- CI caches `tools/wasi-sdk-*` (keyed on `wasm/wasi-sdk.sh`) and clones `lbe/wuffs-mirror-release-c` read-only
  at `7411f488fe2e2c205c3d3b3d28638b7356522930` beside the workspace, then
  runs `make lint`.
- `docs/DEVELOPMENT.md` documents what `make lint` runs.

## Task 9 — Docs

`API.md`, `README.md`, `doc.go`, `docs/DEVELOPMENT.md`: `Close`, `Trim`,
`ErrClosed`, the memory layout, and that guest memory is outside the Go heap,
so `GOMEMLIMIT` and heap profiles no longer count it.

## Task 10 — Verify and hand off

```text
go build ./...
go vet ./...
make format-check
make lint
make test
make test-race
make cover
make test-all
go test -run '^$' -bench . -benchtime=1x ./...
GOOS=darwin GOARCH=arm64 go build ./...
git status --short
```

Haiku runs this block and reports each command's exit code, plus failure
output only. The main model re-runs any failing command and `make lint`
before committing. Every command exits 0. Leave one commit on
`feat/offheap-memory` for user review.

## STOP gates

Stop, report evidence to the user, and make no workaround when:

1. wasm2go c28cf76 cannot generate an imported-memory module.
2. Static data or BSS ends at or above `0x900000`.
3. Any RSS target in Task 6 cannot be met.
4. A change to `internal/wuffswasm/wuffs.go` would require editing it after
   generation.
5. Any Task 10 command fails for a reason this plan does not address.
