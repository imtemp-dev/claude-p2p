---
name: forge-audit
description: >
  Audit a document for completeness. Find missing scenarios, unconsidered
  edge cases, and hidden assumptions. Use after verify and cross-check.
user-invocable: true
allowed-tools: Read Grep Glob Agent
argument-hint: "[file-path]"
context: fork
---

# Completeness Audit

Audit the specified document for missing items.

## Settings

Read `.forge/config/settings.yaml`. If `agents.auditor` is set, pass that model
when spawning Agent(auditor) below.

## Steps

1. Read the target document fully
2. Spawn Agent(auditor) with the following prompt:

   ```
   You are a completeness audit specialist. Read the document at $ARGUMENTS.

   Your goal: find everything the document fails to address that could cause
   problems at runtime, during deployment, or under adversarial conditions.
   Think about failure modes, boundary conditions, unstated assumptions,
   missing integrations, security gaps, and operational concerns. Do not
   limit yourself to a fixed checklist — reason about what this specific
   system needs and what the document leaves unanswered.

   For each missing item, classify:
   - critical: Will cause runtime failure if not addressed
   - major: Important gap that should be filled before implementation
   - minor: Nice to have but not blocking

   Output findings as a numbered list with severity tags.
   ```

3. Collect the auditor's findings
4. Report results with severity counts
