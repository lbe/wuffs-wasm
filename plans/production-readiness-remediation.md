# Production Readiness Remediation

## Objective

Correct the three confirmed production defects:

1. Eliminate stale full-canvas pixels from the still-image guest decode path.
2. Convert Wuffs frame durations without integer overflow.
3. Reduce a new decoder's generated WASM linear-memory baseline to 32 MiB while preserving a 24 MiB host-slot floor and the 256 MiB maximum.

The execution branch is `fix/production-readiness`. The user authorized the worker to create this branch from `main` before Task 1.

## Fixed decisions

- Set `--initial-memory` to `33554432` bytes (32 MiB).
- Set `hostSlotRegionBase` to `0x01800000` (24 MiB).
- Clear the entire guest destination canvas in `decode_image` after destination pixel-buffer construction and before `decode_frame`.
- Preserve fractional flicks during duration conversion.
- Return `ErrDecode` from `DecodeFrame` when a guest duration cannot fit `time.Duration`.
- Regenerate and commit `wasm/wuffs.wasm` and `internal/wuffswasm/wuffs.go` after every guest or linker-input change.
- Update code comments, `README.md`, `API.md`, `doc.go`, and `docs/DEVELOPMENT.md`, including its Mermaid diagram.

## Execution model

The Pi orchestrator uses only the preconfigured `worker` and `reviewer` subagents. The orchestrator performs no repository edits, formatting, builds, tests, commits, or quick fixes. Every worker and reviewer dispatch includes the standing constraints from `plans/pi-runbook.md`, the task whitelist, the exact Verify block, and the executor STOP gates below.

Each worker writes its exact verification command list and results to `tmp/pi_progress.md`. Each reviewer verifies the task diff, the test artifact, documentation artifact, generated artifact where listed, and the proposed commit message. The reviewer approves before the worker commits. The worker commits one reviewed task with `git commit -F tmp/commit_message.txt` and reports the hash and complete commit message.

## Task 0 — Create the execution branch and commit this plan

**Role:** worker, then reviewer, then worker commit.

**Exact approved commit subject:** `docs(plan): create execution branch and commit plan`

**Whitelist:**

- `plans/production-readiness-remediation.md`
- `tmp/pi_progress.md`
- `tmp/commit_message.txt`

1. Confirm that the current branch is `main`, `git status --short` contains only `?? plans/production-readiness-remediation.md`, and `fix/production-readiness` does not exist.
2. Run `git switch -c fix/production-readiness main`.
3. Write `tmp/pi_progress.md` with the branch name, selected plan path, Task 0 completion, and Task 1 as next.
4. Stage the unmodified selected plan and write a proposed commit message to `tmp/commit_message.txt` whose subject is exactly `docs(plan): create execution branch and commit plan`.
5. Stop for reviewer approval.

**Reviewer work:**

1. Run the Plan validation artifact commands below and record each command, exit status, and output in the Task 0 reviewer report.
2. Approve only when every `test -f` command exits 0, the forbidden-word scan exits 1 with no output, `git check-ignore -v plans/production-readiness-remediation.md` exits 1 with no output, and the proposed commit-message subject is exactly `docs(plan): create execution branch and commit plan`.

**Acceptance criteria:**

- `git branch --show-current` prints `fix/production-readiness`.
- The only staged repository file is `plans/production-readiness-remediation.md`.
- `tmp/pi_progress.md` identifies this plan and Task 1.
- The first commit records the exact selected plan before production work begins.

**Verify:**

```text
git branch --show-current
git status --short
test -f tmp/pi_progress.md
git diff --check
git diff --cached --name-only
```

FORBIDDEN before commit: make test, make test-race, make cover, go test ./..., or any test command not listed above.

## Task 1 — Establish the 32 MiB generated memory baseline

**Role:** worker, then reviewer, then worker commit.

**Exact approved commit subject:** `fix(memory): establish 32 MiB generated memory baseline`

**Whitelist:**

- `wasm/build.sh`
- `wasm/build/wuffs.o`
- `wasm/build/wuffs.raw.wasm`
- `memory.go`
- `memory_test.go`
- `wasm/wuffs.wasm`
- `internal/wuffswasm/wuffs.go`
- `README.md`
- `API.md`
- `doc.go`
- `docs/DEVELOPMENT.md`
- `tmp/pi_progress.md`
- `tmp/commit_message.txt`

**RED:**

1. Add an assertion in `TestUnitMemoryLayout` that a new decoder has exactly `32*1024*1024` bytes of guest linear memory and that its default slots end within that memory.
2. Add assertions that `hostSlotRegionBase` equals `0x01800000` and that the default slot base is above the 24 MiB floor.
3. Run the scoped test and record the expected baseline-memory failure in `tmp/pi_progress.md`.

**GREEN:**

1. Change `wasm/build.sh` to link with `--initial-memory=33554432`; preserve the 8 MiB stack and `--max-memory=268435456`.
2. Change `hostSlotRegionBase` to `0x01800000` and update its comment with the 24 MiB guest-allocation floor rationale.
3. Run `make generate`; do not hand-edit generated artifacts.
4. Replace the existing `TestUnitMemoryLayout` assertion and comment requiring `layout.hostBase >= 0x2000000` (32 MiB) with an assertion that `hostSlotRegionBase == 0x01800000` (24 MiB), then update the surrounding `memory_test.go` comments to state the 32 MiB baseline and prove that a `Reserve(32*1024*1024, 2*1024*1024)` call grows memory.
5. Update `README.md`, `API.md`, and `doc.go` to state that `New` starts with 32 MiB guest linear memory, `Reserve` grows it up to the 256 MiB generated maximum, and a `Decoder` remains single-goroutine.
6. Update `docs/DEVELOPMENT.md` build and module-layout text with the 32 MiB initial memory and 24 MiB host-slot floor. Update the Mermaid diagram to show guest linear memory, the 24 MiB host-slot floor, and `Reserve` growth toward the 256 MiB maximum.
7. Run the scoped test until green, then format the edited files.
8. Write a proposed commit message to `tmp/commit_message.txt` whose subject is exactly `fix(memory): establish 32 MiB generated memory baseline`.

**Reviewer work:**

1. Approve only when the proposed commit-message subject is exactly `fix(memory): establish 32 MiB generated memory baseline`.

**Acceptance criteria:**

- A newly generated module allocates exactly 32 MiB in `internal/wuffswasm/wuffs.go`.
- Default slots lie within new-decoder linear memory and begin at or above 24 MiB.
- `Reserve` grows memory for the stated 32 MiB destination request.
- The generated WASM and Go binding are committed together with their linker input.
- All affected code comments and Markdown documents state the same memory contract.

**Verify:**

```text
go test -run '^TestUnitMemoryLayout$' -count=1 .
go build ./...
make format-check
git diff --check
git diff -- wasm/build.sh memory.go memory_test.go wasm/wuffs.wasm internal/wuffswasm/wuffs.go README.md API.md doc.go docs/DEVELOPMENT.md
```

FORBIDDEN before commit: make test, make test-race, make cover, go test ./..., or any test command not listed above.

## Task 2 — Clear still-image guest scratch before decode

**Role:** worker, then reviewer, then worker commit.

**Exact approved commit subject:** `fix(decode): clear still-image guest scratch before decode`

**Whitelist:**

- `wasm/shim.c`
- `wasm/build/wuffs.o`
- `wasm/build/wuffs.raw.wasm`
- `wasm/wuffs.wasm`
- `internal/wuffswasm/wuffs.go`
- `decode_nrgba_integration_test.go`
- `decode_gray_integration_test.go`
- `README.md`
- `API.md`
- `doc.go`
- `docs/DEVELOPMENT.md`
- `tmp/pi_progress.md`
- `tmp/commit_message.txt`

**RED:**

1. In `decode_nrgba_integration_test.go`, add self-contained `encodeOpaqueWhitePNG(t)` and partial-GIF encoder helpers. `encodeOpaqueWhitePNG(t)` must use the standard-library `image/png.Encode` to encode a 3×3 `image.NRGBA` whose every pixel is opaque white. The GIF helper must create a 3×3 GIF with a 1×1 opaque-red first-frame rectangle at `(1,1)` and transparent pixels outside that rectangle.
2. Add `TestIntegrationDecodeNRGBAReusedDecoderClearsPartialCanvas`. Decode the `encodeOpaqueWhitePNG(t)` result through one decoder and caller-owned 3×3 NRGBA destination, then decode the partial GIF through that same decoder and destination; separately decode the partial GIF through a fresh decoder and fresh 3×3 NRGBA destination. Assert transparent black at `(0,0)`, opaque red at `(1,1)`, and unchanged `Rect`, `Stride`, and `Pix` backing pointer for the reused destination. Assert whole-value metadata equality with `if *reusedMeta != *freshMeta`; `Meta` contains only comparable scalar fields.
3. Add `TestIntegrationDecodeGrayReusedDecoderClearsPartialCanvas` with the same named PNG input, partial GIF, reuse sequence, and a fresh decoder/fresh 3×3 Gray destination. Assert zero gray at `(0,0)`, the `color.GrayModel` conversion of opaque red at `(1,1)`, and unchanged `Rect`, `Stride`, and `Pix` backing pointer for the reused destination. Assert whole-value metadata equality with `if *reusedMeta != *freshMeta`.
4. Run the two scoped tests and record their stale-pixel failures in `tmp/pi_progress.md`.

**GREEN:**

1. In `decode_image`, clear `need` bytes at `dst_ptr` after `wuffs_base__pixel_buffer__set_from_slice` succeeds and before `decode_frame` executes.
2. Add a shim comment that explains why still-image decode clears full-canvas scratch and why `decode_frame` already does the same.
3. Run `make generate`; do not hand-edit generated artifacts.
4. Update `README.md`, `API.md`, and `doc.go` to state that successful `DecodeNRGBA` and `DecodeGray` calls populate their entire decoded canvas and do not retain pixels from an earlier decoder use. Do not change the documented `DecodeRGBA` frame-bounds semantics.
5. Update `docs/DEVELOPMENT.md` module-layout text and Mermaid diagram to show that `wuffs_decode_image` clears full-canvas scratch before frame decode.
6. Run the scoped tests until green, then format the edited files.
7. Write a proposed commit message to `tmp/commit_message.txt` whose subject is exactly `fix(decode): clear still-image guest scratch before decode`.

**Reviewer work:**

1. Approve only when the proposed commit-message subject is exactly `fix(decode): clear still-image guest scratch before decode`.

**Acceptance criteria:**

- Reusing a decoder after an opaque image cannot leak prior pixels into partial GIF output from `DecodeNRGBA` or `DecodeGray`.
- A fresh decoder and reused decoder produce identical pixels and metadata for the generated partial GIF.
- Successful `DecodeNRGBA` and `DecodeGray` calls populate their entire decoded canvas; `DecodeRGBA` retains its documented frame-bounds copy semantics.
- The shim, generated WASM, and generated Go binding are committed together.
- Documentation and the Mermaid diagram describe full-canvas initialization.

**Verify:**

```text
go test -run '^TestIntegrationDecodeNRGBAReusedDecoderClearsPartialCanvas$|^TestIntegrationDecodeGrayReusedDecoderClearsPartialCanvas$' -count=1 .
go build ./...
make format-check
git diff --check
git diff -- wasm/shim.c wasm/wuffs.wasm internal/wuffswasm/wuffs.go decode_nrgba_integration_test.go decode_gray_integration_test.go README.md API.md doc.go docs/DEVELOPMENT.md
```

FORBIDDEN before commit: make test, make test-race, make cover, go test ./..., or any test command not listed above.

## Task 3 — Convert frame duration without overflow

**Role:** worker, then reviewer, then worker commit.

**Exact approved commit subject:** `fix(frame): convert frame duration without overflow`

**Whitelist:**

- `decoder.go`
- `frame.go`
- `errors.go`
- `decoder_internal_test.go`
- `decode_frame_integration_test.go`
- `README.md`
- `API.md`
- `doc.go`
- `docs/DEVELOPMENT.md`
- `tmp/pi_progress.md`
- `tmp/commit_message.txt`

**RED:**

1. Add package-internal table-driven tests for a frame-meta duration helper that construct frame-meta bytes with `binary.LittleEndian`.
2. Cover zero flicks, one flick, 100 ms, 20 s, GIF’s 655.35 s maximum delay, the greatest representable `time.Duration` in flicks, and the first unrepresentable flick.
3. Assert exact truncation to nanoseconds for every representable input and `ErrDecode` for the first unrepresentable input in the private duration-helper test only.
4. Add an integration test that encodes a GIF with a 20-second delay and asserts that `DecodeFrame` returns exactly `20*time.Second`.
5. [TASK3-DURATION-OVERFLOW][severity: blocker] Add `TestIntegrationDecodeFrameDurationOverflow` in package `wuffs` that builds a 1×1 single-frame NIA source with the existing `niaEncodeRGBA` test helper, overwrites its little-endian CDD at `src[16:24]` with the private helper test’s first-unrepresentable flick count, calls `Probe` and `Reserve`, seeds `d.lastMeta` and `d.lastFrame` with distinct sentinel values, then calls the public `DecodeFrame`; expect `errors.Is(err, ErrDecode)` with a nil frame and prove both sentinels remain unchanged.
6. Run the scoped tests and record the overflowing 20-second result in `tmp/pi_progress.md`.

**GREEN:**

1. Add a single private duration conversion helper that separates whole seconds from the flick remainder, converts the remainder with unsigned arithmetic, checks the final nanosecond total against `math.MaxInt64`, and returns `ErrDecode` for an unrepresentable result.
2. Change frame-meta parsing and `DecodeFrame` to propagate that error before copying pixels or publishing `lastMeta` and `lastFrame`.
3. Update `Frame.Duration` GoDoc, `DecodeFrame` GoDoc, `ErrDecode` GoDoc, `README.md`, `API.md`, `doc.go`, and `docs/DEVELOPMENT.md` to state flick precision, nanosecond truncation, and `ErrDecode` for a duration outside `time.Duration`.
4. Update the Mermaid diagram in `docs/DEVELOPMENT.md` to show frame metadata passing through host-side checked duration conversion before `DecodeFrame` returns `Frame`.
5. Run the scoped tests until green, then format the edited files.
6. Write a proposed commit message to `tmp/commit_message.txt` whose subject is exactly `fix(frame): convert frame duration without overflow`.

**Reviewer work:**

1. Approve only when the proposed commit-message subject is exactly `fix(frame): convert frame duration without overflow`.

**Acceptance criteria:**

- A valid 20-second GIF delay returns `20*time.Second`.
- Fractional flicks retain their nanosecond-truncated remainder.
- The greatest representable duration succeeds.
- The public `DecodeFrame` path returns `ErrDecode` for the first unrepresentable flick and leaves both `lastMeta` and `lastFrame` unpublished.
- Every affected public contract and diagram states the overflow behavior.

**Verify:**

```text
go test -run '^TestUnitFrameMetaDuration$|^TestIntegrationDecodeFrameTwentySecondGIFDuration$|^TestIntegrationDecodeFrameDurationOverflow$' -count=1 .
go build ./...
make format-check
git diff --check
git diff -- decoder.go frame.go errors.go decoder_internal_test.go decode_frame_integration_test.go README.md API.md doc.go docs/DEVELOPMENT.md
```

FORBIDDEN before commit: make test, make test-race, make cover, go test ./..., or any test command not listed above.

## Task 4 — Run final production verification

**Role:** worker, then reviewer.

**Whitelist:** `coverage.out`, `coverage.html`, `tmp/pi_progress.md`, `tmp/production-readiness-verification.md`.

1. Run the following command. It must reject every current branch except `fix/production-readiness`, fail unless `main..HEAD` contains exactly four commits, require the ordered commits to have the exact approved subjects below, and write the permitted `tmp/production-readiness-verification.md` artifact. The artifact must contain one ordered task record per commit with the task number and label, commit hash, exact subject, and every changed file.

   - Task 0 — `docs(plan): create execution branch and commit plan`
   - Task 1 — `fix(memory): establish 32 MiB generated memory baseline`
   - Task 2 — `fix(decode): clear still-image guest scratch before decode`
   - Task 3 — `fix(frame): convert frame duration without overflow`

```sh
set -eu
branch="$(git branch --show-current)"
test "$branch" = fix/production-readiness
commit_count="$(git rev-list --count main..HEAD)"
test "$commit_count" -eq 4
set -- $(git rev-list --reverse main..HEAD)
test "$#" -eq 4

{
	printf 'branch: %s\n' "$branch"
	printf 'main..HEAD commit count: %s\n' "$commit_count"
} > tmp/production-readiness-verification.md

task_number=0
for expected_subject in \
	'docs(plan): create execution branch and commit plan' \
	'fix(memory): establish 32 MiB generated memory baseline' \
	'fix(decode): clear still-image guest scratch before decode' \
	'fix(frame): convert frame duration without overflow'
do
	commit="$1"
	shift
	test "$(git log -1 --format=%s "$commit")" = "$expected_subject"
	{
		printf 'task %s: %s\n' "$task_number" "$expected_subject"
		printf 'commit: %s\n' "$commit"
		printf 'subject: %s\n' "$expected_subject"
		printf 'changed files:\n'
		git diff-tree --no-commit-id --name-only -r "$commit"
	} >> tmp/production-readiness-verification.md
	task_number=$((task_number + 1))
done
```

After the command succeeds, append the exact history-validation command above and `history-validation exit status: 0` to `tmp/production-readiness-verification.md`.

2. Confirm that `git status --short` has no output, then run the complete repository checks.
3. Append each exact repository-check command, its exit status, and the resulting test-package summaries to `tmp/production-readiness-verification.md`.
4. Update `tmp/pi_progress.md` with final completion and the verification artifact path.
5. Stop without a commit, push, or merge.

**Reviewer work:**

1. Inspect `tmp/production-readiness-verification.md`. Reject unless it identifies `fix/production-readiness`, exactly four `main..HEAD` commits, and four ordered records labeled Task 0 through Task 3.
2. Compare each ordered record's subject against its exact approved conventional-commit subject: Task 0 `docs(plan): create execution branch and commit plan`; Task 1 `fix(memory): establish 32 MiB generated memory baseline`; Task 2 `fix(decode): clear still-image guest scratch before decode`; Task 3 `fix(frame): convert frame duration without overflow`.
3. Compare every changed file in each record with that task's existing whitelist; reject any file outside it. Require the following essential files in the corresponding task record:

   - Task 0: `plans/production-readiness-remediation.md`.
   - Task 1: `wasm/build.sh`, `memory.go`, `memory_test.go`, `wasm/wuffs.wasm`, `internal/wuffswasm/wuffs.go`, `README.md`, `API.md`, `doc.go`, and `docs/DEVELOPMENT.md`.
   - Task 2: `wasm/shim.c`, `wasm/wuffs.wasm`, `internal/wuffswasm/wuffs.go`, `decode_nrgba_integration_test.go`, `decode_gray_integration_test.go`, `README.md`, `API.md`, `doc.go`, and `docs/DEVELOPMENT.md`.
   - Task 3: `decoder.go`, `frame.go`, `errors.go`, `decoder_internal_test.go`, `decode_frame_integration_test.go`, `README.md`, `API.md`, `doc.go`, and `docs/DEVELOPMENT.md`.
4. Approve only when each commit has the required exact subject and essential files, every changed file is permitted by its task whitelist, and no scope escape is present.

**Acceptance criteria:**

- The complete suite, race suite, coverage suite, fuzz seed suite, lint, formatting, and build succeed.
- The verification artifact names each command and exit status.
- The reviewer confirms the ordered four commits have the exact approved subjects, required essential files, Task 0–3 whitelist compliance, and no scope escapes.

**Verify:**

```text
go build ./...
make format-check
make lint
make test
make test-race
make cover
make test-all
test -f tmp/production-readiness-verification.md
test "$(git branch --show-current)" = fix/production-readiness
test "$(git rev-list --count main..HEAD)" -eq 4
git log --reverse --format='commit %H%nsubject %s%nchanged files:' --name-only main..HEAD
git status --short
```

## Executor STOP gates (mandatory)

**Global rules:** `plans/pi-runbook.md` standing constraint 13; **Plan executor STOP (zero deviation)** in the same runbook.

Executors **must not** edit this plan file. On any trigger below or any global rule: **STOP**, message the user with task id and evidence, make no commit, and start no next task until written user direction.

| # | Trigger                                                                                                                                       | Executor action     |
| - | --------------------------------------------------------------------------------------------------------------------------------------------- | ------------------- |
| 1 | Task 0 finds a `main` worktree state other than the untracked selected plan or finds an existing `fix/production-readiness` branch.           | STOP — notify user. |
| 2 | `make generate` cannot produce both tracked generated artifacts from the listed linker and shim sources.                                      | STOP — notify user. |
| 3 | A 32 MiB generated module places a default host slot outside linear memory or below the 24 MiB floor.                                         | STOP — notify user. |
| 4 | The partial-GIF regression test cannot reproduce a fresh-versus-reused decoder pixel difference before the shim change.                       | STOP — notify user. |
| 5 | A duration test contradicts Wuffs flick values, `time.Duration`, or the stated `ErrDecode` boundary.                                          | STOP — notify user. |
| 6 | A required file is outside its task whitelist, a Verify command fails outside the stated RED result, or a reviewer rejects scope or evidence. | STOP — notify user. |
| 7 | A required tool or command is unavailable.                                                                                                    | STOP — notify user. |

## Plan validation artifact

Task 0 reviewer work must run and record these commands in the Task 0 reviewer report:

```text
test -f AGENTS.md
test -f GOALS.md
test -f API.md
test -f HANDOVER.md
test -f plans/pi-runbook.md
rg -i '\boptional\b|if[ ]needed|prefer[ ]|either[ ]' plans/production-readiness-remediation.md
git check-ignore -v plans/production-readiness-remediation.md
```

Each `test -f` command must exit 0. The regular-expression command must exit 1 with no output. `git check-ignore` must exit 1 with no output.

## Execution prompt (for Cursor Agent)

```text
Execute plans/production-readiness-remediation.md Tasks 0–4 in order.
Follow plans/pi-runbook.md (test budget, commits, format).
Obey this plan's ## Executor STOP gates (mandatory) and pi-runbook standing constraint 13.
Progress file: tmp/pi_progress.md.
Do not push unless I instruct.
Begin with Task 0.
```
