# Build workflow

## Goal

Turn an approved design into a scoped implementation plan, then implement only work authorized by that plan and this workflow's contract, with tests and observable verification.

## Inputs and output

- Read the approved `docs/station/design.md`, current workspace contract, and only declared context. On the initial run, `docs/station/plan.md` is the output being created, not a required input. For a later increment, add an existing plan to the workflow's declared inputs before relying on it.
- Before editing, inspect the target state and preserve existing changes. Use test-first development for behavior changes.
- Work in small dependency-ordered batches. Use only command IDs declared in `workspace.json` and allowed by this workflow; an empty command list means do not run project commands.
- Record the implementation plan in `docs/station/plan.md`. The initial scaffold grants documentation paths only, so the first run can prepare this artifact but cannot edit application source. After the plan is approved, add it to this workflow's inputs and explicitly authorize the needed project `readPaths`, `writePaths`, and verification command IDs before a later implementation run.

## Guardrails

- Do not install or download dependencies, call model/provider APIs, initialize Git in a generated workspace, or perform push/PR/deploy/remote writes implicitly.
- Verify results with actual command output. Station observes host changes but cannot sandbox tools used outside its CLI.
- Stop on policy denial or a material conflict with the approved design; record the evidence and decision needed.
