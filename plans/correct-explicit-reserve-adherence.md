# Correct Explicit Reserve Plan Adherence

**Execution protocol:** `plans/pi-runbook.md`

**Branch:** `enforce-explicit-reserve`

**Commit authorized:** No

**Purpose:** Correct the remaining deviation and test-coverage gaps found by the
plan-adherence review of `.pi/tdd-plans/enforce-explicit-reserve.yaml` without
rewriting the valid Cycle 1–4 history.

This is a corrective runbook, not a replacement TDD plan. Task 1 contains one
genuine Red/Green correction inside a single green task boundary. Task 2 adds
verification for behavior that already exists and therefore must not be
represented as a Red phase.

---

## Sources of truth

Read before acting:

1. `AGENTS.md`
2. `plans/pi-runbook.md`
3. This plan
4. `.pi/tdd-plans/enforce-explicit-reserve.yaml`
5. `GOALS.md`
6. `API.md`, especially Constraints, Call order, Errors, and Decoder → Reserve
7. `HANDOVER.md`, only for its caller-owned `Pix` and host-only scope constraints

If these sources conflict with this plan or the current branch, stop and report
the conflict. Do not invent a correction.

---

## Required execution state

- Work only on the existing branch `enforce-explicit-reserve`.
- Preserve the 12 existing explicit-Reserve TDD commits after local `main`.
- Do not amend, rebase, reset, squash, or otherwise rewrite history.
- Do not invoke `tdd-orchestrator` for this runbook.
- Do not edit `.pi/tdd-plans/enforce-explicit-reserve.yaml`, the stale
  seven-cycle guard, or its audit log. They remain historical execution evidence.
- Do not stage or commit. Do not push or merge.
- Do not modify the existing untracked `plans/api-roadmap.md`.
- Do not use Python.
- Do not run `make generate`.
- Do not modify generated artifacts:
  - `internal/wuffswasm/wuffs.go`
  - `wasm/shim.c`
  - `wasm/wuffs.wasm`
  - files under `scripts/`

### Preflight evidence

Before Task 1, the worker must report:

```bash
git branch --show-current
git rev-parse HEAD
git status --short
git diff --name-only
git diff --cached --name-only
sha256sum .pi/tdd-plans/enforce-explicit-reserve.yaml \
  .pi/tdd-state-enforce-explicit-reserve \
  .pi/tdd-audit-enforce-explicit-reserve.log \
  internal/wuffswasm/wuffs.go \
  wasm/shim.c \
  wasm/wuffs.wasm \
  > tmp/correct-explicit-reserve-baseline.sha256
```

Expected branch: `enforce-explicit-reserve`.

Expected HEAD at plan authoring time: `790c70e`. If HEAD differs, stop and report
the new history before editing. The working tree may contain the pre-existing
untracked `plans/api-roadmap.md` and this plan. There must be no staged changes.

### Allowed implementation files

Only these files may change during execution:

- `decoder.go`
- `memory.go`
- `decoder_internal_test.go`
- `memory_reserve_test.go`

The plan itself is not an implementation file and must not be edited during
execution. Any need to change another file is a plan deviation: stop and report.

---

## Task 1 — Make `SrcLen` the sole capacity authority

**Files:** `decoder_internal_test.go`, `decoder.go`, `memory.go`

**Nature:** One contained Red/Green correction. The worker performs Red and
Green before handing the task to the reviewer, so the task boundary remains
green as required by `plans/pi-runbook.md`.

### Red

Add a focused internal-package integration test named
`TestIntegrationSourceCapacityUsesCurrentLayout`.

The test must use separate subtests to prove both structural and behavioral
requirements:

1. `Decoder` has no independent `srcCap` field. Inspect the unexported concrete
   type with `reflect.TypeOf(Decoder{})` and fail if `FieldByName("srcCap")`
   succeeds.
2. `currentLayout.SrcLen` controls rejection. Create a fresh decoder, reduce
   only `d.currentLayout.SrcLen` to a small positive value, and pass a source of
   `SrcLen+1` bytes that remains below the old 64 KiB capacity. `Probe` must
   return an error matching `ErrSrcTooLarge` before invoking the guest.
3. Around the behavioral rejection, capture and compare the complete
   `SlotLayout`, wasm byte length, and `unsafe.SliceData` backing pointer. All
   must remain unchanged.

Run only the new test before editing production code:

```bash
go test -run '^TestIntegrationSourceCapacityUsesCurrentLayout$' -count=1 -v .
```

The test must fail on the pre-correction code for the intended reason: the
`srcCap` field exists and/or the source reaches the guest instead of producing
`ErrSrcTooLarge`. Report the failing assertions. If it passes before production
changes or fails for an unrelated reason, stop and report the plan-fidelity gap.

### Green

Make the smallest production correction:

1. Remove `srcCap` from `Decoder`.
2. Remove its initialization from `New`.
3. Change `checkSrcCapacity` to compare `len(src)` directly and safely against
   `d.currentLayout.SrcLen`. Use a comparison that remains correct when Go `int`
   and `uint32` have different widths.
4. Remove the `srcCap` synchronization from `Reserve`.
5. Do not change public APIs, error identities, slot-growth behavior, guest code,
   generated code, or caller-owned `Pix` behavior.

Run:

```bash
go test -run '^(TestIntegrationSourceCapacityUsesCurrentLayout|TestIntegrationSourceSlotDefinesDecoderCapacity|TestProbeHonorsReservedSourceCapacity|TestDecodeRGBAHonorsReservedSourceCapacity)$' -count=1 -v .
make format-check
make lint
go test ./...
```

### Task 1 acceptance criteria

- The new test produced authentic Red evidence before production edits.
- `Decoder` contains no `srcCap` field or other independent source-capacity
  state.
- `checkSrcCapacity` uses `currentLayout.SrcLen` as its sole capacity authority.
- Oversized-source rejection still matches `ErrSrcTooLarge` and occurs before a
  guest call.
- The rejection preserves the complete layout, wasm byte length, and backing
  pointer.
- Existing fresh, reserved Probe, and reserved DecodeRGBA capacity tests pass.
- Only `decoder.go`, `memory.go`, and `decoder_internal_test.go` changed in this
  task.
- The task ends formatted, lint-clean, buildable, and test-green.

The reviewer must reject Task 1 if the final code merely renames or resynchronizes
an independent capacity field instead of removing the duplicate authority.

---

## Task 2 — Lock existing Reserve boundary behavior

**File:** `memory_reserve_test.go`

**Nature:** Passing regression coverage. The production implementation already
rejects positive `int` requests above `math.MaxUint32` and keeps capacities
monotonic across mixed calls. Do not claim a Red phase, expect a failing test, or
modify production code to manufacture one.

### Add direct positive-`int` overflow coverage

Extend `TestReserveGrowsOnlyRequestedSlotCapacity` with table-driven cases for
destination and source independently:

1. On platforms where `int` is wider than 32 bits, construct a runtime integer
   equal to `uint64(math.MaxUint32) + 1` without a compile-time `int` overflow.
2. Call `Reserve(tooLarge, 0)` and `Reserve(0, tooLarge)` on separate fresh
   decoders.
3. Require a non-nil error for both calls.
4. Require the complete `SlotLayout`, wasm byte length, and backing pointer to
   remain unchanged for each rejection.
5. On a 32-bit platform, skip these two cases with a precise explanation that no
   positive `int` greater than `math.MaxUint32` is representable.

Keep the existing combined-layout overflow and 4096-page maximum cases. Do not
weaken or replace them.

### Add repeated mixed-request coverage

Use one decoder for a sequence that proves monotonic independent capacities:

1. Grow only destination while passing a negative source request; source must
   remain unchanged.
2. Grow only source while passing a negative destination request; destination
   must remain at its previously grown value.
3. Submit smaller, equal, zero, and negative mixed requests; neither capacity may
   decrease.
4. For calls that request no growth, require the complete `SlotLayout`, wasm byte
   length, and backing pointer to remain unchanged.
5. After every call, require `SrcLen` and `DstLen` to be at least their prior
   values and require the installed slots to remain ordered, non-overlapping,
   and within the wasm memory slice.

Do not introduce a new exported sentinel or helper solely for these tests.

### Verification

The new tests are expected to pass immediately against the existing production
implementation:

```bash
go test -run '^TestReserveGrowsOnlyRequestedSlotCapacity$' -count=1 -v .
make format-check
make lint
go test ./...
```

If either new scenario fails, stop and report the exact behavior. Do not expand
Task 2 into an unplanned production change.

### Task 2 acceptance criteria

- Both individual `int > math.MaxUint32` request positions are covered on
  platforms capable of representing the value.
- Every rejected boundary request returns a non-nil error and is failure-atomic.
- One repeated mixed sequence proves neither reserved capacity decreases.
- No-growth calls preserve full layout and wasm memory identity.
- Installed layouts are proven ordered, non-overlapping, and in bounds.
- Existing combined overflow and 4096-page ceiling tests remain intact.
- Only `memory_reserve_test.go` changed in this task.
- No production behavior or public API changed.
- The task ends formatted, lint-clean, buildable, and test-green.

---

## Task 3 — Final proof and scope gate

**Files:** No new edits expected.

The worker runs the complete proof gate. The reviewer independently checks the
diff and command evidence against this plan.

### Focused contract tests

```bash
go test -run '^(TestIntegrationSourceCapacityUsesCurrentLayout|TestIntegrationSourceSlotDefinesDecoderCapacity|TestReserveGrowsOnlyRequestedSlotCapacity|TestProbeHonorsReservedSourceCapacity|TestDecodeRGBAHonorsReservedSourceCapacity|TestIntegrationReserveRetry)$' -count=1 -v .
```

### Repository gates

Run in this order:

```bash
make format-check
make lint
make test
make test-race
go build ./...
go test -run '^TestIntegrationDecodeRGBA_AllocsPerRun$' -count=1 -v .
go test -run '^$' -bench '^BenchmarkDecodeRGBA_PNG_BricksColor$' -benchmem -count=1 .
git diff --check
```

Required allocation results:

- `TestIntegrationDecodeRGBA_AllocsPerRun`: exactly 0 allocations per decode.
- `BenchmarkDecodeRGBA_PNG_BricksColor`: `0 B/op` and `0 allocs/op`.

### Final scope audit

The worker reports:

```bash
git status --short
git diff --name-only HEAD
git diff --name-only HEAD -- internal/wuffswasm/wuffs.go wasm/shim.c wasm/wuffs.wasm scripts
git diff --cached --name-only
git log -1 --oneline
sha256sum -c tmp/correct-explicit-reserve-baseline.sha256
```

The reviewer must confirm:

- The branch remains `enforce-explicit-reserve` at the original HEAD.
- Implementation changes are limited to the four allowed files.
- Generated artifacts and `scripts/` are unchanged.
- `.pi` plan, guard, and audit files are unchanged.
- `plans/api-roadmap.md` is untouched.
- Nothing is staged or committed.
- No public production symbol was added or removed.
- The four-cycle amended plan now has direct evidence for its sole-capacity and
  Reserve-boundary commitments.

If any final command fails, the reviewer rejects the task. The worker may correct
only failures caused by the allowed changes and only within the allowed files.
Unrelated failures or any need to broaden scope require a stop report.

---

## Completion report

After reviewer approval, stop without committing, pushing, merging, editing the
stale TDD guard, or starting roadmap work. Report:

1. Task 1 Red evidence and the production correction.
2. Task 2 regression cases and their results.
3. All focused and repository gate results.
4. Allocation test and benchmark numbers.
5. Final changed-file manifest.
6. Confirmation that generated artifacts, history, staging area, `.pi` state,
   and `plans/api-roadmap.md` remain untouched.
