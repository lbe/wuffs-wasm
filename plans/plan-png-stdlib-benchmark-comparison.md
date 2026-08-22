# Plan: PNG stdlib comparison benchmarks

**Goal:** Add `image/png` comparison benchmarks alongside existing wuffs-go PNG
benchmarks so harvesters.png and bricks-color.png can be compared on the same
machine with `go test -bench`.

**Why:** Plan 1 cycle 8 records wuffs-go baseline only. Original intent was to
compare against Go stdlib before a human go/no-go on the wasm2go path. That work
was never implemented.

**Branch:** `wuffs-go-runtime` (benchmarks-only change; no decode behavior change)

**Relationship to Plan 1:** Plan 1 cycle 8 is **complete**. This is a **post-Plan-1
follow-up**, not a retroactive amendment to executed cycles. Do not edit
`.pi/tdd-plans/wuffs-go-runtime.yaml` cycle 8 acceptance criteria.

**Out of scope:** Automated throughput floor, CI perf gates, native Wuffs C
benchmarks, WEBP stdlib comparison, TDD orchestrator / guard state changes.

---

## Decisions

| Item                    | Choice                                                                                                                                                         |
| ----------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| API under test (wuffs)  | Existing `DecodeRGBA` + `RequiredReserve` — unchanged                                                                                                          |
| API under test (stdlib) | `image/png.Decode` on `bytes.NewReader(src)`                                                                                                                   |
| Fixtures                | Same files: `testdata/harvesters.png`, `testdata/bricks-color.png`                                                                                             |
| Package                 | `decoder_bench_test.go` stays `package wuffs`                                                                                                                  |
| Fairness                | Document that stdlib allocates decoded pixels; wuffs uses 1 alloc (`image.NewRGBA` shell) + wasm dst. Compare **ns/op** and **allocs/op**, not raw MB/s alone. |
| Baseline record test    | Extend `TestBenchmarkPNGBaselineRecord` to also run stdlib once and `t.Logf` both throughputs                                                                  |

---

## Work items

### 1. Shared bench helpers (`decoder_bench_test.go`)

Add:

```go
func benchStdlibPNG(b *testing.B, pngPath string)
```

- Read file once outside timer (same as wuffs benches).
- Loop: `png.Decode(bytes.NewReader(pngSrc))`.
- `b.ReportAllocs()`.

Add exported benchmarks:

- `BenchmarkStdlibPNG_Harvesters`
- `BenchmarkStdlibPNG_BricksColor`

Naming mirrors existing `BenchmarkDecodeRGBA_PNG_*` so
`go test -bench='PNG_' -benchmem` runs all four.

### 2. Comparison record test (`decoder_test.go`, package `wuffs_test`)

Extend `TestBenchmarkPNGBaselineRecord`:

1. Keep existing wuffs loop (10 iterations, log MB/s + ns/op).
2. Add stdlib block: same `harvesters.png` bytes, 10× `png.Decode`, log MB/s + ns/op.
3. Log a single summary line:

   ```
   comparison harvesters: wuffs-go X.XX MB/s (Y ns/op) vs stdlib A.AA MB/s (B ns/op)
   ```

4. Test still passes on decode success only — no assertion on relative speed.

### 3. Developer docs (`docs/DEVELOPMENT.md`)

Add **Performance comparison** section:

```bash
go test -bench='PNG_' -benchmem -count=5 -run=^$
go test -v -run TestBenchmarkPNGBaselineRecord -count=1
```

Explain how to read output and that go/no-go remains manual.

---

## Verification

```bash
make test
go test -bench='BenchmarkDecodeRGBA_PNG|BenchmarkStdlibPNG' -benchmem -count=3 -run=^$
go test -v -run TestBenchmarkPNGBaselineRecord -count=1
make format-check
```

Expected: four benchmarks run; baseline test logs both wuffs and stdlib lines.

---

## Execution order

1. Bench helpers + two stdlib benchmarks
2. Extend `TestBenchmarkPNGBaselineRecord`
3. Run verification locally; capture sample numbers in PR/commit message
4. Update `docs/DEVELOPMENT.md`
5. Single commit: `feat(bench): add image/png comparison benchmarks for PNG perf gate`

---

## Notes for reviewer (you)

- Stdlib will likely show **more allocs** (full decode allocation).
- Wuffs MB/s uses compressed input bytes (same formula as today); stdlib uses
  the same formula for apples-to-apples on source throughput.
- If stdlib is faster on harvesters, that informs go/no-go; it does not fail
  the build.
