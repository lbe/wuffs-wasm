# Pi Runbook — wuffs-wasm Task Execution

**Purpose:** Execution runbook for the main agent orchestrating Pi subagents on
`wuffs-wasm` tasks.

**Applies to:** A plan the user explicitly asks Pi to execute.

**Does not replace the plan or product contract:** The selected plan defines the
implementation steps. Repository instructions and product documents define the
allowed behavior and scope.

---

## Sources of truth

Read the applicable sources before dispatching implementation:

1. `AGENTS.md` — user workflow and engineering rules.
2. The user-selected plan — task boundaries and acceptance criteria.
3. `GOALS.md` — product goals in the owner's words.
4. `API.md` — complete public API contract.
5. `HANDOVER.md` — scope and corrections for the current caller-owned `Pix`
   work.

For API work, `API.md` defines the final contract and `HANDOVER.md` limits the
current pass. Do not implement unrelated portions of `API.md` merely because
they are documented there.

`docs/package-capability-conversation.md` is background material, not a spec.
If a plan conflicts with `AGENTS.md`, `GOALS.md`, `API.md`, `HANDOVER.md`, or the
user's latest instruction, stop and report the conflict before implementation.

---

## Roles

| Role             | Who                       | Allowed                                                                                                                         | Forbidden                                                                                                                        |
| ---------------- | ------------------------- | ------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| **Orchestrator** | Main agent                | Read instructions and plans, choose the next task, draft prompts, evaluate reports, advance gates, notify the user              | Edit implementation files, run implementation commands or tests, commit, push, merge, or apply quick fixes                       |
| **worker**       | Preconfigured Pi subagent | Implement one task, format, run the requested verification, prepare an authorized commit, and commit only after review approval | Start another task, redesign outside the plan, modify unrelated files, push, merge, or touch `main` unless explicitly authorized |
| **reviewer**     | Preconfigured Pi subagent | Review the task diff, proposed commit message, verification evidence, and acceptance criteria; approve or reject with evidence  | Implement fixes, redesign, waive checklist items, or expand scope                                                                |

The Pi subagents are preconfigured. Invoke the exact subagent names `worker` and
`reviewer`; do not override or restate their underlying model selection.

**Hard rule:** The orchestrator only orchestrates. Implementation reads, edits,
formatting, tests, commits, and repository verification are delegated to
`worker` or `reviewer`.

If the required Pi subagents are unavailable, stop and notify the user. Do not
silently replace the workflow with direct implementation.

---

## Standing constraints

Include these constraints in every `worker` and `reviewer` dispatch:

1. Work only on the branch and task named by the plan. Never modify `main`
   unless the user or plan explicitly authorizes it.
2. Read the applicable sources of truth before acting. Do not invent scope.
3. Never use Python unless the user explicitly instructs it.
4. Do not hand-edit `internal/wuffswasm/wuffs.go`.
5. Run `make generate` only when the plan explicitly requires a guest rebuild or
   regenerated binding. Do not regenerate wasm for a host-only correction.
6. Run the narrowest relevant test first, then the broader gates required by the
   plan.
7. Do not weaken, delete, or skip a failing test to force green. If there is no
   clear in-scope production fix, stop and report the failure.
8. If a required command or tool is missing, stop and report it. Do not install
   system-wide software without user approval.
9. Never use `git commit --no-verify` or another hook-skipping mechanism unless
   the user explicitly authorizes it.
10. Never use `git add -f`, `git add --force`, or another ignore override unless
    the user explicitly authorizes it.
11. Never add unused-code suppression directives merely to make an intermediate
    task pass. Introduce a symbol in the same task that uses it, or revise the
    task boundary.
12. Do not commit unless the user or selected plan explicitly requires a commit.

This repository is a Go library. It has no `air` process, HTTP server, or
localhost smoke prerequisite. Do not introduce server smoke checks.

---

## Repository commands

Use the commands defined by the repository:

| Purpose            | Command                                                               |
| ------------------ | --------------------------------------------------------------------- |
| Task-specific test | `go test` with the narrowest applicable `-run` expression             |
| Full tests         | `make test`                                                           |
| Race tests         | `make test-race` when concurrency is affected or the plan requires it |
| Formatting check   | `make format-check`                                                   |
| Apply formatting   | `make format`                                                         |
| Lint               | `make lint`                                                           |
| Coverage gate      | `make cover` when the plan or final gate requires CI parity           |
| Build              | `go build ./...` when public or build-facing code changes             |
| Guest and bindings | `make generate` only when explicitly required                         |

`treefmt` applies `gofmt` and `goimports` to Go, `dprint` to Markdown, and
`shfmt` to shell scripts. Do not refer to nonexistent formatting scripts or
Prettier.

Tests in this repository deliberately distinguish unit and integration tests in
their names. Preserve that distinction. There is no `e2eweb` suite.

Test output may be reported directly. Redirect it to `tmp/` only when the output
is too large for a useful report; preserve the command's exit status and report
the failure details, not merely a filtered success line.

---

## Plan authoring rules

These rules apply to plans executed through this runbook.

### Every task boundary must be green

After each task:

- The tree must compile and pass the task-specific tests.
- All edited file types must pass `make format-check`.
- Go changes must pass the applicable lint, build, and test gates stated by the
  plan.
- A later task must not be required to repair an earlier task's dead code,
  formatting, imports, or broken tests.

### Introduce symbols with their first use

If a function, type, variable, file, or export is only used by a later task,
either defer it to that task or merge the definition and first use into one
task. Do not bridge task boundaries with unused-code suppressions.

### Track artifacts correctly

- `plans/` is not ignored. Plans placed there are ordinary working-tree files.
- `.pi/` is ignored and is appropriate for local TDD orchestration state.
- `tmp/` is ignored and is appropriate for transient reports and proposed
  commit messages.
- Before committing a deliverable, use `git check-ignore` to confirm that its
  destination is tracked normally. If it is ignored, stop and correct the plan
  path rather than force-adding it.

### Protect generated artifacts

- `internal/wuffswasm/wuffs.go` is generated by `wasm2go`; never hand-edit it.
- `wasm/wuffs.wasm` and the generated Go binding change only when the task
  explicitly requires `make generate`.
- Host-only changes such as the caller-owned `Pix` correction must not trigger
  regeneration without evidence of a guest bug.

### Reviewer checklists prove behavior

Each task must include acceptance criteria and commands that prove the behavior,
not merely that files exist. Allocation, ownership, aliasing, error identity,
and concurrency requirements need direct assertions when they are part of the
contract.

### Keep TDD behavior labels short

For `.pi/tdd-plans/*.yaml` plans executed by `tdd-orchestrator`, the `behavior`
field becomes part of the Red/Green/Refactor commit topic.

| Field                 | Purpose                                                              |
| --------------------- | -------------------------------------------------------------------- |
| `behavior`            | Short cycle label: at most 60 characters and approximately 5–8 words |
| `acceptance_criteria` | Full requirements, paths, assertions, and thresholds                 |

Do not put requirement lists or essay-length text in `behavior`. Put the detail
in `acceptance_criteria`.

### Plan anti-patterns

- Helpers, fixtures, or types added only for a later task
- Optional implementation forks that leave the worker to choose product scope
- Commit-required artifacts placed under `.pi/` or `tmp/`
- Formatting instructions that omit a touched file type
- `--no-verify`, force-add, or unused-code suppression used as an unblocker
- Guest regeneration for a host-only change
- A plan that broadens `HANDOVER.md` into the rest of `API.md`

---

## Commit policy

Commits are opt-in. A worker commits only when the user or selected plan
explicitly requests it.

When a commit is authorized:

1. One approved task produces one commit.
2. The worker implements and verifies the task but does **not** commit yet.
3. The worker writes the proposed message to `tmp/commit_message.txt`.
4. The reviewer checks the uncommitted diff, acceptance criteria, verification
   evidence, and proposed message.
5. After approval, the worker stages only the reviewed files and commits with
   `git commit -F tmp/commit_message.txt`.
6. The worker reports the commit hash and full `git log -1 --format=%B`.
7. The reviewer may perform a final metadata and scope check when the plan
   requires it.

Use the repository's established concise commit style:

```text
<type>(optional-scope): <imperative summary>

<body explaining why, when useful>
```

Do not add a synthetic version footer. This repository has no `version.go` and
does not use `Version:` commit-message footers.

If a post-commit check finds a bad message or incorrect contents, stop and ask
the user how to repair history. A follow-up commit cannot correct the metadata
or contents of the bad commit, and history must not be rewritten without
authorization.

---

## Execution loop

1. Dispatch `worker` to report the current branch, confirm the plan and source
   documents, and identify the next incomplete task.
2. Confirm the task is consistent with `AGENTS.md` and the applicable product
   documents.
3. Dispatch `worker` to implement exactly that task, run its verification, and
   prepare the proposed commit message if a commit is authorized.
4. Dispatch `reviewer` to inspect the uncommitted diff and evidence against the
   task checklist.
5. On rejection, dispatch `worker` on the same task with the review findings.
   Repeat for at most three worker attempts.
6. On approval, either:
   - mark the task complete when no commit was requested, or
   - dispatch `worker` to commit the exact reviewed changes and report the hash.
7. If the plan requires post-commit verification, dispatch `reviewer` against
   that hash. A post-commit rejection stops for user direction.
8. At a phase gate, dispatch `worker` to run the specified broader commands and
   `reviewer` to verify the evidence. Do not start the next phase until the gate
   passes.
9. After the final task and gate pass, stop. Do not push or merge unless the user
   explicitly asks.

If the worker fails the same task three times, stop and report the evidence to
the user. Use another preconfigured escalation subagent only when the user or
plan explicitly directs it.

### Progress tracking

When useful, have a subagent maintain `tmp/pi_progress.md`:

```markdown
# Pi progress — <plan name>

- Branch: <branch>
- Last approved: <task and commit, or "uncommitted">
- Next: <task>
```

The orchestrator does not edit this file directly.

---

## Prompt templates

### Dispatch `worker`

```text
You are the preconfigured Pi subagent "worker". Implement exactly one task and
stop after reporting its result.

Repository: wuffs-wasm
Branch: <branch named by the plan>
Plan: <path>
Task: <task ID or heading>
Commit authorized: <yes or no>

Read before acting:
- AGENTS.md
- the selected plan
- GOALS.md, API.md, and HANDOVER.md when the task affects product behavior

Follow the runbook standing constraints and the task's acceptance criteria.
Do not expand scope. Do not hand-edit internal/wuffswasm/wuffs.go. Run make
generate only when the task explicitly requires it.

Run the narrowest relevant test first, then the task's required broader gates.
If a required test fails without a clear in-scope production fix, stop and
report the evidence.

If Commit authorized is yes, do not commit before review. Write the proposed
message to tmp/commit_message.txt.

Report:
1. files changed
2. implementation summary
3. commands run and outcomes
4. proposed commit message, if applicable
5. blockers or scope conflicts
```

### Dispatch `reviewer`

```text
You are the preconfigured Pi subagent "reviewer". Review only; do not implement
or redesign.

Repository: wuffs-wasm
Branch: <branch named by the plan>
Plan: <path>
Task: <same task>
Commit authorized: <yes or no>

Read AGENTS.md, the selected plan, and the applicable GOALS.md, API.md, and
HANDOVER.md sections. Review the uncommitted diff against the task's acceptance
criteria and repository constraints.

Reject with evidence if:
- behavior or scope conflicts with the sources of truth
- unrelated files changed
- generated bindings were hand-edited or regenerated without task authority
- required assertions or verification are missing
- formatting, lint, build, tests, or race checks required by the task failed
- the proposed commit message is inaccurate or includes a nonexistent version
  footer

Reply APPROVE or REJECT. For rejection, list each failing item with evidence and
the required in-scope correction. For approval, give brief evidence for each
acceptance criterion.
```

### Stop report

```text
STOPPED — need your direction

Task: <task>
Problem: <source conflict, repeated failure, missing tool, unauthorized history
rewrite, unavailable subagent, or other blocker>
Evidence: <subagent reports and relevant commands/files>
Recommended correction: <specific next action>
```

---

## Orchestrator anti-patterns

- Implementing, testing, or committing directly to unblock a subagent
- Dispatching work before confirming the plan and source hierarchy
- Letting `API.md` broaden a task explicitly limited by `HANDOVER.md`
- Committing before reviewer approval
- Creating a second commit to disguise a rejected first commit
- Skipping task-specific ownership, aliasing, allocation, or error assertions
- Treating this Go library like a server application
- Referring to `air`, localhost smoke tests, `e2eweb`, `version.go`, or version
  commit footers
- Hand-editing or needlessly regenerating wasm bindings
- Using `--no-verify`, force-add, Python, or unused-code suppressions as an
  unblocker
- Pushing or merging without explicit user instruction

---

## First message to the orchestrator

```text
You are the orchestrator for wuffs-wasm task execution. Read:

plans/pi-runbook.md
<path-to-current-plan.md>

Use the preconfigured Pi subagents "worker" and "reviewer" exactly as defined in
the runbook. You only orchestrate; implementation and verification go through
those subagents. Execute one reviewed task at a time. Do not commit unless the
user or plan explicitly authorizes it. Do not push or merge without explicit
user instruction.
```
