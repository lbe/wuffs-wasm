# CORE-02 Public API Boundary Cleanup

**Execution protocol:** `plans/pi-runbook.md`

**Execution branch:** `core-02-public-api-boundary`

**Commit authorized:** No

**Purpose:** Remove the accidental root-package exports `SlotLayout`,
`(*Decoder).MemoryLayout`, and `ReadMeta` without changing decoder behavior. Keep
required memory-layout inspection available only to tests and lock the implemented
public surface to the portion of `API.md` currently owned by the baseline roadmap
workstreams.

This is a bounded host-only cleanup. It does not authorize any future API from
`API.md`, new format support, guest changes, or generated binding changes.

---

## Sources of truth

Read before acting:

1. `AGENTS.md`
2. The user's latest instruction
3. `plans/pi-runbook.md`
4. This plan
5. `GOALS.md`
6. `API.md`
7. `plans/api-roadmap.md`, especially `CORE-02`
8. `HANDOVER.md`, only for its caller-owned destination and host-only constraints

`API.md` defines the complete target contract, but this plan implements only
`CORE-02`. Do not add APIs assigned to `STILL-01`, `CONV-01`, `ADAPT-01`,
`FORMAT-*`, `REG-01`, `ANIM-01`, or `META-01`.

If the sources conflict or `CORE-01` is not complete, stop and report the conflict.

---

## Execution control

- Use only the preconfigured Pi subagents `worker` and `reviewer` through
  `plans/pi-runbook.md`.
- The orchestrator only orchestrates. It does not read implementation files, edit,
  test, stage, commit, or apply fixes directly during execution.
- The roadmap must name this plan and show `CORE-02` as `Ready` before execution.
  If it does not, stop; approval and roadmap preparation are outside execution.
- If `core-02-public-api-boundary` is already checked out, use it. Otherwise, the
  first worker may create it with `git switch -c core-02-public-api-boundary` only
  when the current branch is `main` and the branch name does not already exist. In
  every other branch state, stop and report it.
- Require an empty index before editing. Preserve all pre-existing unstaged and
  untracked work.
- Do not stage, commit, push, merge, amend, rebase, reset, or rewrite history.
- Do not use Python.
- Do not run `make generate`.
- Do not edit guest or generated artifacts.
- Complete and review Task 1 before starting Task 2.
- Complete and review Task 2 before starting Task 3.
- Follow the runbook's three-attempt limit for a rejected task.

### Preflight evidence

The first worker reports:

```bash
git branch --show-current
git rev-parse HEAD
git status --short
git diff --cached --name-only
git diff --name-only
tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -cf - .pi | sha256sum
mkdir -p tmp/core-02-public-api-boundary
ROADMAP_SNAPSHOT="$(mktemp tmp/core-02-public-api-boundary/api-roadmap.pre-execution.XXXXXX.md)"
cp plans/api-roadmap.md "${ROADMAP_SNAPSHOT}"
chmod a-w "${ROADMAP_SNAPSHOT}"
sha256sum "${ROADMAP_SNAPSHOT}"
```

After any authorized branch creation, the branch must be exactly
`core-02-public-api-boundary`. `git diff --cached --name-only` must be empty. The
worker records the exact `git rev-parse HEAD` output as `BASELINE_HEAD` and the
exact `.pi` archive digest as `BASELINE_PI_SHA256` in its report. Before any edit,
the worker also records the exact snapshot path as `BASELINE_ROADMAP_SNAPSHOT`
and its digest as `BASELINE_ROADMAP_SHA256`. The snapshot is read-only execution
evidence, not an implementation file; no worker may edit, replace, or remove it.
These recorded values are the durable baselines used by every later worker and
reviewer.

### Protected artifacts

These paths must remain unchanged:

- `internal/wuffswasm/wuffs.go`
- `wasm/shim.c`
- `wasm/wuffs.wasm`
- `scripts/`
- `.pi/`

---

## Task 1 — Internalize runtime implementation details

### Allowed files

Only these implementation and test files may change:

- `memory.go`
- `meta.go`
- `decoder.go`
- `export_test.go`
- `api_boundary_test.go` (new)
- `memory_test.go`
- `memory_reserve_test.go`
- `decoder_internal_test.go`
- `decoder_probe_reserved_test.go`
- `decoder_reserve_decode_test.go`
- `decoder_test.go`
- `plans/api-roadmap.md`

The first Task 1 edit to `plans/api-roadmap.md` changes only the `CORE-02` status
from `Ready` to `In progress`. Preserve every pre-existing roadmap change.

### Production changes

1. Rename `SlotLayout` to the unexported `slotLayout`.
2. Rename every layout field to its lower-camel equivalent: `metaOff`, `metaLen`,
   `srcOff`, `srcLen`, `dstOff`, `dstLen`, and `hostBase`.
3. Update `Decoder.currentLayout`, `computeLayout`, `Reserve`, `Probe`,
   `DecodeRGBA`, and internal helpers to use the unexported type and fields.
4. Remove the production method `(*Decoder).MemoryLayout`; do not replace it with
   another exported production accessor.
5. Internalize `ReadMeta`. Prefer one unexported metadata-slot reader and remove
   any now-redundant wrapper; preserve its bounds behavior and decoded fields
   exactly.
6. Update active code comments and godoc so they no longer present the memory
   layout or raw metadata reader as public API.
7. In the roadmap's current-baseline prose, replace the public-style
   `SlotLayout.SrcLen` reference with implementation-neutral wording. Do not alter
   dependencies, workstream scope, or API traceability.

Do not change slot arithmetic, reservation semantics, error mapping, metadata
values, source-capacity enforcement, destination ownership, allocation behavior,
or method signatures that are present in `API.md`.

### Test migration

1. Same-package tests may inspect `d.currentLayout` and the unexported
   `slotLayout` directly. Do not retain a production accessor solely for tests.
2. Replace the external-package assertion in `decoder_test.go` with one narrow
   test-only helper in `export_test.go` that returns only the current destination
   slot length. Do not expose the complete layout to external tests.
3. Preserve all existing layout identity, source-capacity, reservation,
   guest-memory identity, and destination ownership assertions. Rename references;
   do not weaken or remove them.
4. Add `TestUnitPublicAPIBoundary` in `api_boundary_test.go`. Use Go's parser, not
   regular expressions, to inspect root-package non-test Go declarations. Require
   the exact currently implemented public declaration, method, and public-struct
   field inventory below.

#### Required public inventory

- Constants: `FormatPNG`, `FormatWEBP`
- Variables: `ErrBadImage`, `ErrDecode`, `ErrDstTooSmall`, `ErrSrcTooLarge`,
  `ErrUnknownFormat`
- Types: `Decoder`, `DstTooSmallError`, `Meta`
- Functions: `New`
- `Decoder` methods: `DecodeRGBA`, `Probe`, `Reserve`, `Version`, `VersionNum`
- `DstTooSmallError` methods: `Error`, `Is`
- `Meta` fields: `Err`, `Width`, `Height`, `Stride`, `BytesWritten`, `Format`
- `DstTooSmallError` fields: `MinBytes`, `Width`, `Height`, `Stride`
- `Decoder` fields: none exported

The test must exclude `_test.go` files so test-only exports do not become product
API. It must report missing and unexpected entries clearly. Future roadmap tasks
that intentionally add API will update this inventory in their own plans.

### Focused verification

Run the API-boundary test first, then the affected internal contracts:

```bash
go test -run '^TestUnitPublicAPIBoundary$' -count=1 -v .
go test -run '^(TestUnitMemoryLayout|TestReserveGrowsOnlyRequestedSlotCapacity|TestIntegrationSourceSlotDefinesDecoderCapacity|TestIntegrationSourceCapacityUsesCurrentLayout|TestProbeHonorsReservedSourceCapacity|TestDecodeRGBAHonorsReservedSourceCapacity|TestIntegrationProbe_SentinelErrors)$' -count=1 -v .
```

Then run:

```bash
make format-check
make lint
make test
go build ./...
go doc -all .
git diff --check
git diff --exit-code -- internal/wuffswasm/wuffs.go wasm/shim.c wasm/wuffs.wasm scripts
git diff --cached --name-only
```

The worker must report the complete `go doc -all .` exported inventory. The
reviewer independently confirms that `SlotLayout`, `MemoryLayout`, and `ReadMeta`
are absent and every intended baseline export remains present.

The Task 1 reviewer also verifies that the roadmap snapshot still hashes to
`BASELINE_ROADMAP_SHA256`, then compares it directly with the current roadmap:

```bash
sha256sum <BASELINE_ROADMAP_SNAPSHOT>
git diff --no-index --no-ext-diff -- <BASELINE_ROADMAP_SNAPSHOT> plans/api-roadmap.md
```

The expected roadmap diff contains only the `CORE-02` status change from `Ready`
to `In progress` and the authorized implementation-neutral replacement for the
`SlotLayout.SrcLen` baseline wording. `git diff --no-index` is expected to return
status 1 because those two differences are required; any other hunk is rejection
evidence.

`make test-race`, allocation tests, and benchmarks are not required: this task
does not change decoder state, concurrency, memory behavior, or the reusable
decode path. Any behavioral production change is a scope violation, not a reason
to add those gates.

### Task 1 acceptance criteria

- `SlotLayout`, `(*Decoder).MemoryLayout`, and `ReadMeta` are absent from the
  production package API.
- The complete layout type and all its fields are unexported.
- Production code has no replacement public memory-layout or raw-memory reader.
- Same-package tests retain complete internal layout assertions.
- External tests receive only destination-slot length through a test-only helper.
- `TestUnitPublicAPIBoundary` enforces the exact implemented public inventory and
  excludes test files.
- All existing affected tests pass without weakened assertions.
- Formatting, lint, full tests, build, API inspection, and diff checks pass.
- The roadmap says `CORE-02` is `In progress` and no unrelated roadmap content
  changed during this task.
- The read-only roadmap snapshot matches `BASELINE_ROADMAP_SHA256`, and its
  complete diff against the current roadmap contains exactly the two authorized
  Task 1 edits.
- Protected artifacts, history, and the index remain unchanged.

The reviewer rejects Task 1 if production behavior changes, an accidental export
is retained under another name, test access remains in production solely for
tests, or an existing internal invariant loses coverage.

---

## Task 2 — Review traceability and final evidence

Task 2 begins only after Task 1 reviewer approval.

### Worker instructions

1. Change only the `CORE-02` row in `plans/api-roadmap.md` from `In progress` to
   `Review`.
2. Do not change the plan path, dependency graph, workstream definition, or API
   traceability table.
3. Run the review commands and report the complete changed-file manifest.
4. Compare the reported HEAD and `.pi` digest exactly with `BASELINE_HEAD` and
   `BASELINE_PI_SHA256` from preflight; stop on either mismatch.
5. Do not stage or commit.

### Review commands

```bash
make format-check
go test -run '^TestUnitPublicAPIBoundary$' -count=1 -v .
go build ./...
go doc -all .
git diff --check
git status --short
git diff --name-only
git diff --cached --name-only
git diff --exit-code -- internal/wuffswasm/wuffs.go wasm/shim.c wasm/wuffs.wasm scripts
git branch --show-current
git rev-parse HEAD
tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -cf - .pi | sha256sum
```

### Task 2 acceptance criteria

- The Task 1 reviewer approved every API-boundary and regression criterion.
- `CORE-02` names this plan and is `Review` in `plans/api-roadmap.md`.
- `go doc -all .` exposes only the implemented inventory required by Task 1.
- All review commands pass.
- Changes are limited to this plan's allowed files plus the already-created plan
  file.
- Protected artifacts are unchanged; the final `.pi` digest equals
  `BASELINE_PI_SHA256`.
- The branch remains `core-02-public-api-boundary`.
- The final HEAD equals `BASELINE_HEAD`; the index is empty and nothing was
  committed, pushed, or merged.

The Task 2 reviewer inspects the complete uncommitted diff, Task 1 evidence,
review-command evidence, recorded baseline comparisons, and `Review` roadmap
status. The reviewer rechecks `BASELINE_ROADMAP_SHA256` and compares
`BASELINE_ROADMAP_SNAPSHOT` with the current roadmap; the complete roadmap diff
may contain only the authorized baseline-wording edit and the `CORE-02` status
change from the snapshotted `Ready` state to `Review`. Approval accepts the
implementation evidence and authorizes Task 3.

---

## Task 3 — Record accepted completion

Task 3 begins only after the Task 2 reviewer accepts all evidence.

### Worker instructions

1. Change only the `CORE-02` status in `plans/api-roadmap.md` from `Review` to
   `Complete`.
2. Run the completion commands and report their exact output.
3. Do not make any other edit, stage, or commit.

### Completion commands

```bash
make format-check
git diff --check
git status --short
git diff --name-only
git diff --cached --name-only
git diff --exit-code -- internal/wuffswasm/wuffs.go wasm/shim.c wasm/wuffs.wasm scripts
git branch --show-current
git rev-parse HEAD
tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -cf - .pi | sha256sum
```

### Final acceptance criteria

- The Task 2 reviewer accepted the implementation and review evidence before the
  roadmap status changed to `Complete`.
- `CORE-02` names this plan and is `Complete` in `plans/api-roadmap.md`.
- `make format-check` and `git diff --check` pass after the status-only edit.
- The complete changed-file manifest remains within this plan's allowed files
  plus the already-created plan file; the recorded read-only file under `tmp/` is
  only preflight evidence.
- The roadmap snapshot still matches `BASELINE_ROADMAP_SHA256`, and its complete
  diff against `plans/api-roadmap.md` contains only the authorized
  implementation-neutral baseline wording and the `CORE-02` status change from
  the snapshotted `Ready` state to `Complete`.
- Protected artifacts are unchanged; the completion `.pi` digest equals
  `BASELINE_PI_SHA256`.
- The branch remains `core-02-public-api-boundary`.
- The completion HEAD equals `BASELINE_HEAD`; the index is empty and nothing was
  committed, pushed, or merged.

The final reviewer verifies the accepted Task 2 evidence, the status-only Task 3
edit, all completion-command evidence, and the HEAD, `.pi`, and roadmap baseline
comparisons before approving completion. The reviewer independently reruns the
snapshot hash and full `git diff --no-index --no-ext-diff` comparison used in
Task 1; any additional roadmap hunk rejects completion.

---

## Completion report

After final reviewer approval, report:

1. branch and unchanged HEAD;
2. removed production exports;
3. internal and external test-access replacements;
4. API-boundary inventory-test result;
5. focused and repository gate results;
6. complete changed-file manifest;
7. roadmap status;
8. roadmap snapshot path, unchanged digest, and authorized-only diff result;
9. confirmation that generated artifacts, guest files, `.pi`, history, and the
   index are unchanged.

Stop after reporting. Do not commit, push, merge, or begin downstream roadmap
work.
