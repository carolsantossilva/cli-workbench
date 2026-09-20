# Build workflow

## Goal

Implement a bounded increment from an explicitly approved design and plan, with tests and observable verification.

## Inputs and output

- Read the approved `docs/station/design.md`, `docs/station/plan.md`, current workspace contract, and only declared context.
- Before editing, inspect the target state and preserve existing changes. Use test-first development for behavior changes.
- Work in small dependency-ordered batches. Use only command IDs declared in `workspace.json` and allowed by this workflow; an empty command list means do not run project commands.
- Record the implementation plan in `docs/station/plan.md`; source paths may be changed only after their inclusion in the approved design and this workflow's `writePaths`.

## Guardrails

- Do not install or download dependencies, call model/provider APIs, initialize Git in a generated workspace, or perform push/PR/deploy/remote writes implicitly.
- Verify results with actual command output. Station observes host changes but cannot sandbox tools used outside its CLI.
- Stop on policy denial or a material conflict with the approved design; record the evidence and decision needed.
