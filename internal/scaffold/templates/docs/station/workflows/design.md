# Design workflow

## Goal

Specify only the next approved increment well enough to build and verify it. Record components, ownership, data flow, failure behavior, tests, and decisions.

## Inputs and output

- Read the declared brief, decisions, `workspace.json`, and project paths allowed by this workflow.
- Compare technical choices only where alternatives could materially change the design. For information that may have changed, research current primary documentation and record the source, date, extracted fact, influence, and limitation. Check local tool availability separately; a public source does not prove a tool exists locally.
- Write the result to `docs/station/design.md`. Clearly mark unresolved choices and assumptions.

## Guardrails

- Do not turn this workflow into a universal architecture or reopen approved product decisions without a recorded reason.
- Do not create application code, install dependencies, contact providers, or make remote changes.
- Respect the current workflow's `readPaths`, `writePaths`, skills, and commands.
