# Correct FORMAT-02 Plan Adherence

**Execution protocol:** `plans/pi-runbook.md`

**Branch:** `format-02-portable-images`

**Commit authorized:** No

**Purpose:** Correct the implementation and verification deviations found by the
plan-adherence review of `.pi/tdd-plans/format-02-portable-images.yaml` without
rewriting the valid eight-cycle TDD history.

This is a corrective Pi runbook plan, not a replacement TDD plan. Existing
FORMAT-02 behavior that already matches the plan must remain unchanged.

---

## Agent selection

- Use the exact preconfigured Pi subagent `worker` for all coding, formatting,
  generation, and test execution. It is currently configured with the fast model
  `opencode-go/deepseek-v4-flash`.
- Use the exact preconfigured Pi subagent `reviewer` for read-only review after
  each implementation task.
- Do not override either subagent's model in a dispatch.
- The Pi orchestrator only orchestrates. It must not edit files, run tests,
  generate artifacts, or apply corrections directly.
- If `worker` is unavailable or is no longer configured with a fast model, stop
  and notify the user before implementation.

## Sources of truth

Read these in order before acting:

1. `AGENTS.md`
2. `plans/pi-runbook.md`
3. `HANDOVER.md`
4. This plan
5. `.pi/tdd-plans/format-02-portable-images.yaml`
6. `GOALS.md`
7. `API.md`
8. `README.md`
9. `plans/api-roadmap.md`

If these sources conflict with this plan or the current branch, stop and report
the exact conflict. Do not invent a correction.

## Required execution state

- Work only on the existing branch `format-02-portable-images`.
- Preserve the 24 existing FORMAT-02 Red, Green, and Refactor commits.
- Do not amend, rebase, reset, squash, or otherwise rewrite history.
- Do not invoke `tdd-orchestrator`; this plan corrects already-executed work.
- Do not edit the original TDD plan, TDD state, or TDD audit log.
- Do not stage, commit, push, merge, or change branches.
- Do not update FORMAT-02 from `Review` to `Complete`. That remains a separate
  user-authorized roadmap-maintenance action after evidence acceptance.
- Do not use Python.
- Do not use `git add -f` or any hook-skipping option.
- Do not hand-edit `internal/wuffswasm/wuffs.go` or `wasm/wuffs.wasm`.
- Run `make generate` only at its two explicitly authorized points: in Task 2
  after editing `wasm/shim.c`, where it may update the generated artifacts, and
  in Task 3 solely as a reproducibility check, where it must leave the shim and
  both generated artifacts byte-for-byte unchanged.
- Do not modify `wasm/wuffs_config.h`, the Wuffs release sources, or files under
  `scripts/`.
- Keep the shared declaration-test helpers. Their behavior-preserving
  consolidation is accepted by this corrective plan; do not restore duplicated
  local parser closures or rewrite the historical plan.

### Expected starting point

The FORMAT-02 implementation tip at plan authoring time is:

```text
512bf81a5cc9d7a133f204980d0309793673c8b6
```

The plan file itself may be the sole untracked or newly committed file. If HEAD
differs, the worker may proceed only when every intervening commit changes this
plan and no implementation file. Otherwise stop and report the history change.
There must be no staged implementation change and no other dirty implementation
file.

### Preflight evidence

Before Task 1, the worker reports:

```bash
git branch --show-current
git rev-parse HEAD
git status --short
git diff --name-only
git diff --cached --name-only
git log --reverse --format='%h %s' main..HEAD
git diff --name-only main...HEAD -- wasm internal/wuffswasm
sha256sum plans/format-02-adherence-remediation.md \
  > tmp/format-02-remediation-plan.sha256
sha256sum wasm/wuffs_config.h \
  .pi/tdd-plans/format-02-portable-images.yaml \
  .pi/tdd-state-format-02-portable-images \
  .pi/tdd-audit-format-02-portable-images.log \
  ../wuffs-mirror-release-c/release/c/wuffs-v0.4.c \
  > tmp/format-02-remediation-protected.sha256
find scripts -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum \
  > tmp/format-02-remediation-scripts.sha256
```

Required preflight results:

- The branch is `format-02-portable-images`.
- The existing FORMAT-02 history contains the expected 24 ordered phase commits,
  except for an optional later commit containing only this plan.
- The index contains no changes.
- No implementation file is dirty.
- The plan checksum records the exact untracked plan approved for execution.
- Before remediation, only `wasm/shim.c`, `wasm/wuffs.wasm`, and
  `internal/wuffswasm/wuffs.go` differ from `main` among guest and generated
  artifacts.

## Allowed implementation files

Only these files may change during execution:

- `export_test.go`
- `format02_contract_test.go` (new test-helper file)
- `decode_npbm_integration_test.go`
- `decode_qoi_integration_test.go`
- `decode_tga_integration_test.go`
- `decode_wbmp_integration_test.go`
- `wasm/shim.c`
- `wasm/wuffs.wasm` through `make generate` only
- `internal/wuffswasm/wuffs.go` through `make generate` only

The plan itself is not an implementation file and must not be edited during
execution. Any need to change another file is a plan deviation: stop and report.

---

## Task 1 — Prove ownership and guest-memory identity

**Nature:** Passing characterization coverage for behavior that should already
exist. Do not manufacture a Red failure and do not change production code.

**Files:**

- `export_test.go`
- `format02_contract_test.go`
- `decode_npbm_integration_test.go`
- `decode_qoi_integration_test.go`
- `decode_tga_integration_test.go`
- `decode_wbmp_integration_test.go`

### Test-only guest-memory snapshot

Add a test-only, comparable `GuestMemoryState` in `export_test.go`. It must
capture exactly:

- the backing pointer of the complete wasm memory slice using
  `unsafe.SliceData`;
- the wasm memory slice length;
- every field of `Decoder.currentLayout`: `metaOff`, `metaLen`, `srcOff`,
  `srcLen`, `dstOff`, `dstLen`, and `hostBase`.

Add `CaptureGuestMemoryState(*Decoder) GuestMemoryState`. The helper exists only
in `_test.go`; do not widen the production API. Preserve
`CurrentDstSlotLen` because existing tests use it.

Add `format02_contract_test.go` in package `wuffs_test` with small shared helpers
that:

1. capture and compare `GuestMemoryState` before and after an operation;
2. capture and compare an `image.RGBA` destination's `Pix` backing pointer,
   length, capacity, `Rect`, and `Stride`.

The helpers must report the operation and fixture name on failure. Do not add a
new top-level test function merely to hold helpers.

### NPBM evidence

Extend `TestIntegrationNPBMDecodeCharacterization` so every successful P5 and P6
case at Maxval 255 and 65535 proves:

- `Probe` preserves the complete guest-memory state after explicit `Reserve`;
- `DecodeRGBA` preserves the complete guest-memory state;
- `DecodeRGBA` preserves the caller's complete destination layout;
- existing format, geometry, bytes-written, pixel, failure, and allocation
  assertions remain intact.

Do not replace the caller destination or weaken the existing pixel assertions.

### QOI evidence

Extend `TestIntegrationQOIDecodeCharacterization` so the RGB, RGBA, and corrupt
QOI cases prove guest-memory preservation around both `Probe` and `DecodeRGBA`.
Add complete destination-layout preservation to the successful RGBA case. Keep
the existing RGB and corrupt-input destination checks and all pixel assertions.

### TGA evidence

Update the existing TGA assertion helpers so:

- every successful `Probe` checks complete guest-memory preservation;
- every successful `DecodeRGBA` checks complete guest-memory preservation and
  the full caller destination layout;
- the 256x1 and 1x256 decode cases use the same complete decode assertion rather
  than bypassing it;
- `CurrentDstSlotLen` is no longer presented as proof of full guest-memory
  preservation.

Keep every positive matrix, collision, corruption, unsupported-input, pixel, and
allocation case unchanged.

### WBMP evidence

Update the existing WBMP assertion helpers so every successful `Probe` and
`DecodeRGBA` checks complete guest-memory preservation. Preserve the complete
caller destination-layout assertion, independent oracle, boundaries, negatives,
priority cases, corruption behavior, and allocation test.

### Task 1 verification

Run the narrow tests first:

```bash
go test . -run '^(TestIntegrationNPBMDecodeCharacterization|TestIntegrationQOIDecodeCharacterization|TestIntegrationTGADecodeCharacterization|TestIntegrationWBMPDecodeCharacterization)$' -count=1 -v
```

Then run:

```bash
make format-check
make lint
make test
go build ./...
git diff --check
```

Do not run `make generate` in Task 1.

### Task 1 acceptance criteria

- Every planned NPBM, QOI, TGA, and WBMP ownership case directly verifies the
  caller-owned destination properties applicable to that case.
- Reserved `Probe` and `DecodeRGBA` calls for all four formats directly verify
  the wasm backing pointer, wasm byte length, and complete slot layout.
- The new state helper is test-only and comparable.
- No production API or production Go code changes.
- Existing format, pixel, error, collision, and zero-allocation evidence remains
  intact and passes.
- Only the six Task 1 files change.
- The task ends formatted, lint-clean, buildable, and test-green.

The reviewer rejects Task 1 if the tests compare only `dstLen`, omit the wasm
backing pointer or byte length, skip one of the four formats, or fail to cover
successful NPBM and QOI RGBA destination identity.

After Task 1 approval and before Task 2 edits, the worker records the approved
Task 1 boundary:

```bash
sha256sum export_test.go format02_contract_test.go \
  decode_npbm_integration_test.go decode_qoi_integration_test.go \
  decode_wbmp_integration_test.go \
  > tmp/format-02-remediation-task1-protected.sha256
mkdir -p /tmp/wuffs-go-format-02-remediation.omL1QH/wasm
cp decode_tga_integration_test.go \
  /tmp/wuffs-go-format-02-remediation.omL1QH/decode_tga_integration_test.go
cp wasm/shim.c /tmp/wuffs-go-format-02-remediation.omL1QH/wasm/shim.c
```

Task 2 must not change any file recorded in the Task 1 protected checksum.

---

## Task 2 — Align signatureless recognition with the plan

**Nature:** One contained Red/Green correction. The task boundary must be green
before review.

**Files:**

- `decode_tga_integration_test.go`
- `wasm/shim.c`
- `wasm/wuffs.wasm` through `make generate`
- `internal/wuffswasm/wuffs.go` through `make generate`

### Red — truncated indexed palette classification

Add one subtest named
`truncated indexed palette is recognized then rejected by Wuffs` to
`TestIntegrationTGADecodeCharacterization`, using this exact 18-byte TGA source:

```go
[]byte{
	0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x18, 0x00,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x08, 0x00,
}
```

This is a complete, accepted indexed TGA header for a 1x1 image with one 24-bit
palette entry, but the palette and pixel payload are absent. Require both
`Probe` and `DecodeRGBA` to:

- return an error matching `ErrDecode`;
- not return an error matching `ErrUnknownFormat`;
- return nil metadata;
- preserve complete guest-memory state;
- preserve a sentinel destination's bytes and complete layout for
  `DecodeRGBA`.

Run the focused test before editing `wasm/shim.c`:

```bash
go test . -run '^TestIntegrationTGADecodeCharacterization$/^truncated_indexed_palette_is_recognized_then_rejected_by_Wuffs$' -count=1 -v
```

The test must fail because the current shim returns `ErrUnknownFormat`. If it
passes or fails for another reason, stop and report the plan-fidelity problem.

### Green — delegate payload validation

In `sniff_tga`:

- remove the indexed color-map payload-length calculation and rejection;
- retain every planned 18-byte structural header restriction;
- update comments so they do not claim the color map must be present;
- leave image ID, palette, and pixel payload validation to Wuffs.

Do not broaden any type, depth, geometry, descriptor, origin, attribute,
color-map type, first-entry, or color-map length rule.

### Green — restore literal-signature priority

In `sniff_fourcc`, order recognition as follows:

1. all literal-signature formats, including NIE, ETC2, HNSM, and TH;
2. the conservative TGA fallback;
3. the canonical WBMP fallback;
4. unknown format.

Do not change any literal signature or deferred decoder behavior. Existing ETC2,
HNSM, NIE, and TH sentinels must continue to return `ErrDecode`, not a verified
format or `ErrUnknownFormat`.

### Generate and verify

After both `wasm/shim.c` corrections:

```bash
make generate
```

Do not hand-edit either generated artifact. Then run:

```bash
go test . -run '^(TestIntegrationTGADecodeCharacterization|TestIntegrationTGADecodeRGBAAllocsPerRun|TestIntegrationWBMPDecodeCharacterization|TestIntegrationWBMPDecodeRGBAAllocsPerRun)$' -count=1 -v
make format-check
make lint
make test
go build ./...
git diff --check
```

### Task 2 acceptance criteria

- The new truncated-palette subtest supplies authentic Red evidence against the
  current shim and passes after the correction.
- TGA recognition depends only on the exact planned structural header fields.
- Wuffs, not `sniff_tga`, validates image ID, palette, and pixel payload
  availability.
- ETC2, HNSM, NIE, and TH literal signatures are checked before TGA and WBMP.
- TGA remains before WBMP.
- Every existing TGA/WBMP positive, negative, collision, ownership, memory, and
  allocation test passes.
- Only `wasm/shim.c`, `wasm/wuffs.wasm`, and
  `internal/wuffswasm/wuffs.go` change in Task 2 in addition to the TGA test.
- `wasm/wuffs_config.h`, Wuffs release sources, and `scripts/` remain unchanged.
- The task ends formatted, lint-clean, buildable, and test-green.

The reviewer must inspect the recognizer order directly. Passing sentinels alone
does not prove the plan-required ordering because their current bytes do not
collide with either fallback.

---

## Task 3 — Final proof and scope gate

**Files:** No new edits expected.

The fast `worker` runs the complete gate. The `reviewer` independently examines
the complete uncommitted remediation diff and the reported evidence.

### Focused FORMAT-02 evidence

```bash
go test . -run '^(TestUnitFormat(NPBM|QOI|TGA|WBMP)Declaration|TestUnitFormatNamesFORMAT02|TestIntegration(NPBM|QOI|TGA|WBMP)DecodeCharacterization|TestIntegration(NPBM|QOI|TGA|WBMP)DecodeRGBAAllocsPerRun|TestIntegrationDecodeReaderFormatMatrix|TestIntegrationVerifiedFormatDocumentationInventory)$' -count=1 -v
```

Every named `AllocsPerRun` test must report exactly zero allocations.

### Repository gates

Run in this order:

```bash
make format-check
make lint
make test
go build ./...
sha256sum wasm/wuffs.wasm internal/wuffswasm/wuffs.go wasm/shim.c \
  > tmp/format-02-remediation-before-final-generate.sha256
make generate
sha256sum -c tmp/format-02-remediation-before-final-generate.sha256
git diff --check
```

The checksum validation after the final `make generate` must pass, proving that
it made no byte changes to either generated artifact or `wasm/shim.c`.

### Final scope audit

The worker reports:

```bash
git branch --show-current
git status --short
git diff --cached --name-only
git diff --name-only
git diff --name-only HEAD -- wasm internal/wuffswasm
git diff --name-only HEAD -- wasm/wuffs_config.h scripts
git diff --name-only main...HEAD -- wasm internal/wuffswasm
git log --reverse --format='%h %s' main..HEAD
sha256sum -c tmp/format-02-remediation-plan.sha256
sha256sum -c tmp/format-02-remediation-protected.sha256
sha256sum -c tmp/format-02-remediation-scripts.sha256
sha256sum -c tmp/format-02-remediation-task1-protected.sha256
diff -u /tmp/wuffs-go-format-02-remediation.omL1QH/decode_tga_integration_test.go \
  decode_tga_integration_test.go
diff -u /tmp/wuffs-go-format-02-remediation.omL1QH/wasm/shim.c wasm/shim.c
```

The two `diff -u` commands are expected to show only the Task 2 TGA test and shim
corrections; treat those expected content differences as review evidence, not
gate failures. All checksum validations must pass. The reviewer independently
verifies the plan's immutability, generated-artifact provenance through
`make generate`, and the before/after generated-artifact checksum result.

The reviewer confirms:

- The branch is unchanged and existing history was not rewritten.
- Nothing is staged or committed.
- The complete remediation diff contains only allowed files.
- Full caller destination and guest-memory identity evidence exists for all four
  FORMAT-02 formats.
- TGA payload validation and signature ordering now match the original plan.
- Generated artifacts are reproducible and limited to the authorized files.
- `wasm/wuffs_config.h`, Wuffs release sources, scripts, the original TDD plan,
  state, audit log, and roadmap are unchanged.
- FORMAT-02 remains `Review`.

If a final command fails, the reviewer rejects Task 3. The worker may correct
only failures caused by this plan and only within the allowed files. Unrelated
failures or any need to broaden scope require a stop report.

---

## Completion report

After reviewer approval, stop without staging, committing, pushing, merging, or
updating the roadmap. Report:

1. Task 1 ownership and complete guest-memory evidence added for each format.
2. Task 2 authentic Red result and the corrected TGA classification.
3. Final literal-signature ordering.
4. Focused test and zero-allocation results.
5. Repository gate results.
6. Generated-artifact reproducibility and exact changed-file manifest.
7. Confirmation that history, index, original TDD records, Wuffs sources,
   `wasm/wuffs_config.h`, scripts, and roadmap remain unchanged.

## First message to the Pi orchestrator

```text
You are the orchestrator for the wuffs-wasm FORMAT-02 adherence remediation.
Read:

AGENTS.md
plans/pi-runbook.md
HANDOVER.md
plans/format-02-adherence-remediation.md
.pi/tdd-plans/format-02-portable-images.yaml

Use the exact preconfigured Pi subagents "worker" and "reviewer". The worker is
the fast coding model and performs every read, edit, generation step, and test;
the reviewer remains read-only. Execute one reviewed task at a time. Do not
stage, commit, push, merge, rewrite history, or update FORMAT-02 from Review.
```
