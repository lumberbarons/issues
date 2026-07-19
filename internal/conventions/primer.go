package conventions

// PrimerStatic is the conventions-and-cheatsheet half of `issues prime`,
// kept deliberately terse: the whole primer targets ~600 tokens.
const PrimerStatic = `Workflow: issues ready → issues start <n> → branch (feat/|fix/|chore/) → PR "Fixes #n".
Close via PR; issues close is for wontfix/duplicate only. Never work an epic directly.
File discovered work: issues search <terms> first (dupes), then issues create ... --discovered-from <n>.

Conventions (enforced by the tool's write path):
- One priority label P0(critical)..P4(backlog), default P2; one type label bug|enhancement|task.
- Area labels sparingly — only once several issues would share one. No title prefixes; labels carry the metadata.
- Dependencies are native (--blocked-by), never body text. Epics are sub-issue trees.
- Bodies: ### Where / ### Problem or ### Goal / ### Fix or ### Approach / ### Done when (checklist). Omit empty sections.
- Missing priority renders P? and sorts last; issues missing priority/type are untriaged, not broken — triage them via issues set.
- start refuses claimed issues (exit 3): pick the next ready item. Untriaged issues need start --priority.

Output: one line per issue — #n priority type (areas) title [blocked by #m; epic done/total; in progress @user].
list sorts ready work first, then claimed, blocked, epics. Prefer text output; --json on list commands emits NDJSON.

Commands: ready | list [--label X --epic N --closed] | show <n> | search <terms> | triage |
create --type T --title "..." [--priority Pn --area X --blocked-by N --parent N --discovered-from N --body-file F] |
start <n> [--priority Pn] | set <n> [--priority Pn --type T --add-area X --remove-area X --parent N --no-parent --title "..."] |
close <n> --reason "..." [--completed | --duplicate-of M] | block <n> --on <m> | unblock <n> --from <m> |
epic create --title "..." [--children N,N] | epic status [<n>] |
apply <plan.jsonl> [--dry-run] (batch create from a JSONL plan; schema: issues apply --help). All take --json.`

// ClaudeSnippet is what `issues init` prints for the repo's CLAUDE.md: the
// point of prime is that the snippet stays this short.
const ClaudeSnippet = `## Issue tracking

Work is tracked in GitHub Issues via the ` + "`issues`" + ` CLI.
Run ` + "`issues prime`" + ` at session start — it prints the conventions,
the ready work, and current state. When in doubt: ` + "`issues ready`" + `.`
