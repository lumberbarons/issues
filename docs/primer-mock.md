# issues primer — lumberbarons/solar-controller

Workflow: issues ready → issues start <n> → branch (feat/|fix/|chore/) → PR body "Fixes #n".
PRs close issues; `issues close` is only for wontfix/duplicate.
File discovered work as you go: issues create ... --discovered-from <n>.
Never work an epic directly — work its children; `ready` already excludes epics.

Conventions (enforced by the tool):
- Every issue: one priority label P0(critical)–P4(backlog), one type label bug|enhancement|task, zero+ area labels.
- Dependencies are native (--blocked-by), never body text. Cycles are refused.
- Epics are sub-issue trees titled "Epic: ...".
- Body sections: ### Where / ### Problem or ### Goal / ### Fix or ### Approach / ### Done when (checklist).

Commands:
  issues ready | list [--label X --epic N --closed] | show <n>
  issues create --type T --title "..." --goal|--problem "..." --approach|--fix "..." --done-when "..." (repeatable)
                [--where X --priority P --area A --blocked-by N --parent N --discovered-from N]
  issues start <n> | close <n> --reason "..." | block <n> --on <m> | unblock <n> --from <m>
  issues epic create --title "..." [--children N,N] | epic status [<n>]
  All commands: --json, --repo owner/name.

## Ready (8 of 19 open)
#117 P1 bug (tests)  Tautological assertions on state the code cannot modify
#120 P2 enhancement  Voltgo BLE battery controller: scaffold, client, collector
#119 P2 enhancement (web-ui)  Proper auth: login flow with sessions, API keys
#118 P2 enhancement (web-ui)  Simple bearer-token auth, prompting only when required
#126 P2 bug (collector)  Modbus reconnect loops forever on stale file descriptor
#131 P3 task (tests)  Golden-file tests for renderer output
#133 P3 task  Extract shared retry/backoff helper from collectors
#135 P4 enhancement (docs)  Architecture overview diagram in README

## In progress (2)
#124 P2 bug (tests)  /api/info verified by substring matching  @lumberbarons
#128 P1 bug (collector)  Victron collector drops readings during BLE rescan  @lumberbarons

## Blocked (4)
#121 P2 enhancement  Voltgo poller wiring  ← blocked by #120
#122 P2 enhancement  Voltgo web-ui cards  ← blocked by #120 #121
#127 P2 task (deploy)  Canary rollout for collector config  ← blocked by #126
#134 P3 task  Remove legacy /api/v0 endpoints  ← blocked by #119

## Epics
#137 Voltgo BLE battery controller support  1/6
#140 Epic: Auth and session hardening  0/4
