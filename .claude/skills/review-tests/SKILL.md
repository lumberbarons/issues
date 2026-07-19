---
name: review-tests
description: Reviews tests for issues that coverage tools miss — falsifiability, isolation hazards, dead expectations, tautological assertions, missing edge cases, and unclear test names. Use when asked to review tests, audit a test suite, check test quality, validate test isolation, or check whether tests actually catch regressions. Also invoke when a user says things like "review these tests", "audit tests/", "are these tests any good", "do these tests catch real bugs", "check test isolation", "is this tested", or asks for a quality-level (not coverage-percentage) read of unit, integration, or e2e tests.
---

# Tests Review

Review tests in the specified path for quality issues.

> [!IMPORTANT]
> Consult [REFERENCE.md](REFERENCE.md) for the expected output format and level of detail.

## Scope

Determine the review scope before discovering files:

- If `$ARGUMENTS` is non-empty, treat it as a path (file or directory) and run:
  ```bash
  .claude/skills/review-tests/scripts/discover-files.sh "$ARGUMENTS"
  ```
- If `$ARGUMENTS` is empty, scope to files added or modified on the current branch relative to the default branch:
  ```bash
  .claude/skills/review-tests/scripts/discover-files.sh
  ```

Handle the script's exit codes:
- **0 with output** — use the listed paths as input to the discovery step below.
- **0 with empty output** — branch has no diff vs the default branch. Tell the user and ask which path to review.
- **non-zero** — script prints a message to stderr (path not found, not a git repo, on the default branch with no path, detached HEAD, or default branch indeterminate). Relay the message and ask the user which path to review.

The script returns paths language-blind. The discovery step below filters to test files; if the filter matches nothing but the script's output was non-empty, the language may not be in the pattern list — apply judgment to identify test files in the output.

## Workflow

### Step 1 — Discover test files

From the script's output, filter to test files using language-appropriate patterns: `*_test.go`, `test_*.py`, `*_test.py`, `*.test.ts`, `*.test.js`, `*.spec.ts`, `*.spec.js`, `__tests__/**`, etc. Record the full file list and count.

### Step 2 — Choose execution strategy

- **1–2 files → Direct mode**: Read the files, evaluate against the Quality Criteria below, attach a short pattern label to each finding (same scheme as parallel mode — see Parallel Review Mode → Spawn subagents, item 7), then proceed to Pattern Collapsing.
- **3+ files → Parallel mode**: Batch files, spawn subagents, collect results, merge, then proceed to Pattern Collapsing.

### Parallel Review Mode

Use this mode when 3 or more test files are discovered.

#### Batching

Group files into batches based on total file count:

| Total files | Files per batch | ~Subagents |
|-------------|-----------------|------------|
| 3–10        | 1               | 3–10       |
| 11–20       | 2               | 6–10       |
| 21+         | 3               | 7–10       |

#### Spawn subagents

For each batch, use `Agent(subagent_type="general-purpose")`. **Spawn all subagents in a single message** so they run in parallel.

Each subagent prompt MUST include:

1. The file paths in its batch (instruct the subagent to read them)
2. The **Quality Criteria** section from this skill — copy it verbatim into the prompt
3. The **Severity** section from this skill — copy it verbatim into the prompt
4. The structured output format below
5. The explicit instruction: **"Do NOT use the Bash tool. Do NOT run any shell commands. Use only Read, Grep, and Glob tools. Return findings only."** — the review is static analysis of test files, so shell access adds latency and side-effect risk without enabling anything the read-only tools can't already do.
6. The explicit instruction: **"For every P2 and P3 finding, you MUST state a concrete falsifiability claim in the `explanation` field: 'if \<specific production change\> were made, this test would still incorrectly pass.' Omit findings that lack this claim. Two exceptions: P1 tautological tests (the claim is implicit — no production change can fail the test) and unclear-test-name findings (judged on readability, not falsifiability)."**
7. The explicit instruction: **"For the `pattern` field, use a short, reusable label that names the underlying anti-pattern (e.g., 'module-scope mutable mocks', 'tautological assertions'). If two findings in your batch stem from the same root cause, they MUST use the same pattern label."**

Instruct each subagent to return findings in this exact delimited format (one block per finding):

```
---FINDING---
priority: P<1|2|3>
location: <file:line>
title: <short title>
category: <Completeness|Usefulness|Coverage Gaps|Output Validation|Isolation|Readability|Integration Test Specifics>
pattern: <short label for the underlying anti-pattern, e.g. "module-scope mutable mocks" or "tautological assertions" — use the SAME label across findings that share the same root cause>
explanation: <what is wrong or missing and why it matters>
fix: <concrete prescription>
done_when: <verifiable criterion>
---END---
```

If the subagent finds no issues for its batch, it should return `---NO-FINDINGS---`.

#### Collect and merge

After all subagents return:

1. Parse each subagent's structured findings
2. Combine into a single list, sorted by priority (P1 first)
3. Deduplicate: if two findings share the same `location` (file:line) AND the same `category`, keep only the one with the highest priority
4. Group findings by `pattern` label — findings from different subagents that used the same (or very similar) pattern label share a root cause and will be collapsed in the Pattern Collapsing step

#### Error fallback

If a subagent fails or returns unparseable output, review those files directly (as in direct mode) and include a note in the report: `Note: Files [list] were reviewed directly due to subagent failure.`

### Pattern Collapsing

Both direct mode and parallel mode flow into this step before producing the final report.

After merging all findings, look for findings that share the **same root cause** — i.e., the same testing anti-pattern repeated across multiple test files. Examples:

- Multiple test files flagged for "module-level mutable mock leaks between tests" → one pattern: "test suite uses module-scope mocks instead of per-test setup"
- Multiple test files flagged for "globalThis.fetch replaced at module scope" → one pattern: "fetch mocking is done at import time instead of in beforeEach"
- Multiple test files flagged for "assertions only check error status, not return values" → one pattern: "tests validate calls were made but not results returned"

When you identify a shared root cause:

1. **Collapse** the N per-file findings into **one finding** that names the pattern, lists all affected files, and prescribes the codebase-wide fix
2. **Set severity** to the highest severity among the collapsed findings
3. **Keep separate** any findings that happen to share a category but have genuinely different root causes

This is critical: N findings for N instances of the same pattern creates noise. One finding that names the pattern and lists the affected locations is actionable.

## Quality Criteria

### Completeness
- Edge cases covered
- Error paths tested
- Boundary conditions checked
- Happy path and failure scenarios both present

### Usefulness
- Tests validate behavior, not implementation details
- Tests would fail if the code broke
- High coverage alone is not proof of quality — a tautological test covers lines without catching regressions

### Coverage Gaps
- Production code paths in the review scope should not ship with zero test coverage
- Use the asymmetry: **zero coverage is a strong signal of a real gap; high coverage is a weak signal of quality.** Gap findings carry a built-in falsifiability claim — "no test exercises this code, so a regression here would ship silently." Severity, however, follows export status (see the Severity section): exported/public zero-coverage is **P2**; internal/private zero-coverage is **P3**.
- Distinguish genuine gaps from indirect coverage. A function is not a gap if it is exercised transitively by a higher-level test (handler test that flows through a service). Prefer file- and module-level gaps ("this source file has no test that imports or exercises it") over symbol-level grep, which is too noisy in layered code.
- When a language-standard coverage tool has already been run and a report is available, trust it over static heuristics — it correctly recognizes indirect coverage.

### Output Validation
- Assertions check actual results
- Not just "no error thrown"
- Expected values are meaningful, not arbitrary

### Isolation
- No shared mutable state between tests
- Tests can run in any order
- Tests can run in parallel
- External dependencies mocked/stubbed appropriately

### Readability
- Clear, descriptive test names
- Obvious arrange-act-assert structure
- Test intent immediately clear
- Setup/teardown not hiding important context

### Integration Test Specifics
- Proper cleanup of test data
- Appropriate use of test fixtures
- Reasonable timeouts configured
- Clear distinction from unit tests

## Severity

Severity is assigned **per-finding by falsifiability**, not by pattern. Pattern labels exist only to drive Pattern Collapsing — they do not set severity. Two findings sharing a pattern label can have different severities if their falsifiability characteristics differ.

The driving question for every finding: **"What fraction of meaningful regressions in the production behavior this test claims to cover would still slip past it?"** If the answer is "none — the test would catch every realistic regression," there is no finding to report.

For every finding except the two exceptions noted below, you MUST be able to state a concrete claim of the form: *"if `<specific production change>` were made, this test would still incorrectly pass."* If you cannot state such a claim with a specific production change, the finding is below the reporting bar — omit it.

**Exceptions (no falsifiability claim required):**
- P1 tautological tests — the claim is implicit (no production change can fail the test).
- Unclear test names — judged on readability of CI failures, not falsifiability.

### P1 — the test cannot fail, or it masks a known bug
- Assertions against a freshly-constructed mock, or asserting truthiness on a value that is always truthy.
- Test simulates events differently than the runtime does in a way that hides real production behavior (e.g., synthetic event in jsdom that the browser would dispatch twice).
- Shared mutable state that **actually causes** test interference (tests fail or produce different results depending on run order). Fragile-but-currently-passing shared state is P2, not P1.

### P2 — the test would still pass after deleting a *central* production behavior the test claims to verify
The test is making a load-bearing claim that doesn't actually hold. Reserved for:
- **Dead expectations**: a fake/spy records a value (counter, captured arg, expected reason) that no assertion ever inspects, so the wrapped helper could be stubbed to a no-op and the test still passes.
- **Coverage gaps on exported/public production code** in the review scope: a whole file or whole exported function with no test exercising it directly or transitively. (Branch-level gaps inside an otherwise-tested function are P3.)
- **Loose matching that defeats the test's purpose**: substring-matching a JSON body where decoding-and-comparing is required to actually verify the contract; regex matching that would pass on the very regression the test exists to catch.
- **Real isolation/cleanup hazards**: leaked goroutines or listeners under `t.Parallel`; fragile shared state that will interfere as soon as a sibling test is added.
- **Unclear test names** that obscure intent in CI failures (no falsifiability claim required).

### P3 — the test catches the central regression, but a *peripheral* related regression would slip past
Most "could assert more" findings live here.
- Test asserts some fields of a returned struct but not all; deleting an *unchecked* field's production wiring still passes.
- One error branch of an exported method is uncovered while the happy path is covered.
- Missing edge case where you can name the specific input that would expose a realistic regression.
- Coverage gaps on clearly internal/private helpers.

### The P2/P3 boundary
Ask whether the missing assertion or coverage targets the **primary contract** of the function under test (P2) or a **secondary** aspect that a separate test could reasonably own (P3).

If the test's name or arrange-act says it verifies behavior X, and X is not actually verified, that is **P2** — the test is misleading. If X *is* verified but adjacent behavior Y is not, that is **P3**. When in doubt, ask: would a reader skim this test, conclude "X is covered," and stop looking? If yes and X is not actually covered, P2.

## Reporting Cap

After Pattern Collapsing, cap the report at **10 findings total**. The cap is a ceiling, not a target — if there are only 4 real findings, report 4. **Do not manufacture filler to reach 10.** A short report of high-impact findings beats a padded report of weak ones, and a padded report trains the reader to ignore the long tail.

Selection rules, applied in order:

1. **Include every P1.** Never truncate a P1 — these are tests that cannot fail or actively mask production bugs, and they don't belong in a tail. If P1s alone exceed 10, report all of them and skip P2/P3 entirely.
2. **Fill remaining budget with P2s, then P3s**, ordered by impact within each tier. Impact favors findings whose falsifiability claim names a more central production behavior, and pattern-collapsed findings that span more locations.
3. **Footer the tail.** When findings exceed the cap, end the report with: `Note: N additional findings omitted (X P2, Y P3) — re-run after addressing these to surface what remains.` When findings fit under the cap, no footer is needed.

The reasoning: large reports cause churn and rarely land as a single PR — they age out, get partially applied, or split attention away from the highest-impact issues. Iterating in batches of 10 is what humans actually do, and re-running after fixes surfaces issues that only become visible once the most pressing ones are out of the way. The cap also creates healthy pressure against the "asked to find things, so finds things" failure mode — if your candidate finding wouldn't make it into the top 10, it probably isn't worth the reader's attention.

## Output

You MUST produce a report following the exact structure shown in `REFERENCE.md`. When using parallel mode, the lead assembles the unified report from subagent findings. The report format is identical regardless of execution mode.

Each finding MUST include:

- **Priority** (P1/P2/P3) in the H3 header
- **Location** (file:line) on its own line
- **Explanation** of the problem or missing coverage and why it matters
- **Fix** — concrete prescription. For quality bugs, specify exactly what must change (e.g., "replace the shared `db` package-level var with a per-test instance constructed in each test's setup"). For coverage tasks, specify exactly what scenario or assertion is missing (e.g., "add a test case where the input slice is nil; the current table has only empty-slice and non-empty cases")
- **Done when** — a verifiable criterion checkable by reading the test file. Example: "TestFoo has no package-level mutable state; all state is initialized inside t.Run or TestFoo itself." NOT: "The test is properly isolated."
