---
name: forge-assess
description: >
  Assess a document's current level (1-3) and determine the next action needed.
  This is the brain of the adaptive loop — it decides what to do next.
user-invocable: true
allowed-tools: Read Bash
argument-hint: "[file-path]"
---

# Document Assessment

Assess the document and decide the next action.

## Steps

1. Run level assessment via forge binary:
   ```bash
   forge verify $ARGUMENTS
   ```
   This returns the current level score and missing criteria.

2. Build situational awareness:
   - Read changelog.jsonl (last 5 entries) to know what was just done
   - Check if simulation has run (look for "simulate" action in changelog)
   - Check if debates exist and whether conclusions are reflected in the draft
   - Read scope.md to keep boundaries in mind

3. Read the document fully yourself and evaluate:
   - What level is this document at? (1=understanding, 2=design, 3=implementation-ready)
   - What specific content is missing to reach the next level?
   - Are there uncertain technical decisions that need debate?
   - Are there scenarios that haven't been walked through?
   - Are there internal contradictions?

4. Decide the next action based on assessment:

   **If information is insufficient** → recommend `/research`
   "Need to investigate [specific topic] before proceeding."

   **If technical decision is uncertain** → recommend `/debate`
   "The choice between [A] and [B] needs expert discussion."

   **If gaps may exist** → recommend `/simulate`
   "Walk through [specific scenarios] to find blind spots."

   **If content needs to be added** → recommend IMPROVE
   "Add [specific items] to reach Level [N]. Then run /verify."

   **If contradictions are suspected** → recommend `/verify`
   "Check sections [X] and [Y] for consistency."

   **If completeness is uncertain** → recommend `/audit`
   "Review for missing error cases, edge cases, security."

   **If mermaid diagrams are missing or incomplete** → recommend IMPROVE
   "Add state machine and flow diagrams with all execution paths enumerated."

   **If Level 3 criteria all met** → recommend `/sync-check` then finalize
   "Document appears complete. Run sync-check before finalizing."

5. Output your assessment:
   ```
   ## Assessment
   Current Level: [X.Y]
   Missing for next level: [list]

   ## Recommended Action
   [ACTION]: [specific instruction]

   ## Rationale
   [Why this action is needed now]
   ```

## Simulate Timing

**Priority rule**: If verify-log shows critical=0 has been achieved AND no simulation
has run yet (check changelog.jsonl for "simulate" action), strongly recommend /simulate
BEFORE further IMPROVE cycles.

Rationale: structural verification (critical=0) means the spec is internally consistent.
Now is the best time to test scenarios — simulation catches behavioral gaps (failure modes,
race conditions, edge cases) that verification cannot find. Running simulation early
prevents discovering these gaps late and needing additional rework iterations.

## Important

- Always be specific. Not "needs more detail" but "add function signatures for auth module."
- Consider what has already been done (check .forge/state/{id}/ for previous research, debates, simulations).
- If previous debates exist, check if their conclusions are reflected in the current draft.
