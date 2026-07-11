# issues — an agentic-first CLI for GitHub Issues

## Vision

An opinionated, single-user CLI that makes GitHub Issues work the way `bd` (beads)
works: instant answers to "what should I work on?", conventions enforced by the tool
instead of prose in CLAUDE.md, and token-lean output designed for agent context
windows. GitHub Issues stays the single source of truth — humans get the web UI,
PRs auto-close issues via `Fixes #n`, and nothing needs syncing.

The tool exists because the raw `gh` CLI has (since v2.94.0) all the *primitives* —
native sub-issues, blocked-by dependencies, JSON fields — but none of the *opinions*:
no ready-work detection, no convention enforcement, verbose output, and every agent
session re-derives the same jq pipelines from scratch.

### Non-goals

- Multi-user / team workflows. This is for one human and their agents.
- A local database or offline mode (v1). Every command hits the API; a cache is a
  later milestone only if latency actually hurts.
- General GitHub client. Anything the tool doesn't have an opinion about, use `gh`.

## Concepts borrowed from beads

| beads | this tool | GitHub mechanism |
|-------|-----------|------------------|
| `bd ready` — zero open blockers | `issues ready` | `blockedBy` (native dependencies), filtered + priority-sorted |
| `bd prime` — session-start context injection | `issues prime` | generated from live repo state + built-in conventions |
| hierarchical IDs (`bd-a3f8.1`) for epics | `issues epic` | native sub-issues (`parent` / `subIssues`) |
| `bd update --claim` — claim: assign + in-progress | `issues start` | assign `@me` + `in-progress` label |
| priorities 0–4 | P0–P4 labels | labels (issue *types* are org-only; labels work on personal repos) |
| token-lean, JSON-optional output | same | `--json` on every command, compact text default |
| `bd remember` — persistent insights | deferred (open question) | overlaps with Claude Code's own memory system |

Explicitly *not* borrowed: Dolt/git storage, hash IDs, sync — GitHub is the backend,
so the entire distributed-state problem beads solves disappears.

## Enshrined conventions

These are the conventions already proven in solar-controller's CLAUDE.md, moved from
prose into code. The tool *enforces* them; `prime` *teaches* them.

- **Priority labels**, every issue gets exactly one: `P0` (critical) → `P4` (backlog).
  Default `P2`.
- **Type labels**, exactly one: `bug`, `enhancement`, `task`.
- **Area labels**, zero or more, flat names (`tests`, `web-ui`, ...). Created
  sparingly.
- **No title prefixes** — type/priority/area live in labels. Exception: `Epic: ` on
  parent issues, added by the tool.
- **Dependencies are native** (`--blocked-by`), never body text. The tool refuses to
  create cycles.
- **Epics are sub-issue trees.** Epics are never worked directly; `ready` excludes
  them.
- **Discovered work** links back: `Discovered while working on #123` in the body,
  via `--discovered-from 123`.
- **Body template**: `### Where` / `### Problem`|`### Goal` / `### Fix`|`### Approach` /
  `### Done when` (checklist). Scaffolded by `create`, sections omitted when empty.
- **Workflow**: `ready` → `start` → branch (`feat/`|`fix/`|`chore/`) → PR with
  `Fixes #n`. Closing via PR is the norm; `close` is for wontfix/duplicate.
- **Drift**: the tool enforces conventions on its own write paths, but the web UI
  can still violate them (two priority labels, missing type, epic worked directly).
  `prime` warns about violations it sees — it already has the data. A fuller
  `doctor --fix` is deferred to M4.

## Command surface (v1)

```
issues prime                      # session-start context (see below)
issues ready                      # open, non-epic, zero *open* blockers; sorted P0→P4, then oldest
issues list [--label X] [--epic N] [--closed]
issues show <n>                   # detail: body, deps, parent, children, recent comments
issues create --type bug|enhancement|task [--priority P0..P4] [--area X]
              [--blocked-by N...] [--parent N] [--discovered-from N]
              --title "..." [--body-file F | --edit]
issues start <n>                  # claim: assign @me, add in-progress label
issues set <n> [--priority P0..P4] [--type bug|enhancement|task] [--add-area X]
           [--remove-area X] [--parent N | --no-parent] [--title "..."]
                                  # retriage/edit within conventions (swaps the old
                                  # priority/type label, never stacks a second one)
issues close <n> --reason "..."   # comment + close (not-planned unless --completed
                                  # or --duplicate-of M)
issues block <n> --on <m>         # add dependency (cycle-checked)
issues unblock <n> --from <m>
issues epic create --title "..." [--children N,N,N]
issues epic status [<n>]          # progress rollup per epic
issues init                       # bootstrap labels in a repo; print CLAUDE.md snippet
```

Global flags: `--json` (structured output, stable schema), `--repo owner/name`
(default: detect from git remote via go-gh).

### `issues prime`

The session-start ritual, modeled on `bd prime`: one command whose output an agent
injects at the top of a session (via CLAUDE.md instruction or hook) instead of
maintaining hand-written workflow prose. Three parts:

1. **Static primer** — the conventions and workflow above, compressed to a few
   hundred tokens, including the tool's own command cheatsheet.
2. **Live state** — ready work (top N by priority), in-progress issues and their
   assignee, epics with progress (`#137 Voltgo 2/6`), and open-blocker counts.
3. **Warnings** — convention violations spotted in the fetched data (`⚠ #42 has two
   priority labels`). Section omitted entirely when the repo is clean, which is the
   normal case.

Sketch:

```
# issues primer — lumberbarons/solar-controller
Workflow: issues ready → issues start <n> → branch (feat/|fix/|chore/) → PR "Fixes #n".
File discovered work with --discovered-from. Never work an epic directly.

## Ready (3 of 14 open)
#120 P2 enhancement  Voltgo BLE battery controller: scaffold, client, collector
#117 P1 bug (tests)  Tautological assertions on state the code cannot modify
#119 P2 enhancement  Proper auth: login flow with sessions, API keys

## In progress (1)
#124 P2 bug (tests)  /api/info verified by substring matching  @lumberbarons

## Epics
#137 Voltgo BLE battery controller support  0/6
```

Everything after the header is one line per issue: `#n priority type (areas) title`.
No URLs, no timestamps, no prose. Target: whole primer under ~600 tokens for a
typical repo.

## Output principles

- Default output is compact fixed-column text, one line per issue, no ANSI when not
  a TTY, no URLs (agents know `#n` + repo), stable sort order.
- `--json` everywhere, with a flat schema (deps as number arrays, not
  `{nodes:[...]}` wrappers — hide GraphQL shapes from consumers).
- Errors are one line, actionable, exit codes meaningful (`ready` with no results
  exits 0 with `no ready work`; auth failure exits 4; etc.).

## Architecture

- **Language/runtime**: Go (single static binary, `go install`-able).
- **CLI framework**: `github.com/urfave/cli/v3` (v3.10.1 at time of writing).
- **GitHub access**: `github.com/cli/go-gh/v2` (v2.13.0) — reuses `gh`'s stored
  credentials and host config, provides REST + GraphQL clients and repo detection
  from the git remote. No own auth flow at all; `gh auth login` is a prerequisite.
- **API strategy**: one GraphQL query per command where possible. `ready`/`prime`
  fetch all open issues with `blockedBy`, `parent`, `subIssues`, labels, assignees
  in a single paginated query and filter client-side — avoids N+1 and stays trivially
  inside rate limits for single-user scale.
- **Layout**:
  - `cmd/issues/` — main, urfave/cli command wiring
  - `internal/gh/` — thin API layer (interface, so commands are testable against a fake)
  - `internal/model/` — Issue/Epic domain types, ready/cycle logic (pure, unit-tested)
  - `internal/render/` — text + JSON renderers (golden-file tests)
  - `internal/conventions/` — labels, body template, primer text (the opinions live here)
- **Testing**: unit tests against a fake API layer; golden files for renderer output;
  one integration smoke test behind a build tag that hits a real scratch repo.

## Milestones

- **M0 — scaffold**: module, urfave/cli v3 skeleton, go-gh auth + repo detection,
  `issues list` (proves the GraphQL query and renderer end-to-end). The query must
  include `parent`/`subIssues`/`blockedBy` from day one — this milestone verifies
  the exact field names, any feature headers, and whether the API rejects dependency
  cycles natively (if it does, our cycle check is just a friendlier error).
- **M1 — read**: `ready`, `show`, `epic status`, `prime` v1. *This is the payoff
  milestone — adopt in solar-controller immediately.*
- **M2 — write**: `create` (template + label enforcement), `set` (retriage —
  priority changes are the most common tracker operation, and doing them through
  the tool is what keeps the one-label invariants true), `block`/`unblock` with
  cycle detection, `start`, `close`, `epic create`.
- **M3 — bootstrap**: `init` (create label set in a fresh repo, emit the CLAUDE.md
  snippet that says little more than "run `issues prime`"). Replace solar-controller's
  hand-written conventions section with it.
- **M4 — polish**: `--json` everywhere, `doctor --fix` (bulk convention repair, if
  drift turns out to happen in practice), pagination hardening, maybe a read cache,
  maybe `remember`, maybe a `gh` extension alias (`gh-issues`) for distribution.

## Open questions

- **Name.** Working name `issues` (binary and repo). Generic but reads perfectly in
  agent transcripts (`issues ready`, `issues prime`). Alternatives if it collides:
  `iz`, `ghi`, `beans`.
- **`remember`.** beads couples memory to the tracker; Claude Code has its own
  memory system. Skip, or implement as comments on a pinned "agent notes" issue?
  Deferred to M4 — need real usage first.
- **in-progress signal.** Label (visible, filterable) vs assignee-only (no label
  churn). v1: both — assign is the claim, label is the visibility.
- **Multi-repo prime.** Someday `issues prime --all-repos` for a workspace overview?
  Out of scope for v1.
