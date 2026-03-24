---
paths:
  - ".forge/**"
---

# Forge Implementation Protocol

## Execution Rules

1. **Spec before code**: Never start implementing without a finalized `final.md`. Run `/recipe blueprint` first.
2. **Build verification is mandatory**: After writing each file, run the build. Do not proceed to the next file if the build fails.
3. **Fix code, not spec**: During implementation, fix the code to match the spec. Only update the spec during the `/sync` step.
4. **Test after implement**: Always run `/test` after implementation completes.
5. **Sync after test**: Always run `/sync` after tests pass to keep spec and code aligned.
6. **Status tracking**: Update `tasks.json` after every file completion. Run `/status` at the end.
7. **Phase transitions**: Update recipe.json phase at each lifecycle step via `forge recipe log {id} --phase <phase>`.

## Prerequisites

| Skill | Requires |
|-------|----------|
| `/implement` | final.md exists, phase is "finalize" or "implement" |
| `/test` | tasks.json exists, all tasks done or skipped |
| `/sync` | test-results.json exists, status=pass |
| `/status` | No prerequisites (reads whatever exists) |

## Phase Lifecycle and Resume

```
finalize → implement → test → sync → status → complete
```

On resume, check recipe phase and skip completed sub-steps:
- phase=implement → resume task loop
- phase=test → check test-results.json, skip if pass
- phase=sync → check deviation.md, skip if exists
- phase=status → run status only

## Resilience Rules

### Task State Machine
```
pending → in_progress → done
                     → blocked → skipped (user decision)
                              → pending (user retry)
```

- Set `in_progress` BEFORE starting work on a task (crash safety)
- Set `done` AFTER build passes
- Persist `retry_count` and `last_error` in tasks.json (survives compaction)

### Stagnation Detection
If `last_error` is substantially the same across consecutive retries:
- Do NOT repeat the same fix
- Try a fundamentally different approach
- If stuck, mark as blocked early (don't waste all 5 retries on the same error)

### Environment Fail-Fast
Before the first task, run the build command on the existing codebase:
- "command not found" → stop immediately, report missing tool
- Build errors in existing code → note them, proceed (they're not from our changes)

### Oscillation Detection (Test Loop)
Track which tests fail per iteration. If a test that was passing starts failing again:
- The fix for one test is breaking another
- Stop the loop, report the conflict
- Human intervention required

### Dependency Cascade
When a task is blocked:
- All tasks with `depends_on` referencing it → auto-skip
- Transitive: if t-003 depends on t-002 which depends on blocked t-001 → skip both

### Blocked Task Recovery
Users can recover blocked tasks:
- **Skip**: mark as `skipped`, proceed without it
- **Retry**: reset `retry_count` to 0, set status to `pending`, re-enter loop
- **Manual fix**: user fixes the code, then marks task as `done` manually

## CLI Usage

```bash
# Set phase + log action
forge recipe log {id} --phase implement --action implement --output tasks.json --result "5 tasks"

# Log task progress
forge recipe log {id} --action implement --result "task t-001 done"

# Transition to next phase
forge recipe log {id} --phase test
```

## State Files

Implementation artifacts go to `.forge/state/recipes/{id}/`:
- `tasks.json`: task decomposition, progress, retry counts, errors
- `test-results.json`: test execution results
- `deviation.md`: spec↔code differences

Project-level status:
- `.forge/state/project-status.md`: all recipes overview

## Max Retry Limits

| Operation | Max Retries | On Failure |
|-----------|-------------|------------|
| Build per file | 5 (persisted) | Mark task `blocked` |
| Test suite | 5 iterations | Report failures |
| Sync validation | 1 | Report deviations |
