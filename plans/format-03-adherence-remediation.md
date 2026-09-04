# Correct FORMAT-03 Plan Adherence

**Execution protocol:** `plans/pi-runbook.md`

**Branch:** `format-03-remaining-images`

**Commit authorized:** No

**Purpose:** Correct the verification gaps found by the plan-adherence review of
`.pi/tdd-plans/format-03-remaining-images.yaml` without rewriting the completed
ten-cycle, thirty-commit TDD history.

This is a corrective Pi runbook plan, not a replacement TDD plan. Preserve all
FORMAT-03 behavior that already matches the approved TDD plan.

---

## Agent selection

- The Pi orchestrator may read governing instructions and plan documents needed
  to route and evaluate the work.
- Use the exact preconfigured Pi subagent `worker` for every
  implementation-related repository inspection, edit, formatting action,
  generation command, and test command.
- Use the exact preconfigured Pi subagent `reviewer` for read-only review after
  each task.
- Do not override either subagent's configured model. The `worker` is the fast
  coding model.
- Apart from reading those governing instructions and plan documents, the Pi
  orchestrator only orchestrates; it must not inspect implementation files,
  edit files, run commands, stage changes, or apply quick fixes.
- If either required subagent is unavailable, stop and notify the user.

## Sources of truth

Read these in order before acting:

1. `AGENTS.md`
2. `plans/pi-runbook.md`
3. This plan
4. `.pi/tdd-plans/format-03-remaining-images.yaml`
5. `GOALS.md`
6. `API.md`
7. `README.md`
8. `plans/api-roadmap.md`

The plan-adherence review that produced this remediation identified exactly four
required corrections:

1. successful recognizer-priority `Probe` calls do not verify complete guest
   memory preservation;
2. rejected `DecodeRGBA` calls routed through `assertRejectedPerOp` do not verify
   caller destination identity or unchanged pixels;
3. the ETC2 and NIE priority tables omit WebP, and the HNSM priority table omits
   NIE;
4. `git diff --check` fails because `testdata/README` has an extra blank line at
   EOF.

Do not expand this remediation into a general code review or unrelated cleanup.

## Required execution state

- Work only on `format-03-remaining-images`.
- Preserve the thirty FORMAT-03 Red, Green, and Refactor commits ending at the
  expected implementation tip:

  ```text
  f0f3caf
  ```

- If HEAD differs, proceed only when every intervening commit is a non-merge
  commit with exactly one parent and changes exactly one path,
  `plans/format-03-adherence-remediation.md`. Reject empty commits, merges,
  commits with zero or multiple changed paths, and commits changing any other
  path. Otherwise stop and report the changed history.
- The plan file may be the only untracked or newly committed file. The index
  must be empty and no implementation file may be dirty at preflight.
- Do not amend, rebase, reset, squash, or otherwise rewrite history.
- Do not invoke `tdd-orchestrator`; its FORMAT-03 execution is complete.
- Do not edit the original FORMAT-03 TDD plan, TDD state, or audit log.
- Do not stage, commit, push, merge, change branches, or update FORMAT-03 from
  `Review` to `Complete`.
- Do not use Python, `git add -f`, a hook-skipping option, or an unused-code
  suppression.
- Do not edit `wasm/shim.c`, `wasm/wuffs.wasm`,
  `internal/wuffswasm/wuffs.go`, `wasm/wuffs_config.h`, Wuffs release sources,
  or files under `scripts/`.
- Task 1 must not run `make generate`. Task 2 may run it only as a
  reproducibility check protected by before/after checksums; it must produce no
  byte changes.

### Preflight evidence

Before Task 1, the worker reports:

```bash
git branch --show-current
git rev-parse --short HEAD
git status --short
git diff --name-only
git diff --cached --name-only
git log --reverse --format='%h %s' d6f362b..f0f3caf
git merge-base --is-ancestor f0f3caf HEAD
for commit in $(git rev-list --reverse f0f3caf..HEAD); do
  git rev-list --parents -n 1 "$commit"
  git diff-tree --no-commit-id --name-only -r "$commit"
done
git diff --name-only d6f362b..HEAD -- wasm internal/wuffswasm
sha256sum .pi/tdd-plans/format-03-remaining-images.yaml \
  > tmp/format-03-remediation-tdd-plan.sha256
sha256sum wasm/shim.c wasm/wuffs.wasm internal/wuffswasm/wuffs.go \
  wasm/wuffs_config.h ../wuffs-mirror-release-c/release/c/wuffs-v0.4.c \
  > tmp/format-03-remediation-protected.sha256
find scripts -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum \
  > tmp/format-03-remediation-scripts.sha256
```

Required results:

- branch `format-03-remaining-images` at implementation tip `f0f3caf`, except
  for an optional later plan-only commit;
- `git merge-base --is-ancestor f0f3caf HEAD` exits zero, proving the required
  implementation tip is an ancestor of HEAD; otherwise stop;
- when HEAD is `f0f3caf`, the post-tip audit loop has empty overall output;
  when HEAD is later, the loop prints each intervening commit and its parent
  set followed by its changed paths; every commit must have exactly one parent
  and exactly one changed path, and that path must be
  `plans/format-03-adherence-remediation.md`; reject empty commits, merges,
  zero or multiple changed paths, and any other path;
- exactly thirty ordered FORMAT-03 phase commits in
  `d6f362b..f0f3caf`;
- empty index and no dirty implementation files;
- the FORMAT-03 guest/generated diff contains only `wasm/shim.c`,
  `wasm/wuffs.wasm`, and `internal/wuffswasm/wuffs.go`;
- protected checksums are recorded before remediation.

## Allowed files

Only these implementation files may change:

- `format03_contract_test.go`
- `decode_etc2_integration_test.go`
- `decode_nie_integration_test.go`
- `decode_hnsm_integration_test.go`
- `testdata/README`

This plan file is not an implementation file and must not be edited during Pi
execution. Any need to change another file is a plan deviation: stop and report
the exact reason.

---

## Task 1 — Complete FORMAT-03 contract evidence

**Nature:** Passing verification correction. Do not change production behavior,
guest code, generated artifacts, fixtures, manifests, or public documentation.

### Guest-memory proof for successful priority probes

Update `assertRecognizerPriority` in `format03_contract_test.go` so every
successful `Probe`:

1. captures `wuffs.CaptureGuestMemoryState(d)` after explicit reservation and
   immediately before `Probe`;
2. calls `Probe` exactly once;
3. calls `assertGuestMemoryPreserved` immediately after `Probe`, before any
   fatal metadata assertion;
4. preserves the existing nil-error, non-nil metadata, expected FourCC, and
   non-shadowing assertions.

The comparison must include the complete state already represented by
`GuestMemoryState`: wasm backing pointer, wasm byte length, and all current
layout fields. Do not replace it with a destination-slot-length check.

### Destination proof for rejected DecodeRGBA calls

Update `assertRejectedPerOp` in `format03_contract_test.go`. For its
`DecodeRGBA` branch only:

1. prefill the destination pixels with a nonzero sentinel;
2. capture the complete destination state before the call: Pix backing pointer,
   length, capacity, Rect, and Stride;
3. retain a byte copy of the prefilled pixels;
4. capture the returned metadata as well as the error;
5. after the call, verify complete guest-memory preservation, complete
   destination-state preservation, unchanged sentinel pixels, nil metadata,
   `errors.Is(err, ErrUnknownFormat)`, and not `errors.Is(err, ErrDecode)`.

For the `Probe` branch, capture returned metadata and require it to be nil while
preserving the existing complete guest-memory and error checks. Do not allocate
inside any `AllocsPerRun` measured closure; this helper is used only by rejected
input characterization.

### Complete priority matrices

Add only the missing canonical fixtures:

- `decode_etc2_integration_test.go`: add
  `bricks-color.lossless.webp` with `wuffs.FormatWEBP` to the existing
  recognizer-priority table.
- `decode_nie_integration_test.go`: add
  `bricks-color.lossless.webp` with `wuffs.FormatWEBP` to the existing
  recognizer-priority table.
- `decode_hnsm_integration_test.go`: add `49.bn4.nie` with
  `wuffs.FormatNIE` to the existing recognizer-priority table.

Do not remove, replace, or weaken any existing priority case. The shared helper
must apply the new complete guest-memory assertion to every old and new case.

### Restore the diff-check gate

Remove only the extra final blank line reported by:

```text
testdata/README:259: new blank line at EOF.
```

The file must end with exactly one newline after its final content line. Do not
reword or reformat unrelated documentation.

### Task 1 verification

Run the narrow evidence first:

```bash
go test . -run '^(TestIntegrationETC2DecodeCharacterization|TestIntegrationNIEDecodeCharacterization|TestIntegrationTHDecodeCharacterization|TestIntegrationHNSMDecodeCharacterization)$' -count=1 -v
```

Then run:

```bash
make format-check
make lint
make test
go build ./...
git diff --check d6f362b
git diff --check
```

Do not run `make generate` in Task 1.

### Task 1 acceptance criteria

- Every successful `Probe` performed by `assertRecognizerPriority` directly
  proves complete guest-memory preservation.
- Every rejected `Probe` and `DecodeRGBA` performed by `assertRejectedPerOp`
  returns nil metadata and preserves complete guest memory.
- Every rejected `DecodeRGBA` performed by that helper preserves Pix backing
  pointer, length, capacity, Rect, Stride, and every prefilled pixel byte.
- ETC2 and NIE priority evidence includes WebP; HNSM priority evidence includes
  NIE; all existing cases remain.
- The four FORMAT-03 characterization tests retain their fixture, oracle,
  corruption, truncation, signature, collision, priority, ownership, guest
  memory, and zero-allocation assertions.
- `git diff --check d6f362b` and `git diff --check` both pass.
- Only the five allowed files change.
- The task ends formatted, lint-clean, test-green, and buildable.

The reviewer rejects Task 1 if either helper proves only error identity, any
destination field is omitted, any required priority fixture is absent, an
existing priority row is removed, or any file outside the allowed list changes.

---

## Task 2 — Final proof and scope gate

**Files:** No new edits expected.

The worker runs the complete proof. The reviewer independently inspects the
uncommitted remediation diff and evidence. Any correction must remain within
Task 1's allowed files; otherwise stop for user direction.

### Focused FORMAT-03 evidence

```bash
go test . -run '^(TestUnitFormat(ETC2|HNSM|NIE|TH)Declaration|TestIntegration(ETC2|HNSM|NIE|TH)DecodeCharacterization|TestIntegrationDecodeReaderFORMAT03Matrix|TestIntegrationVerifiedFormatDocumentationInventory)$' -count=1 -v
```

The reviewer must inspect the characterization output and source to confirm the
embedded `AllocsPerRun` subtests cover every manifest fixture and require
exactly zero allocations.

### Repository gates and reproducibility

Run in this order:

```bash
make format-check
make lint
make test
make test-race
go build ./...
git diff --check d6f362b
git diff --check
sha256sum wasm/shim.c wasm/wuffs.wasm internal/wuffswasm/wuffs.go \
  > tmp/format-03-remediation-before-generate.sha256
make generate
sha256sum -c tmp/format-03-remediation-before-generate.sha256
```

The checksum validation after `make generate` must pass, proving the command
made no byte changes to the shim or either generated artifact.

### Final scope audit

The worker reports:

```bash
git branch --show-current
git rev-parse --short HEAD
git status --short
git diff --cached --name-only
git diff --name-only
git diff --name-only d6f362b..HEAD -- wasm internal/wuffswasm
git diff --name-only HEAD -- wasm internal/wuffswasm
git diff --name-only HEAD -- wasm/wuffs_config.h scripts
git log --reverse --format='%h %s' d6f362b..f0f3caf
git merge-base --is-ancestor f0f3caf HEAD
for commit in $(git rev-list --reverse f0f3caf..HEAD); do
  git rev-list --parents -n 1 "$commit"
  git diff-tree --no-commit-id --name-only -r "$commit"
done
sha256sum -c tmp/format-03-remediation-tdd-plan.sha256
sha256sum -c tmp/format-03-remediation-protected.sha256
sha256sum -c tmp/format-03-remediation-scripts.sha256
```

The reviewer confirms:

- the branch and exactly thirty-commit FORMAT-03 history in
  `d6f362b..f0f3caf` are unchanged;
- `git merge-base --is-ancestor f0f3caf HEAD` exits zero, proving the required
  implementation tip is an ancestor of HEAD; otherwise reject;
- HEAD is `f0f3caf` with empty overall output from the post-tip audit loop, or
  the loop prints each intervening commit and its parent set followed by its
  changed paths; every intervening commit has exactly one parent and exactly
  one changed path, and that path is
  `plans/format-03-adherence-remediation.md`; empty commits, merges, zero or
  multiple changed paths, and any other path are rejected;
- the index is empty and no remediation implementation changes have been committed;
- the remediation diff contains exactly the five allowed files;
- the original FORMAT-03 TDD plan is byte-identical;
- guest and generated artifacts are byte-identical to their pre-remediation
  checksums and reproducible through `make generate`;
- `wasm/wuffs_config.h`, Wuffs release sources, and `scripts/` are unchanged;
- complete priority, guest-memory, rejected-destination, and diff-check evidence
  now satisfies the original plan;
- FORMAT-03 remains `Review` and REG-01 remains `Not planned`.

If a command fails, the reviewer rejects Task 2. The worker may correct only a
failure caused by this remediation and only within the allowed files. Any other
failure requires a stop report.

---

## Completion report

After both tasks receive reviewer approval, stop without staging, committing,
pushing, merging, rewriting history, or updating the roadmap. Report:

1. the complete guest-memory proof added to successful priority probes;
2. the complete destination and nil-metadata proof added to rejected decode
   paths;
3. the three restored priority fixtures;
4. the corrected `git diff --check` result;
5. focused test, allocation, repository-gate, and build results;
6. generated-artifact reproducibility and protected-file checksum results;
7. the exact five-file remediation diff and confirmation that history, index,
   original TDD records, guest sources, generated artifacts, scripts, and
   roadmap status remain unchanged.

## First message to the Pi orchestrator

```text
You are the orchestrator for the wuffs-wasm FORMAT-03 adherence remediation.
Read:

AGENTS.md
plans/pi-runbook.md
plans/format-03-adherence-remediation.md
.pi/tdd-plans/format-03-remaining-images.yaml

Use the exact preconfigured Pi subagents "worker" and "reviewer". The worker is
the fast coding model and performs every implementation-related repository
inspection, edit, formatting action, generation command, and test; the reviewer
remains read-only. The orchestrator may read the governing instructions and
plan documents needed to route and evaluate the work, but must perform no
implementation action. Execute one reviewed task at a time. Do not stage,
commit, push, merge, rewrite history, regenerate guest artifacts outside the
explicit reproducibility check, or update FORMAT-03 from Review.
```
