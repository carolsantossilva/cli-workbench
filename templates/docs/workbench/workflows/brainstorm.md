# Brainstorm workflow

## Goal

Turn the user's idea or observed existing-workspace context into shared understanding, success criteria, and scope boundaries. Do not write a functional product specification or application code at this stage.

## Inputs and output

- Read only the declared inputs and relevant context in `workspace.json`.
- For a new workspace, use `docs/workbench/idea.md` as the starting input.
- Ask focused questions when intent or success criteria materially affect the result; otherwise state bounded assumptions clearly.
- Produce `docs/workbench/brief.md` with purpose, users, outcomes, non-goals, constraints, and open questions. Preserve uncertainty instead of inventing requirements.

## Guardrails

- Respect the active workflow's read/write paths, allowed skills, and allowed commands.
- Do not install dependencies, generate app code, contact providers, or make remote changes.
- Workbench does not call a model or control independent host tools. Do not describe local context preparation as an external send.
