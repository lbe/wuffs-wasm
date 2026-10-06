# API Roadmap

## Purpose

This document coordinates the work required to move the repository from its
current implementation to the public contract in `API.md`.

It records dependencies, execution-plan types, and completion status. It is not
an executable plan and must not be passed directly to either Pi or
`tdd-orchestrator`. Each implementation slice requires its own linked executable
plan.

## Sources of truth

Read these in priority order before creating or executing a child plan:

1. `AGENTS.md`
2. The user's latest instruction
3. `GOALS.md`
4. `API.md`
5. `HANDOVER.md` when work concerns the caller-owned `DecodeRGBA` correction
6. This roadmap

`API.md` defines the complete target contract. A narrower handover or child plan
limits the scope of its individual implementation pass.

## Plan types

### TDD plan

- Schema-driven YAML under `.pi/tdd-plans/`
- Executed by `tdd-orchestrator`
- Uses dependency-ordered Red, Green, and Refactor cycles
- Appropriate for new or changed observable behavior

### Pi runbook plan

- Markdown under `plans/`
- Executed by Pi through the preconfigured `worker` and `reviewer` subagents
- Governed by `plans/pi-runbook.md`
- Appropriate for bounded refactors, cleanup, documentation, and work whose
  primary proof is not naturally a failing behavioral test

### Roadmap

- Tracks work, dependencies, plan locations, and status
- Is not consumed by an implementation orchestrator
- Does not duplicate child-plan cycles, task instructions, or acceptance details

Do not combine plan types. A TDD plan is not governed by the Pi runbook, and a Pi
runbook plan is not executed by `tdd-orchestrator`.

## Product invariants

Every child plan must preserve these constraints:

- No CGO.
- Consumer-facing APIs live in the root package.
- The reusable hot path accepts `[]byte`, not `io.Reader`.
- The caller owns destination `Pix`, `Rect`, and `Stride`.
- Decode methods never allocate, replace, or alias the caller's `Pix`.
- Guest pixel memory is scratch; caller-owned Go memory is the product buffer.
- Only explicit `Reserve` may grow or relocate guest scratch memory.
- Repeated decode after `Reserve` into a correctly sized destination performs zero
  Go heap allocations on success.
- A `Decoder` is used by one goroutine at a time; concurrent callers use separate
  decoder instances.
- Generated bindings are never hand-edited.
- Guest and generated artifacts change only in plans that explicitly require a
  guest rebuild.
- Only fixture-backed, passing formats are documented or registered as supported.
- Image decoding is the product boundary; unrelated Wuffs codecs remain out of
  scope.

## Current baseline

The repository currently provides:

- `New`, `Version`, and `VersionNum`
- `(*Decoder).Probe`
- `(*Decoder).Reserve`
- caller-owned `(*Decoder).DecodeRGBA`
- `ErrBadImage`, `ErrDstTooSmall`, and `*DstTooSmallError`
- verified PNG and lossless WebP decode paths
- destination ownership, padded-stride, retry, persistence, concurrency, golden
  pixel, and allocation coverage
- zero-allocation repeated RGBA decode after explicit reservation

The explicit-Reserve workstream is complete. Source and destination reservations
grow independently and monotonically, the current source slot is the sole source
capacity authority, and neither `Probe` nor `DecodeRGBA` reserves or relocates
guest memory internally.

## Status values

| Status        | Meaning                                                              |
| ------------- | -------------------------------------------------------------------- |
| `Not planned` | No approved executable plan exists                                   |
| `Ready`       | An approved executable plan exists but has not been completed        |
| `In progress` | Its designated orchestrator is currently executing the plan          |
| `Review`      | Implementation is complete and awaiting final review                 |
| `Complete`    | Acceptance criteria passed and completion evidence was accepted      |
| `Blocked`     | Work cannot proceed until a recorded dependency or decision resolves |

## Work registry

| ID          | Capability                                        | Plan type | Plan file                                         | Depends on          | Guest changes | Status        |
| ----------- | ------------------------------------------------- | --------- | ------------------------------------------------- | ------------------- | ------------- | ------------- |
| `CORE-01`   | Explicit `Reserve` semantics                      | TDD       | `.pi/tdd-plans/enforce-explicit-reserve.yaml`     | Current baseline    | No            | `Complete`    |
| `CORE-02`   | Remove accidental public exports                  | Runbook   | `plans/core-02-public-api-boundary.md`            | `CORE-01`           | No            | `Complete`    |
| `STILL-01`  | `DecodeNRGBA` and `DecodeGray`                    | TDD       | `.pi/tdd-plans/still-01-decode-destinations.yaml` | `CORE-02`           | No            | `Complete`    |
| `CONV-01`   | Package-level allocating decode helpers           | Runbook   | `plans/conv-01-adherence-remediation.md`          | `STILL-01`          | No            | `Complete`    |
| `ADAPT-01`  | Reader and config adapters                        | TDD       | `.pi/tdd-plans/adapt-01-reader-adapters.yaml`     | `CONV-01`           | No            | `Complete`    |
| `FORMAT-01` | Verify common-image batch: BMP, GIF, JPEG         | TDD       | `.pi/tdd-plans/format-01-common-images.yaml`      | `CORE-01`           | No            | `Complete`    |
| `FORMAT-02` | Verify portable-image batch: NPBM, QOI, TGA, WBMP | TDD       | `.pi/tdd-plans/format-02-portable-images.yaml`    | `CORE-01`           | Yes           | `Complete`    |
| `FORMAT-03` | Verify remaining-image batch: ETC2, HNSM, NIE, TH | TDD       | `.pi/tdd-plans/format-03-remaining-images.yaml`   | `CORE-01`           | Yes           | `Complete`    |
| `REG-01`    | Register verified formats                         | TDD       | `.pi/tdd-plans/reg-01-register-formats.yaml`      | `ADAPT-01`, formats | No            | `Complete`    |
| `ANIM-01`   | Animation APIs                                    | TDD       | `plans/anim-01-animation-apis.md`                 | `CORE-02`           | Yes           | `Complete`    |
| `META-01`   | Metadata APIs                                     | TDD       | To be created                                     | `CORE-02`           | Yes           | `Not planned` |

## Dependency graph

```text
Current caller-owned RGBA baseline
                |
             CORE-01
                |
             CORE-02
          /      |       \
   STILL-01   ANIM-01   META-01
      |
   CONV-01
      |
   ADAPT-01
      |
    REG-01

CORE-01 ──> FORMAT-01, FORMAT-02, FORMAT-03 ──> REG-01
```

After `CORE-02`, the still-image, animation, and metadata tracks are technically
independent. Their displayed order is a priority recommendation, not an
implementation dependency. Format verification may proceed after `CORE-01`, but
`REG-01` must wait for both the adapter infrastructure and an explicitly verified
format set.

## Workstream definitions

### `CORE-01` — Explicit reservation

- Make source and destination scratch reservations monotonic.
- Make the current source slot the authoritative capacity.
- Remove internal `Reserve` calls from `Probe` and `DecodeRGBA`.
- Prove those methods cannot grow or relocate guest memory.
- Preserve guest-reported destination-scratch errors and zero-allocation decode.

### `CORE-02` — Public API boundary

- Compare exported symbols with `API.md`.
- Internalize implementation details such as raw metadata readers and memory
  layout inspection unless `API.md` explicitly includes them.
- Move necessary test access behind test-only helpers.
- Do not add new product behavior in this cleanup.

### `STILL-01` — Caller-owned still-image destinations

- Add `(*Decoder).DecodeNRGBA` and `(*Decoder).DecodeGray`.
- Apply the same ownership, shape validation, explicit reservation, persistence,
  padded-stride, retry, and zero-allocation contract as `DecodeRGBA`.
- Reuse shared validation only when type-specific bytes-per-pixel and conversion
  behavior remain explicit and testable.

### `CONV-01` — Allocating convenience APIs

- Add package-level `Probe`, `Decode`, `DecodeNRGBA`, `DecodeGray`, and
  `DecodeConfig`.
- Keep their allocating behavior clearly separated from reusable `Decoder`
  methods.
- Compose existing verified primitives instead of creating alternate decode
  paths.

### `ADAPT-01` — Standard-library adapters

- Add `DecodeReader` and `DecodeConfigReader` with standard `image` package
  semantics.
- Document that reading an `io.Reader` and allocating the returned image are not
  zero-allocation operations.
- Do not register formats in this workstream.

### `FORMAT-03` — Remaining-image formats

- Verify the remaining Wuffs image decoders (ETC2, HNSM, NIE, TH) as one batch.
- Add the format constant, guest dispatch support when needed, fixtures, probe
  coverage, golden decode evidence, corrupt-input behavior, and documentation.
- Do not advertise support until the complete format-specific plan passes.
- Keep generated changes isolated to the format plan that requires them.

### `REG-01` — Format registration

- Register only formats whose verification work is complete.
- Verify `image.Decode` and `image.DecodeConfig` behavior for every registered
  format.
- Keep registration opt-in through `RegisterFormats` as required by `API.md`.

### `ANIM-01` — Animation

- Add `Frame`, `Disposal`, `FrameCount`, `LoopCount`, and `DecodeFrame`.
- Preserve caller-owned canvas memory and explicit reservation semantics.
- Cover frame bounds, duration, disposal, overwrite/blend behavior, loop count,
  index errors, and frame-zero compatibility with `DecodeRGBA`.

### `META-01` — Metadata

- Add `Metadata`, `Chromaticities`, and opt-in metadata extraction.
- Cover absence, malformed payloads, format association, gamma,
  chromaticities, sRGB intent, modification time, and raw EXIF/ICC/XMP blobs.
- Do not decode pixels or mutate caller image memory while reading metadata.

## API traceability

| `API.md` area                       | Roadmap owner                         |
| ----------------------------------- | ------------------------------------- |
| Decoder lifecycle and reservation   | `CORE-01`                             |
| Public surface conformance          | `CORE-02`                             |
| RGBA/NRGBA/Gray decoder methods     | Baseline, `STILL-01`                  |
| Package-level convenience functions | `CONV-01`                             |
| Reader/config adapters              | `ADAPT-01`                            |
| Format constants and support        | `FORMAT-01`, `FORMAT-02`, `FORMAT-03` |
| `RegisterFormats`                   | `REG-01`                              |
| Animation types and methods         | `ANIM-01`                             |
| Metadata types and method           | `META-01`                             |

Every public symbol in `API.md` must map to the baseline or one roadmap ID. Add
a roadmap entry before approving a child plan for an unmapped symbol.

## Common completion gates

Each child plan must select and state the applicable gates rather than relying
on this list as executable instructions:

- focused unit or integration tests for the behavior under change
- `make format-check`
- `make lint`
- `make test`
- `make test-race` when decoder state, memory, or concurrency is affected
- `go build ./...`
- exact `errors.Is` and `errors.As` assertions for public error contracts
- caller slice pointer, length, capacity, rectangle, and stride identity checks
- guest memory length, backing pointer, and layout identity checks when implicit
  growth or relocation is prohibited
- allocation tests and `-benchmem` evidence for reusable decode paths
- golden pixel or metadata fixtures where output correctness is involved
- generated-artifact checks for host-only changes
- documentation and API traceability updates

Passing broad tests does not replace a child plan's focused behavioral proof.

## Decision log

| Decision                                         | Rationale                                                       |
| ------------------------------------------------ | --------------------------------------------------------------- |
| `Reserve` is monotonic per slot                  | Smaller requests must not invalidate reusable capacity          |
| Initial source reservation is 64 KiB             | Matches the public contract and removes split capacity state    |
| Current `SrcLen` is authoritative                | Prevents logical and physical source limits from diverging      |
| `Probe` and decode never reserve implicitly      | Makes allocation and guest-memory growth caller-controlled      |
| Only `Reserve` may grow or relocate guest memory | Preserves predictable reusable hot paths                        |
| Guest scratch errors remain guest-reported       | Retains authoritative decoded dimensions and required size      |
| Guest scratch and caller pixels remain separate  | Prevents wasm aliasing and preserves caller ownership           |
| Format support is evidence-based                 | Prevents unverified formats from being advertised or registered |

## Roadmap maintenance

- Update a row to `Ready` only after its executable plan is approved and saved.
- Update a row to `Complete` only after its designated orchestrator finishes and
  the completion evidence is accepted.
- Record the final plan path before implementation begins.
- When dependencies or scope change, update the registry, dependency graph, and
  traceability table together.
- Do not place implementation instructions, TDD cycles, worker prompts, or commit
  messages in this file.
