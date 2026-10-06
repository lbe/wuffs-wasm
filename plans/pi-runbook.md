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

If the orchestrator cannot proceed without violating the hard rule (for example,
subagents cannot be spawned, or only the orchestrator can run a required
command), stop immediately, notify the user with a recommended correction, and
do not implement the work yourself.

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
    the user explicitly authorizes it. **A plan alone is not authorization.** If
    a path is ignored or a plan appears to require force-add, stop and notify
    the user.
11. Never add unused-code suppression directives (for example `//nolint:unused`
    or `//lint:ignore U1000`) unless the user explicitly authorizes it. **A plan
    alone is not authorization.** Introduce a symbol in the same task that uses
    it, revise the task boundary, or stop and notify the user.
12. **Worker test budget:** Run only the current task's **Verify** commands
    before any commit. Never run `make test`, `make test-race`, `make cover`,
    `go test ./...`, or another full-suite command unless that task's Verify
    block or the plan's **final** verification task explicitly requires it. See
    **Worker test budget** below.
13. **Plan executor STOP (zero deviation):** If any plan step fails, conflicts
    with facts (tools, APIs, tests), is ambiguous, or requires a path outside
    the task whitelist, stop immediately, notify the user with task id and
    evidence, and do not edit the plan, change assertions or helpers to "match
    intent," commit, or start the next task. Await written user direction. See
    **Plan executor STOP (zero deviation)** below. Audit-plan workers edit only
    the plan file named in the audit; they never implement product code or
    `git commit`.
14. Do not commit unless the user or selected plan explicitly requires a commit.

This repository is a Go library. It has no `air` process, HTTP server, or
localhost smoke prerequisite. Do not introduce server smoke checks.

---

## Worker test budget (mandatory)

Workers must not burn wall time re-running broad gates when the plan scoped
verification to a narrow command set.

### Before commit (worker)

1. Copy the plan task's **Verify** block verbatim into the worker run.
2. Run only those commands (plus `go build ./...` when the Verify block says
   so).
3. **Forbidden** on every task except the plan's designated final verification
   task: `make test`, `make test-race`, `make cover`, `go test ./...`, or any
   test command not listed in that task's Verify block.
4. Allowed scoped patterns when the Verify block lists them: `go test` with
   `-run`, package-scoped `go test ./path/...`, `make format-check`, `make
   lint`, and redirects to `tmp/` when output is too large.

### On commit

5. The worker does not run a full suite immediately before commit unless the
   Verify block explicitly requires it and the plan author intended a double
   run (almost never — do not do this by default).

### Orchestrator

6. Paste the task **Verify** block into every worker dispatch.
7. Stop the run if a worker report lists `make test`, `make test-race`,
   `make cover`, or `go test ./...` and the current task is not the plan's
   final full-suite task.
8. Do not tell workers to "run the full suite to be safe."

### Reviewer

9. Reject if the worker report's verification section includes any command not
   in the task **Verify** block (except output from `git commit` hooks on the
   same task, when hooks exist).
10. Reject if the worker ran `make test`, `make test-race`, `make cover`, or
    `go test ./...` on a non-final task.

### Plan author

11. Put scoped Verify commands on every committing task; reserve **`make test`**
    (and race or cover when CI parity matters) for **one** final task unless the
    user explicitly orders otherwise.
12. Whitelist every path that must change for that commit's tree to pass format,
    lint, build, and the task's tests. Do not rely on a later task to fix
    compile or gate failures from an earlier whitelist.

### Reviewer — plan executor STOP (mandatory reject)

13. Reject if the worker edited any `plans/*.md` or the plan path under
    execution (unless the task is explicitly an audit-plan **Worker**
    iteration editing only that plan file).
14. Reject if the worker continued after a failing RED or Verify step without a
    user message authorizing the fix path.
15. Reject if the worker added or changed tests or helpers to work around a plan
    contradiction without user approval documented in chat.

---

## Plan executor STOP (zero deviation)

Applies to any agent executing a plan under this runbook or acting as `worker` /
`reviewer`.

### Forbidden without explicit user approval in chat

- Editing, renaming, or "fixing" any `plans/*.md` (or the plan path named in the
  prompt), except audit-plan workers editing only the audited plan file
- Changing task scope, Verify commands, whitelists, or assertions because tests
  failed or steps seem wrong
- Substituting a different approach while claiming the same intent
- Continuing to the next task after any STOP trigger below
- Committing after a STOP trigger before the user replies with written direction

### STOP immediately — notify the user — do not commit — do not proceed

Stop on the **first** occurrence. The message must include task id, what failed
or conflicts, exact command output or plan line, and that execution is stopped
awaiting user direction.

| Trigger                          | Examples                                                                   |
| -------------------------------- | -------------------------------------------------------------------------- |
| Plan step impossible or false    | Assertion contradicts library behavior or documented API                   |
| Verify or RED fails unexpectedly | Fails after a GREEN step when the plan said it should pass                 |
| Plan ambiguity                   | Two interpretations; whitelist does not list a file the compiler requires  |
| Missing prerequisite             | Required tool or command missing                                           |
| Scope escape                     | Change needed outside the task whitelist                                   |
| Production vs test fix unclear   | Failing test might need prod fix, test fix, or plan fix — default **STOP** |
| Audit or review role             | Audit-plan and reviewers never implement product code or `git commit`      |

Do not weaken tests, edit the plan, or add helpers to unblock unless the user
explicitly authorizes that exact change in a follow-up message.

### Allowed

- Implement only what the current task and whitelist say
- Fix production code when the plan's GREEN step requires it and tests validate
  that step
- Run only commands in the task **Verify** block (plus hooks on commit when
  present)

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
- Do not use or instruct unused-code suppressions unless the user has explicitly
  approved that suppression for that symbol. A plan must not pre-authorize them;
  if the only way to land a task is such a suppression, rewrite the task split
  instead.

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
- Never write a plan that requires `git add -f` or `--force`. If a deliverable
  path is ignored, fix the plan path — do not instruct force-add.

### Executor STOP gates (mandatory)

Executable plans **must** contain this section (copy and adapt triggers; add
plan-specific gates).

**Global STOP rules (reference only — do not copy into execution prompts):**
standing constraint **13** and **Plan executor STOP (zero deviation)** in this
runbook. Plan-specific triggers live **only** in the table in this section.

```markdown
## Executor STOP gates (mandatory)

**Global rules:** `plans/pi-runbook.md` standing constraint 13; **Plan executor STOP (zero deviation)** in the same runbook.

Executors **must not** edit this plan file. On any trigger below (or any global rule): **STOP**, message the user (task id, evidence), **no commit**, **no next task** until written user direction.

| # | Trigger                                                         | Executor action                                       |
| - | --------------------------------------------------------------- | ----------------------------------------------------- |
| 1 | Prerequisite check fails                                        | STOP — notify user                                    |
| 2 | Plan step contradicts runtime, libs, or tests                   | STOP — notify user (do not change plan or assertions) |
| 3 | Verify fails for reason not explained by current RED/GREEN step | STOP — notify user                                    |
| 4 | Required file not in task whitelist                             | STOP — notify user                                    |
| 5 | <plan-specific gates>                                           | STOP — notify user                                    |
```

### No optionality in plans (mandatory)

Plans executed under this runbook are **specs**, not essays.

- **Forbidden anywhere in the plan document:** the whole word `optional`
  (case-insensitive), and phrases such as "if needed", "prefer", "consider",
  "either … or …" for design forks, "follow-up (not this plan)", "as needed",
  and "not a gate" when they leave scope open.
- **Required:** one decision per design choice; explicit whitelists (concrete
  paths, not `*/*` globs alone); every committing task has an exact **Verify**
  block plus the FORBIDDEN footer below; name tests or functions when only a
  subset uses an alternate helper.
- Before publishing a plan, run:
  `rg -i '\boptional\b' plans/<this-plan>.md` — must be empty. Same scan for
  "prefer ", "if needed", and "either " when they introduce forks.

See also the **audit-plan** skill: precondition abort on `\boptional\b`.

### Every committing task: Verify block and test-budget footer

Each task that commits must end its **Verify** section with this line
(verbatim):

```text
FORBIDDEN before commit: make test, make test-race, make cover, go test ./..., or any test command not listed above.
```

The **Verify** block lists scoped tests only. Exactly **one** plan task (final
verification, usually no commit) may require `make test` or broader gates.
Intermediate tasks must not.

**Whitelist rule:** If changing production behavior in task N breaks a test
outside the stated file list, add that test file to task N's whitelist or merge
tasks — do not leave gate failures for task N+k.

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
- Verify blocks that omit scoped tests, push `make test` into every task, or tell
  workers to "run full tests to be safe"
- Task N whitelist that excludes test files that must change for task N's commit
  to pass format, lint, build, or tests
- Missing `## Executor STOP gates (mandatory)` on an executable plan
- Essay-length `behavior` strings in `.pi/tdd-plans/*.yaml`

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
9. Stop if a worker report violates the **Worker test budget** (full suite on a
   non-final task, or commands outside the task Verify block).
10. After the final task and gate pass, stop. Do not push or merge unless the user
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

TEST BUDGET: Run ONLY the commands in this task's Verify block before any
commit. FORBIDDEN unless Verify or the plan's final verification task lists
them: make test, make test-race, make cover, go test ./..., or any test not in
Verify.

ZERO DEVIATION: Read `<path-to-plan.md>` section `## Executor STOP gates
(mandatory)` plus standing constraint 13 and **Plan executor STOP (zero
deviation)** in plans/pi-runbook.md. Do not edit the plan.

Run the narrowest relevant test first, then the task's required broader gates.
If a required test fails without a clear in-scope production fix, stop and
report the evidence.

If Commit authorized is yes, do not commit before review. Write the proposed
message to tmp/commit_message.txt.

Run before commit (paste task Verify block here):
<Verify commands from plan>

Report:
1. files changed
2. implementation summary
3. verify commands run (exact command lines only — every line must appear in the
   task Verify block unless commit hooks on the same task)
4. proposed commit message, if applicable
5. blockers or scope conflicts
```

Orchestrator: paste the task **Verify** block into the dispatch under "Run before
commit". Do not paste STOP boilerplate into worker prompts — the plan STOP
section and this runbook are the source of truth.

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

PLAN EXECUTOR STOP (mandatory reject):
- worker edited `plans/*.md` (except audit-plan Worker editing only the audited
  plan file)
- worker continued after Verify or RED failure without user-written direction in
  chat
- worker changed tests or helpers to work around a plan contradiction without
  user approval

TEST BUDGET (mandatory reject):
- worker report lists make test, make test-race, make cover, go test ./..., or
  any command not in the task Verify block (unless this task is the plan's final
  full-suite task)
- worker ran a full suite "to be safe" before a non-final task

Reply APPROVE or REJECT. For rejection, list each failing item with evidence and
the required in-scope correction. For approval, give brief evidence for each
acceptance criterion.
```

### Standard execution prompt (Cursor Agent)

Each executable plan **should** end with `## Execution prompt (for Cursor Agent)`
using this shape (no STOP boilerplate in the paste):

```text
Execute <path-to-this-plan.md> Tasks <range> in order.
Follow plans/pi-runbook.md (test budget, commits, format).
Obey this plan's ## Executor STOP gates (mandatory) and pi-runbook standing constraint 13.
Progress file: <path from plan header, if any>.
Do not push unless I instruct.
Begin with <first gate or Task N>.
```

Orchestrators and users paste only that block (or the plan's published copy).
Workers load STOP rules from the plan and this runbook.

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
- Telling a worker to run `make test` or `go test ./...` "to be safe" on a
  non-final task
- Approving a task when the worker violated plan executor STOP or edited the
  plan under execution
- Copying STOP boilerplate into worker prompts instead of pointing at the plan
  and this runbook

---

## First message to the orchestrator

```text
You are the orchestrator for wuffs-wasm task execution. Read:

plans/pi-runbook.md
<path-to-current-plan.md>

Use the preconfigured Pi subagents "worker" and "reviewer" exactly as defined in
the runbook. You only orchestrate; implementation and verification go through
those subagents. If you cannot proceed without acting yourself, STOP and notify
me with a recommended fix. Execute one reviewed task at a time. Obey worker
test budget and plan executor STOP rules. Do not commit unless the user or plan
explicitly authorizes it. Do not push or merge without explicit user instruction.
```
