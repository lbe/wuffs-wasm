# CONV-01 Adherence Remediation

**Execution protocol:** `plans/pi-runbook.md`

**Execution branch:** `conv-01-allocating-helpers`

**Commit authorized:** Yes — one reviewed commit per task

**Purpose:** Close the verification gaps found after the original six-cycle
CONV-01 execution. Preserve the implemented public behavior while replacing the
inadequate concrete-decoder test seam, completing the missing package-level
`Probe` and `DecodeConfig` proofs, and enforcing the complete RGBA, NRGBA, and
Gray allocating-pipeline contract.

This is a bounded host-only adherence remediation. It does not authorize new
public API, changed pixel semantics, reader adapters, image registration,
additional formats, guest changes, generated artifacts, or documentation
changes.

---

## Sources of truth

Read before acting:

1. `AGENTS.md`
2. The user's latest instruction
3. `plans/pi-runbook.md`
4. This plan
5. `.pi/tdd-plans/conv-01-allocating-helpers.yaml`
6. `plans/api-roadmap.md`, especially `CONV-01`
7. `GOALS.md`
8. `API.md`
9. `HANDOVER.md`, only for caller-owned destination and host-only constraints

The approved TDD plan defines the original acceptance contract. This tracked
plan defines the bounded remediation needed to satisfy it. If either conflicts
with `AGENTS.md`, the user's latest instruction, `GOALS.md`, or the implemented
public contract in `API.md`, stop and report the conflict.

Do not treat any acceptance item as advisory or defer it to another workstream.

---

## Confirmed adherence gaps

The remediation must close every item below:

1. Package `Probe` lacks direct PNG, lossless WebP, large-input, exact-error,
   exact-reservation, no-destination-work, and direct Meta-detachment tests.
2. `DecodeConfig` lacks deterministic source-Reserve failure, exact error
   identity, exact reservation, and short-circuit tests.
3. RGBA lacks deterministic destination-Reserve, Probe-error, unsafe-metadata,
   primitive-decode-error, exact-reservation, call-order, and direct
   Meta-detachment tests.
4. NRGBA and Gray tests named as destination-Reserve failures actually force the
   first source Reserve to fail.
5. NRGBA lacks direct Meta-detachment, unsafe-metadata,
   primitive-decode-error, and exact-reservation tests.
6. Gray lacks unsafe-metadata and primitive-decode-error tests; its current
   reservation test observes final capacity rather than exact calls.
7. The current `func() *Decoder` factory cannot provide a fake that records or
   injects `Reserve`, `Probe`, and typed decode behavior.
8. Some error tests use `errors.Is` where the approved plan requires exact
   identity.

The existing PNG, lossless WebP, alpha-pixel, large-input, independent-storage,
public API inventory, and reusable zero-allocation tests remain required and
must not be weakened.

---

## Execution control

- Use only the preconfigured Pi subagents `worker` and `reviewer` through
  `plans/pi-runbook.md`.
- The orchestrator only orchestrates. It does not edit implementation files,
  run repository tests, stage, commit, amend, or apply fixes.
- This plan and the `CONV-01` roadmap preparation must already be committed
  before execution.
- The roadmap must name this plan and show `CONV-01` as `Ready`. Otherwise stop;
  roadmap preparation is outside Pi execution.
- The current branch must be exactly `conv-01-allocating-helpers`; do not create
  another branch.
- Require a clean worktree and empty index before Task 1.
- Do not amend, rebase, reset, squash, cherry-pick, or otherwise rewrite the
  existing CONV-01 history.
- Do not push, merge, or modify `main`.
- Do not use Python, `git add -f`, or `git commit --no-verify`.
- Do not run `make generate`.
- Do not install tools or modify system configuration.
- Complete and review each task before starting the next task.
- One approved task produces one new commit through the commit procedure in
  `plans/pi-runbook.md`.
- Each task permits at most three worker attempts total, including the initial
  attempt. A reviewer rejection may trigger another worker attempt only while
  fewer than three total attempts have occurred. Do not move omitted acceptance
  items into a later task.
- Do not add broad lint suppression, unused-code suppression, or linter-evasion
  helpers. A `//nolint:errorlint` directive is permitted only immediately on a
  deliberate exact-identity comparison and must justify why identity is the
  contract.
- Stop after Task 4 for the independent adherence gate. Task 5 requires a new,
  explicit user instruction after a clean adherence review.

### Preflight evidence

The first worker reports:

```bash
APPROVED_ORIGINAL_TDD_HEAD=c790ba95cd3197c7a0fd74e603015b78c810d4e1
APPROVED_ORIGINAL_TDD_BASE_HEAD=09ce6b67eda0c8073199bbe0c48a53d31b2eed6b
git branch --show-current
git rev-parse HEAD
git status --short
git diff --cached --name-only
git diff --name-only
git ls-files --error-unmatch plans/conv-01-adherence-remediation.md
git ls-files --error-unmatch plans/api-roadmap.md
git diff --exit-code HEAD -- plans/conv-01-adherence-remediation.md plans/api-roadmap.md
git rev-parse HEAD^
git diff-tree --no-commit-id --name-only -r HEAD
git rev-parse "${APPROVED_ORIGINAL_TDD_HEAD}^{commit}"
git rev-parse "${APPROVED_ORIGINAL_TDD_BASE_HEAD}^{commit}"
git merge-base "${APPROVED_ORIGINAL_TDD_BASE_HEAD}" "${APPROVED_ORIGINAL_TDD_HEAD}"
test "$(git rev-parse HEAD^)" = "${APPROVED_ORIGINAL_TDD_HEAD}"
test "$(git diff-tree --no-commit-id --name-only -r HEAD | sort)" = $'plans/api-roadmap.md\nplans/conv-01-adherence-remediation.md'
test "$(git merge-base "${APPROVED_ORIGINAL_TDD_BASE_HEAD}" "${APPROVED_ORIGINAL_TDD_HEAD}")" = "${APPROVED_ORIGINAL_TDD_BASE_HEAD}"
PREPARATION_HEAD="$(git rev-parse HEAD)"
ORIGINAL_TDD_HEAD="${APPROVED_ORIGINAL_TDD_HEAD}"
ORIGINAL_TDD_BASE_HEAD="${APPROVED_ORIGINAL_TDD_BASE_HEAD}"
git rev-list --count "${ORIGINAL_TDD_BASE_HEAD}..${ORIGINAL_TDD_HEAD}"
git rev-list --count "${ORIGINAL_TDD_BASE_HEAD}..${PREPARATION_HEAD}"
git rev-list --merges "${ORIGINAL_TDD_BASE_HEAD}..${PREPARATION_HEAD}"
tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner --format=gnu -cf - .pi | sha256sum
mkdir -p tmp/conv-01-adherence-remediation
ROADMAP_SNAPSHOT="$(mktemp tmp/conv-01-adherence-remediation/api-roadmap.pre-execution.XXXXXX.md)"
cp plans/api-roadmap.md "${ROADMAP_SNAPSHOT}"
chmod a-w "${ROADMAP_SNAPSHOT}"
sha256sum "${ROADMAP_SNAPSHOT}"
HISTORY_SNAPSHOT="$(mktemp tmp/conv-01-adherence-remediation/history.pre-execution.XXXXXX.txt)"
git log --reverse --format='%H %s' "${ORIGINAL_TDD_BASE_HEAD}..${PREPARATION_HEAD}" > "${HISTORY_SNAPSHOT}"
chmod a-w "${HISTORY_SNAPSHOT}"
sha256sum "${HISTORY_SNAPSHOT}"
```

The branch must be `conv-01-allocating-helpers`, the worktree and index must be
clean, and both preparation files must be tracked and match HEAD. The approved
immutable anchors are `c790ba95cd3197c7a0fd74e603015b78c810d4e1` for
`ORIGINAL_TDD_HEAD` and `09ce6b67eda0c8073199bbe0c48a53d31b2eed6b`
for `ORIGINAL_TDD_BASE_HEAD`; neither may be derived from current `HEAD`, a
relative revision, or the current `main`. Before deriving `PREPARATION_HEAD`,
prove that current `HEAD` has the approved original TDD HEAD as its exact parent
and that its complete changed-path set is exactly the tracked plan and roadmap
preparation files. The roadmap change must name this plan and mark `CONV-01`
`Ready`.

The range `ORIGINAL_TDD_BASE_HEAD..ORIGINAL_TDD_HEAD` must contain exactly the
18 approved TDD phase commits in their original order. The anchored range
`ORIGINAL_TDD_BASE_HEAD..PREPARATION_HEAD` must contain exactly 19 commits,
contain no merge commit, and end with the single tracked preparation commit.
Only after those parent and changed-path proofs pass, record current `HEAD` as
`PREPARATION_HEAD`, retain the two approved literals as `ORIGINAL_TDD_HEAD` and
`ORIGINAL_TDD_BASE_HEAD`, and record the exact ordered anchored history snapshot
path and digest as `BASELINE_HISTORY_SNAPSHOT` and
`BASELINE_HISTORY_SHA256`. Record the complete deterministic `.pi/` archive
digest as `BASELINE_PI_ARCHIVE_SHA256`, the roadmap snapshot path as
`BASELINE_ROADMAP_SNAPSHOT`, and its digest as `BASELINE_ROADMAP_SHA256`.

The roadmap and history snapshots are read-only execution evidence. No worker
may edit, replace, or remove them. After each task commit, record its exact HEAD
as `TASK_<N>_HEAD`; later history checks must match those hashes in order.

### Protected paths

These paths must remain unchanged from `PREPARATION_HEAD`:

- `decoder.go`
- `convert.go`
- `API.md`
- `README.md`
- `api_boundary_test.go`
- `internal/wuffswasm/`
- `wasm/`
- `scripts/`
- `.pi/`

The public API inventory, existing integration fixtures, generated artifacts,
guest code, reusable Decoder methods, and public documentation are already
correct. A requested change to a protected path is a scope conflict; stop and
report it.

### Allowed remediation paths

Only these paths may be added, changed, renamed, or removed during Tasks 1–4:

- `convenience.go`
- `convenience_test.go`
- `decoder_config_test.go`
- `decoder_decode_unit_test.go`
- `decoder_decode_nrgba_unit_test.go`
- `decoder_decode_gray_unit_test.go`
- `decode_integration_test.go` (test-comment corrections only)
- `decode_nrgba_integration_test.go` (test-comment corrections only)
- `decode_gray_integration_test.go` (test-comment corrections only)
- new root-package `_test.go` files used for the shared fake and contract tests
- `plans/api-roadmap.md`

Task 5 may change only `plans/api-roadmap.md`.

### Mandatory post-commit verification for Tasks 1–3

After the uncommitted reviewer approves a Task 1–3 diff and proposed message,
the worker stages only the approved task files through the runbook commit
procedure. Require no unstaged diff, record `git write-tree` as
`TASK_<N>_REVIEWED_TREE`, and commit normally with hooks enabled.

The worker then records `TASK_<N>_HEAD` and runs:

```bash
git rev-parse HEAD
git rev-parse HEAD^{tree}
git show -s --format=fuller HEAD
git show --check --stat --oneline HEAD
git diff-tree --no-commit-id --name-only -r HEAD
git status --short
git diff --cached --name-only
git diff --exit-code HEAD
git merge-base --is-ancestor <PREPARATION_HEAD> HEAD
git rev-list --count <PREPARATION_HEAD>..HEAD
git rev-list --reverse <PREPARATION_HEAD>..HEAD
git diff --exit-code <PREPARATION_HEAD>..HEAD -- decoder.go convert.go API.md README.md api_boundary_test.go internal/wuffswasm wasm scripts
tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner --format=gnu -cf - .pi | sha256sum
```

Substitute every recorded value literally. `HEAD` must equal `TASK_<N>_HEAD`,
its tree must equal `TASK_<N>_REVIEWED_TREE`, and the commit message must equal
the reviewed message. The range after `PREPARATION_HEAD` must contain exactly
the recorded Task 1 through Task N hashes in order and no other commit. The
worktree and index must be clean, proving that normal commit hooks neither
changed nor left uncommitted content. Protected paths and the complete `.pi/`
archive digest must remain unchanged.

Dispatch a fresh post-commit reviewer. It must independently verify the
committed tree, actual HEAD and message, hook-enabled commit procedure,
append-only task sequence, protected paths, complete `.pi/` digest, authorized
file scope, and clean worktree and index. The task is not approved, and the next
task must not begin, until this review passes.

---

## Required private seam

Replace the concrete factory dependency with one private interface implemented
by `*Decoder`. The exact unexported names may follow local style, but the seam
must provide these operations:

```go
Reserve(dstBytes, srcBytes int) error
Probe(src []byte) (*Meta, error)
DecodeRGBA(dst *image.RGBA, src []byte) (*Meta, error)
DecodeNRGBA(dst *image.NRGBA, src []byte) (*Meta, error)
DecodeGray(dst *image.Gray, src []byte) (*Meta, error)
```

Requirements:

- `*Decoder` satisfies the interface without modifying `decoder.go`.
- Package exports continue to create a fresh real Decoder per call.
- `probeWithDecoder`, `decodePrep`, and typed allocating helpers accept the
  private abstraction, not `func() *Decoder`.
- The public API and behavior remain unchanged.
- No mutable package-global hooks, factories, call logs, or test switches.
- Tests use one per-test fake or recorder instance. Tests may run in parallel.
- Existing per-call host allocator closures remain test seams used only to
  record allocation order and prove allocation was skipped. A nil-returning
  allocator is not a supported failure mode and must not be added as one.

### Recorder vocabulary

The shared test fake records ordered operations and Reserve arguments. Use one
consistent vocabulary across all helpers:

- `Reserve(0, len(src))`
- `Probe(src)`
- `Reserve(checkedGuestBytes, len(src))`
- typed host allocation
- typed decode

The fake must support unique per-test errors at source Reserve, Probe,
destination Reserve, and typed decode. Tests compare the returned error with the
injected value using exact identity (`got == want`), not only `errors.Is`.

---

## Task 1 — Repair the seam and prove package Probe

### Roadmap transition

Change only the `CONV-01` status from `Ready` to `In progress`. Preserve its
plan path, dependency, and all unrelated roadmap content.

### Implementation

1. Add the private decoder-operation interface and production factory described
   above.
2. Update package `Probe`, `DecodeConfig`, `decodePrep`, and the three allocating
   decode helpers to use it.
3. Preserve the existing sequence and returned values. This is a private
   testability refactor, not a behavior change.
4. Mechanically adapt existing RGBA, NRGBA, and Gray unit-test call sites to the
   new private abstraction so Task 1 compiles and remains green. Preserve their
   current assertions; Task 3 owns their contract-matrix consolidation and new
   typed-helper coverage.
5. Add the shared per-test recorder with the minimum fields needed by Task 1.
   It may implement the complete interface, but do not add unused later-task
   scaffolding. Follow the execution-control restriction for any deliberate
   exact-identity assertion that requires `//nolint:errorlint`.

### Package Probe tests

Add direct tests that prove:

1. Valid PNG and lossless WebP match `(*Decoder).Probe`, including Width,
   Height, Stride, Format, and `BytesWritten == 0`.
2. Valid encoded input larger than the default source slot succeeds.
3. Config-readable truncated PNG succeeds exactly when `(*Decoder).Probe`
   succeeds and returns matching metadata.
4. Empty, unknown-format, and non-config-readable truncated input return the
   exact error emitted by `(*Decoder).Probe` and nil Meta.
5. Every successful call records exactly
   `Reserve(0, len(src))` followed by `Probe(src)` and no other decoder or
   allocator operation.
6. Injected source-Reserve failure returns the exact injected error and records
   only `Reserve(0, len(src))`; Probe and every destination/decode/allocation
   operation are skipped.
7. Returned Meta is value-equal to the decoder's Meta but pointer-distinct.
   Mutating the decoder's `lastMeta` and wasm metadata memory does not change the
   returned value.
8. Repeated package calls return independent Meta pointers; mutating one does
   not affect another.
9. No destination slot growth or pixel decode occurs.

Name unit and integration tests according to their actual level.

### Focused verification

Run the new Probe unit and integration tests by exact test name, then:

```bash
go test -run '^TestUnitPublicAPIBoundary$' -count=1 -v .
go test -count=1 .
go build ./...
make format-check
make lint
git diff --check
```

### Task 1 acceptance criteria

- The private interface is implemented and used without public API changes.
- Tests use only per-test fake state and allocator closures.
- Every package Probe requirement above has a direct assertion.
- Exact call order, Reserve arguments, error identity, detachment, and
  short-circuit behavior are proven.
- Existing typed-helper unit tests compile and pass after only the permitted
  mechanical call-site adaptation.
- Existing tests, API inventory, formatting, lint, and build pass.
- `CONV-01` is `In progress`.
- Protected paths and the complete `.pi/` archive digest are unchanged.
- Reviewer approves the uncommitted diff and proposed message before the worker
  creates the Task 1 commit.
- The mandatory post-commit worker verification and fresh reviewer pass before
  Task 2 begins.

---

## Task 2 — Complete DecodeConfig contracts

Task 2 begins only after Task 1 mandatory post-commit worker verification and
fresh reviewer approval.

### Test remediation

Preserve the existing real PNG, lossless WebP, large-input, and truncated-input
integration coverage. Add or correct direct seam tests that prove:

1. A successful call records exactly
   `Reserve(0, len(src))` followed by `Probe(src)`.
2. Successful DecodeConfig performs no destination Reserve, host allocation, or
   pixel decode.
3. Injected source-Reserve failure returns zero `image.Config` and the exact
   injected error; Probe and every later operation are skipped.
4. Injected Probe failure returns zero `image.Config` and the exact injected
   error; every later operation is skipped.
5. Config-readable truncation succeeds with the same dimensions as package
   Probe.
6. Empty, unknown-format, and non-config-readable truncated input return zero
   config and exact primitive error identity. Replace `errors.Is` where this
   contract requires equality.
7. The returned successful config is
   `image.Config{ColorModel: color.RGBAModel, Width, Height}`.

Do not add another parser or decode pixels to obtain config.

### Focused verification

Run the DecodeConfig unit and integration tests by exact test name, then:

```bash
go test -run '^TestUnitPublicAPIBoundary$' -count=1 -v .
go test -count=1 .
go build ./...
make format-check
make lint
git diff --check
```

### Task 2 acceptance criteria

- Every DecodeConfig requirement has a direct assertion.
- Exact errors, exact call sequence, source-only reservation, and skipped work
  are proven through the shared per-test recorder.
- Existing package Probe and DecodeConfig integration behavior remains green.
- No production behavior or protected path changes.
- Reviewer approves the uncommitted diff and proposed message before the worker
  creates the Task 2 commit.
- The mandatory post-commit worker verification and fresh reviewer pass before
  Task 3 begins.

---

## Task 3 — Complete the typed decode contract matrix

Task 3 begins only after Task 2 mandatory post-commit worker verification and
fresh reviewer approval.

### Test organization

1. Build one shared typed contract matrix: one helper table containing RGBA,
   NRGBA, and Gray adapters, crossed with one common case table for the shared
   success, failure, ordering, reservation, allocator, and nil-output contracts.
   Every common case must execute through that same matrix for all three typed
   helpers. Three independent typed suites, even if structurally similar, do
   not satisfy this requirement.
2. Rewrite or remove the existing corruption-based tests when they duplicate or
   falsely claim destination-Reserve coverage.
3. Preserve all intended assertions. Reject any net loss of source-Reserve,
   Probe-error, typed-output, alpha, format, large-input, independent-storage,
   or reusable zero-allocation coverage.
4. Correct inaccurate test comments that claim coverage not present in that
   file. Do not change successful integration assertions merely for cleanup.

### Required shared typed contract matrix

Through the single shared matrix, prove each item independently for `Decode`,
`DecodeNRGBA`, and `DecodeGray`. Keep type-specific output, color-model, and
Meta assertions outside the common case table only where their types genuinely
differ:

1. **Success order:** exactly
   `Reserve(0, len(src))` → `Probe(src)` →
   `Reserve(checkedGuestBytes, len(src))` → typed allocation → typed decode.
2. **Guest size:** `checkedGuestBytes == Width*Height*4` for all three helpers,
   including Gray's one-byte host output.
3. **Source Reserve failure:** exact injected error, nil image, nil Meta, and no
   later operation.
4. **Probe failure:** exact injected error, nil image, nil Meta, and no
   destination Reserve, allocation, or typed decode.
5. **Unsafe metadata:** zero, overflowing, or otherwise unrepresentable geometry
   returns exactly `ErrBadImage`, nil image, and nil Meta before destination
   Reserve, allocation, or typed decode.
6. **Destination Reserve failure:** configure the fake to fail only its second
   Reserve call. Require the exact injected error, nil image, nil Meta, and no
   allocation or typed decode.
7. **Typed decode failure:** after both successful Reserve calls and allocation,
   return a unique injected typed-decode error. Require exact identity and nil
   image and Meta outputs.
8. **Meta detachment:** the returned Meta is value-equal to, but pointer-distinct
   from, the typed decode Meta. Mutating the captured real Decoder's `lastMeta`
   and wasm metadata memory does not change the returned Meta.
9. **Allocator use:** allocator injection records order and proves it was skipped
   after earlier failures. Do not define nil allocator output as a recoverable
   failure contract.

### Existing integration behavior that must remain green

- Tight RGBA, NRGBA, and Gray image layout.
- Exact alpha fixture pixels for RGBA and NRGBA.
- Exact `color.GrayModel` alpha-fixture output, including translucent and fully
  transparent pixels.
- PNG and lossless WebP dimensions and Format metadata.
- Large valid input succeeds through automatic source and destination Reserve.
- Independent image backing storage and Meta pointers across package calls.
- Gray guest stride and host `BytesWritten` semantics.
- Empty, unknown, and truncated failures return nil image and Meta.
- Reusable Decoder RGBA, NRGBA, and Gray paths retain zero allocations.

### Focused verification

Run the complete new unit matrix and the three allocating integration suites by
exact test name. Then run the existing reusable allocation tests:

```bash
go test -run '^(TestIntegrationDecodeRGBA_AllocsPerRun|TestIntegrationDecodeNRGBANoAllocs|TestIntegrationDecodeGrayNoAllocs)$' -count=1 -v .
go test -run '^TestUnitPublicAPIBoundary$' -count=1 -v .
go test -count=1 .
go build ./...
make format-check
make lint
git diff --check
```

### Task 3 acceptance criteria

- One shared helper-by-case matrix runs every common contract case for all three
  typed helpers; no helper-specific duplicate suite substitutes for it.
- Destination-Reserve tests demonstrably reach and fail the second Reserve call.
- Exact call tuples, stage ordering, exact errors, nil outputs, skipped work, and
  Meta detachment are directly asserted.
- Misleading corruption-based tests are corrected or removed without reducing
  required coverage.
- Existing integration and zero-allocation tests remain green.
- No public behavior, docs, guest, generated artifact, or protected path changes.
- Reviewer approves the uncommitted diff and proposed message before the worker
  creates the Task 3 commit.
- The mandatory post-commit worker verification and fresh reviewer pass before
  Task 4 begins.

---

## Task 4 — Record review state and final execution evidence

Task 4 begins only after Task 3 mandatory post-commit worker verification and
fresh reviewer approval.

### Roadmap transition

Change only the `CONV-01` status from `In progress` to `Review`.

### Pre-commit final execution gates

Run:

```bash
make format-check
make lint
make test
make test-race
go build ./...
go test -run '^TestUnitPublicAPIBoundary$' -count=1 -v .
go test -run '^(TestIntegrationDecodeRGBA_AllocsPerRun|TestIntegrationDecodeNRGBANoAllocs|TestIntegrationDecodeGrayNoAllocs)$' -count=1 -v .
go doc -all .
git diff --check
git status --short
git diff --cached --name-only
git diff --exit-code PREPARATION_HEAD..HEAD -- decoder.go convert.go API.md README.md api_boundary_test.go internal/wuffswasm wasm scripts
tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner --format=gnu -cf - .pi | sha256sum
sha256sum <BASELINE_ROADMAP_SNAPSHOT>
git diff --no-index --no-ext-diff -- <BASELINE_ROADMAP_SNAPSHOT> plans/api-roadmap.md
```

Substitute the recorded preparation hash and snapshot path literally; do not
use unresolved placeholders in dispatched commands. The `.pi/` digest must
equal `BASELINE_PI_ARCHIVE_SHA256`.

The roadmap snapshot diff may contain only the `CONV-01` status change from
snapshotted `Ready` to `Review`. Any plan-path, dependency, scope, or unrelated
roadmap change is rejection evidence. `git diff --no-index` status 1 is expected
for the authorized status difference.

`make cover` is not required. Direct contract assertions, full tests, and the
race suite are the required evidence.

After the reviewer approves the uncommitted diff and proposed message, the
worker stages only the approved Task 4 change through the runbook commit
procedure. Require no unstaged diff, record `git write-tree` as
`TASK_4_REVIEWED_TREE`, and commit normally with hooks enabled.

### Mandatory post-commit verification

The worker records `TASK_4_HEAD` and runs the following after the commit:

```bash
git rev-parse HEAD
git rev-parse HEAD^{tree}
git show -s --format=fuller HEAD
git show --check --stat --oneline HEAD
git diff-tree --no-commit-id --name-only -r HEAD
git status --short
git diff --cached --name-only
git diff --exit-code HEAD
git merge-base --is-ancestor <PREPARATION_HEAD> HEAD
git rev-parse <PREPARATION_HEAD>^
git rev-list --count <ORIGINAL_TDD_BASE_HEAD>..<ORIGINAL_TDD_HEAD>
git rev-list --count <ORIGINAL_TDD_BASE_HEAD>..<PREPARATION_HEAD>
git rev-list --merges <ORIGINAL_TDD_BASE_HEAD>..<PREPARATION_HEAD>
git log --reverse --format='%H %s' <ORIGINAL_TDD_BASE_HEAD>..<PREPARATION_HEAD> > tmp/conv-01-adherence-remediation/history.task4.txt
cmp <BASELINE_HISTORY_SNAPSHOT> tmp/conv-01-adherence-remediation/history.task4.txt
git rev-list --count <PREPARATION_HEAD>..HEAD
git rev-list --reverse <PREPARATION_HEAD>..HEAD
git diff --exit-code <PREPARATION_HEAD>..HEAD -- decoder.go convert.go API.md README.md api_boundary_test.go internal/wuffswasm wasm scripts
tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner --format=gnu -cf - .pi | sha256sum
sha256sum <BASELINE_ROADMAP_SNAPSHOT>
git diff --no-index --no-ext-diff -- <BASELINE_ROADMAP_SNAPSHOT> plans/api-roadmap.md
```

Substitute every recorded value literally. `HEAD` must equal `TASK_4_HEAD`, its
tree must equal `TASK_4_REVIEWED_TREE`, and the commit message must equal the
reviewed message. `PREPARATION_HEAD` must remain an ancestor, its parent must
equal `ORIGINAL_TDD_HEAD`, the original range must contain exactly 18 TDD
commits and 19 commits including preparation, no merge, and the exact recorded
history snapshot. The range after `PREPARATION_HEAD` must contain exactly the
four recorded Task 1–4 hashes in order and no other commit. The worktree and
index must be clean, proving that normal commit hooks neither changed nor left
uncommitted content. The `.pi/` digest must equal
`BASELINE_PI_ARCHIVE_SHA256`.

Dispatch a post-commit reviewer. It must verify the committed tree, actual HEAD
and message, hook-enabled commit procedure, exact Task 1–4 sequence, unchanged
18-commit TDD prefix, complete `.pi/` digest, protected paths, authorized
roadmap diff, and clean worktree and index. Task 4 is not approved until this
review passes.

### Reviewer adherence crosswalk

The Task 4 reviewer must explicitly classify every confirmed adherence gap at
the top of this plan as satisfied or unsatisfied and cite the test that proves
it. The reviewer rejects:

- any deferred or advisory acceptance item;
- any exact-identity requirement tested only with `errors.Is`;
- any destination-Reserve test that fails during source Reserve;
- final-capacity inspection offered as proof of exact Reserve calls;
- distinct package-call pointers offered as the sole proof of Decoder/wasm Meta
  detachment;
- a fake or hook stored in mutable package-global state;
- a public, generated, guest, documentation, or unrelated change.

### Task 4 acceptance criteria

- Every focused, full, race, lint, formatting, build, API, allocation, and diff
  gate passes.
- The complete public inventory remains exactly the approved CONV-01 inventory.
- All protected paths are unchanged from `PREPARATION_HEAD`; the complete
  `.pi/` archive digest equals `BASELINE_PI_ARCHIVE_SHA256`.
- The branch contains only the preparation commit plus approved remediation
  commits after the exact recorded sequence of 18 TDD phase commits; ancestry,
  parent, commit count, and ordered hashes prove no history was rewritten or
  extra commit added.
- `CONV-01` is `Review` and the roadmap diff contains only its authorized status
  transition.
- `HEAD`, committed tree, commit message, hook effects, worktree, and index pass
  the mandatory post-commit worker checks and reviewer verification.
- Nothing was pushed or merged.

After Task 4 approval and commit, Pi stops. It must not begin Task 5 without the
independent gate below.

---

## Independent adherence gate

The user runs a fresh `$plan-adherence-review` against this remediation plan and
the complete `conv-01-allocating-helpers` branch.

- A clean review with no `Partial`, `Missing`, or `Deviated` item makes Task 5
  eligible.
- Any unresolved item leaves `CONV-01` at `Review`.
- Pi does not design or improvise a correction from the adherence report.
- Failed findings return to the planning agent for a bounded, user-approved plan
  amendment. Pi executes only that approved amendment.
- Even after a clean review, Pi requires an explicit user instruction to resume
  Task 5.

---

## Task 5 — Record accepted completion

Task 5 begins only after the user explicitly states that the independent
adherence review passed and instructs Pi to resume.

### Worker instructions

1. Confirm the branch is `conv-01-allocating-helpers`, HEAD is the approved Task
   4 commit, and the worktree and index are clean.
2. Change only the `CONV-01` status from `Review` to `Complete`.
3. Run:

```bash
make format-check
git diff --check
git status --short
git diff --cached --name-only
git diff --exit-code PREPARATION_HEAD..HEAD -- decoder.go convert.go API.md README.md api_boundary_test.go internal/wuffswasm wasm scripts
tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner --format=gnu -cf - .pi | sha256sum
sha256sum <BASELINE_ROADMAP_SNAPSHOT>
git diff --no-index --no-ext-diff -- <BASELINE_ROADMAP_SNAPSHOT> plans/api-roadmap.md
```

Substitute the recorded preparation hash and snapshot path literally. The
`.pi/` digest must equal `BASELINE_PI_ARCHIVE_SHA256`.
4. Prepare a concise roadmap-completion commit message in
`tmp/commit_message.txt`.
5. Do not commit until the reviewer verifies the explicit user authorization,
Task 4 evidence, independent adherence result, roadmap-only diff, and proposed
message.
6. After reviewer approval, stage only `plans/api-roadmap.md`, require no
unstaged diff, record `git write-tree` as `TASK_5_REVIEWED_TREE`, and commit
normally with hooks enabled.
7. Record `TASK_5_HEAD`, then run the Task 4 mandatory post-commit verification
with Task 5 substitutions: `HEAD` and its tree must equal the recorded Task 5
values; the post-preparation range must contain exactly the five recorded
Task 1–5 hashes in order; the committed-file list must contain only
`plans/api-roadmap.md`; and the roadmap snapshot diff may contain only the
transition from snapshotted `Ready` to `Complete`.
8. Dispatch a post-commit reviewer to verify committed contents, final HEAD and
message, normal hook effects, exact history, protected paths, complete `.pi/`
digest, authorized roadmap diff, and clean worktree and index. Task 5 is not
complete until this review passes.

The final roadmap snapshot diff may contain only the `CONV-01` status change
from snapshotted `Ready` to `Complete`.

### Task 5 acceptance criteria

- The user explicitly authorized resumption after a clean independent adherence
  review.
- Only `plans/api-roadmap.md` changed in Task 5.
- `CONV-01` names this plan and is `Complete`.
- Protected paths and the complete `.pi/` archive digest remain unchanged.
- Formatting and diff checks pass.
- Reviewer approves before the worker creates the Task 5 commit.
- A post-commit reviewer verifies the final committed tree, HEAD, message, hook
  effects, exact five-commit remediation sequence, and clean worktree and index.
- Nothing was pushed, merged, squashed, amended, rebased, or otherwise rewritten.

---

## Completion report

After Task 5, report:

1. Branch and final HEAD.
2. The preparation commit and each remediation commit hash and subject.
3. Complete changed-file manifest relative to `PREPARATION_HEAD`.
4. Test names proving each original adherence gap.
5. Focused, full, race, lint, formatting, build, API, allocation, and diff-gate
   results.
6. Public API inventory result.
7. Protected-path, deterministic `.pi/` archive digest, and immutable original
   18-commit ancestry, parent, count, and sequence results.
8. Roadmap and history snapshot paths, unchanged digests, and authorized-only
   final roadmap diff.
9. Confirmation that `CONV-01` is `Complete`.
10. Confirmation that no push, merge, squash, amend, rebase, reset, guest
    generation, Python, force-add, or hook bypass occurred.

Stop after reporting. The user owns the squash-merge decision.
