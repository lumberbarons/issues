# issues

An opinionated, agentic-first CLI for tracking work in GitHub Issues. Inspired by
[beads](https://github.com/steveyegge/beads), backed entirely by GitHub — native
sub-issues and dependencies, priority/type/area labels, ready-work detection, and an
`issues prime` command that injects tracker conventions and live state into a coding
agent's context at session start.

GitHub Issues stays the single source of truth: humans get the web UI, PRs auto-close
issues via `Fixes #n`, and nothing needs syncing.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/lumberbarons/issues/main/install.sh | bash
```

or, with a Go toolchain:

```sh
go install github.com/lumberbarons/issues/cmd/issues@latest
```

Authentication comes from the [`gh` CLI](https://cli.github.com/) — run
`gh auth login` once and `issues` reuses its stored credentials. The target
repository is detected from the git remote (`--repo owner/name` overrides).

## Quickstart

```sh
issues init          # bootstrap the label set in a repo; prints a CLAUDE.md snippet
issues prime         # session-start context: conventions + ready work + live state
issues ready         # what should I work on? (priority-sorted, zero open blockers)
issues start 42      # claim it: assign @me + in-progress (refuses claimed work, exit 3)
# ...branch, PR with "Fixes #42"...
```

## Commands

```
issues prime                      # session-start context for agents
issues ready                      # open, non-epic, zero open blockers; P0→P4 then P?
issues list [--label X] [--epic N] [--closed]
issues show <n>                   # body, deps, parent, children, recent comments
issues create --type bug|enhancement|task --title "..."
              [--priority P0..P4] [--area X] [--blocked-by N] [--parent N]
              [--discovered-from N] [--body-file F | --edit]
issues start <n> [--priority P0..P4] [--force]
issues triage                     # issues missing priority/type labels
issues set <n> [--priority ..] [--type ..] [--add-area X] [--remove-area X]
           [--parent N | --no-parent] [--title "..."]
issues close <n> --reason "..." [--completed | --duplicate-of M]
issues block <n> --on <m>         # native dependency, cycle-checked
issues unblock <n> --from <m>
issues epic create --title "..." [--children N,N]
issues epic status [<n>]
issues init
```

Every command takes `--json` (stable flat schema) and `--repo owner/name`. Exit
codes are meaningful: `3` means "already claimed, pick the next ready item", `4`
means "run `gh auth login`".

## Design

See [DESIGN.md](DESIGN.md) for the conventions the tool enforces, the read-path
normalization rules, and the API strategy.
