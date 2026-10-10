# BENCH-01 — Decode benchmarks (WebP, GIF, JPEG)

**Execution protocol:** `plans/pi-runbook.md`

**Execution branch:** `bench-01-decode-benchmarks`

**Progress file:** `tmp/pi_progress.md`

**Commit authorized:** Yes

**Roadmap:** None — maintainer/performance hygiene only; do **not** edit
`plans/api-roadmap.md`.

**Purpose:** Extend the existing PNG benchmark pattern to **WebP**, **GIF**, and
**JPEG**: reusable `DecodeRGBA` benches with `benchmem`, optional **stdlib**
baselines on the same fixtures (WebP via `golang.org/x/image/webp`), and
**extended throughput logging** tests mirroring
`TestBenchmarkPNGBaselineRecord`. No CI performance gate and no wasm guest changes.

This plan does **not** add animation (`DecodeFrame`) benchmarks, metadata benches,
new large fixtures, or roadmap rows.

---

## Sources of truth

Read before acting:

1. `AGENTS.md`
2. The user's latest instruction
3. `plans/pi-runbook.md`
4. This plan
5. `docs/DEVELOPMENT.md` (PNG benchmark methodology)
6. `decoder_bench_test.go` and `TestBenchmarkPNGBaselineRecord` in `decoder_test.go`

If sources conflict, stop and report the conflict.

---

## Fixed fixtures (do not add or swap)

Use only these checked-in files and dimensions (from existing integration tests):

| Fixture     | Path                                  | Width × height | Notes                           |
| ----------- | ------------------------------------- | -------------- | ------------------------------- |
| WebP        | `testdata/bricks-color.lossless.webp` | 160 × 120      | Lossless WebP only              |
| GIF (still) | `testdata/bricks-nodither.gif`        | 160 × 120      | Single frame; `DecodeRGBA` only |
| JPEG        | `testdata/hat.jpeg`                   | 90 × 112       | Stride 360 (width × 4)          |

PNG benchmarks in `decoder_bench_test.go` must remain behavior-identical after
refactors (same benchmark names and fixtures).

**WebP stdlib baseline (owner decision, fixed):** use
`golang.org/x/image/webp` in test code only (`decoder_bench_test.go`). Add the
module requirement via `go mod tidy` after the import; do not vendor WebP decode
into the product package.

---

## Benchmark conventions (fixed)

- **Wuffs hot path:** one `Decoder`, `RequiredReserve(d, width*height*4, len(src))`,
  one `image.NewRGBA` **outside** `b.ResetTimer()`, loop `DecodeRGBA`, `b.ReportAllocs()`.
  Expect **0 allocs/op** on success (same standard as `BenchmarkDecodeRGBA_PNG_BricksColor`).
- **Stdlib path:** read bytes once outside the timer; each iteration decodes from a
  fresh `bytes.Reader` (same as `benchStdlibPNG`). Higher allocs are expected.
- **Naming:**
  - `BenchmarkDecodeRGBA_WEBP_BricksColor`
  - `BenchmarkDecodeRGBA_GIF_BricksNodither`
  - `BenchmarkDecodeRGBA_JPEG_Hat`
  - `BenchmarkStdlibWEBP_BricksColor` (`webp.Decode`)
  - `BenchmarkStdlibGIF_BricksNodither` (`gif.Decode`)
  - `BenchmarkStdlibJPEG_Hat` (`jpeg.Decode`)
- **Extended logging tests** (`package wuffs_test`, `decoder_test.go`):
  - `TestBenchmarkWEBPBaselineRecord`
  - `TestBenchmarkGIFBaselineRecord`
  - `TestBenchmarkJPEGBaselineRecord`
    Each mirrors `TestBenchmarkPNGBaselineRecord`: 10 iterations, log wuffs MB/s and
    ns/op, 10 iterations stdlib counterpart, one comparison log line. **No**
    throughput assertions — human go/no-go only.

---

## Product invariants (must hold after every task)

- No CGO. No edits to `wasm/shim.c`, `wasm/wuffs.wasm`, or
  `internal/wuffswasm/wuffs.go`.
- Do not add benchmarks to `.github/workflows/ci.yml`.
- Do not weaken existing PNG benchmarks or integration tests.
- GIF benches use **still** `DecodeRGBA` only (not `DecodeFrame` / `muybridge.gif`).

---

## Execution control

- Use only Pi subagents `worker` and `reviewer` per `plans/pi-runbook.md`.
- Branch `bench-01-decode-benchmarks`: create with
  `git switch -c bench-01-decode-benchmarks` only when on `main`, branch absent,
  working tree clean.
- Each committing task: worker implements → Verify → `tmp/commit_message.txt` →
  reviewer → `git commit -F tmp/commit_message.txt` → append hash to
  `tmp/pi_progress.md`.

---

## Executor STOP gates (mandatory)

On any trigger below (or `plans/pi-runbook.md` standing constraint 13): **STOP**,
message the user (task id, evidence), **no commit**, **no next task** until written
user direction.

| # | Trigger                                                                                   | Executor action |
| - | ----------------------------------------------------------------------------------------- | --------------- |
| 1 | Working tree not clean at preflight                                                       | STOP            |
| 2 | Plan step contradicts runtime or tests                                                    | STOP            |
| 3 | Verify fails for reason not explained by current task                                     | STOP            |
| 4 | Edit outside task allowed files                                                           | STOP            |
| 5 | `go mod tidy` changes modules beyond `golang.org/x/image` (and its tree) for WebP benches | STOP            |

---

## Preflight

### Worker instructions

1. Working tree and index **clean** on `main`; else STOP.
2. `git switch -c bench-01-decode-benchmarks` (or confirm already on branch).
3. Record `BASELINE_HEAD` in `tmp/pi_progress.md`.

### Allowed files

- `tmp/pi_progress.md`, `tmp/bench-01-decode-benchmarks/` (gitignored logs — do not stage)

### Verify

```bash
git branch --show-current
git status --short
git diff --name-only
git diff --cached --name-only
```

### Acceptance

- Branch `bench-01-decode-benchmarks`; clean tree; progress file has baseline HEAD.

---

## Task 1 — Generalize bench helpers (keep PNG green)

### Allowed files

- `decoder_bench_test.go`

### Worker instructions

1. Refactor `benchDecodeRGBA` to accept `(b, path string, width, height int)` without
   changing PNG benchmark names or timed behavior.
2. Keep `benchStdlibPNG` or generalize to a shared stdlib helper; PNG stdlib benches
   unchanged.

### Verify

```bash
go test -bench='PNG_' -benchmem -count=1 -run=^$ .
make format-check
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`.

### Acceptance

- All `PNG_` benchmarks run; refactor only.

---

## Task 2 — Wuffs WebP / GIF / JPEG benchmarks

### Allowed files

- `decoder_bench_test.go`

### Worker instructions

1. Add `BenchmarkDecodeRGBA_WEBP_BricksColor`, `BenchmarkDecodeRGBA_GIF_BricksNodither`,
   `BenchmarkDecodeRGBA_JPEG_Hat` per **Fixed fixtures** and **Naming**.
2. Confirm `0 allocs/op` on each (fail the task if not — investigate `RequiredReserve`
   sizing before weakening the bench).

### Verify

```bash
go test -bench='DecodeRGBA_(WEBP|GIF|JPEG)_' -benchmem -count=1 -run=^$ .
make format-check
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`.

### Acceptance

- Three new wuffs benchmarks pass and report zero heap allocs per iteration.

---

## Task 3 — Stdlib baselines + `golang.org/x/image/webp`

### Allowed files

- `decoder_bench_test.go`
- `go.mod`
- `go.sum`

### Worker instructions

1. Add imports: `image/jpeg`, `image/gif`, `golang.org/x/image/webp`.
2. Add `BenchmarkStdlibWEBP_BricksColor`, `BenchmarkStdlibGIF_BricksNodither`,
   `BenchmarkStdlibJPEG_Hat`.
3. Run `go mod tidy`; commit only the `x/image` dependency chain required for WebP
   test imports.

### Verify

```bash
go test -bench='Stdlib(WEBP|GIF|JPEG)_' -benchmem -count=1 -run=^$ .
make format-check
go build ./...
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`.

### Acceptance

- Stdlib benchmarks run; `go.mod` lists `golang.org/x/image` for WebP decode in tests.

---

## Task 4 — Extended baseline record tests

### Allowed files

- `decoder_test.go` (`package wuffs_test`)

### Worker instructions

1. Add `TestBenchmarkWEBPBaselineRecord`, `TestBenchmarkGIFBaselineRecord`,
   `TestBenchmarkJPEGBaselineRecord` mirroring `TestBenchmarkPNGBaselineRecord`
   (wuffs + matching stdlib decoder, log MB/s and ns/op, no assertions on throughput).
2. Use the same fixtures and dimensions as Task 2–3.

### Verify

```bash
go test -v -run '^TestBenchmark(WEBP|GIF|JPEG)BaselineRecord$' -count=1 .
make format-check
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`.

### Acceptance

- Three record tests pass and emit comparison logs.

---

## Task 5 — Documentation

### Allowed files

- `docs/DEVELOPMENT.md`
- `README.md`

### Worker instructions

1. **DEVELOPMENT.md** — Extend **Performance comparison** with WebP/GIF/JPEG bench
   commands, stdlib pairing (including `x/image/webp`), and interpretation (same
   rules as PNG: wuffs 0 allocs after setup; stdlib allocates per decode).
2. **README.md** — Update **Performance** to state benchmarks cover PNG, WebP, GIF,
   and JPEG; point to DEVELOPMENT for commands. Keep the existing PNG example or add
   a single combined `-bench` regex example.

Do not edit `documentation_contract_integration_test.go` unless README wording
drifts from an existing inventory assertion (none required for this plan).

### Verify

```bash
make format-check
go build ./...
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`.

### Acceptance

- Docs describe all four formats and how to run benches + record tests.

---

## Task 6 — Final gates

### Allowed files

- `tmp/pi_progress.md` (append final HEAD and task hashes)
- `tmp/bench-01-decode-benchmarks/final-gates.log` (gitignored — write, do not stage)

### Worker instructions

1. Run every command in **Verify** below; save combined output to
   `tmp/bench-01-decode-benchmarks/final-gates.log`.
2. Commit authorized: **no** (verification only unless an earlier task left uncommitted
   work — then STOP and finish that task first).

### Verify

```bash
make format-check
make lint
go test -bench='PNG_|DecodeRGBA_(WEBP|GIF|JPEG)_|Stdlib(WEBP|GIF|JPEG|PNG)_' -benchmem -count=1 -run=^$ .
go test -v -run '^TestBenchmark(PNG|WEBP|GIF|JPEG)BaselineRecord$' -count=1 .
go build ./...
git status --porcelain --untracked-files=all
```

FORBIDDEN before commit: staging `tmp/` logs.

### Final acceptance

- All Verify commands pass.
- `tmp/pi_progress.md` lists every task commit hash.
- Working tree clean except ignored `tmp/` artifacts.

---

## Completion report (orchestrator → user)

1. Branch `bench-01-decode-benchmarks` and final HEAD.
2. New benchmark names (wuffs + stdlib).
3. `go.mod` change summary (`golang.org/x/image` for WebP tests).
4. Example `go test -bench=…` one-liner from DEVELOPMENT.md.
5. Path to `tmp/bench-01-decode-benchmarks/final-gates.log`.

Stop after reporting. Do not push unless instructed.

---

## Execution prompt (paste to Pi orchestrator)

```text
Execute plans/bench-01-decode-benchmarks.md Preflight through Task 6 in order.
Follow plans/pi-runbook.md (worker test budget, commits, format).
Progress file: tmp/pi_progress.md.
Worker model: fast; do not refactor unrelated code.
Do not edit plans/api-roadmap.md.
Do not push unless I instruct.
Begin with Preflight only on a clean main.
```
