# Plan: Correct Caller-Owned `Pix` Adherence Deviations

## Goal

Correct only the deviations found after Pi implemented the caller-owned
`image.RGBA.Pix` work. Preserve the approved implementation and its zero-allocation
behavior while bringing the code, tests, and README into exact agreement with the
original plan.

This is a follow-up correction plan. Do not reimplement the feature or expand the
scope described by `HANDOVER.md`.

## Execution control

- **Runbook:** `plans/pi-runbook.md`
- **Required Pi subagents:** `worker` and `reviewer`
- **Existing branch:** Run the entire plan in the already checked-out
  `fix-decode-rgba-pix-ownership` branch. Do not create, rename, or switch branches.
- **Commit authorized:** No. Do not stage or commit.
- **Execution order:** Complete and approve one task before starting the next.
- **Review rule:** The `reviewer` must inspect the uncommitted diff and command
  evidence for every task, including the snapshot comparisons defined below.
- **Attempt limit:** Follow the runbook's three-attempt limit for a rejected task.
- **Generated code:** Do not run `make generate` and do not edit generated or guest
  artifacts.

The Pi orchestrator must only orchestrate. It must delegate all reads, edits,
formatting, tests, and repository checks to the preconfigured `worker` or
`reviewer` subagents as specified by the runbook.

## Sources of truth

Read these in order before making changes:

1. `AGENTS.md`
2. `plans/pi-runbook.md`
3. `GOALS.md`
4. `API.md`
5. `HANDOVER.md`
6. This plan

Conflict rules:

- `API.md` defines the complete product contract.
- `HANDOVER.md` limits this pass to the caller-owned `DecodeRGBA` correction.
- This plan corrects audit deviations in the current implementation; it does not
  authorize unrelated API work.
- If the working tree no longer matches the baseline or this plan conflicts with a
  higher-priority source, stop and report the conflict.

## Verified implementation baseline

The caller-owned destination implementation is already present and was verified
before this plan was written:

- `DecodeRGBA` decodes into separate wasm scratch and writes converted pixels into
  the caller's `dst.Pix`.
- Successful repeated decode preserves the `Pix` pointer, length, capacity,
  `Rect`, and `Stride`.
- `ErrBadImage`, `ErrDstTooSmall`, and `*DstTooSmallError.Is` exist.
- The allocation test reports zero allocations.
- `BenchmarkDecodeRGBA_PNG_BricksColor` reports `0 B/op` and `0 allocs/op`.
- Direct formatting, lint, test, race, and build commands passed during the audit.
- Generated artifacts are unchanged.

Do not disturb these approved behaviors while correcting the deviations below.

## Allowed files

Only these files may be modified:

- `decoder.go`
- `decoder_test.go`
- `README.md`

No other file may change. In particular, leave these already approved files
unchanged:

- `errors.go`
- `errors_test.go`
- `decoder_bench_test.go`
- `docs/DEVELOPMENT.md`
- `scripts/gen_golden.go`
- `internal/wuffswasm/wuffs.go`
- `wasm/wuffs.wasm`
- `wasm/shim.c`

## Execution snapshots

The existing working tree is the uncommitted implementation baseline, so Git's
ordinary diff cannot identify changes made by this correction plan. Before Task
1 edits anything, its `worker` must create one out-of-tree snapshot directory,
record its absolute path as `snapshot_dir`, and report that path to the Pi
orchestrator. The orchestrator must pass the same path to every later `worker`
and `reviewer`:

```sh
snapshot_dir="$(mktemp -d /tmp/wuffs-go-fix-handover-issues.XXXXXX)"
git branch --show-current > "$snapshot_dir/initial.branch"
git rev-parse HEAD > "$snapshot_dir/initial.head"
git status --short > "$snapshot_dir/initial.status"
git diff --cached --binary > "$snapshot_dir/initial.index.patch"
find . -path './.git' -prune -o -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > "$snapshot_dir/task1.before.sha256"
find . -path './.git' -prune \
  -o -path './decoder.go' -prune \
  -o -path './decoder_test.go' -prune \
  -o -path './README.md' -prune \
  -o -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > "$snapshot_dir/initial.protected.sha256"
mkdir "$snapshot_dir/task1-before"
cp --parents decoder.go decoder_test.go "$snapshot_dir/task1-before"
test "$(cat "$snapshot_dir/initial.branch")" = 'fix-decode-rgba-pix-ownership'
test ! -s "$snapshot_dir/initial.index.patch"
while IFS= read -r status_line; do
  index_status="${status_line:0:1}"
  worktree_status="${status_line:1:1}"
  if { [ "$index_status" != ' ' ] && [ "$index_status" != '?' ]; } ||
    { [ "$index_status" = ' ' ] && [ "$worktree_status" = 'A' ]; }; then
    exit 1
  fi
done < "$snapshot_dir/initial.status"
```

Before proceeding, require `initial.branch` to contain exactly
`fix-decode-rgba-pix-ownership`, require `initial.index.patch` to be empty, and
validate every entry in `initial.status`. The status check permits the existing
unstaged and untracked dirty baseline but rejects any non-empty index column and
the porcelain `A` intent-to-add form. If any check fails, stop without changing
the branch or index. The Task 1
reviewer must independently write `task1.after.sha256`, compare it with
`task1.before.sha256`, and diff the saved `decoder.go` and `decoder_test.go`
against their current contents. Only those two paths may differ. Use these exact
commands and treat the expected content differences in the three `diff` outputs
as review evidence, not gate failures:

```sh
find . -path './.git' -prune -o -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > "$snapshot_dir/task1.after.sha256"
diff -u "$snapshot_dir/task1.before.sha256" "$snapshot_dir/task1.after.sha256"
diff -u "$snapshot_dir/task1-before/decoder.go" decoder.go
diff -u "$snapshot_dir/task1-before/decoder_test.go" decoder_test.go
```

After Task 1 approval and before Task 2 edits, the Task 2 `worker` must create its
own starting manifest and content copy in the same directory:

```sh
find . -path './.git' -prune -o -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > "$snapshot_dir/task2.before.sha256"
mkdir "$snapshot_dir/task2-before"
cp --parents README.md "$snapshot_dir/task2-before"
```

The Task 2 reviewer must independently write `task2.after.sha256`, compare it
with `task2.before.sha256`, and diff the saved `README.md` against its current
contents. Only `README.md` may differ. Use these exact commands and treat the
expected content differences as review evidence, not gate failures:

```sh
find . -path './.git' -prune -o -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > "$snapshot_dir/task2.after.sha256"
diff -u "$snapshot_dir/task2.before.sha256" "$snapshot_dir/task2.after.sha256"
diff -u "$snapshot_dir/task2-before/README.md" README.md
```

Keep the snapshot directory through final review so the final reviewer can
compare a fresh workspace manifest with `task1.before.sha256` and prove that
this plan changed only the three allowed files despite the pre-existing dirty
baseline.

## Required corrections

### Checked decoded-byte calculation

In `decoder.go`, calculate the decoded scratch length without recomputing the
product in `int`:

```go
decodedBytes := uint64(width) * uint64(height) * 4
if decodedBytes > math.MaxUint32 {
	return ErrBadImage
}
pixLen := uint32(decodedBytes)
```

Use the existing import and error-return style where applicable. The final code
must not contain `uint32(width * height * 4)` or another unchecked `int`
multiplication for this value.

### Accurate `DecodeRGBA` godoc

Correct the `DecodeRGBA` error documentation so its cases are mutually exclusive:

- `ErrBadImage` covers malformed or incompatible destination shape, including an
  empty rectangle, nil or empty `Pix`, non-zero `Rect.Min`, decoded dimension
  mismatch, and an unrepresentable host layout.
- `*DstTooSmallError`, matching `ErrDstTooSmall`, covers insufficient `Stride` or
  insufficient `Pix` length for otherwise matching dimensions.

Do not describe short non-empty `Pix` as `ErrBadImage`.

### Exact host-capacity tests

In the existing destination-validation integration tests:

- For the too-small-stride case, retain the `errors.As` assertion and add a
  positive `errors.Is(err, wuffs.ErrDstTooSmall)` assertion.
- For the short-`Pix` case, retain the `errors.As` assertion and add a positive
  `errors.Is(err, wuffs.ErrDstTooSmall)` assertion.
- Make the short-`Pix` case use a valid padded stride, not a tight stride:
  - choose `stride > width*4`;
  - allocate `Pix` with length `stride*height - 1`;
  - require `DstTooSmallError.Width` and `.Height` to equal the decoded dimensions;
  - require `DstTooSmallError.Stride` to equal the padded destination stride;
  - require `DstTooSmallError.MinBytes` to equal `stride*height`.

Keep the unrepresentable-layout test's negative `errors.Is` assertion. Do not
weaken exact field checks already present.

### Guest-scratch retry ownership proof

In the existing `TestIntegrationReserveRetry` flow, capture these destination
properties before the decode that fails because guest scratch is too small:

- `unsafe.SliceData(dst.Pix)`
- `len(dst.Pix)`
- `cap(dst.Pix)`
- `dst.Rect`
- `dst.Stride`

Assert that every property is unchanged:

1. immediately after the expected guest-scratch failure; and
2. after `Reserve` followed by a successful retry into the same destination.

Retain the existing successful pixel-content assertion. Do not allocate a
replacement destination for the retry.

### Restore concurrent-test panic reporting

The prior implementation removed existing panic-recovery wrappers from
`TestIntegrationConcurrentDecoders` even though that cleanup was not authorized.
Restore them:

- restore the `fmt` import required by the wrappers;
- add a deferred recovery wrapper to each goroutine;
- in each goroutine, register `defer wg.Done()` before registering the recovery
  defer; because defers execute LIFO, this makes recovery run first and assign
  `err1` or `err2` before `wg.Done()` can unblock `wg.Wait()`;
- convert a recovered panic to `fmt.Errorf(...)` and assign it to that
  goroutine's existing result variable: `err1` for the first goroutine and
  `err2` for the second;
- keep using correctly sized caller-owned destinations in both goroutines.

Do not otherwise redesign or refactor the concurrency test.

### Correct README sequence and terminology

Update the caller-owned decode example and nearby prose in `README.md` to show
this exact order:

1. `Probe`
2. create a correctly sized `image.NewRGBA`
3. `Reserve`
4. `DecodeRGBA`

Replace wording that says conversion happens "in-place" with wording that clearly
states that the guest decodes into wasm scratch and the host copies/converts from
that scratch into the caller-owned `Pix` buffer.

Retain the documented ownership, error, reuse, and zero-allocation guarantees.

## Task 1 — Correct code and integration proofs

### Worker instructions

1. Confirm the already checked-out branch is `fix-decode-rgba-pix-ownership` and
   report the current working-tree state. If another branch is checked out, stop
   and report it; do not switch branches.
2. Read all sources of truth and the existing relevant code and tests.
3. Modify only `decoder.go` and `decoder_test.go`.
4. Apply every required correction except the README correction.
5. Format only the edited Go files with
   `gofmt -w decoder.go decoder_test.go`. Do not run `make format` or any
   repository-wide formatting command.
6. Run the narrow tests first, then the broader task gates.
7. Report the diff summary and complete command output or concise passing evidence.
8. Do not stage or commit.

### Acceptance criteria

- The decoded-byte product is formed in `uint64`, checked once against
  `math.MaxUint32`, and cast from that checked value.
- `DecodeRGBA` godoc no longer assigns short non-empty `Pix` to `ErrBadImage`.
- Both host-capacity error cases positively match `ErrDstTooSmall` through
  `errors.Is` and retain `errors.As` coverage.
- The short-`Pix` case uses padded stride and checks every structured error field
  against the padded layout.
- The retry test proves destination identity after both failure and successful
  retry.
- Each concurrent decoder goroutine converts a recovered panic to an error in
  its corresponding `err1` or `err2` result variable, with `defer wg.Done()`
  registered before the recovery defer so recovery assigns the result before
  `wg.Wait()` can return.
- Existing ownership, golden-pixel, allocation, and error behavior remains green.
- Only `decoder.go` and `decoder_test.go` change during this task.

### Worker verification

Run in this order:

```sh
go test -run 'TestIntegrationDecodeRGBA_(DstValidation|UnrepresentableHostLayout)|TestIntegrationReserveRetry|TestIntegrationConcurrentDecoders' -count=1 -v
go test -run '^TestIntegrationConcurrentDecoders$' -race -count=1 -v
go test ./... -count=1
go build ./...
make format-check
```

If the actual destination-validation test name differs, use the narrowest regular
expression that selects the existing destination validation and unrepresentable
layout tests. Report the exact substituted command; do not rename tests solely to
match this plan.

### Reviewer checklist

- Inspect the uncommitted diff against every Task 1 acceptance criterion.
- Search the changed code for unchecked `width * height * 4` arithmetic and reject
  any path that recomputes the checked product in `int`.
- Confirm both capacity-error tests use positive `errors.Is` and successful
  `errors.As` assertions.
- Confirm the short-`Pix` test has a genuinely padded valid stride and exact
  `Width`, `Height`, `Stride`, and `MinBytes` assertions.
- Confirm the retry test compares pointer, length, capacity, rectangle, and stride
  after both the failed and successful calls.
- Confirm both concurrency goroutines assign recovered panics to their
  corresponding `err1` or `err2` result variable without changing their
  caller-owned destination setup.
- Confirm each goroutine registers `defer wg.Done()` before its recovery defer,
  so LIFO execution writes `err1` or `err2` before `wg.Done()` can unblock
  `wg.Wait()`.
- Generate `task1.after.sha256`, compare it with `task1.before.sha256`, and diff
  both saved Task 1 files against their current contents. Confirm no disallowed
  file changed and no test was weakened or deleted.
- Verify the reported commands passed, then approve or reject with file-specific
  evidence.

Do not start Task 2 until the reviewer approves Task 1.

## Task 2 — Correct README workflow

### Worker instructions

1. Modify only `README.md`.
2. Reorder the example to `Probe` → `image.NewRGBA` → `Reserve` → `DecodeRGBA`.
3. Correct the scratch-to-caller conversion wording.
4. Preserve the existing public error and zero-allocation documentation.
5. Format only `README.md` with `dprint fmt README.md`. Do not run `make format`
   or any repository-wide formatting command. Then run the task checks.
6. Report the diff and evidence without staging or committing.

### Acceptance criteria

- The example visibly allocates the correctly sized destination after `Probe` and
  before `Reserve`.
- The example calls `DecodeRGBA` only after `Reserve`.
- The surrounding prose describes separate wasm scratch and caller-owned `Pix`;
  it does not call the conversion in-place.
- No API guarantees are removed or broadened.
- Only `README.md` changes during this task.

### Worker verification

```sh
make format-check
```

Also inspect the rendered code sequence and search the changed paragraph for the
misleading phrase `in-place`.

### Reviewer checklist

- Confirm the example has the exact required call order.
- Confirm the destination dimensions derive from `Probe` metadata.
- Confirm the wording distinguishes guest scratch from caller-owned destination
  memory.
- Confirm ownership, reusable-buffer, error, and zero-allocation statements remain
  accurate.
- Generate `task2.after.sha256`, compare it with `task2.before.sha256`, and diff
  the saved Task 2 `README.md` against its current contents. Confirm no file
  other than `README.md` changed during Task 2.
- Verify formatting evidence, then approve or reject with exact evidence.

Do not start Task 3 until the reviewer approves Task 2.

## Task 3 — Final proof gate

### Worker instructions

1. Do not edit files unless a required gate exposes an in-scope defect. If that
   happens, stop and report it for a return to the applicable task.
2. Run every final command in order.
3. Record the allocation count and benchmark allocation columns explicitly.
4. Verify repository scope and protected artifacts.
5. Report the final uncommitted working-tree state. Do not stage or commit.

### Final commands

```sh
make format-check
make lint
make test
make test-race
go build ./...
go test -run '^TestIntegrationDecodeRGBA_AllocsPerRun$' -count=1 -v
go test -run '^$' -bench '^BenchmarkDecodeRGBA_PNG_BricksColor$' -benchmem -count=1
git status --short
git diff --stat
git diff -- internal/wuffswasm/wuffs.go wasm/wuffs.wasm wasm/shim.c
git branch --show-current
git rev-parse HEAD
test "$(git branch --show-current)" = "$(cat "$snapshot_dir/initial.branch")"
test "$(git rev-parse HEAD)" = "$(cat "$snapshot_dir/initial.head")"
git diff --cached --quiet
git status --short > "$snapshot_dir/final.status"
while IFS= read -r status_line; do
  index_status="${status_line:0:1}"
  worktree_status="${status_line:1:1}"
  if { [ "$index_status" != ' ' ] && [ "$index_status" != '?' ]; } ||
    { [ "$index_status" = ' ' ] && [ "$worktree_status" = 'A' ]; }; then
    exit 1
  fi
done < "$snapshot_dir/final.status"
find . -path './.git' -prune -o -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > "$snapshot_dir/final.sha256"
find . -path './.git' -prune \
  -o -path './decoder.go' -prune \
  -o -path './decoder_test.go' -prune \
  -o -path './README.md' -prune \
  -o -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > "$snapshot_dir/final.protected.sha256"
diff -u "$snapshot_dir/initial.protected.sha256" "$snapshot_dir/final.protected.sha256"
if diff -u "$snapshot_dir/task1.before.sha256" "$snapshot_dir/final.sha256" > "$snapshot_dir/final.manifest.diff"; then
  false
else
  manifest_diff_status=$?
  test "$manifest_diff_status" -eq 1
fi
cat "$snapshot_dir/final.manifest.diff"
```

The protected-manifest `diff` is the scope gate and must return zero: it fails on
any content change outside the three allowed files. The full-manifest comparison
must return exactly one because the three allowed files changed; its captured
output is review evidence, not a failing gate. A return greater than one is a
command error and fails the compound command, while a return of zero also fails
because it means the expected corrections are absent.

### Final acceptance criteria

- Every formatting, lint, test, race, and build command passes.
- `TestIntegrationDecodeRGBA_AllocsPerRun` reports zero allocations.
- `BenchmarkDecodeRGBA_PNG_BricksColor` reports `0 B/op` and `0 allocs/op`.
- The only new corrections relative to the pre-plan working tree are in
  `decoder.go`, `decoder_test.go`, and `README.md`, as proved by the final
  manifest comparison against `task1.before.sha256`.
- `internal/wuffswasm/wuffs.go`, `wasm/wuffs.wasm`, and `wasm/shim.c` have no diff.
- No remaining `API.md` work, guest regeneration, auto-grow correction, or unrelated
  cleanup is included.
- Nothing is staged or committed.
- Both the cached diff and the porcelain status index checks prove that no staged
  or intent-to-add entry exists at the initial or final boundary.
- The final branch exactly matches `initial.branch`, the final `HEAD` exactly
  matches `initial.head`, and the branch remains
  `fix-decode-rgba-pix-ownership`.

### Reviewer checklist

- Review the complete uncommitted diff against this plan, `HANDOVER.md`, and the
  original caller-owned `Pix` requirements.
- Confirm every previously identified deviation is corrected exactly once.
- Confirm the original approved behavior and zero-allocation evidence remain intact.
- Compare `final.sha256` with `task1.before.sha256`; confirm the only workspace
  content changes are `decoder.go`, `decoder_test.go`, and `README.md`, inspect
  the captured expected full-manifest diff, and require the protected-manifest
  comparison to pass. Confirm protected artifacts are clean.
- Confirm the branch remains `fix-decode-rgba-pix-ownership` and the work remains
  uncommitted by verifying both final comparison commands against
  `initial.branch` and `initial.head`, plus both the cached-diff and porcelain
  status index checks.
- Approve only if every final command and acceptance criterion passes.

## Completion report

After final reviewer approval, report:

- branch name;
- each task's approval status;
- files changed by this correction plan;
- final gate results;
- allocation-test result;
- benchmark `B/op` and `allocs/op`;
- confirmation that generated artifacts are unchanged; and
- confirmation that nothing was staged or committed.

Stop after reporting. Do not commit, push, merge, or begin unrelated API work.
