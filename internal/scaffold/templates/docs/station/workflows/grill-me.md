# Grill-me workflow

## Goal

Challenge the current brief: reveal ambiguous terms, untested assumptions, edge cases, risks, and decisions that could change scope or architecture.

## Inputs and output

- Read the declared brief and relevant decision context only.
- Ask concise, concrete questions; group related questions only when that makes answering easier.
- Record answers, rejected options, unresolved questions, and their consequences in `docs/station/decisions.md`.
- Do not create a separate grill-me report or silently rewrite the brief as if an assumption were approved.

## Guardrails

- Stay within declared workflow paths and use only skills/commands allowed by `workspace.json`.
- Do not create app code, install dependencies, send data to a provider, or perform remote actions.
- Escalate a material scope or architecture ambiguity through the decisions artifact rather than guessing.
