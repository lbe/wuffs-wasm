# BENCH-02 — Benchmark fixture tiers (M / L) and latency-focused reporting

**Execution protocol:** `plans/pi-runbook.md`

**Execution branch:** `bench-02-benchmark-fixtures`

**Progress file:** `tmp/pi_progress.md`

**Commit authorized:** Yes

**Depends on:** BENCH-01 complete (`decoder_bench_test.go` WebP/GIF/JPEG + stdlib baselines).

**Roadmap:** None — do **not** edit `plans/api-roadmap.md`.

**Purpose:** Add **medium** and **large** decode benchmark fixtures with clear
provenance, extend wuffs vs stdlib benches and baseline record tests for those
tiers, and document **execution time (ms/op)** as the primary comparison metric.
Compressed throughput (MB/s) remains a secondary log line only.

**Fixture policy (fixed):**

- **Do not** copy from `../wuffs/test/data/` or any other local checkout.
- **Download** M/L bytes from the public internet using `scripts/fetch_bench_fixtures.sh`
  (bash + `curl`; pinned upstream revision in the manifest).
- **All downloaded fixture bytes and generated fetch artifacts are gitignored** —
  they are **not** committed. The repo commits only the manifest, fetch script,
  `.gitignore` rules, docs, and benchmark code.
- L-tier WebP includes **lossless and lossy** harvesters assets — both required.

S-tier fixtures already committed under `testdata/` (BENCH-01) are unchanged.

---

## Sources of truth

1. `AGENTS.md`
2. The user's latest instruction
3. `plans/pi-runbook.md`
4. This plan
5. `plans/bench-01-decode-benchmarks.md`
6. `docs/DEVELOPMENT.md`

**Upstream bytes (download only, Apache-2.0):**
[google/wuffs `test/data`](https://github.com/google/wuffs/tree/main/test/data),
pinned by git commit in `testdata/bench/manifest.tsv` (committed).

---

## Gitignored layout (fixed)

| Path                               | Contents                                         |
| ---------------------------------- | ------------------------------------------------ |
| `testdata/bench/m/`                | Downloaded M-tier files                          |
| `testdata/bench/l/`                | Downloaded L-tier files                          |
| `tmp/bench-02-benchmark-fixtures/` | Fetch logs, verify output (already under `tmp/`) |

**Committed under `testdata/bench/` only:**

- `testdata/bench/manifest.tsv` — `sha256<TAB>relative_path<TAB>wuffs_test_data_basename`
- `testdata/bench/README.md` — how to fetch, license, pinned revision, no binaries

Add to root `.gitignore` (exact lines, Task 0):

```gitignore
# BENCH-02 benchmark fixtures (downloaded; never commit)
testdata/bench/m/
testdata/bench/l/
```

Do **not** gitignore `testdata/bench/manifest.tsv` or `testdata/bench/README.md`.

---

## Pinned manifest (committed in Task 0)

**Wuffs git commit for URLs (fixed until owner bumps manifest):**
`1f513d802343ec2f8a5c6e987cd071e655d36936`

**URL template (fixed):**

`https://raw.githubusercontent.com/google/wuffs/<WUFFS_COMMIT>/test/data/<basename>`

**`testdata/bench/manifest.tsv` rows** (`path` is relative to `testdata/bench/`):

| sha256                                                             | path                               | basename                         |
| ------------------------------------------------------------------ | ---------------------------------- | -------------------------------- |
| `c6a5374f1d8e8162de1cbdd85bd52660e6be57cd790f605ceabdc5c6e6d04a2e` | `m/hibiscus.regular.png`           | `hibiscus.regular.png`           |
| `906c6c383cd1e9536ce8df8b91d3f78627b1c0a5a42662a06119d0472e4c7289` | `m/hibiscus.regular.jpeg`          | `hibiscus.regular.jpeg`          |
| `5ca1345294d22b9db016078f872316ec47811697f918c4427a537d9cd0814726` | `m/hibiscus.regular.lossless.webp` | `hibiscus.regular.lossless.webp` |
| `8eb6f6df68e98b404615f8853f4ca5ec027d27afaeda20ba69dfdeb60b91e74e` | `m/hibiscus.regular.gif`           | `hibiscus.regular.gif`           |
| `445426e9202f1b67d438c686f3a8184013d7d8a1826cb5be3415e627b5da40cb` | `l/harvesters.png`                 | `harvesters.png`                 |
| `13657e9d92363f1c5d7dda4535dcba3d7e4158d77dd6e8e7a128705c07f4d499` | `l/harvesters.jpeg`                | `harvesters.jpeg`                |
| `12c135327e961d0fbc4efc994d9ad7f5bb79eadbdc2686356078ed95812d1c8c` | `l/harvesters.lossless.webp`       | `harvesters.lossless.webp`       |
| `08cf3c48b76182f797f56b97ce69898d0c28abc132a9a51a9ebc67cb6f0547a7` | `l/harvesters.lossy.webp`          | `harvesters.lossy.webp`          |
| `58c297d017200571c847fea54bd3a44f562086e12cd28b6cebd9089227037c7d` | `l/harvesters.gif`                 | `harvesters.gif`                 |

First line of `manifest.tsv` (committed header comment): `# wuffs_commit=1f513d802343ec2f8a5c6e987cd071e655d36936`

---

## Fixture tiers

| Tier  | Decoded pixels (approx) | On-disk (after fetch)             |
| ----- | ----------------------- | --------------------------------- |
| **S** | ≤ 160×120               | Committed `testdata/` (BENCH-01)  |
| **M** | 312×442 (~0.14 MP)      | `testdata/bench/m/*` (gitignored) |
| **L** | 1165×859 (~1.0 MP)      | `testdata/bench/l/*` (gitignored) |

**L-tier PNG for benchmarks:** use `testdata/bench/l/harvesters.png` (downloaded),
not `testdata/harvesters.png`. Migrate `BenchmarkDecodeRGBA_PNG_Harvesters` and
`TestBenchmarkPNGBaselineRecord` to the gitignored path in Task 2 / Task 3.
Leave committed `testdata/harvesters.png` in place for unrelated integration tests.

---

## Benchmark naming (fixed)

**M-tier** (`testdata/bench/m/`)

- `BenchmarkDecodeRGBA_PNG_HibiscusRegular` / `BenchmarkStdlibPNG_HibiscusRegular`
- `BenchmarkDecodeRGBA_JPEG_HibiscusRegular` / `BenchmarkStdlibJPEG_HibiscusRegular`
- `BenchmarkDecodeRGBA_WEBP_HibiscusRegular` / `BenchmarkStdlibWEBP_HibiscusRegular`
- `BenchmarkDecodeRGBA_GIF_HibiscusRegular` / `BenchmarkStdlibGIF_HibiscusRegular`

**L-tier** (`testdata/bench/l/`)

- `BenchmarkDecodeRGBA_PNG_Harvesters` / `BenchmarkStdlibPNG_Harvesters`
- `BenchmarkDecodeRGBA_JPEG_Harvesters` / `BenchmarkStdlibJPEG_Harvesters`
- `BenchmarkDecodeRGBA_WEBP_HarvestersLossless` / `BenchmarkStdlibWEBP_HarvestersLossless`
- `BenchmarkDecodeRGBA_WEBP_HarvestersLossy` / `BenchmarkStdlibWEBP_HarvestersLossy`
- `BenchmarkDecodeRGBA_GIF_Harvesters` / `BenchmarkStdlibGIF_Harvesters`

**Record tests** — primary log line:

`comparison <tag>: wuffs-go <ms/op> ms/op (<ns/op> ns/op) vs stdlib <ms/op> ms/op`

**Record test names (fixed):**

| Test                                                | Fixture                                                              |
| --------------------------------------------------- | -------------------------------------------------------------------- |
| `TestBenchmarkPNGHibiscusRegularBaselineRecord`     | `m/hibiscus.regular.png`                                             |
| `TestBenchmarkJPEGHibiscusRegularBaselineRecord`    | `m/hibiscus.regular.jpeg`                                            |
| `TestBenchmarkWEBPHibiscusRegularBaselineRecord`    | `m/hibiscus.regular.lossless.webp`                                   |
| `TestBenchmarkGIFHibiscusRegularBaselineRecord`     | `m/hibiscus.regular.gif`                                             |
| `TestBenchmarkPNGBaselineRecord`                    | `l/harvesters.png` (L-tier; was committed `testdata/harvesters.png`) |
| `TestBenchmarkJPEGHarvestersBaselineRecord`         | `l/harvesters.jpeg`                                                  |
| `TestBenchmarkWEBPHarvestersLosslessBaselineRecord` | `l/harvesters.lossless.webp`                                         |
| `TestBenchmarkWEBPHarvestersLossyBaselineRecord`    | `l/harvesters.lossy.webp`                                            |
| `TestBenchmarkGIFHarvestersBaselineRecord`          | `l/harvesters.gif`                                                   |

S-tier record tests (`TestBenchmarkWEBPBaselineRecord`, GIF, JPEG on bricks/hat)
keep committed fixtures; **all** record tests use the ms/op-primary log line via
shared `recordDecodeBaseline` / `logBaselineComparison`.

---

## Fetch contract (fixed)

**`scripts/fetch_bench_fixtures.sh`:**

- Bash only (no Python).
- Reads `testdata/bench/manifest.tsv`; skips `#` lines.
- `curl -fsSL` each URL into `testdata/bench/<path>`; create parent dirs.
- After download, `sha256sum` each file and **exit non-zero** on mismatch.
- Idempotent: safe to re-run.

**`Makefile` target `bench-fixtures`:** runs the script from repo root.

**Benchmarks** (`decoder_bench_test.go`, `package wuffs`): call in-package
`requireBenchFixtures` (created in **Task 0**). If any listed gitignored path is
missing, **fail the benchmark** with exactly:
`run make bench-fixtures from repo root (network required)`.

**Record tests** (`decoder_test.go`, `package wuffs_test`): call `loadBenchFixture`
only (see Task 3); it uses exported `SkipBenchFixturesIfMissing` from
`export_test.go`.

Do **not** invoke the fetch script or `curl` from `go test`.

**Tasks 1–5 Verify blocks** that run benches or record tests assume **Task 0**
is committed and the executor has run `make bench-fixtures` locally (network).
CI does not run `make bench-fixtures`.

---

## CI policy (fixed)

- Default CI (`make cover`) does **not** run `make bench-fixtures`.
- `decoder_bench_test.go` (`package wuffs`) calls in-package `requireBenchFixtures` →
  **fatal** when M/L files absent (`go test -bench`).
- `decoder_test.go` (`package wuffs_test`) uses `loadBenchFixture`, which calls
  exported `SkipBenchFixturesIfMissing` → **skip** when files absent.
- `export_test.go` exports `BenchFixturesOK`, `RequireBenchFixtures`, and
  `SkipBenchFixturesIfMissing` for **`wuffs_test` only** (implemented in Task 0).

---

## Product invariants

- No wasm changes. No CI benchmark jobs or fixture downloads.
- Wuffs benches: **0 allocs/op** after setup.
- GIF: `DecodeRGBA` frame 0 only.
- No committed binary under `testdata/bench/m/` or `testdata/bench/l/`.

---

## Executor STOP gates

| # | Trigger                                                                                    |
| - | ------------------------------------------------------------------------------------------ |
| 1 | Worker copies fixtures from `../wuffs/` or commits bytes under `testdata/bench/m/` or `l/` |
| 2 | `fetch_bench_fixtures.sh` SHA-256 verify fails                                             |
| 3 | `git status` shows staged files under gitignored bench paths                               |
| 4 | Verify fails outside current task                                                          |
| 5 | Edit outside allowed files                                                                 |

---

## Preflight

Branch hygiene only. **Do not** run `make bench-fixtures` in Preflight — that
target does not exist until **Task 0**.

### Worker instructions

1. Working tree and index **clean** on `main`; else STOP.
2. `git switch -c bench-02-benchmark-fixtures` (or confirm already on branch).
3. Record `BASELINE_HEAD` in `tmp/pi_progress.md`.

### Allowed files

- `tmp/pi_progress.md`, `tmp/bench-02-benchmark-fixtures/` (logs — do not stage)

### Verify

```bash
git branch --show-current
git status --short
git diff --name-only
```

Expected branch: `bench-02-benchmark-fixtures`. Expected `git status --short`: empty.

Commit authorized: **no**.

---

## Task 0 — Fetch infrastructure (script, manifest, Makefile, test helper)

Creates everything required before `make bench-fixtures` can succeed. Complete
this task and commit before starting Task 1.

### Allowed files

- `.gitignore`
- `testdata/bench/manifest.tsv`
- `testdata/bench/README.md`
- `scripts/fetch_bench_fixtures.sh`
- `Makefile` (add `bench-fixtures` to `.PHONY` / `help` only — do not change other targets)
- `bench_fixture_test.go` (new, `package wuffs`)
- `export_test.go` (CI fixture hooks only)

### Worker instructions

Execute in order. Do **not** copy bytes from `../wuffs/` or any local checkout.

1. **`.gitignore`** — append the **Gitignored layout** block (comment + two path lines; exact text).

2. **`testdata/bench/manifest.tsv`** — create the file. Line 1 must be the
   header comment from **Pinned manifest**. Following lines: one row per table
   row, format `sha256<TAB>relative_path<TAB>basename` (no spaces). Use the
   nine rows in **Pinned manifest** exactly.

3. **`testdata/bench/README.md`** — document:
   - M/L fixtures are **not** in git; run `make bench-fixtures` from repo root
     (network, `curl`).
   - Pinned Wuffs commit and URL template from **Pinned manifest**.
   - Apache-2.0 / Wuffs attribution link.
   - S-tier fixtures remain under committed `testdata/` (BENCH-01).

4. **`scripts/fetch_bench_fixtures.sh`** — new executable bash script:
   - `set -euo pipefail`.
   - Resolve repo root: `ROOT="$(cd "$(dirname "$0")/.." && pwd)"`; `cd "$ROOT"`.
   - Parse `WUFFS_COMMIT` from line 1 of `testdata/bench/manifest.tsv`
     (`# wuffs_commit=...`).
   - For each non-`#` line in `manifest.tsv`: read `hash`, `relpath`, `basename`;
     `dest="$ROOT/testdata/bench/$relpath"`; `mkdir -p "$(dirname "$dest")"`;
     `url="https://raw.githubusercontent.com/google/wuffs/${WUFFS_COMMIT}/test/data/${basename}"`;
     `curl -fsSL "$url" -o "$dest"`.
   - After all downloads, for each manifest row: `sha256sum` the file at
     `testdata/bench/$relpath` and compare to the manifest hash; print which
     file failed and **exit 1** on mismatch.
   - Safe to re-run (overwrites existing files).

5. **`Makefile`** — add:
   - `bench-fixtures` to the existing `.PHONY` list (or a dedicated `.PHONY: bench-fixtures` line).
   - One line in `help` echo output: `make bench-fixtures - Download M/L benchmark fixtures (network)`.
   - Target:

     ```makefile
     .PHONY: bench-fixtures
     bench-fixtures:
     	bash scripts/fetch_bench_fixtures.sh
     ```

6. **`bench_fixture_test.go`** + **`export_test.go`** — path constants for all nine
   manifest files; `requireBenchFixtures` (fatal) and `skipBenchFixturesIfMissing`
   (skip); export the three CI hooks per **CI policy**.

7. Run `make format` (formats the new shell script via shfmt).

8. Run `make bench-fixtures`; save stdout+stderr to
   `tmp/bench-02-benchmark-fixtures/task0-fetch.log`.

### Verify

Run in this order (per `plans/pi-runbook.md` — **before** commit):

```bash
test -f scripts/fetch_bench_fixtures.sh
test -f testdata/bench/manifest.tsv
test -f testdata/bench/README.md
grep -q '^bench-fixtures:' Makefile
make format-check
make bench-fixtures
test -f testdata/bench/m/hibiscus.regular.png
test -f testdata/bench/l/harvesters.lossy.webp
! git ls-files --error-unmatch testdata/bench/m/hibiscus.regular.png 2>/dev/null
git ls-files testdata/bench/
go test -run=^$ -count=1 .
```

FORBIDDEN before commit: `go test -bench`, record tests, editing `decoder_bench_test.go`.

9. After **Verify** succeeds: **commit** all Task 0 allowed files. **Never** `git add`
   under `testdata/bench/m/` or `testdata/bench/l/`.

After commit, `git ls-files testdata/bench/` must list **only**
`testdata/bench/manifest.tsv` and `testdata/bench/README.md`.

Commit authorized: **yes** (Task 0 allowed files only — **no** image bytes).

---

## Task 1 — M-tier benchmarks

**Prerequisite:** Task 0 committed; local `make bench-fixtures` succeeded at least once.

### Allowed files

- `decoder_bench_test.go`
- `bench_fixture_test.go`

### Worker instructions

1. At the start of each new M-tier benchmark, call `requireBenchFixtures(b)`.
2. Fixture dir: `testdata/bench/m/`. Decoded size **312×442** for all hibiscus benches.
3. Add the eight benchmarks named in **Benchmark naming** (M-tier).

### Verify

```bash
make bench-fixtures
go test -bench='HibiscusRegular' -benchmem -count=1 -run=^$ .
make format-check
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`.

Commit authorized: **yes**.

---

## Task 2 — L-tier benchmarks

**Prerequisite:** Task 1 committed.

### Allowed files

- `decoder_bench_test.go`
- `bench_fixture_test.go`

### Worker instructions

1. Call `requireBenchFixtures(b)` in each new L-tier benchmark.
2. Migrate existing `BenchmarkDecodeRGBA_PNG_Harvesters` / stdlib PNG harvesters to
   `testdata/bench/l/harvesters.png` (not `testdata/harvesters.png`).
3. Decoded size **1165×859** for all harvesters benches.
4. Add all L-tier benchmarks in **Benchmark naming**, including WebP **lossless and lossy**.

### Verify

```bash
make bench-fixtures
go test -bench='Harvesters' -benchmem -count=1 -run=^$ .
make format-check
```

FORBIDDEN before commit: `make test`, `make test-race`, `make cover`, `go test ./...`.

Commit authorized: **yes**.

---

## Task 3 — Baseline record tests

**Prerequisite:** Task 2 committed.

### Allowed files

- `decoder_test.go` (`package wuffs_test` — duplicate path constants or export helper via `export_test.go` if needed)
- `bench_fixture_test.go`, `export_test.go` (if needed)

### Worker instructions

1. In `decoder_test.go` (`package wuffs_test`), implement `loadBenchFixture` **before**
   any new record tests. `loadBenchFixture` calls exported `SkipBenchFixturesIfMissing`;
   M/L record tests **only** call `loadBenchFixture` (do not call
   `SkipBenchFixturesIfMissing` directly).
2. Add `recordDecodeBaseline` / `logBaselineComparison`; retrofit S-tier record tests to ms/op-primary logs.
3. Implement all nine M/L record tests with fixed names in **Record test names**; each
   uses `loadBenchFixture` for its fixture path.

### Verify

```bash
make bench-fixtures
go test -v -run 'TestBenchmark(PNG|JPEG|WEBP|GIF)(HibiscusRegular|Harvesters|BaselineRecord)' -count=1 .
make format-check
```

FORBIDDEN before commit: `make cover`, `go test ./...` (M/L record tests skip without fixtures).

Commit authorized: **yes**.

---

## Task 4 — Documentation

**Prerequisite:** Task 3 committed.

### Allowed files

- `docs/DEVELOPMENT.md`
- `README.md` (Performance)
- `testdata/README` (pointer to `testdata/bench/README.md`)

### Worker instructions

Document `make bench-fixtures`, **CI policy**, gitignored paths, ms/op-first metric.

### Verify

```bash
make format-check
go build ./...
```

Commit authorized: **yes**.

---

## Task 5 — Final gates

### Allowed files

- `tmp/pi_progress.md` (append final HEAD and task hashes)
- `tmp/bench-02-benchmark-fixtures/final-gates.log` (gitignored — write, do not stage)

### Worker instructions

1. Run every command in **Verify** below; save combined output to
   `tmp/bench-02-benchmark-fixtures/final-gates.log`.
2. Commit authorized: **no** (verification only).

### Verify

```bash
make format-check
make lint
make bench-fixtures
go test -bench='PNG_|HibiscusRegular|Harvesters' -benchmem -count=1 -run=^$ .
go test -v -run 'TestBenchmark.*BaselineRecord' -count=1 .
go build ./...
git status --porcelain --untracked-files=all
git ls-files testdata/bench/
```

**Final acceptance:** `git ls-files testdata/bench/` lists only `manifest.tsv` and
`README.md` (no `m/` or `l/` blobs).

---

## Completion report

1. HEAD, pinned Wuffs commit, manifest path.
2. New benchmark names.
3. Sample record-test log (ms/op).
4. `final-gates.log` path.
5. Confirmation: zero tracked files under `testdata/bench/m/` or `l/`.

---

## Execution prompt

```text
Execute plans/bench-02-benchmark-fixtures.md in order:
Preflight → Task 0 → Task 1 → Task 2 → Task 3 → Task 4 → Task 5.
Follow plans/pi-runbook.md. Progress: tmp/pi_progress.md.
Task 0 must create scripts/fetch_bench_fixtures.sh and Makefile bench-fixtures
before any Verify runs make bench-fixtures.
Do not copy from ../wuffs. Download only via make bench-fixtures / the script.
Do not commit testdata/bench/m/ or testdata/bench/l/ bytes.
Do not edit plans/api-roadmap.md. Do not push unless I instruct.
Begin with Preflight only on a clean main; implement Task 0 before Task 1.
```
